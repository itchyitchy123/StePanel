package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestMeasureSiteUsageBoundsAndSkipsSymlinks(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("1234"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("a.txt", filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	usage, err := measureSiteUsage(root, 10)
	if err != nil || usage.Bytes != 4 || usage.Files != 1 || !usage.Complete {
		t.Fatalf("usage=%#v err=%v", usage, err)
	}
	usage, err = measureSiteUsage(root, 1)
	if !errors.Is(err, errUsageLimit) || usage.Complete {
		t.Fatalf("bounded usage=%#v err=%v", usage, err)
	}
}
