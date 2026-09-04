package main

import "testing"

func TestCloudDNSRecordValidation(t *testing.T) {
	valid := []cloudDNSRequest{{Type: "A", Name: "@", Target: "192.0.2.10", TTL: 300}, {Type: "AAAA", Name: "www", Target: "2001:db8::1", TTL: 300}, {Type: "MX", Name: "@", Target: "10 mail.example.com", TTL: 300}, {Type: "SRV", Name: "_https._tcp", Target: "10 5 443 service.example.com", TTL: 300}}
	for _, record := range valid {
		if !cloudDNSRecordValid(record) {
			t.Errorf("valid record rejected: %#v", record)
		}
	}
	invalid := []cloudDNSRequest{{Type: "A", Name: "@", Target: "example.com", TTL: 300}, {Type: "A", Name: "@", Target: "192.0.2.1", TTL: 10}, {Type: "MX", Name: "@", Target: "mail.example.com", TTL: 300}}
	for _, record := range invalid {
		if cloudDNSRecordValid(record) {
			t.Errorf("invalid record accepted: %#v", record)
		}
	}
}

func TestDNSRecordExists(t *testing.T) {
	value := map[string]any{"data": []any{map[string]any{"type": "A", "name": "www.example.com.", "target": "192.0.2.10"}}}
	if !dnsRecordExists(value, cloudDNSRequest{Type: "a", Name: "www.example.com", Target: "192.0.2.10"}) {
		t.Fatal("identical DNS record was not detected")
	}
}
