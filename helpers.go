package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

const maxCommandOutput = 64 << 10

type boundedBuffer struct{ data []byte }

func (b *boundedBuffer) Write(p []byte) (int, error) {
	if len(b.data) < maxCommandOutput {
		n := maxCommandOutput - len(b.data)
		if n > len(p) {
			n = len(p)
		}
		b.data = append(b.data, p[:n]...)
	}
	return len(p), nil
}

func runBoundedCommand(_ context.Context, cmd *exec.Cmd) ([]byte, error) {
	var output boundedBuffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	err := cmd.Run()
	if err != nil {
		return output.data, fmt.Errorf("%w: %s", err, string(output.data))
	}
	return output.data, nil
}

func runBoundedCommandInput(ctx context.Context, cmd *exec.Cmd, input io.Reader) ([]byte, error) {
	cmd.Stdin = input
	return runBoundedCommand(ctx, cmd)
}

// helperCommand runs narrowly scoped privileged helpers through sudo when the
// packaged installation configures it. Development and test configurations can
// leave Sudo empty and execute their helper directly.
func helperCommand(cfg Config, path string, args ...string) *exec.Cmd {
	if cfg.Sudo == "" {
		return exec.Command(path, args...)
	}
	return exec.Command(cfg.Sudo, append([]string{"--non-interactive", path}, args...)...)
}

func helperCommandContext(ctx context.Context, cfg Config, path string, args ...string) *exec.Cmd {
	if cfg.Sudo == "" {
		return exec.CommandContext(ctx, path, args...)
	}
	return exec.CommandContext(ctx, cfg.Sudo, append([]string{"--non-interactive", path}, args...)...)
}

func siteHelper(cfg Config, action, site string) error {
	if cfg.SiteCtl == "" {
		return nil
	}
	return helperCommand(cfg, cfg.SiteCtl, action, site).Run()
}

// openRegularNoFollow opens a file descriptor without following symlinks and
// verifies that it is still the same inode observed during a directory walk.
// This closes the validation/use race for site files, which are writable by
// separate site identities while backups and scans run.
func openRegularNoFollow(path string, expected os.FileInfo) (*os.File, os.FileInfo, error) {
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, nil, err
	}
	file := os.NewFile(uintptr(fd), path)
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, nil, err
	}
	if !info.Mode().IsRegular() || expected != nil && !sameFileInfo(expected, info) {
		_ = file.Close()
		return nil, nil, errors.New("file changed or is not a regular file")
	}
	return file, info, nil
}

// openWriteNoFollow opens the destination itself without following a symlink.
func openWriteNoFollow(path string, mode os.FileMode) (*os.File, error) {
	fd, err := syscall.Open(path, syscall.O_WRONLY|syscall.O_CREAT|syscall.O_TRUNC|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, uint32(mode.Perm()))
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), path), nil
}

func sameFileInfo(a, b os.FileInfo) bool {
	if a == nil || b == nil {
		return false
	}
	aStat, aOK := a.Sys().(*syscall.Stat_t)
	bStat, bOK := b.Sys().(*syscall.Stat_t)
	if aOK && bOK {
		return aStat.Dev == bStat.Dev && aStat.Ino == bStat.Ino
	}
	return a.Size() == b.Size() && a.ModTime().Equal(b.ModTime()) && a.Mode() == b.Mode()
}

func writeAtomic(path string, data []byte, mode os.FileMode) error {
	root := filepath.Dir(path)
	if err := os.MkdirAll(root, 0750); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(root, ".stepanel-write-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err = tmp.Chmod(mode); err == nil {
		_, err = tmp.Write(data)
	}
	if err == nil {
		err = tmp.Sync()
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	dir, err := os.Open(root)
	if err != nil {
		return err
	}
	syncErr := dir.Sync()
	closeErr := dir.Close()
	if syncErr != nil {
		return syncErr
	}
	return closeErr
}

func rejectSymlinkParents(path, root string) error {
	root, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return err
	}
	if path != root && !strings.HasPrefix(path, root+string(os.PathSeparator)) {
		return errors.New("path escapes configured root")
	}
	if info, statErr := os.Lstat(root); statErr == nil && info.Mode()&os.ModeSymlink != 0 {
		return errors.New("configured root is a symlink")
	}
	rel, err := filepath.Rel(root, filepath.Dir(path))
	if err != nil {
		return err
	}
	current := root
	if rel != "." {
		for _, part := range strings.Split(rel, string(os.PathSeparator)) {
			current = filepath.Join(current, part)
			info, statErr := os.Lstat(current)
			if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
				return statErr
			}
			if statErr == nil && info.Mode()&os.ModeSymlink != 0 {
				return errors.New("destination contains a symlinked parent")
			}
		}
	}
	return nil
}

func acquireProcessLock(path string) (*os.File, error) {
	fd, err := syscall.Open(path, syscall.O_CREAT|syscall.O_RDWR|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return nil, err
	}
	lock := os.NewFile(uintptr(fd), path)
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = lock.Close()
		return nil, errors.New("another StePanel instance is already running")
	}
	return lock, nil
}
