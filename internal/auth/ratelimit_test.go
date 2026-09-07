package auth

import (
	"fmt"
	"net/http/httptest"
	"testing"
)

func TestLimiterWindowAndReset(t *testing.T) {
	limiter := NewLimiter()
	for i := 0; i < 5; i++ {
		if !limiter.Allow("192.0.2.10") {
			t.Fatalf("attempt %d unexpectedly rejected", i+1)
		}
	}
	if limiter.Allow("192.0.2.10") {
		t.Fatal("sixth attempt should be rejected")
	}
	limiter.Reset("192.0.2.10")
	if !limiter.Allow("192.0.2.10") {
		t.Fatal("reset limiter still rejected login")
	}
}

func TestLimiterBoundsDistinctClients(t *testing.T) {
	limiter := NewLimiter()
	for i := 0; i < maxKeys; i++ {
		if !limiter.Allow(fmt.Sprintf("192.0.2.%d", i)) {
			t.Fatalf("client %d was unexpectedly rejected", i)
		}
	}
	if limiter.Allow("198.51.100.1") {
		t.Fatal("limiter accepted state beyond its configured bound")
	}
}

func TestClientIPDoesNotTrustForwardedHeaders(t *testing.T) {
	req := httptest.NewRequest("POST", "/login", nil)
	req.RemoteAddr = "192.0.2.10:1234"
	req.Header.Set("X-Forwarded-For", "198.51.100.20")
	if got := ClientIP(req); got != "192.0.2.10" {
		t.Fatalf("client IP = %q, want peer address", got)
	}
}
