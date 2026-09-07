package main

import (
	"path/filepath"
	"testing"
)

func TestDNSDesiredStateSurvivesTransitions(t *testing.T) {
	store, err := OpenDNSDesiredStore(filepath.Join(t.TempDir(), "dns.json"))
	if err != nil {
		t.Fatal(err)
	}
	request := cloudDNSRequest{DomainID: "123", RecordID: "456", Type: "a", Name: "www", Target: "192.0.2.10", TTL: 300}
	if err := store.markPending(request, "update", "admin"); err != nil {
		t.Fatal(err)
	}
	items := store.list("123")
	if len(items) != 1 || items[0].State != "pending" || items[0].Type != "A" {
		t.Fatalf("pending DNS state = %#v", items)
	}
	if err := store.markResult(request, "update", nil); err != nil {
		t.Fatal(err)
	}
	items = store.list("123")
	if len(items) != 1 || items[0].State != "applied" {
		t.Fatalf("applied DNS state = %#v", items)
	}
	if err := store.markPending(request, "delete", "admin"); err != nil {
		t.Fatal(err)
	}
	if err := store.markResult(request, "delete", nil); err != nil {
		t.Fatal(err)
	}
	if len(store.list("123")) != 0 {
		t.Fatal("deleted DNS desired record remained")
	}
}
