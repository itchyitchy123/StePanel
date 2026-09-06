package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenSiteAccessStoreNormalizesLegacyEntries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "access.json")
	if err := os.WriteFile(path, []byte(`{"demo":{"site":"","sftp_enabled":true,"keys":[]}}`), 0600); err != nil {
		t.Fatal(err)
	}
	store, err := OpenSiteAccessStore(path)
	if err != nil {
		t.Fatal(err)
	}
	access := store.values["demo"]
	if access.Site != "demo" || access.State != "applied" {
		t.Fatalf("normalized access = %#v", access)
	}
}

func TestApplySiteAccessSendsOnlyValidatedPolicyAndKeysToHelper(t *testing.T) {
	dir := t.TempDir()
	inputPath := filepath.Join(dir, "input")
	helper := filepath.Join(dir, "helper")
	script := "#!/bin/sh\nprintf '%s %s %s\\n' \"$2\" \"$3\" \"$4\" > \"" + inputPath + "\"\ncat >> \"" + inputPath + "\"\n"
	if err := os.WriteFile(helper, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	app := &App{Config: Config{SiteCtl: helper}}
	access := SiteAccess{Site: "demo", SFTPEnabled: true, ShellEnabled: false, Keys: []SSHKey{{PublicKey: "ssh-ed25519 AAAA"}}}
	if err := app.applySiteAccess(context.Background(), access); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(inputPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "demo 1 0") || !strings.Contains(string(data), "ssh-ed25519 AAAA") {
		t.Fatalf("helper input = %q", data)
	}
}
