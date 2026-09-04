package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
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
