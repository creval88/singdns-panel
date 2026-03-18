package services

import (
	"strings"
	"testing"
	"time"

	cfgpkg "singdns-panel/internal/config"
)

func TestIsManagedCronLine_SubscriptionUpdateOnly(t *testing.T) {
	svc := &SingBoxService{cfg: cfgpkg.ServiceConfig{CtlPath: "/usr/local/bin/sbctl.sh"}}

	if !svc.isManagedCronLine("0 3 * * * SINGDNS_CONFIG='/opt/singdns-panel/app/configs/panel.json' '/opt/singdns-panel/singdns-panel' subscription-update") {
		t.Fatal("expected subscription-update cron line to be managed")
	}
	if svc.isManagedCronLine("0 3 * * * /bin/bash /usr/local/bin/sbctl.sh update") {
		t.Fatal("expected legacy sbctl update cron line to NOT be managed")
	}
}

func TestFilterManagedCronLines(t *testing.T) {
	svc := &SingBoxService{}
	in := []string{
		"MAILTO=\"\"",
		"0 3 * * * SINGDNS_CONFIG='/opt/singdns-panel/app/configs/panel.json' '/opt/singdns-panel/singdns-panel' subscription-update",
		"15 2 * * * /usr/bin/echo keep-me",
	}
	out := svc.filterManagedCronLines(in)
	joined := strings.Join(out, "\n")
	if strings.Contains(joined, "subscription-update") {
		t.Fatal("managed cron line should be removed")
	}
	if !strings.Contains(joined, "keep-me") {
		t.Fatal("non-managed cron line should be kept")
	}
}

func TestCronLineSummaryAndParse(t *testing.T) {
	line := "0 3 */2 * * SINGDNS_CONFIG='/x' '/y' subscription-update"
	s := cronLineSummary(line)
	if !strings.Contains(s, "每隔 2 天 3:00") {
		t.Fatalf("unexpected summary: %s", s)
	}

	info := &CronInfo{}
	parseCronLine(line, info)
	if info.Days != 2 || info.Hour != 3 {
		t.Fatalf("unexpected parse result: days=%d hour=%d", info.Days, info.Hour)
	}
	if strings.TrimSpace(info.NextRun) == "" {
		t.Fatal("next run should not be empty")
	}
}

func TestFormatDurationMS(t *testing.T) {
	if got := formatDurationMS(0); got != "" {
		t.Fatalf("expected empty duration for 0ms, got %q", got)
	}
	if got := formatDurationMS(250); got != "250ms" {
		t.Fatalf("unexpected duration text: %q", got)
	}
	if got := formatDurationMS(int64((1500 * time.Millisecond).Milliseconds())); !strings.Contains(got, "1.5") {
		t.Fatalf("expected ~1.5s duration, got %q", got)
	}
}
