package main

import "testing"

func TestSafeArchivePath(t *testing.T) {
	valid := []string{"homedir/public_html/index.php", "mysql/site.sql", "homedir/.well-known/acme-challenge/token"}
	for _, path := range valid {
		if !safeArchivePath(path) { t.Errorf("expected archive path to be valid: %q", path) }
	}
	invalid := []string{"/etc/passwd", "../../etc/passwd", "homedir/../../etc/passwd", "..", `homedir\\..\\etc\\passwd`}
	for _, path := range invalid {
		if safeArchivePath(path) { t.Errorf("expected archive path to be rejected: %q", path) }
	}
}

func TestSafeUser(t *testing.T) {
	for _, value := range []string{"stephen", "site_01", "site-name"} {
		if safeUser(value) != value { t.Errorf("expected username %q to be accepted", value) }
	}
	for _, value := range []string{"Stephen", "site name", "site/../../etc", ""} {
		if safeUser(value) != "" { t.Errorf("expected username %q to be rejected", value) }
	}
}
