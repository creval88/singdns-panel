package services

import (
	"encoding/json"
	"strings"
)

func isSingboxOutboundsOnly(content string) bool {
	_, ok := parseSingboxOutboundsOnly(content)
	return ok
}

func parseSingboxOutboundsOnly(content string) ([]map[string]any, bool) {
	var raw map[string]any
	if err := json.Unmarshal([]byte(content), &raw); err != nil {
		return nil, false
	}
	if len(raw) != 1 {
		return nil, false
	}
	list, ok := raw["outbounds"].([]any)
	if !ok || len(list) == 0 {
		return nil, false
	}
	nodes := make([]map[string]any, 0, len(list))
	for _, item := range list {
		m, ok := item.(map[string]any)
		if !ok || m == nil {
			continue
		}
		norm, ok := normalizeNodeMap(m)
		if !ok {
			continue
		}
		nodes = append(nodes, norm)
	}
	if len(nodes) == 0 {
		return nil, false
	}
	return nodes, true
}

func parseClashProxiesContent(content string) ([]map[string]any, bool) {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" || !strings.Contains(trimmed, "proxies:") {
		return nil, false
	}
	lines := strings.Split(strings.ReplaceAll(trimmed, "\r\n", "\n"), "\n")
	inProxies := false
	entries := make([]string, 0)
	for _, line := range lines {
		raw := strings.TrimSpace(line)
		if raw == "" {
			continue
		}
		if !inProxies {
			if strings.HasPrefix(raw, "proxies:") {
				inProxies = true
			}
			continue
		}
		if strings.HasPrefix(raw, "-") {
			entry := strings.TrimSpace(strings.TrimPrefix(raw, "-"))
			if entry != "" {
				entries = append(entries, entry)
			}
		}
	}
	if len(entries) == 0 {
		return nil, false
	}
	nodes := make([]map[string]any, 0, len(entries))
	for _, entry := range entries {
		var obj map[string]any
		if err := json.Unmarshal([]byte(entry), &obj); err != nil {
			continue
		}
		norm, ok := normalizeNodeMap(obj)
		if !ok {
			continue
		}
		nodes = append(nodes, norm)
	}
	if len(nodes) == 0 {
		return nil, false
	}
	return nodes, true
}

func normalizeNodeMap(in map[string]any) (map[string]any, bool) {
	if in == nil {
		return nil, false
	}
	typ, _ := asStringField(in["type"])
	typ = strings.ToLower(strings.TrimSpace(typ))
	switch typ {
	case "vmess", "vless", "trojan", "shadowsocks", "ss":
	default:
		return nil, false
	}
	if typ == "ss" {
		typ = "shadowsocks"
	}
	out := map[string]any{"type": typ}

	tag, _ := asStringField(in["tag"])
	if tag == "" {
		tag, _ = asStringField(in["name"])
	}
	server, _ := asStringField(in["server"])
	if server == "" {
		server, _ = asStringField(in["add"])
	}
	port, _ := asIntField(in["server_port"])
	if port == 0 {
		port, _ = asIntField(in["port"])
	}
	if tag == "" {
		tag = server
	}
	if tag == "" || server == "" || port <= 0 {
		return nil, false
	}
	out["tag"] = tag
	out["server"] = server
	out["server_port"] = port

	switch typ {
	case "vless", "vmess":
		uuid, _ := asStringField(in["uuid"])
		if uuid == "" {
			uuid, _ = asStringField(in["id"])
		}
		if uuid == "" {
			return nil, false
		}
		out["uuid"] = uuid
	case "trojan":
		pwd, _ := asStringField(in["password"])
		if pwd == "" {
			return nil, false
		}
		out["password"] = pwd
	case "shadowsocks":
		method, _ := asStringField(in["method"])
		pwd, _ := asStringField(in["password"])
		if method == "" || pwd == "" {
			return nil, false
		}
		out["method"] = method
		out["password"] = pwd
	}
	if flow, _ := asStringField(in["flow"]); flow != "" {
		out["flow"] = flow
	}
	applyTLSFields(out, in)
	return out, true
}

func applyTLSFields(out, source map[string]any) {
	tlsOut := map[string]any{}
	enabled := false

	if tlsMap, ok := source["tls"].(map[string]any); ok {
		for k, v := range tlsMap {
			tlsOut[k] = v
		}
		if b, ok := tlsOut["enabled"].(bool); ok {
			enabled = b
		} else {
			enabled = true
			tlsOut["enabled"] = true
		}
	} else if b, ok := source["tls"].(bool); ok {
		enabled = b
		if b {
			tlsOut["enabled"] = true
		}
	}

	if sni, _ := asStringField(source["server_name"]); sni != "" {
		tlsOut["server_name"] = sni
		enabled = true
	}
	if sni, _ := asStringField(source["servername"]); sni != "" {
		tlsOut["server_name"] = sni
		enabled = true
	}
	if sni, _ := asStringField(source["sni"]); sni != "" {
		tlsOut["server_name"] = sni
		enabled = true
	}
	if insecure, ok := source["insecure"].(bool); ok {
		tlsOut["insecure"] = insecure
		enabled = true
	}
	if skip, ok := source["skip-cert-verify"].(bool); ok {
		tlsOut["insecure"] = skip
		enabled = true
	}

	if fp, _ := asStringField(source["client-fingerprint"]); fp != "" {
		tlsOut["utls"] = map[string]any{"enabled": true, "fingerprint": fp}
		enabled = true
	}
	if utlsMap, ok := tlsOut["utls"].(map[string]any); ok {
		if _, ok := utlsMap["enabled"]; !ok {
			utlsMap["enabled"] = true
		}
		tlsOut["utls"] = utlsMap
		enabled = true
	}

	reality := map[string]any{}
	if r, ok := source["reality-opts"].(map[string]any); ok {
		if pk, _ := asStringField(r["public-key"]); pk != "" {
			reality["public_key"] = pk
		}
		if sid, _ := asStringField(r["short-id"]); sid != "" {
			reality["short_id"] = sid
		}
	}
	if tlsMap, ok := source["tls"].(map[string]any); ok {
		if r, ok := tlsMap["reality"].(map[string]any); ok {
			if pk, _ := asStringField(r["public_key"]); pk != "" {
				reality["public_key"] = pk
			}
			if sid, _ := asStringField(r["short_id"]); sid != "" {
				reality["short_id"] = sid
			}
		}
	}
	if len(reality) > 0 {
		reality["enabled"] = true
		tlsOut["reality"] = reality
		enabled = true
	}
	if enabled {
		tlsOut["enabled"] = true
		out["tls"] = tlsOut
	}
}
