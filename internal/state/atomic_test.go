package state

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteAtomicReplacesAndProtectsState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "state.json")
	if err := WriteAtomic(path, []byte("new\n"), 0600); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "new\n" {
		t.Fatalf("state = %q", data)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("state mode = %o, want 600", info.Mode().Perm())
	}
	if err := WriteAtomic(path, []byte("replacement\n"), 0640); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(path)
	if err != nil || string(data) != "replacement\n" {
		t.Fatalf("replacement = %q err=%v", data, err)
	}
}
