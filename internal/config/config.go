package config

import (
	"encoding/json"
	"fmt"
	"os"
)

type Config struct {
	Listen      string            `json:"listen"`
	SessionKey  string            `json:"session_key"`
	AuditLog    string            `json:"audit_log"`
	Auth        AuthConfig        `json:"auth"`
	Services    Services          `json:"services"`
	PanelUpdate PanelUpdateConfig `json:"panel_update"`
}

type AuthConfig struct {
	Username     string `json:"username"`
	PasswordHash string `json:"password_hash"`
}

type Services struct {
	SingBox ServiceConfig `json:"singbox"`
	MosDNS  MosDNSConfig  `json:"mosdns"`
}

type ServiceConfig struct {
	ServiceName string `json:"service_name"`
	ConfigPath  string `json:"config_path"`
	URLPath     string `json:"url_path"`
	BinPath     string `json:"bin_path"`
	CtlPath     string `json:"ctl_path"`
}

type MosDNSConfig struct {
	ServiceName string `json:"service_name"`
	CtlPath     string `json:"ctl_path"`
	WebURL      string `json:"web_url"`
}

type PanelUpdateConfig struct {
	ReleaseDir     string `json:"release_dir"`
	UpgradeCommand string `json:"upgrade_command"`
	BaseURL        string `json:"base_url"`
	Channel        string `json:"channel"`
	Arch           string `json:"arch"`
}

func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var cfg Config
	if err := json.Unmarshal(b, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if cfg.Listen == "" {
		cfg.Listen = ":9999"
	}
	if cfg.AuditLog == "" {
		cfg.AuditLog = "logs/audit.log"
	}
	if cfg.Services.SingBox.ServiceName == "" {
		cfg.Services.SingBox.ServiceName = "sing-box"
	}
	if cfg.Services.MosDNS.ServiceName == "" {
		cfg.Services.MosDNS.ServiceName = "mosdns"
	}
	return &cfg, nil
}
