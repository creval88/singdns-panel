package services

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	cfgpkg "singdns-panel/internal/config"
)

func TestSubscriptionHistoryAndUpdateEvents_RoundTrip(t *testing.T) {
	tmp := t.TempDir()
	svc := &SingBoxService{
		cfg: cfgpkg.ServiceConfig{ConfigPath: filepath.Join(tmp, "panel.json")},
	}

	svc.AppendSubscriptionHistory("https://example.com/sub")
	history, err := svc.SubscriptionHistory()
	if err != nil {
		t.Fatalf("SubscriptionHistory error: %v", err)
	}
	if len(history) != 1 {
		t.Fatalf("expected 1 history item, got %d", len(history))
	}
	if history[0].URL != "https://example.com/sub" {
		t.Fatalf("unexpected history url: %s", history[0].URL)
	}

	svc.AppendSubscriptionUpdateEventDetailed("ok", "update", "complete", "https://example.com/sub", "done", 0)
	events, err := svc.SubscriptionUpdateEvents(10)
	if err != nil {
		t.Fatalf("SubscriptionUpdateEvents error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 update event, got %d", len(events))
	}
	if events[0].Status != "ok" || events[0].Stage != "complete" {
		t.Fatalf("unexpected event: %+v", events[0])
	}
	if !strings.Contains(events[0].URL, "example.com") {
		t.Fatalf("unexpected event url: %s", events[0].URL)
	}
}

func TestSubscriptionUpdateEvents_Compat5Columns(t *testing.T) {
	tmp := t.TempDir()
	svc := &SingBoxService{
		cfg: cfgpkg.ServiceConfig{ConfigPath: filepath.Join(tmp, "panel.json")},
	}

	logPath := svc.subscriptionUpdateLogPath()
	legacy := "2026-03-18 00:00:00\tok\tupdate\thttps://example.com/sub\tlegacy format\n"
	if err := os.WriteFile(logPath, []byte(legacy), 0644); err != nil {
		t.Fatalf("write legacy log failed: %v", err)
	}
	events, err := svc.SubscriptionUpdateEvents(10)
	if err != nil {
		t.Fatalf("SubscriptionUpdateEvents error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Stage != "update" {
		t.Fatalf("expected stage fallback to action, got %s", events[0].Stage)
	}
}
