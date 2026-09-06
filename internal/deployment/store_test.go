package deployment

import (
	"path/filepath"
	"testing"
	"time"
)

func TestStorePersistsAndFiltersRecords(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "deployments.json"))
	if err != nil {
		t.Fatal(err)
	}
	created := time.Now().UTC()
	if err := store.Add(Record{ID: "one", Site: "alpha", Stage: "build", State: "completed", CreatedAt: created}); err != nil {
		t.Fatal(err)
	}
	if err := store.Add(Record{ID: "two", Site: "beta", Stage: "activation", State: "completed", CreatedAt: created.Add(time.Second)}); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(filepath.Join(filepath.Dir(store.path), "deployments.json"))
	if err != nil {
		t.Fatal(err)
	}
	items := reopened.List("alpha")
	if len(items) != 1 || items[0].ID != "one" {
		t.Fatalf("unexpected filtered records: %#v", items)
	}
	all := reopened.List("")
	if len(all) != 2 || all[0].ID != "two" {
		t.Fatalf("unexpected ordered records: %#v", all)
	}
}

func TestStoreRetainsOriginalStateWhenPersistenceFails(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "deployments.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Add(Record{ID: "one", Site: "alpha", CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	store.path = t.TempDir()
	if err := store.Add(Record{ID: "two", Site: "alpha", CreatedAt: time.Now().UTC()}); err == nil {
		t.Fatal("expected persistence failure")
	}
	items := store.List("alpha")
	if len(items) != 1 || items[0].ID != "one" {
		t.Fatalf("failed write changed in-memory state: %#v", items)
	}
}
