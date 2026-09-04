package main

import "testing"

func TestValidateOffsiteTarget(t *testing.T) {
	for _, target := range []string{"s3:bucket/stepanel", "b2:bucket/backups", "ssh:host:/srv/backups"} {
		if err := validateOffsiteTarget(target); err != nil {
			t.Errorf("valid target %q rejected: %v", target, err)
		}
	}
	for _, target := range []string{"", "--bad", "bucket/path", "s3:bucket bad"} {
		if err := validateOffsiteTarget(target); target != "" && err == nil {
			t.Errorf("invalid target %q accepted", target)
		}
	}
}
