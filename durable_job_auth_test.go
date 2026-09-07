package main

import "testing"

func TestAuthorizeDurableSiteJobRechecksSuspensionAndOwnership(t *testing.T) {
	app := &App{Auth: Auth{Username: "admin"}, Accounts: &AccountStore{accounts: map[string]HostingAccount{
		"customer": {Username: "customer", Plan: "starter", Sites: []string{"owned"}},
	}}}
	if err := app.authorizeDurableSiteJob("owned", "customer", false); err != nil {
		t.Fatalf("owned customer job rejected: %v", err)
	}
	app.Accounts.accounts["customer"] = HostingAccount{Username: "customer", Plan: "starter", Sites: []string{"owned"}, Suspended: true}
	if err := app.authorizeDurableSiteJob("owned", "customer", false); err == nil {
		t.Fatal("suspended customer job was accepted")
	}
	if err := app.authorizeDurableSiteJob("unowned", "admin", false); err != nil {
		t.Fatalf("administrator job rejected: %v", err)
	}
	if err := app.authorizeDurableSiteJob("owned", "scheduler", true); err != nil {
		t.Fatalf("scheduled job rejected: %v", err)
	}
}
