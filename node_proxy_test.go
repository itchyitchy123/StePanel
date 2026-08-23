package main

import "testing"

func TestLocalBackendValidation(t *testing.T) {
	valid := []string{"http://127.0.0.1:3000", "http://localhost:8080", "http://192.168.1.20:9000"}
	for _, value := range valid {
		if _, err := localBackend(value); err != nil {
			t.Errorf("localBackend(%q) failed: %v", value, err)
		}
	}
	invalid := []string{"https://127.0.0.1:3000", "http://example.com:3000", "http://127.0.0.1", "http://127.0.0.1:3000/admin", "http://user:pass@127.0.0.1:3000"}
	for _, value := range invalid {
		if _, err := localBackend(value); err == nil {
			t.Errorf("localBackend(%q) unexpectedly passed", value)
		}
	}
}
