package main

import (
    "encoding/json"
    "os"
    "sync"
    "time"
)

var auditMu sync.Mutex
func Audit(path, action, username, detail string) error { auditMu.Lock(); defer auditMu.Unlock(); if err := os.MkdirAll(filepathDir(path), 0750); err != nil { return err }; file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600); if err != nil { return err }; defer file.Close(); return json.NewEncoder(file).Encode(map[string]any{"time": time.Now().UTC(), "action": action, "username": username, "detail": detail}) }
func filepathDir(path string) string { for i := len(path)-1; i >= 0; i-- { if path[i] == '/' { if i == 0 { return "/" }; return path[:i] } }; return "." }
