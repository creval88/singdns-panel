package services

import (
	"os"
	"path/filepath"
	"testing"

	cfgpkg "singdns-panel/internal/config"
)

func TestParseManualNodeLines_VLESSRealitySample(t *testing.T) {
	raw := `vless://0d1b4dc9-e011-44ce-a413-3e4c3e2cbbc0@blogyt.zbc.ink:54298?type=tcp&encryption=none&security=reality&pbk=0s9BTv0JRMIh3emRj9AJsIhIMnQqXEIZnWZTGJewCyw&fp=chrome&sni=www.speedtest.net&sid=8fc51d72&spx=%2F&flow=xtls-rprx-vision#Reality-YT-HK`
	nodes, result, err := parseManualNodeLines(raw)
	if err != nil {
		t.Fatalf("parseManualNodeLines err: %v", err)
	}
	if result == nil || result.Success != 1 || result.Failed != 0 || result.ParsedNodes != 1 {
		t.Fatalf("unexpected result: %#v", result)
	}
	if len(nodes) != 1 {
		t.Fatalf("unexpected nodes len=%d", len(nodes))
	}
	n := nodes[0]
	if n["tag"] != "Reality-YT-HK" || n["type"] != "vless" {
		t.Fatalf("unexpected basic node: %#v", n)
	}
	if n["server"] != "blogyt.zbc.ink" || n["server_port"] != 54298 {
		t.Fatalf("unexpected server fields: %#v", n)
	}
	if n["uuid"] != "0d1b4dc9-e011-44ce-a413-3e4c3e2cbbc0" || n["flow"] != "xtls-rprx-vision" {
		t.Fatalf("unexpected uuid/flow: %#v", n)
	}
	tls, _ := n["tls"].(map[string]any)
	if tls == nil || tls["enabled"] != true || tls["server_name"] != "www.speedtest.net" {
		t.Fatalf("unexpected tls: %#v", tls)
	}
	reality, _ := tls["reality"].(map[string]any)
	if reality == nil || reality["enabled"] != true || reality["public_key"] != "0s9BTv0JRMIh3emRj9AJsIhIMnQqXEIZnWZTGJewCyw" || reality["short_id"] != "8fc51d72" {
		t.Fatalf("unexpected reality: %#v", reality)
	}
	utls, _ := tls["utls"].(map[string]any)
	if utls == nil || utls["enabled"] != true || utls["fingerprint"] != "chrome" {
		t.Fatalf("unexpected utls: %#v", utls)
	}
}

func TestParseManualNodeLines_WithDuplicateAndInvalid(t *testing.T) {
	raw := `vless://0d1b4dc9-e011-44ce-a413-3e4c3e2cbbc0@blogyt.zbc.ink:54298?security=reality&pbk=0s9BTv0JRMIh3emRj9AJsIhIMnQqXEIZnWZTGJewCyw&fp=chrome&sni=www.speedtest.net&sid=8fc51d72&flow=xtls-rprx-vision#Reality-YT-HK
invalid://x
vless://0d1b4dc9-e011-44ce-a413-3e4c3e2cbbc0@blogyt.zbc.ink:54298?security=reality&pbk=0s9BTv0JRMIh3emRj9AJsIhIMnQqXEIZnWZTGJewCyw&fp=chrome&sni=www.speedtest.net&sid=8fc51d72&flow=xtls-rprx-vision#Reality-YT-HK`
	_, result, err := parseManualNodeLines(raw)
	if err != nil {
		t.Fatalf("parseManualNodeLines err: %v", err)
	}
	if result.Total != 3 || result.Success != 1 || result.Failed != 1 || result.Ignored != 1 || result.ParsedNodes != 1 {
		t.Fatalf("unexpected summary: %#v", result)
	}
	if len(result.LineResults) != 3 {
		t.Fatalf("unexpected line results len=%d", len(result.LineResults))
	}
	if result.LineResults[1].Status != "failed" {
		t.Fatalf("line2 status=%s", result.LineResults[1].Status)
	}
	if result.LineResults[2].Status != "ignored" {
		t.Fatalf("line3 status=%s", result.LineResults[2].Status)
	}
}

func TestManualNodesDraftSaveAndRead(t *testing.T) {
	tmp := t.TempDir()
	s := &SingBoxService{cfg: cfgpkg.ServiceConfig{ConfigPath: filepath.Join(tmp, "config.json")}}

	res, err := s.SaveManualNodesDraft("vless://a\nss://b")
	if err != nil {
		t.Fatalf("SaveManualNodesDraft err: %v", err)
	}
	if res == nil || res.Action != "manual_nodes.save" {
		t.Fatalf("unexpected save result: %#v", res)
	}
	got, err := s.ReadManualNodesDraft()
	if err != nil {
		t.Fatalf("ReadManualNodesDraft err: %v", err)
	}
	if got != "vless://a\nss://b" {
		t.Fatalf("unexpected draft content: %q", got)
	}

	_, err = s.SaveManualNodesDraft("")
	if err != nil {
		t.Fatalf("clear draft err: %v", err)
	}
	got, err = s.ReadManualNodesDraft()
	if err != nil {
		t.Fatalf("ReadManualNodesDraft after clear err: %v", err)
	}
	if got != "" {
		t.Fatalf("expected empty draft after clear, got: %q", got)
	}

	if _, err := os.Stat(filepath.Join(tmp, "manual-nodes.txt")); err != nil {
		t.Fatalf("expected draft file exists: %v", err)
	}
}
