package main

import (
    "encoding/json"
    "os"
    "path/filepath"
    "sync"
    "time"
)

var auditMu sync.Mutex
func Audit(path, action, username, detail string) error { auditMu.Lock(); defer auditMu.Unlock(); if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil { return err }; file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600); if err != nil { return err }; defer file.Close(); return json.NewEncoder(file).Encode(map[string]any{"time": time.Now().UTC(), "action": action, "username": username, "detail": detail}) }
