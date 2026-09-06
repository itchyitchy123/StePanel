package main

import (
	"path/filepath"
	"testing"
)

func TestFinalizeTaskDeletionRollsBackMemoryOnPersistFailure(t *testing.T) {
	root := t.TempDir()
	store, err := OpenTaskStore(filepath.Join(root, "tasks.json"))
	if err != nil {
		t.Fatal(err)
	}
	store.path = root // Deliberately make the file target a directory.
	task := ScheduledTask{Site: "demo", Name: "nightly", Runtime: "shell", Command: "true", State: "pending", Deleted: true}
	key := task.Site + "/" + task.Name
	store.values[key] = task
	app := &App{Tasks: store}

	store.mu.Lock()
	err = app.finalizeTaskDeletionLocked(key, task)
	store.mu.Unlock()
	if err == nil {
		t.Fatal("expected task state persistence failure")
	}
	if got, ok := store.values[key]; !ok || got != task {
		t.Fatalf("task state after failed persistence = %#v, found=%v; want %#v", got, ok, task)
	}
}
