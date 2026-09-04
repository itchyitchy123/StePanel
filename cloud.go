package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

type CloudInventory struct {
	Provider      string   `json:"provider"`
	Servers       any      `json:"servers,omitempty"`
	DNS           any      `json:"dns,omitempty"`
	LoadBalancers any      `json:"load_balancers,omitempty"`
	Snapshots     any      `json:"snapshots,omitempty"`
	Warnings      []string `json:"warnings,omitempty"`
}

func (a *App) cloudInventory(w http.ResponseWriter, _ *http.Request) {
	provider := strings.ToLower(strings.TrimSpace(a.Config.CloudProvider))
	if provider == "" {
		writeJSON(w, http.StatusOK, CloudInventory{Warnings: []string{"no cloud provider configured"}})
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	var inv CloudInventory
	var err error
	switch provider {
	case "linode":
		inv, err = linodeInventory(ctx)
	case "aws":
		inv, err = cliCloudInventory(ctx, "aws", "AWS")
	case "openstack":
		inv, err = cliCloudInventory(ctx, "openstack", "OpenStack")
	default:
		err = errors.New("unsupported cloud provider")
	}
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"provider": provider, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, inv)
}

func (a *App) cloudAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || !a.Auth.CSRF(r) {
		http.Error(w, "invalid request", http.StatusForbidden)
		return
	}
	var in struct{ Provider, Action, ID string }
	if err := decodeJSON(w, r, 4096, &in); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	in.Provider = strings.ToLower(strings.TrimSpace(in.Provider))
	if in.Provider == "" {
		in.Provider = a.Config.CloudProvider
	}
	if in.Provider == "" || (a.Config.CloudProvider != "" && in.Provider != a.Config.CloudProvider) {
		http.Error(w, "provider is not configured for this installation", http.StatusUnprocessableEntity)
		return
	}
	if !cloudIDPattern.MatchString(in.ID) || (in.Action != "start" && in.Action != "stop" && in.Action != "reboot" && in.Action != "snapshot") {
		http.Error(w, "invalid provider, action, or resource ID", http.StatusUnprocessableEntity)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := executeCloudAction(ctx, in.Provider, in.Action, in.ID); err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	if err := AuditAs(a.Config.AuditLog, a.Auth.Username, "cloud."+in.Action, in.ID, in.Provider); err != nil {
		http.Error(w, "cloud action completed but audit persistence is unavailable", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"provider": in.Provider, "action": in.Action, "id": in.ID})
}

var cloudIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)

func executeCloudAction(ctx context.Context, provider, action, id string) error {
	switch provider {
	case "linode":
		token := os.Getenv("STEPANEL_LINODE_TOKEN")
		if token == "" {
			return errors.New("STEPANEL_LINODE_TOKEN is not configured")
		}
		path := "/linode/instances/" + id
		if action == "snapshot" {
			path += "/backups"
		} else {
			path += "/" + map[string]string{"start": "boot", "stop": "shutdown", "reboot": "reboot"}[action]
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.linode.com/v4"+path, nil)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		res, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
		if err != nil {
			return err
		}
		defer res.Body.Close()
		if res.StatusCode/100 != 2 {
			return fmt.Errorf("Linode API returned %s", res.Status)
		}
		return nil
	case "aws":
		args := []string{"ec2"}
		switch action {
		case "start":
			args = append(args, "start-instances")
		case "stop":
			args = append(args, "stop-instances")
		case "reboot":
			args = append(args, "reboot-instances")
		case "snapshot":
			args = append(args, "create-snapshot", "--volume-id")
		}
		args = append(args, id, "--output", "json")
		return runCloudCLI(ctx, "aws", "AWS", args...)
	case "openstack":
		args := []string{"server"}
		switch action {
		case "start":
			args = append(args, "start")
		case "stop":
			args = append(args, "stop")
		case "reboot":
			args = append(args, "reboot", "--hard")
		case "snapshot":
			args = append(args, "backup", "create", "--name", "stepanel-"+id)
		}
		args = append(args, id, "-f", "json")
		return runCloudCLI(ctx, "openstack", "OpenStack", args...)
	default:
		return errors.New("unsupported cloud provider")
	}
}

func runCloudCLI(ctx context.Context, command, provider string, args ...string) error {
	if _, err := exec.LookPath(command); err != nil {
		return fmt.Errorf("%s CLI is not installed", provider)
	}
	out, err := exec.CommandContext(ctx, command, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s action failed: %w: %s", provider, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func linodeInventory(ctx context.Context) (CloudInventory, error) {
	token := os.Getenv("STEPANEL_LINODE_TOKEN")
	if token == "" {
		return CloudInventory{}, errors.New("STEPANEL_LINODE_TOKEN is not configured")
	}
	client := &http.Client{Timeout: 10 * time.Second}
	get := func(path string) (any, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.linode.com/v4"+path, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Accept", "application/json")
		res, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		defer res.Body.Close()
		if res.StatusCode/100 != 2 {
			return nil, fmt.Errorf("Linode API returned %s", res.Status)
		}
		var value any
		if err := json.NewDecoder(res.Body).Decode(&value); err != nil {
			return nil, err
		}
		return value, nil
	}
	paths := []struct{ name, path string }{{"servers", "/linode/instances"}, {"dns", "/domains"}, {"load_balancers", "/nodebalancers"}, {"snapshots", "/account/linode/backups"}}
	inv := CloudInventory{Provider: "linode"}
	for _, item := range paths {
		value, err := get(item.path)
		if err != nil {
			inv.Warnings = append(inv.Warnings, item.name+": "+err.Error())
			continue
		}
		switch item.name {
		case "servers":
			inv.Servers = value
		case "dns":
			inv.DNS = value
		case "load_balancers":
			inv.LoadBalancers = value
		case "snapshots":
			inv.Snapshots = value
		}
	}
	return inv, nil
}

func cliCloudInventory(ctx context.Context, command, provider string) (CloudInventory, error) {
	if _, err := exec.LookPath(command); err != nil {
		return CloudInventory{}, fmt.Errorf("%s CLI is not installed", provider)
	}
	run := func(args ...string) (any, error) {
		out, err := exec.CommandContext(ctx, command, args...).Output()
		if err != nil {
			return nil, fmt.Errorf("%s command failed: %w", provider, err)
		}
		var value any
		if err := json.Unmarshal(out, &value); err != nil {
			return nil, fmt.Errorf("%s returned invalid JSON: %w", provider, err)
		}
		return value, nil
	}
	inv := CloudInventory{Provider: strings.ToLower(provider)}
	if provider == "AWS" {
		inv.Servers, _ = run("ec2", "describe-instances", "--output", "json")
		inv.DNS, _ = run("route53", "list-hosted-zones", "--output", "json")
		inv.LoadBalancers, _ = run("elbv2", "describe-load-balancers", "--output", "json")
		inv.Snapshots, _ = run("ec2", "describe-snapshots", "--owner-ids", "self", "--output", "json")
	} else {
		inv.Servers, _ = run("server", "list", "-f", "json")
		inv.DNS, _ = run("recordset", "list", "-f", "json")
		inv.LoadBalancers, _ = run("loadbalancer", "list", "-f", "json")
		inv.Snapshots, _ = run("server", "backup", "list", "-f", "json")
	}
	return inv, nil
}
