package usage

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestMeasureBoundsAndSkipsSymlinks(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("1234"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("a.txt", filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	result, err := Measure(root, 10)
	if err != nil || result.Bytes != 4 || result.Files != 1 || !result.Complete {
		t.Fatalf("usage=%#v err=%v", result, err)
	}
	result, err = Measure(root, 1)
	if !errors.Is(err, ErrLimit) || result.Complete {
		t.Fatalf("bounded usage=%#v err=%v", result, err)
	}
}
