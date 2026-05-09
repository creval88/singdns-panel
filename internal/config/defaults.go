package config

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

const defaultPasswordHash = "$2a$10$mhgzMC./.jG65Pw8OhUo1Ocmw9UrwsatLMrk7Ii95Ag0DcCKcR1/a"

const DefaultConfigTemplate = `{
  "listen": ":9999",
  "session_key": "change-me",
  "audit_log": "logs/audit.log",
  "auth": {
    "username": "admin",
    "password_hash": "` + defaultPasswordHash + `"
  },
  "panel_update": {
    "release_dir": "/opt/singdns-panel/updates",
    "upgrade_command": "",
    "base_url": "https://github.com/creval88/singdns-panel/releases/latest/download/latest.json",
    "channel": "beta",
    "arch": "amd64"
  },
  "monitor": {
    "enabled": false,
    "api_base": "http://127.0.0.1:9090",
    "default_proxy_group": "默认代理",
    "primary_group": "香港手动",
    "fallback_group": "自建节点",
    "test_url": "http://www.gstatic.com/generate_204",
    "timeout_ms": 5000,
    "primary_max_stable_delay_ms": 150,
    "fallback_max_stable_delay_ms": 500,
    "disable_primary_group_optimization": false,
    "fail_threshold": 2,
    "success_threshold": 2,
    "recheck_interval_sec": 1,
    "auto_failback": false,
    "state_file": "/opt/singdns-panel/data/monitor-state.json"
  },
  "services": {
    "singbox": {
      "service_name": "sing-box",
      "config_path": "/etc/sing-box/config.json",
      "template_path": "/opt/singdns-panel/app/configs/singbox-template.json",
      "url_path": "/etc/sing-box/url.txt",
      "bin_path": "/usr/bin/sing-box",
      "ctl_path": "/usr/local/bin/sbctl.sh"
    },
    "mosdns": {
      "service_name": "mosdns",
      "ctl_path": "/usr/local/bin/mdctl.sh",
      "web_url": "http://10.0.0.8:9099/log"
    }
  }
}`

type InitialConfig struct {
	Content  string
	Username string
	Password string
}

func GenerateInitialConfig() (*InitialConfig, error) {
	sessionKey, err := randomToken(32)
	if err != nil {
		return nil, fmt.Errorf("generate session key: %w", err)
	}
	password, err := randomToken(18)
	if err != nil {
		return nil, fmt.Errorf("generate initial password: %w", err)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash initial password: %w", err)
	}
	content := strings.Replace(DefaultConfigTemplate, `"session_key": "change-me"`, fmt.Sprintf(`"session_key": "%s"`, sessionKey), 1)
	content = strings.Replace(content, defaultPasswordHash, string(hash), 1)
	return &InitialConfig{Content: content, Username: "admin", Password: password}, nil
}

func randomToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
