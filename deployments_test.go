package main

import (
	"path/filepath"
	"testing"
	"time"
)

func TestDeploymentStorePersistsNewestFirst(t *testing.T) {
	path := filepath.Join(t.TempDir(), "deployments.json")
	store, err := OpenDeploymentStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.add(Deployment{ID: "one", Site: "site", Stage: "build", State: "completed", CreatedAt: time.Now().Add(-time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if err := store.add(Deployment{ID: "two", Site: "site", Stage: "activation", State: "completed", CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenDeploymentStore(path)
	if err != nil {
		t.Fatal(err)
	}
	items := reopened.list("site")
	if len(items) != 2 || items[0].ID != "two" {
		t.Fatalf("deployment order = %#v", items)
	}
}
