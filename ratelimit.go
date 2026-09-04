package main

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

type loginAttempt struct {
	count int
	start time.Time
}

type loginLimiter struct {
	mu      sync.Mutex
	attempt map[string]loginAttempt
	lastGC  time.Time
}

const maxLoginLimiterKeys = 10_000

func newLoginLimiter() *loginLimiter {
	return &loginLimiter{attempt: make(map[string]loginAttempt), lastGC: time.Now()}
}

func (l *loginLimiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	if now.Sub(l.lastGC) >= time.Minute {
		for candidate, attempt := range l.attempt {
			if now.Sub(attempt.start) >= 15*time.Minute {
				delete(l.attempt, candidate)
			}
		}
		l.lastGC = now
	}
	attempt := l.attempt[key]
	if attempt.start.IsZero() || now.Sub(attempt.start) >= 15*time.Minute {
		if len(l.attempt) >= maxLoginLimiterKeys {
			return false
		}
		l.attempt[key] = loginAttempt{count: 1, start: now}
		return true
	}
	if attempt.count >= 5 {
		return false
	}
	attempt.count++
	l.attempt[key] = attempt
	return true
}

func (l *loginLimiter) Reset(key string) { l.mu.Lock(); delete(l.attempt, key); l.mu.Unlock() }

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	if strings.TrimSpace(r.RemoteAddr) != "" {
		return strings.TrimSpace(r.RemoteAddr)
	}
	return "unknown"
}
