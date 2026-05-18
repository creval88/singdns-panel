package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
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
	Enabled                         bool     `json:"enabled"`
	APIBase                         string   `json:"api_base"`
	DefaultProxyGroup               string   `json:"default_proxy_group"`
	PrimaryGroup                    string   `json:"primary_group"`
	FallbackGroup                   string   `json:"fallback_group"`
	TestURL                         string   `json:"test_url"`
	TimeoutMS                       int      `json:"timeout_ms"`
	PrimaryMaxStableDelayMS         int      `json:"primary_max_stable_delay_ms"`
	FallbackMaxStableDelayMS        int      `json:"fallback_max_stable_delay_ms"`
	DisablePrimaryGroupOptimization bool     `json:"disable_primary_group_optimization"`
	FailThreshold                   int      `json:"fail_threshold"`
	SuccessThreshold                int      `json:"success_threshold"`
	RecheckIntervalSec              int      `json:"recheck_interval_sec"`
	AutoFailback                    bool     `json:"auto_failback"`
	StateFile                       string   `json:"state_file"`
	QualityCheckEnabled             bool     `json:"quality_check_enabled"`
	ProbeURLs                       []string `json:"probe_urls"`
	MinProbeSuccess                 int      `json:"min_probe_success"`
	QualityScoreThreshold           int      `json:"quality_score_threshold"`
	DownloadTestURL                 string   `json:"download_test_url"`
	MinDownloadKBps                 int      `json:"min_download_kbps"`
	LocalProxyURL                   string   `json:"local_proxy_url"`
	DayCheckIntervalMin             int      `json:"day_check_interval_min"`
	PeakCheckIntervalMin            int      `json:"peak_check_interval_min"`
	DownloadPrecheckDisabled        bool     `json:"download_precheck_disabled"`
	VideoCheckEnabled               bool     `json:"video_check_enabled"`
	VideoDayCheckEnabled            bool     `json:"video_day_check_enabled"`
	VideoPeakCheckEnabled           bool     `json:"video_peak_check_enabled"`
	VideoDayMinDownloadKBps         int      `json:"video_day_min_download_kbps"`
	VideoPeakMinDownloadKBps        int      `json:"video_peak_min_download_kbps"`
	VideoPeakStart                  string   `json:"video_peak_start"`
	VideoPeakEnd                    string   `json:"video_peak_end"`
	VideoDownloadDurationSec        int      `json:"video_download_duration_sec"`
	VideoDownloadWindowSec          int      `json:"video_download_window_sec"`
	VideoDownloadMaxLowWindows      int      `json:"video_download_max_low_windows"`
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
	if len(cfg.Monitor.ProbeURLs) == 0 {
		cfg.Monitor.ProbeURLs = []string{
			"http://www.gstatic.com/generate_204",
			"https://api.github.com/rate_limit",
			"https://www.google.com/generate_204",
		}
	}
	if cfg.Monitor.MinProbeSuccess <= 0 {
		cfg.Monitor.MinProbeSuccess = minInt(2, len(cfg.Monitor.ProbeURLs))
	}
	if cfg.Monitor.QualityScoreThreshold <= 0 {
		cfg.Monitor.QualityScoreThreshold = 70
	}
	if cfg.Monitor.DownloadTestURL == "" || strings.Contains(cfg.Monitor.DownloadTestURL, "bytes=262144") || strings.Contains(cfg.Monitor.DownloadTestURL, "speed.cloudflare.com/__down") {
		cfg.Monitor.DownloadTestURL = "https://proof.ovh.net/files/100Mb.dat"
	}
	if cfg.Monitor.MinDownloadKBps <= 0 {
		cfg.Monitor.MinDownloadKBps = 80
	}
	if cfg.Monitor.LocalProxyURL == "" {
		cfg.Monitor.LocalProxyURL = "socks5://127.0.0.1:7891"
	}
	if cfg.Monitor.DayCheckIntervalMin <= 0 {
		cfg.Monitor.DayCheckIntervalMin = 5
	}
	if cfg.Monitor.PeakCheckIntervalMin <= 0 {
		cfg.Monitor.PeakCheckIntervalMin = 1
	}
	missingVideoTiming := cfg.Monitor.VideoDownloadDurationSec <= 0 && cfg.Monitor.VideoDownloadWindowSec <= 0 && cfg.Monitor.VideoDownloadMaxLowWindows <= 0
	if !cfg.Monitor.VideoCheckEnabled && cfg.Monitor.VideoDayMinDownloadKBps <= 0 && cfg.Monitor.VideoPeakMinDownloadKBps <= 0 {
		cfg.Monitor.VideoCheckEnabled = true
	}
	if cfg.Monitor.VideoCheckEnabled && !cfg.Monitor.VideoDayCheckEnabled && !cfg.Monitor.VideoPeakCheckEnabled {
		cfg.Monitor.VideoPeakCheckEnabled = true
	}
	if cfg.Monitor.VideoDayMinDownloadKBps <= 0 {
		cfg.Monitor.VideoDayMinDownloadKBps = maxInt(cfg.Monitor.MinDownloadKBps, 1000)
	}
	if cfg.Monitor.VideoPeakMinDownloadKBps <= 0 {
		cfg.Monitor.VideoPeakMinDownloadKBps = maxInt(cfg.Monitor.VideoDayMinDownloadKBps, 3000)
	}
	if strings.TrimSpace(cfg.Monitor.VideoPeakStart) == "" {
		cfg.Monitor.VideoPeakStart = "19:00"
	}
	if strings.TrimSpace(cfg.Monitor.VideoPeakEnd) == "" {
		cfg.Monitor.VideoPeakEnd = "23:59"
	}
	if cfg.Monitor.VideoDownloadDurationSec <= 0 {
		cfg.Monitor.VideoDownloadDurationSec = 10
	}
	if cfg.Monitor.VideoDownloadWindowSec <= 0 {
		cfg.Monitor.VideoDownloadWindowSec = 2
	}
	if missingVideoTiming {
		cfg.Monitor.VideoDownloadMaxLowWindows = 1
	} else if cfg.Monitor.VideoDownloadMaxLowWindows < 0 {
		cfg.Monitor.VideoDownloadMaxLowWindows = 0
	}
	return &cfg, nil
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
