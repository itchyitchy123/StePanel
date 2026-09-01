package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateWPressTreeEnforcesEntryLimit(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "one"), "1")
	writeTestFile(t, filepath.Join(root, "two"), "2")
	if err := validateWPressTree(root, 2); err == nil {
		t.Fatal("oversized WPress entry count was accepted")
	}
}

func TestValidateWPressTreeRejectsSymlink(t *testing.T) {
	root := t.TempDir()
	if err := os.Symlink("/etc/passwd", filepath.Join(root, "unsafe")); err != nil {
		t.Fatal(err)
	}
	if err := validateWPressTree(root, 10); err == nil {
		t.Fatal("WPress symlink was accepted")
	}
}
