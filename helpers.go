package main

import (
	"context"
	"os/exec"
)

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
