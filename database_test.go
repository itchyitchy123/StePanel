package main

import "testing"

func TestMySQLCompatibleDatabaseModes(t *testing.T) {
	for engine, want := range map[string]bool{"": true, "mysql": true, "mariadb": true, "postgresql": false} {
		if got := mysqlCompatible(Config{DBEngine: engine}); got != want {
			t.Fatalf("mysqlCompatible(%q) = %t, want %t", engine, got, want)
		}
	}
}

func TestDatabaseHostDescribesLocalConnections(t *testing.T) {
	for _, host := range []string{"", "localhost", "127.0.0.1"} {
		if got := databaseHost(Config{DBHost: host}); got != "local socket" {
			t.Fatalf("databaseHost(%q) = %q", host, got)
		}
	}
	if got := databaseHost(Config{DBHost: "db.internal"}); got != "db.internal" {
		t.Fatalf("remote database host = %q", got)
	}
}
