package services

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	cfgpkg "singdns-panel/internal/config"
)

func TestResolveRemoteRelease_FromManifest(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/manifest.json", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"channels": map[string]any{
				"beta": map[string]any{
					"amd64": map[string]any{
						"version": "v1.2.3",
						"url":     "https://example.com/singdns-panel-v1.2.3-amd64.tar.gz",
						"sha256":  "abc123",
					},
				},
			},
		})
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	svc := NewPanelService("v1.0.0", cfgpkg.PanelUpdateConfig{
		BaseURL: server.URL + "/manifest.json",
		Channel: "beta",
		Arch:    "amd64",
	})

	rel, err := svc.ResolveRemoteRelease()
	if err != nil {
		t.Fatalf("ResolveRemoteRelease error: %v", err)
	}
	if rel == nil {
		t.Fatal("expected remote release, got nil")
	}
	if rel.Version != "v1.2.3" {
		t.Fatalf("unexpected version: %s", rel.Version)
	}
	if rel.Channel != "beta" || rel.Arch != "amd64" {
		t.Fatalf("unexpected channel/arch: %s/%s", rel.Channel, rel.Arch)
	}
	if rel.ManifestURL != server.URL+"/manifest.json" {
		t.Fatalf("unexpected manifest url: %s", rel.ManifestURL)
	}
}

func TestProbeRemoteRelease_HeadOK(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/manifest.json", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"channels": map[string]any{
				"beta": map[string]any{
					"amd64": map[string]any{
						"version": "v2.0.0",
						"url":     "REPLACE_ME/pkg.tar.gz",
					},
				},
			},
		})
	})
	mux.HandleFunc("/pkg.tar.gz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	// 覆盖 manifest 中占位 URL
	mux = http.NewServeMux()
	mux.HandleFunc("/manifest.json", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"channels": map[string]any{
				"beta": map[string]any{
					"amd64": map[string]any{
						"version": "v2.0.0",
						"url":     server.URL + "/pkg.tar.gz",
					},
				},
			},
		})
	})
	mux.HandleFunc("/pkg.tar.gz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	server.Config.Handler = mux

	svc := NewPanelService("v1.0.0", cfgpkg.PanelUpdateConfig{
		BaseURL: server.URL + "/manifest.json",
		Channel: "beta",
		Arch:    "amd64",
	})

	probe, err := svc.ProbeRemoteRelease()
	if err != nil {
		t.Fatalf("ProbeRemoteRelease error: %v", err)
	}
	if probe == nil {
		t.Fatal("expected probe result, got nil")
	}
	if !probe.PackageOK {
		t.Fatalf("expected package_ok=true, got false (status=%d message=%s)", probe.PackageStatus, probe.PackageMessage)
	}
	if probe.PackageStatus != http.StatusOK {
		t.Fatalf("unexpected package status: %d", probe.PackageStatus)
	}
}

func TestProbeRemoteRelease_FallbackToRangeGET(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/manifest.json", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"channels": map[string]any{
				"beta": map[string]any{
					"amd64": map[string]any{
						"version": "v2.1.0",
						"url":     "REPLACE_ME/pkg.tar.gz",
					},
				},
			},
		})
	})
	mux.HandleFunc("/pkg.tar.gz", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if r.Header.Get("Range") == "bytes=0-0" {
			w.WriteHeader(http.StatusPartialContent)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	// 覆盖 manifest 中占位 URL
	mux = http.NewServeMux()
	mux.HandleFunc("/manifest.json", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"channels": map[string]any{
				"beta": map[string]any{
					"amd64": map[string]any{
						"version": "v2.1.0",
						"url":     server.URL + "/pkg.tar.gz",
					},
				},
			},
		})
	})
	mux.HandleFunc("/pkg.tar.gz", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if r.Header.Get("Range") == "bytes=0-0" {
			w.WriteHeader(http.StatusPartialContent)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	server.Config.Handler = mux

	svc := NewPanelService("v1.0.0", cfgpkg.PanelUpdateConfig{
		BaseURL: server.URL + "/manifest.json",
		Channel: "beta",
		Arch:    "amd64",
	})

	probe, err := svc.ProbeRemoteRelease()
	if err != nil {
		t.Fatalf("ProbeRemoteRelease error: %v", err)
	}
	if probe.PackageStatus != http.StatusPartialContent {
		t.Fatalf("expected 206 from range GET fallback, got %d", probe.PackageStatus)
	}
	if !probe.PackageOK {
		t.Fatalf("expected package_ok=true on 206, message=%s", probe.PackageMessage)
	}
}

func TestProbeRemoteRelease_BadStatus(t *testing.T) {
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	defer server.Close()

	mux.HandleFunc("/manifest.json", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"channels": map[string]any{
				"beta": map[string]any{
					"amd64": map[string]any{
						"version": "v2.2.0",
						"url":     server.URL + "/pkg.tar.gz",
					},
				},
			},
		})
	})
	mux.HandleFunc("/pkg.tar.gz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	server.Config.Handler = mux

	svc := NewPanelService("v1.0.0", cfgpkg.PanelUpdateConfig{
		BaseURL: server.URL + "/manifest.json",
		Channel: "beta",
		Arch:    "amd64",
	})

	probe, err := svc.ProbeRemoteRelease()
	if err != nil {
		t.Fatalf("ProbeRemoteRelease should not return hard error for 404 package, got: %v", err)
	}
	if probe.PackageOK {
		t.Fatalf("expected package_ok=false for 404")
	}
	if probe.PackageStatus != http.StatusNotFound {
		t.Fatalf("unexpected package status: %d", probe.PackageStatus)
	}
}

func TestPanelUpgradePreflightLocalRequiresCompleteRelease(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	rel := filepath.Join(tmp, "v1.2.3")
	if err := os.MkdirAll(filepath.Join(rel, "bin"), 0755); err != nil {
		t.Fatalf("mkdir release: %v", err)
	}
	if err := os.WriteFile(filepath.Join(rel, "VERSION"), []byte("v1.2.3\n"), 0644); err != nil {
		t.Fatalf("write version: %v", err)
	}
	if err := os.WriteFile(filepath.Join(rel, "bin", "singdns-panel"), []byte("binary"), 0755); err != nil {
		t.Fatalf("write binary: %v", err)
	}

	svc := NewPanelService("v1.0.0", cfgpkg.PanelUpdateConfig{ReleaseDir: tmp})
	preflight, err := svc.UpgradePreflight("local")
	if err != nil {
		t.Fatalf("UpgradePreflight: %v", err)
	}
	if preflight.OK {
		t.Fatalf("expected preflight to fail without upgrade.sh: %#v", preflight)
	}
	if !preflightHasCheck(preflight, "升级脚本", false) {
		t.Fatalf("expected failed upgrade script check: %#v", preflight.Checks)
	}
}

func TestPanelUpgradePreflightLocalPassesCompleteRelease(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	rel := filepath.Join(tmp, "v1.2.3")
	if err := os.MkdirAll(filepath.Join(rel, "bin"), 0755); err != nil {
		t.Fatalf("mkdir release: %v", err)
	}
	if err := os.WriteFile(filepath.Join(rel, "VERSION"), []byte("v1.2.3\n"), 0644); err != nil {
		t.Fatalf("write version: %v", err)
	}
	if err := os.WriteFile(filepath.Join(rel, "bin", "singdns-panel"), []byte("binary"), 0755); err != nil {
		t.Fatalf("write binary: %v", err)
	}
	if err := os.WriteFile(filepath.Join(rel, "upgrade.sh"), []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatalf("write upgrade script: %v", err)
	}

	svc := NewPanelService("v1.0.0", cfgpkg.PanelUpdateConfig{ReleaseDir: tmp})
	preflight, err := svc.UpgradePreflight("local")
	if err != nil {
		t.Fatalf("UpgradePreflight: %v", err)
	}
	if !preflight.OK {
		t.Fatalf("expected preflight ok, got %#v", preflight)
	}
}

func TestUpgradeTaskStepsAreRecordedAndCloned(t *testing.T) {
	t.Parallel()

	svc := NewPanelService("v1.0.0", cfgpkg.PanelUpdateConfig{})
	task := svc.NewTask("local", "", "")
	svc.MarkTaskStep(task.ID, "running", "执行升级脚本", "stdout line")
	svc.MarkTaskFailed(task.ID, os.ErrPermission)

	got := svc.Task(task.ID)
	if got == nil {
		t.Fatal("expected task")
	}
	if len(got.Steps) < 3 {
		t.Fatalf("expected task steps to be recorded, got %#v", got.Steps)
	}
	if got.Steps[len(got.Steps)-1].Status != "failed" {
		t.Fatalf("expected final failed step, got %#v", got.Steps)
	}
	got.Steps[0].Message = "mutated"

	again := svc.Task(task.ID)
	if again.Steps[0].Message == "mutated" {
		t.Fatalf("task clone leaked mutable steps")
	}
}

func preflightHasCheck(preflight *PanelUpgradePreflight, name string, ok bool) bool {
	if preflight == nil {
		return false
	}
	for _, check := range preflight.Checks {
		if check.Name == name && check.OK == ok {
			return true
		}
	}
	return false
}
