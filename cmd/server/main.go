package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	authpkg "singdns-panel/internal/auth"
	cfgpkg "singdns-panel/internal/config"
	"singdns-panel/internal/handlers"
	"singdns-panel/internal/services"
	"singdns-panel/internal/webassets"
)

func loadConfigWithNormalizedSingBox(cfgPath string) (*cfgpkg.Config, *services.SystemdService, error) {
	cfg, err := cfgpkg.Load(cfgPath)
	if err != nil {
		return nil, nil, err
	}
	systemd := services.NewSystemdService()
	normalized, changed := services.NormalizeSingBoxServiceConfig(cfg.Services.SingBox, systemd)
	cfg.Services.SingBox = normalized
	if changed {
		if err := cfg.Save(cfgPath); err != nil {
			log.Printf("warning: normalize sing-box bin path to %s but failed to persist config %s: %v", normalized.BinPath, cfgPath, err)
		} else {
			log.Printf("normalized sing-box bin path to %s based on systemd ExecStart", normalized.BinPath)
		}
	}
	return cfg, systemd, nil
}

func main() {
	cfgPath := os.Getenv("SINGDNS_CONFIG")
	if cfgPath == "" {
		cfgPath = "configs/panel.json"
	}

	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "hash-password":
			os.Exit(authpkg.RunHashCLI())
		case "init-config":
			path := cfgPath
			if len(os.Args) > 2 {
				path = os.Args[2]
			}
			if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				log.Fatal(err)
			}
			initial, err := cfgpkg.GenerateInitialConfig()
			if err != nil {
				log.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(initial.Content), 0640); err != nil {
				log.Fatal(err)
			}
			log.Printf("wrote default config to %s", path)
			log.Printf("initial login username: %s", initial.Username)
			log.Printf("initial login password: %s", initial.Password)
			return
		case "subscription-update":
			cfg, systemd, err := loadConfigWithNormalizedSingBox(cfgPath)
			if err != nil {
				log.Fatal(err)
			}
			singbox := services.NewSingBoxService(cfg.Services.SingBox, systemd, cfgPath)
			if err := singbox.RunScheduledSubscriptionUpdate(); err != nil {
				log.Fatal(err)
			}
			log.Printf("subscription update completed")
			return
		case "subscription-import":
			rawURL := ""
			if len(os.Args) > 2 {
				rawURL = os.Args[2]
			}
			if len(os.Args) > 3 {
				cfgPath = os.Args[3]
			}
			cfg, systemd, err := loadConfigWithNormalizedSingBox(cfgPath)
			if err != nil {
				log.Fatal(err)
			}
			singbox := services.NewSingBoxService(cfg.Services.SingBox, systemd, cfgPath)
			if strings.TrimSpace(rawURL) == "" {
				rawURL, err = singbox.ReadFullConfigSubscriptionURL()
				if err != nil {
					log.Fatal(err)
				}
			} else {
				if _, err := singbox.SaveFullConfigSubscriptionURL(rawURL); err != nil {
					log.Fatal(err)
				}
			}
			log.Printf("subscription import target config: %s", cfg.Services.SingBox.ConfigPath)
			res, err := singbox.ImportSubscriptionFromURL(rawURL)
			if err != nil {
				log.Fatal(err)
			}
			if res != nil {
				log.Printf("subscription import completed: %s", res.Message)
			} else {
				log.Printf("subscription import completed")
			}
			return
		case "monitor-run":
			cfg, systemd, err := loadConfigWithNormalizedSingBox(cfgPath)
			if err != nil {
				log.Fatal(err)
			}
			singbox := services.NewSingBoxService(cfg.Services.SingBox, systemd, cfgPath)
			monitor := services.NewMonitorService(cfg.Monitor, singbox)
			res, err := monitor.RunOnce()
			if err != nil {
				log.Fatal(err)
			}
			if res != nil && strings.TrimSpace(res.Message) != "" {
				fmt.Println(res.Message)
			}
			return
		case "monitor-status":
			cfg, systemd, err := loadConfigWithNormalizedSingBox(cfgPath)
			if err != nil {
				log.Fatal(err)
			}
			singbox := services.NewSingBoxService(cfg.Services.SingBox, systemd, cfgPath)
			monitor := services.NewMonitorService(cfg.Monitor, singbox)
			st, err := monitor.Status()
			if err != nil {
				log.Fatal(err)
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			if err := enc.Encode(st); err != nil {
				log.Fatal(err)
			}
			return
		case "monitor-history":
			cfg, systemd, err := loadConfigWithNormalizedSingBox(cfgPath)
			if err != nil {
				log.Fatal(err)
			}
			singbox := services.NewSingBoxService(cfg.Services.SingBox, systemd, cfgPath)
			monitor := services.NewMonitorService(cfg.Monitor, singbox)
			summary, err := monitor.HistorySummary()
			if err != nil {
				log.Fatal(err)
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			if err := enc.Encode(summary); err != nil {
				log.Fatal(err)
			}
			return
		}
	}

	cfg, systemd, err := loadConfigWithNormalizedSingBox(cfgPath)
	if err != nil {
		log.Fatal(err)
	}
	tplFS, err := fs.Sub(webassets.FS, "templates")
	if err != nil {
		log.Fatal(err)
	}
	tpls, err := template.New("").Funcs(template.FuncMap{
		"toJSON": func(v any) template.JS {
			b, _ := json.Marshal(v)
			if len(b) == 0 {
				return template.JS("null")
			}
			return template.JS(b)
		},
	}).ParseFS(tplFS, "*.html")
	if err != nil {
		log.Fatal(err)
	}
	staticFS, err := fs.Sub(webassets.FS, "static")
	if err != nil {
		log.Fatal(err)
	}
	singboxSvc := services.NewSingBoxService(cfg.Services.SingBox, systemd, cfgPath)
	app := &handlers.App{
		Config:       cfg,
		ConfigPath:   cfgPath,
		Sessions:     authpkg.NewSessionManager("singdns_session"),
		Limiter:      authpkg.NewLoginLimiter(5, 15*time.Minute),
		Templates:    tpls,
		SingBox:      singboxSvc,
		MosDNS:       services.NewMosDNSService(cfg.Services.MosDNS, systemd),
		Network:      services.NewNetworkService(),
		Monitor:      services.NewMonitorService(cfg.Monitor, singboxSvc),
		Audit:        services.NewAuditService(cfg.AuditLog),
		Panel:        services.NewPanelService(Version, cfg.PanelUpdate),
		PanelVersion: Version,
	}

	r := chi.NewRouter()
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))

	r.Get("/healthz", app.HealthAPI)
	r.Get("/api/csrf", app.CSRFTokenAPI)
	r.Get("/login", app.LoginPage)
	r.Post("/login", app.LoginPost)
	r.Post("/logout", app.Logout)

	r.Group(func(pr chi.Router) {
		pr.Use(app.Sessions.Require)
		pr.Use(app.CSRFMiddleware)
		pr.Get("/", app.Dashboard)
		pr.Get("/singbox", app.SingBoxPage)
		pr.Get("/config-center", app.ConfigCenterPage)
		pr.Get("/mosdns", app.MosDNSPage)
		pr.Get("/logs", app.LogsPage)
		pr.Get("/audit", app.AuditPage)
		pr.Get("/system", app.SystemPage)

		pr.Get("/api/dashboard", app.DashboardAPI)
		pr.Get("/api/diagnostics/quick", app.QuickDiagnosticsAPI)
		pr.Get("/api/panel/version", app.PanelVersionAPI)
		pr.Get("/api/panel/update-config", app.PanelUpdateConfigAPI)
		pr.Post("/api/panel/update-config", app.PanelUpdateConfigSaveAPI)
		pr.Get("/api/panel/probe-remote", app.PanelProbeRemoteAPI)
		pr.Get("/api/panel/upgrade/preflight", app.PanelUpgradePreflightAPI)
		pr.Get("/api/panel/upgrade/task", app.PanelUpgradeTaskAPI)
		pr.Post("/api/panel/upgrade", app.PanelUpgradeAPI)
		pr.Post("/api/panel/upgrade/remote", app.PanelRemoteUpgradeAPI)
		pr.Get("/api/system/install/status", app.SystemInstallStatusAPI)
		pr.Post("/api/system/install/singbox", app.SystemInstallSingBoxAPI)
		pr.Post("/api/system/install/singbox/switch-binary", app.SystemSwitchSingBoxBinaryAPI)
		pr.Post("/api/system/install/mosdns", app.SystemInstallMosDNSAPI)
		pr.Post("/api/system/install/mosdns/upload", app.SystemInstallMosDNSUploadAPI)
		pr.Post("/api/system/install/singbox/upload", app.SystemInstallSingBoxUploadAPI)
		pr.Get("/api/system/install/preflight", app.SystemInstallPreflightAPI)
		pr.Post("/api/system/ip-forward/enable", app.SystemEnableIPForwardAPI)
		pr.Get("/api/system/network", app.SystemNetworkStatusAPI)
		pr.Post("/api/system/network", app.SystemNetworkSaveAPI)
		pr.Post("/api/auth/password", app.ChangePasswordAPI)
		pr.Get("/api/singbox/overview", app.SingBoxOverviewAPI)
		pr.Get("/api/singbox/config-center/overview", app.ConfigCenterOverviewAPI)
		pr.Get("/api/singbox/config-center/draft", app.ConfigCenterDraftAPI)
		pr.Post("/api/singbox/config-center/validate", app.ConfigCenterValidateAPI)
		pr.Post("/api/singbox/config-center/save", app.ConfigCenterSaveAPI)
		pr.Get("/api/singbox/status", app.SingBoxStatusAPI)
		pr.Get("/api/singbox/config", app.SingBoxConfigAPI)
		pr.Get("/api/singbox/config/meta", app.SingBoxConfigMetaAPI)
		pr.Get("/api/singbox/template/config", app.SingBoxTemplateConfigAPI)
		pr.Post("/api/singbox/action/{action}", app.SingBoxActionAPI)
		pr.Post("/api/singbox/template/config/validate", app.SingBoxTemplateConfigValidateAPI)
		pr.Post("/api/singbox/template/config/save", app.SingBoxTemplateConfigSaveAPI)
		pr.Post("/api/singbox/config/validate", app.SingBoxConfigValidateAPI)
		pr.Post("/api/singbox/config/save", app.SingBoxConfigSaveAPI)
		pr.Get("/api/singbox/subscription", app.SingBoxSubscriptionAPI)
		pr.Get("/api/singbox/subscription/history", app.SingBoxSubscriptionHistoryAPI)
		pr.Get("/api/singbox/subscription/updates", app.SingBoxSubscriptionUpdatesAPI)
		pr.Post("/api/singbox/subscription", app.SingBoxSubscriptionSaveAPI)
		pr.Post("/api/singbox/subscription/full", app.SingBoxSubscriptionSaveFullAPI)
		pr.Post("/api/singbox/subscription/update", app.SingBoxSubscriptionUpdateAPI)
		pr.Post("/api/singbox/subscription/update/full", app.SingBoxSubscriptionUpdateFullAPI)
		pr.Post("/api/singbox/subscription/update/nodes", app.SingBoxSubscriptionUpdateNodesAPI)
		pr.Get("/api/singbox/manual-nodes", app.SingBoxManualNodesAPI)
		pr.Post("/api/singbox/manual-nodes", app.SingBoxManualNodesSaveAPI)
		pr.Post("/api/singbox/manual-nodes/import", app.SingBoxManualNodesImportAPI)
		pr.Get("/api/singbox/version", app.SingBoxVersionAPI)
		pr.Get("/api/singbox/upgrade/overview", app.SingBoxUpgradeOverviewAPI)
		pr.Post("/api/singbox/upgrade", app.SingBoxUpgradeAPI)
		pr.Get("/api/singbox/ip-forward", app.SingBoxIPForwardStatusAPI)
		pr.Post("/api/singbox/upgrade/upload", app.SingBoxUpgradeUploadAPI)
		pr.Post("/api/singbox/upgrade/rollback", app.SingBoxUpgradeRollbackAPI)
		pr.Get("/api/singbox/cron", app.SingBoxCronGetAPI)
		pr.Post("/api/singbox/cron", app.SingBoxCronSetAPI)
		pr.Delete("/api/singbox/cron", app.SingBoxCronDeleteAPI)
		pr.Get("/api/monitor/config", app.MonitorConfigAPI)
		pr.Post("/api/monitor/config", app.MonitorConfigSaveAPI)
		pr.Get("/api/monitor/cron", app.MonitorCronGetAPI)
		pr.Post("/api/monitor/cron", app.MonitorCronSetAPI)
		pr.Delete("/api/monitor/cron", app.MonitorCronDeleteAPI)
		pr.Post("/api/monitor/run", app.MonitorRunAPI)
		pr.Get("/api/singbox/backups", app.SingBoxBackupsAPI)
		pr.Get("/api/singbox/backups/status", app.SingBoxBackupStatusAPI)
		pr.Get("/api/singbox/backups/diff", app.SingBoxBackupDiffAPI)
		pr.Post("/api/singbox/backups/create", app.SingBoxCreateBackupAPI)
		pr.Post("/api/singbox/backups/delete", app.SingBoxDeleteBackupAPI)
		pr.Post("/api/singbox/backups/restore", app.SingBoxRestoreBackupAPI)

		// Clash API 反向代理（解决跨域，统一鉴权）
		pr.HandleFunc("/api/clash/*", app.ClashProxyAPI)
		pr.HandleFunc("/api/clash", app.ClashProxyAPI)

		pr.Get("/api/mosdns/status", app.MosDNSStatusAPI)
		pr.Post("/api/mosdns/action/{action}", app.MosDNSActionAPI)
		pr.Get("/api/mosdns/config", app.MosDNSConfigAPI)
		pr.Post("/api/mosdns/config", app.MosDNSConfigSaveAPI)
		pr.Post("/api/panel/restart", app.PanelRestartAPI)
		pr.Get("/api/logs/{name}", app.ServiceLogsAPI)
		pr.Get("/api/audit", app.AuditAPI)
	})

	log.Printf("singdns-panel listening on %s", cfg.Listen)
	log.Fatal(http.ListenAndServe(cfg.Listen, r))
}
