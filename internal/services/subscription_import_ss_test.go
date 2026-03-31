package services

import "testing"

func TestParseSSLine_UserinfoBase64URL_NoPadding(t *testing.T) {
	line := "ss://MjAyMi1ibGFrZTMtYWVzLTEyOC1nY206akdud1kwUGJzV3hUcUF6bmd3aWpkQT09OkFXRHRDYlBjVklBemJHc2UyRmJ0Tnc9PQ@b7e3d98.mnmjutnn.sbs:19300/?group=QW15VGVsZWNvbQ#香港%2001"
	node, err := parseSSLine(line)
	if err != nil {
		t.Fatalf("parseSSLine err: %v", err)
	}
	if node["type"] != "shadowsocks" {
		t.Fatalf("type=%v", node["type"])
	}
	if node["method"] != "2022-blake3-aes-128-gcm" {
		t.Fatalf("method=%v", node["method"])
	}
	if node["server"] != "b7e3d98.mnmjutnn.sbs" {
		t.Fatalf("server=%v", node["server"])
	}
	if node["server_port"] != 19300 {
		t.Fatalf("port=%v", node["server_port"])
	}
	if node["tag"] != "香港 01" {
		t.Fatalf("tag=%v", node["tag"])
	}
}
