package handlers

import (
	"testing"

	"singdns-panel/internal/services"
)

func TestBuildDashboardHealthCatchesOperationalRisks(t *testing.T) {
	t.Parallel()

	health := buildDashboardHealth(
		&services.ServiceStatus{Name: "sing-box", Active: true},
		&services.ServiceStatus{Name: "mosdns", Active: true},
		&services.ServiceStatus{Name: "singdns-panel", Active: true},
		&services.SubscriptionStatus{Configured: true},
		nil,
		&services.BackupStatus{Count: 0},
		&services.ConfigStatus{ServerJSONValid: false},
		&services.ClashAPIInfo{Enabled: false},
		&services.HostStats{CPUPercent: "91.0%", MemPercent: "92.5%"},
	)

	if health.Level != "bad" {
		t.Fatalf("expected bad health, got %#v", health)
	}
	for _, want := range []string{
		"sing-box 配置 JSON 无效",
		"暂无 sing-box 配置备份",
		"Clash API 未启用",
		"CPU 使用率过高",
		"内存使用率过高",
	} {
		if !containsString(health.Issues, want) {
			t.Fatalf("expected issue %q, got %#v", want, health.Issues)
		}
	}
	if health.BackupCount != 0 || health.ConfigJSONValid || health.ClashAPIEnabled {
		t.Fatalf("unexpected status flags: %#v", health)
	}
}

func TestBuildDashboardHealthOKWhenCoreSignalsAreHealthy(t *testing.T) {
	t.Parallel()

	health := buildDashboardHealth(
		&services.ServiceStatus{Name: "sing-box", Active: true},
		&services.ServiceStatus{Name: "mosdns", Active: true},
		&services.ServiceStatus{Name: "singdns-panel", Active: true},
		&services.SubscriptionStatus{Configured: true},
		[]services.SubscriptionUpdateEvent{{Status: "ok", Time: "2026-05-09 10:00:00"}},
		&services.BackupStatus{Count: 2},
		&services.ConfigStatus{ServerJSONValid: true},
		&services.ClashAPIInfo{Enabled: true},
		&services.HostStats{CPUPercent: "10.0%", MemPercent: "40.0%"},
	)

	if health.Level != "ok" {
		t.Fatalf("expected ok health, got %#v", health)
	}
	if len(health.Issues) != 0 {
		t.Fatalf("expected no issues, got %#v", health.Issues)
	}
	if health.BackupCount != 2 || !health.ConfigJSONValid || !health.ClashAPIEnabled {
		t.Fatalf("unexpected status flags: %#v", health)
	}
}

func containsString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
