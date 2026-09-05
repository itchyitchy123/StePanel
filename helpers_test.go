package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestHelperCommandDirect(t *testing.T) {
	command := helperCommand(Config{}, "/helper", "one", "two")
	if want := []string{"/helper", "one", "two"}; !reflect.DeepEqual(command.Args, want) {
		t.Fatalf("command args = %q, want %q", command.Args, want)
	}
}

func TestRunBoundedCommandHonorsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	_, err := runBoundedCommand(ctx, exec.CommandContext(ctx, "sh", "-c", "sleep 1"))
	if err == nil {
		t.Fatal("expected cancelled helper command to fail")
	}
}

func TestHelperCommandUsesNonInteractiveSudo(t *testing.T) {
	config := Config{Sudo: "/usr/bin/sudo"}
	command := helperCommandContext(context.Background(), config, "/helper", "argument")
	if want := []string{"/usr/bin/sudo", "--non-interactive", "/helper", "argument"}; !reflect.DeepEqual(command.Args, want) {
		t.Fatalf("command args = %q, want %q", command.Args, want)
	}
}

func TestOpenRegularNoFollowRejectsReplacedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "site-file")
	if err := os.WriteFile(path, []byte("first"), 0600); err != nil {
		t.Fatal(err)
	}
	expected, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	replacement := filepath.Join(filepath.Dir(path), "replacement")
	if err := os.WriteFile(replacement, []byte("replacement"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, path); err != nil {
		t.Fatal(err)
	}
	file, _, err := openRegularNoFollow(path, expected)
	if file != nil {
		_ = file.Close()
	}
	if err == nil {
		t.Fatal("replaced file was accepted")
	}
}
