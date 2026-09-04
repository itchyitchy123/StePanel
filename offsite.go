package main

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var offsiteTargetPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/@+-]{1,240}$`)

func validateOffsiteTarget(target string) error {
	if target == "" {
		return nil
	}
	if strings.HasPrefix(target, "-") || !offsiteTargetPattern.MatchString(target) || !strings.Contains(target, ":") {
		return errors.New("STEPANEL_OFFSITE_TARGET must be an rclone destination such as s3:bucket/stepanel")
	}
	return nil
}

func uploadOffsite(cfg Config, result BackupResult) error {
	if cfg.OffsiteTarget == "" {
		return nil
	}
	if err := validateOffsiteTarget(cfg.OffsiteTarget); err != nil {
		return err
	}
	destination := strings.TrimRight(cfg.OffsiteTarget, "/") + "/" + result.Site + "/" + filepath.Base(result.Path)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	defer cancel()
	cmd := exec.CommandContext(ctx, "rclone", "copyto", result.Path, destination, "--immutable")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("offsite upload failed: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}
