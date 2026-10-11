package webapi

import (
	"testing"
	"time"
)

func TestPollingExpiresPendingOAuthJobsWithoutCallback(t *testing.T) {
	now := time.Date(2026, 10, 11, 18, 0, 0, 0, time.UTC)
	box := newBoxOAuthManager(func() time.Time { return now })
	box.jobs["box"] = &boxOAuthJob{ID: "box", Status: "pending", state: "box-state", expires: now.Add(time.Minute)}
	box.byState["box-state"] = "box"
	sf := newSalesforceOAuthManager(func() time.Time { return now })
	sf.jobs["sf"] = &salesforceOAuthJob{ID: "sf", Status: "pending", state: "sf-state", expires: now.Add(time.Minute)}
	sf.byState["sf-state"] = "sf"
	if job, _ := box.get("box"); job.Status != "pending" {
		t.Fatalf("early Box status: %s", job.Status)
	}
	if job, _ := sf.get("sf"); job.Status != "pending" {
		t.Fatalf("early Salesforce status: %s", job.Status)
	}
	now = now.Add(time.Minute)
	if job, ok := box.get("box"); !ok || job.Status != "failed" || job.Message == "" {
		t.Fatalf("expired Box job: %#v", job)
	}
	if job, ok := sf.get("sf"); !ok || job.Status != "failed" || job.Message == "" {
		t.Fatalf("expired Salesforce job: %#v", job)
	}
	if _, err := box.lookup("box-state"); err == nil {
		t.Fatal("expired Box state accepted")
	}
	if _, err := sf.lookup("sf-state"); err == nil {
		t.Fatal("expired Salesforce state accepted")
	}
	// Polling an already successful login does not turn it into a timeout.
	box.jobs["done"] = &boxOAuthJob{ID: "done", Status: "active", expires: now.Add(-time.Minute)}
	sf.jobs["done"] = &salesforceOAuthJob{ID: "done", Status: "active", expires: now.Add(-time.Minute)}
	if job, _ := box.get("done"); job.Status != "active" {
		t.Fatal("active Box job expired")
	}
	if job, _ := sf.get("done"); job.Status != "active" {
		t.Fatal("active Salesforce job expired")
	}
}
