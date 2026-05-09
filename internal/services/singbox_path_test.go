package services

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizeSingBoxBinPathPrefersSystemdBinary(t *testing.T) {
	t.Parallel()

	got := normalizeSingBoxBinPath("/usr/bin/sing-box", "/usr/local/bin/sing-box")
	if got != "/usr/local/bin/sing-box" {
		t.Fatalf("expected systemd binary, got %q", got)
	}
}

func TestNormalizeSingBoxBinPathFallsBackToConfiguredBinary(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	configured := filepath.Join(tmp, "sing-box")
	if err := os.WriteFile(configured, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatalf("write fake binary: %v", err)
	}

	got := normalizeSingBoxBinPath(configured, "")
	if got != configured {
		t.Fatalf("expected configured binary, got %q", got)
	}
}

func TestNormalizeSingBoxBinPathKeepsSystemdBinaryEvenWhenMissing(t *testing.T) {
	t.Parallel()

	want := "/opt/custom/sing-box"
	got := normalizeSingBoxBinPath("/usr/bin/sing-box", want)
	if got != want {
		t.Fatalf("expected systemd binary to be preserved, got %q", got)
	}
}
