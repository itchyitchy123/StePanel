package main

import (
	"net/http/httptest"
	"testing"
)

func TestLoginLimiter(t *testing.T) {
	limiter := newLoginLimiter()
	request := httptest.NewRequest("POST", "/login", nil)
	request.RemoteAddr = "192.0.2.10:1234"
	key := clientIP(request)
	for i := 0; i < 5; i++ {
		if !limiter.Allow(key) {
			t.Fatalf("attempt %d unexpectedly rejected", i+1)
		}
	}
	if limiter.Allow(key) {
		t.Fatal("sixth attempt should be rejected")
	}
	limiter.Reset(key)
	if !limiter.Allow(key) {
		t.Fatal("reset limiter still rejected login")
	}
}
