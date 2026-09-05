package main

import (
	"context"
	"os/exec"
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
