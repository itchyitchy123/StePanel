package main

import "testing"

func TestParseGitRepository(t *testing.T) {
	for _, test := range []struct {
		raw string
		ok  bool
		ssh bool
	}{
		{"https://github.com/acme/site.git", true, false},
		{"git@github.com:acme/site.git", true, true},
		{"https://user:token@github.com/acme/site.git", false, false},
		{"ssh://git@github.com/acme/site.git", false, false},
		{"git@evil.example:acme/site.git", false, false},
	} {
		got, err := parseGitRepository(test.raw, "github.com")
		if (err == nil) != test.ok || err == nil && got.Private != test.ssh {
			t.Fatalf("parseGitRepository(%q) = %#v, %v", test.raw, got, err)
		}
	}
}
