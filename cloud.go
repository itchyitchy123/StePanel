package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
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

type CloudActionResult struct {
	Provider    string    `json:"provider"`
	Action      string    `json:"action"`
	ID          string    `json:"id"`
	CompletedAt time.Time `json:"completed_at"`
}

type cloudDNSRequest struct {
	DomainID string `json:"domain_id"`
	RecordID string `json:"record_id,omitempty"`
	Type     string `json:"type,omitempty"`
	Name     string `json:"name,omitempty"`
	Target   string `json:"target,omitempty"`
	TTL      int    `json:"ttl,omitempty"`
}

type cloudLBRequest struct {
	NodeBalancerID string `json:"nodebalancer_id"`
	ConfigID       string `json:"config_id"`
	NodeID         string `json:"node_id,omitempty"`
	Address        string `json:"address,omitempty"`
	Label          string `json:"label,omitempty"`
	Port           int    `json:"port,omitempty"`
	Weight         int    `json:"weight,omitempty"`
	Action         string `json:"action"`
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
	id, err := newJobID("cloud")
	if err != nil {
		http.Error(w, "could not create cloud job", http.StatusInternalServerError)
		return
	}
	if err := a.Jobs.SubmitCloud(id, in.ID, func() (CloudActionResult, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := executeCloudAction(ctx, in.Provider, in.Action, in.ID); err != nil {
			_ = AuditAs(a.Config.AuditLog, a.Auth.Username, "cloud."+in.Action+".failed", in.ID, err.Error())
			return CloudActionResult{}, err
		}
		result := CloudActionResult{Provider: in.Provider, Action: in.Action, ID: in.ID, CompletedAt: time.Now().UTC()}
		if err := AuditAs(a.Config.AuditLog, a.Auth.Username, "cloud."+in.Action, in.ID, in.Provider); err != nil {
			return result, err
		}
		return result, nil
	}); err != nil {
		if errors.Is(err, ErrJobBusy) {
			http.Error(w, err.Error(), http.StatusTooManyRequests)
		} else {
			http.Error(w, "could not persist cloud job", http.StatusInternalServerError)
		}
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"job_id": id, "status_url": "/api/jobs/" + id})
}

func (a *App) cloudDNS(w http.ResponseWriter, r *http.Request) {
	if a.Config.CloudProvider != "linode" {
		http.Error(w, "Linode DNS integration requires STEPANEL_CLOUD_PROVIDER=linode", 422)
		return
	}
	if r.Method == http.MethodGet {
		domain := r.URL.Query().Get("domain_id")
		if !cloudNumericID.MatchString(domain) {
			http.Error(w, "invalid domain_id", 422)
			return
		}
		value, err := linodeAPIRequest(r.Context(), http.MethodGet, "/domains/"+domain+"/records", nil)
		if err != nil {
			http.Error(w, err.Error(), 503)
			return
		}
		writeJSON(w, 200, value)
		return
	}
	if !a.Auth.CSRF(r) {
		http.Error(w, "invalid request", 403)
		return
	}
	var in cloudDNSRequest
	if err := decodeJSON(w, r, 8192, &in); err != nil {
		http.Error(w, "invalid JSON", 400)
		return
	}
	if !cloudNumericID.MatchString(in.DomainID) {
		http.Error(w, "invalid domain_id", 422)
		return
	}
	if r.Method == http.MethodDelete {
		if !cloudNumericID.MatchString(in.RecordID) {
			http.Error(w, "invalid record_id", 422)
			return
		}
		a.queueDNSJob(w, in, "delete")
		return
	}
	if r.Method != http.MethodPost || !cloudDNSRecordValid(in) {
		http.Error(w, "invalid DNS record", 422)
		return
	}
	in.Type = strings.ToUpper(in.Type)
	action := "create"
	if in.RecordID != "" {
		action = "update"
	}
	a.queueDNSJob(w, in, action)
}

func (a *App) cloudLoadBalancer(w http.ResponseWriter, r *http.Request) {
	if a.Config.CloudProvider != "linode" {
		http.Error(w, "Linode load balancer integration requires STEPANEL_CLOUD_PROVIDER=linode", 422)
		return
	}
	if r.Method != http.MethodPost || !a.Auth.CSRF(r) {
		http.Error(w, "invalid request", 403)
		return
	}
	var in cloudLBRequest
	if err := decodeJSON(w, r, 8192, &in); err != nil {
		http.Error(w, "invalid JSON", 400)
		return
	}
	if !cloudNumericID.MatchString(in.NodeBalancerID) || !cloudNumericID.MatchString(in.ConfigID) || (in.Action != "add" && in.Action != "remove") {
		http.Error(w, "invalid load balancer action or ID", 422)
		return
	}
	if in.Action == "remove" {
		if !cloudNumericID.MatchString(in.NodeID) {
			http.Error(w, "invalid node_id", 422)
			return
		}
	} else if net.ParseIP(in.Address) == nil || in.Port < 1 || in.Port > 65535 || in.Weight < 1 || in.Weight > 100 {
		http.Error(w, "invalid backend address, port, or weight", 422)
		return
	}
	id, err := newJobID("loadbalancer")
	if err != nil {
		http.Error(w, "could not create load balancer job", 500)
		return
	}
	if err := a.Jobs.SubmitCloud(id, in.NodeBalancerID, func() (CloudActionResult, error) {
		var path, method string
		var body any
		if in.Action == "remove" {
			path = "/nodebalancers/" + in.NodeBalancerID + "/configs/" + in.ConfigID + "/nodes/" + in.NodeID
			method = http.MethodDelete
		} else {
			path = "/nodebalancers/" + in.NodeBalancerID + "/configs/" + in.ConfigID + "/nodes"
			method = http.MethodPost
			body = map[string]any{"address": in.Address, "label": in.Label, "port": in.Port, "weight": in.Weight}
		}
		if _, err := linodeAPIRequest(context.Background(), method, path, body); err != nil {
			return CloudActionResult{}, err
		}
		result := CloudActionResult{Provider: "linode", Action: "loadbalancer." + in.Action, ID: in.NodeBalancerID, CompletedAt: time.Now().UTC()}
		if err := AuditAs(a.Config.AuditLog, a.Auth.Username, "cloud.loadbalancer."+in.Action, in.NodeBalancerID, in.Address); err != nil {
			return result, err
		}
		return result, nil
	}); err != nil {
		if errors.Is(err, ErrJobBusy) {
			http.Error(w, err.Error(), 429)
		} else {
			http.Error(w, "could not persist load balancer job", 500)
		}
		return
	}
	writeJSON(w, 202, map[string]string{"job_id": id, "status_url": "/api/jobs/" + id})
}

var cloudNumericID = regexp.MustCompile(`^[0-9]{1,12}$`)
var dnsTypePattern = regexp.MustCompile(`^(A|AAAA|CNAME|MX|TXT|NS|SRV)$`)
var dnsNamePattern = regexp.MustCompile(`^[A-Za-z0-9_.*@-]{1,253}$`)

func cloudDNSRecordValid(in cloudDNSRequest) bool {
	typ := strings.ToUpper(in.Type)
	if !dnsTypePattern.MatchString(typ) || !dnsNamePattern.MatchString(in.Name) || len(in.Target) == 0 || len(in.Target) > 512 || in.TTL < 30 || in.TTL > 604800 {
		return false
	}
	switch typ {
	case "A":
		return net.ParseIP(in.Target) != nil && strings.Count(in.Target, ".") == 3
	case "AAAA":
		return net.ParseIP(in.Target) != nil
	case "CNAME", "NS":
		return dnsNamePattern.MatchString(strings.TrimSuffix(in.Target, "."))
	case "MX":
		parts := strings.Fields(in.Target)
		return len(parts) == 2 && numeric(parts[0]) && dnsNamePattern.MatchString(strings.TrimSuffix(parts[1], "."))
	case "SRV":
		parts := strings.Fields(in.Target)
		return len(parts) == 4 && numeric(parts[0]) && numeric(parts[1]) && numeric(parts[2]) && dnsNamePattern.MatchString(strings.TrimSuffix(parts[3], "."))
	default:
		return true
	}
}
func numeric(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func (a *App) queueDNSJob(w http.ResponseWriter, in cloudDNSRequest, action string) error {
	id, err := newJobID("dns")
	if err != nil {
		http.Error(w, "could not create DNS job", 500)
		return nil
	}
	err = a.Jobs.SubmitCloud(id, in.DomainID, func() (CloudActionResult, error) {
		var path, method string
		var body any
		if action == "delete" {
			path = "/domains/" + in.DomainID + "/records/" + in.RecordID
			method = http.MethodDelete
		} else {
			if action == "create" {
				if existing, err := linodeAPIRequest(context.Background(), http.MethodGet, "/domains/"+in.DomainID+"/records", nil); err == nil && dnsRecordExists(existing, in) {
					return CloudActionResult{}, errors.New("an identical DNS record already exists")
				}
			}
			path = "/domains/" + in.DomainID + "/records"
			method = http.MethodPost
			if action == "update" {
				path += "/" + in.RecordID
				method = http.MethodPut
			}
			body = map[string]any{"type": in.Type, "name": in.Name, "target": in.Target, "ttl_sec": in.TTL}
		}
		if _, err := linodeAPIRequest(context.Background(), method, path, body); err != nil {
			return CloudActionResult{}, err
		}
		result := CloudActionResult{Provider: "linode", Action: "dns." + action, ID: in.DomainID, CompletedAt: time.Now().UTC()}
		if err := AuditAs(a.Config.AuditLog, a.Auth.Username, "cloud.dns."+action, in.DomainID, in.Name); err != nil {
			return result, err
		}
		return result, nil
	})
	if err != nil {
		if errors.Is(err, ErrJobBusy) {
			http.Error(w, err.Error(), 429)
		} else {
			http.Error(w, "could not persist DNS job", 500)
		}
		return nil
	}
	writeJSON(w, 202, map[string]string{"job_id": id, "status_url": "/api/jobs/" + id})
	return nil
}

func dnsRecordExists(value any, in cloudDNSRequest) bool {
	root, ok := value.(map[string]any)
	if !ok {
		return false
	}
	data, ok := root["data"].([]any)
	if !ok {
		return false
	}
	for _, item := range data {
		record, ok := item.(map[string]any)
		if !ok {
			continue
		}
		typ, _ := record["type"].(string)
		name, _ := record["name"].(string)
		target, _ := record["target"].(string)
		if strings.EqualFold(typ, in.Type) && strings.EqualFold(strings.TrimSuffix(name, "."), strings.TrimSuffix(in.Name, ".")) && strings.EqualFold(strings.TrimSuffix(target, "."), strings.TrimSuffix(in.Target, ".")) {
			return true
		}
	}
	return false
}

func linodeAPIRequest(ctx context.Context, method, path string, payload any) (any, error) {
	token := os.Getenv("STEPANEL_LINODE_TOKEN")
	if token == "" {
		return nil, errors.New("STEPANEL_LINODE_TOKEN is not configured")
	}
	var reader io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, "https://api.linode.com/v4"+path, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	res, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode/100 != 2 {
		return nil, fmt.Errorf("Linode API returned %s", res.Status)
	}
	if res.StatusCode == http.StatusNoContent {
		return nil, nil
	}
	var value any
	if err := json.NewDecoder(res.Body).Decode(&value); err != nil {
		return nil, err
	}
	return value, nil
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
