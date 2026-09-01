package main

import (
	"context"
	"reflect"
	"testing"
)

func TestHelperCommandDirect(t *testing.T) {
	command := helperCommand(Config{}, "/helper", "one", "two")
	if want := []string{"/helper", "one", "two"}; !reflect.DeepEqual(command.Args, want) {
		t.Fatalf("command args = %q, want %q", command.Args, want)
	}
}

func TestHelperCommandUsesNonInteractiveSudo(t *testing.T) {
	config := Config{Sudo: "/usr/bin/sudo"}
	command := helperCommandContext(context.Background(), config, "/helper", "argument")
	if want := []string{"/usr/bin/sudo", "--non-interactive", "/helper", "argument"}; !reflect.DeepEqual(command.Args, want) {
		t.Fatalf("command args = %q, want %q", command.Args, want)
	}
}
