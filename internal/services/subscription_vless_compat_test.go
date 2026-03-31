package services

import (
	"testing"
)

func TestParseVLESSLine_RealityUTLSFlow(t *testing.T) {
	line := "vless://ce1c7040-f250-4fc1-9a6e-555ab2d14ab7@sssss.ss888.online:29858?security=reality&sni=www.tesla.com&pbk=swvPMHYCpCCZionjEkjLnngAQyhrsHka4mZcCp4-0gY&sid=9054d0&fp=chrome&flow=xtls-rprx-vision#%F0%9F%87%BA%F0%9F%87%B8%20Reality-DM-US"
	node, err := parseVLESSLine(line)
	if err != nil {
		t.Fatalf("parseVLESSLine err: %v", err)
	}
	if node["type"] != "vless" || node["tag"] != "🇺🇸 Reality-DM-US" {
		t.Fatalf("unexpected basic fields: %#v", node)
	}
	if node["server"] != "sssss.ss888.online" || node["server_port"] != 29858 {
		t.Fatalf("unexpected server fields: %#v", node)
	}
	if node["uuid"] != "ce1c7040-f250-4fc1-9a6e-555ab2d14ab7" || node["flow"] != "xtls-rprx-vision" {
		t.Fatalf("unexpected uuid/flow: %#v", node)
	}
	tls, ok := node["tls"].(map[string]any)
	if !ok || tls["enabled"] != true || tls["server_name"] != "www.tesla.com" {
		t.Fatalf("unexpected tls: %#v", node["tls"])
	}
	reality, ok := tls["reality"].(map[string]any)
	if !ok || reality["enabled"] != true || reality["public_key"] != "swvPMHYCpCCZionjEkjLnngAQyhrsHka4mZcCp4-0gY" || reality["short_id"] != "9054d0" {
		t.Fatalf("unexpected reality: %#v", tls["reality"])
	}
	utls, ok := tls["utls"].(map[string]any)
	if !ok || utls["enabled"] != true || utls["fingerprint"] != "chrome" {
		t.Fatalf("unexpected utls: %#v", tls["utls"])
	}
}

func TestParseSubscriptionNodes_SingboxOutboundsSnippet(t *testing.T) {
	raw := `{"outbounds":[{"tag":"🇺🇸 Reality-DM-US","type":"vless","server":"sssss.ss888.online","server_port":29858,"uuid":"ce1c7040-f250-4fc1-9a6e-555ab2d14ab7","tls":{"enabled":true,"server_name":"www.tesla.com","insecure":false,"reality":{"enabled":true,"public_key":"swvPMHYCpCCZionjEkjLnngAQyhrsHka4mZcCp4-0gY","short_id":"9054d0"},"utls":{"enabled":true,"fingerprint":"chrome"}},"flow":"xtls-rprx-vision"}]}`
	nodes, err := parseSubscriptionNodes(raw)
	if err != nil {
		t.Fatalf("parseSubscriptionNodes err: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("nodes len=%d", len(nodes))
	}
	n := nodes[0]
	if n["tag"] != "🇺🇸 Reality-DM-US" || n["type"] != "vless" || n["flow"] != "xtls-rprx-vision" {
		t.Fatalf("unexpected node: %#v", n)
	}
	tls, _ := n["tls"].(map[string]any)
	if tls == nil || tls["enabled"] != true || tls["server_name"] != "www.tesla.com" || tls["insecure"] != false {
		t.Fatalf("unexpected tls: %#v", tls)
	}
	reality, _ := tls["reality"].(map[string]any)
	if reality == nil || reality["enabled"] != true || reality["public_key"] != "swvPMHYCpCCZionjEkjLnngAQyhrsHka4mZcCp4-0gY" || reality["short_id"] != "9054d0" {
		t.Fatalf("unexpected reality: %#v", reality)
	}
	utls, _ := tls["utls"].(map[string]any)
	if utls == nil || utls["enabled"] != true || utls["fingerprint"] != "chrome" {
		t.Fatalf("unexpected utls: %#v", utls)
	}
}

func TestParseSubscriptionNodes_ClashVLESSProxy(t *testing.T) {
	raw := "proxies:\n - {\"type\":\"vless\",\"name\":\"🇺🇸 Reality-DM-US\",\"server\":\"sssss.ss888.online\",\"port\":29858,\"uuid\":\"ce1c7040-f250-4fc1-9a6e-555ab2d14ab7\",\"tls\":true,\"flow\":\"xtls-rprx-vision\",\"client-fingerprint\":\"chrome\",\"skip-cert-verify\":false,\"reality-opts\":{\"public-key\":\"swvPMHYCpCCZionjEkjLnngAQyhrsHka4mZcCp4-0gY\",\"short-id\":\"9054d0\"},\"network\":\"tcp\",\"udp\":true,\"servername\":\"www.tesla.com\"}"
	nodes, err := parseSubscriptionNodes(raw)
	if err != nil {
		t.Fatalf("parseSubscriptionNodes err: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("nodes len=%d", len(nodes))
	}
	n := nodes[0]
	if n["tag"] != "🇺🇸 Reality-DM-US" || n["type"] != "vless" || n["server_port"] != 29858 || n["flow"] != "xtls-rprx-vision" {
		t.Fatalf("unexpected node: %#v", n)
	}
	tls, _ := n["tls"].(map[string]any)
	if tls == nil || tls["enabled"] != true || tls["server_name"] != "www.tesla.com" || tls["insecure"] != false {
		t.Fatalf("unexpected tls: %#v", tls)
	}
	reality, _ := tls["reality"].(map[string]any)
	if reality == nil || reality["enabled"] != true || reality["public_key"] != "swvPMHYCpCCZionjEkjLnngAQyhrsHka4mZcCp4-0gY" || reality["short_id"] != "9054d0" {
		t.Fatalf("unexpected reality: %#v", reality)
	}
	utls, _ := tls["utls"].(map[string]any)
	if utls == nil || utls["enabled"] != true || utls["fingerprint"] != "chrome" {
		t.Fatalf("unexpected utls: %#v", utls)
	}
}
