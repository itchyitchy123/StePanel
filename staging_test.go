package main

import "testing"

func TestStagingBasicAuthSupportMatchesVHostHelpers(t *testing.T) {
	for _, webserver := range []string{"caddy", "apache"} {
		if !stagingBasicAuthSupported(webserver) {
			t.Fatalf("%s should support staging Basic Auth", webserver)
		}
	}
	for _, webserver := range []string{"openlitespeed", "", "nginx"} {
		if stagingBasicAuthSupported(webserver) {
			t.Fatalf("%s must not advertise unsupported staging Basic Auth", webserver)
		}
	}
}
