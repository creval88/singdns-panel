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
	Monitor     MonitorConfig     `json:"monitor"`
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
	ServiceName  string `json:"service_name"`
	ConfigPath   string `json:"config_path"`
	TemplatePath string `json:"template_path"`
	URLPath      string `json:"url_path"`
	BinPath      string `json:"bin_path"`
	CtlPath      string `json:"ctl_path"`
}

type MosDNSConfig struct {
	ServiceName string `json:"service_name"`
	CtlPath     string `json:"ctl_path"`
	WebURL      string `json:"web_url"`
}

type MonitorConfig struct {
	Enabled                         bool   `json:"enabled"`
	APIBase                         string `json:"api_base"`
	DefaultProxyGroup               string `json:"default_proxy_group"`
	PrimaryGroup                    string `json:"primary_group"`
	FallbackGroup                   string `json:"fallback_group"`
	TestURL                         string `json:"test_url"`
	TimeoutMS                       int    `json:"timeout_ms"`
	PrimaryMaxStableDelayMS         int    `json:"primary_max_stable_delay_ms"`
	FallbackMaxStableDelayMS        int    `json:"fallback_max_stable_delay_ms"`
	DisablePrimaryGroupOptimization bool   `json:"disable_primary_group_optimization"`
	FailThreshold                   int    `json:"fail_threshold"`
	SuccessThreshold                int    `json:"success_threshold"`
	RecheckIntervalSec              int    `json:"recheck_interval_sec"`
	AutoFailback                    bool   `json:"auto_failback"`
	StateFile                       string `json:"state_file"`
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
	if cfg.Services.SingBox.BinPath == "" {
		cfg.Services.SingBox.BinPath = "/usr/bin/sing-box"
	}
	if cfg.Services.MosDNS.ServiceName == "" {
		cfg.Services.MosDNS.ServiceName = "mosdns"
	}
	if cfg.Monitor.APIBase == "" {
		cfg.Monitor.APIBase = "http://127.0.0.1:9090"
	}
	if cfg.Monitor.DefaultProxyGroup == "" {
		cfg.Monitor.DefaultProxyGroup = "默认代理"
	}
	if cfg.Monitor.PrimaryGroup == "" {
		cfg.Monitor.PrimaryGroup = "香港手动"
	}
	if cfg.Monitor.FallbackGroup == "" {
		cfg.Monitor.FallbackGroup = "自建节点"
	}
	if cfg.Monitor.TestURL == "" {
		cfg.Monitor.TestURL = "http://www.gstatic.com/generate_204"
	}
	if cfg.Monitor.TimeoutMS <= 0 {
		cfg.Monitor.TimeoutMS = 5000
	}
	if cfg.Monitor.PrimaryMaxStableDelayMS <= 0 {
		cfg.Monitor.PrimaryMaxStableDelayMS = 150
	}
	if cfg.Monitor.FallbackMaxStableDelayMS <= 0 {
		cfg.Monitor.FallbackMaxStableDelayMS = 500
	}
	if cfg.Monitor.FailThreshold <= 0 {
		cfg.Monitor.FailThreshold = 2
	}
	if cfg.Monitor.SuccessThreshold <= 0 {
		cfg.Monitor.SuccessThreshold = 2
	}
	if cfg.Monitor.RecheckIntervalSec <= 0 {
		cfg.Monitor.RecheckIntervalSec = 1
	}
	if cfg.Monitor.StateFile == "" {
		cfg.Monitor.StateFile = "data/monitor-state.json"
	}
	return &cfg, nil
}
