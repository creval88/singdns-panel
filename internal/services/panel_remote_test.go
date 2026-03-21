package services

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
