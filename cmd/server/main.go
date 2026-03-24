package main

import (
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	authpkg "singdns-panel/internal/auth"
	cfgpkg "singdns-panel/internal/config"
	"singdns-panel/internal/handlers"
	"singdns-panel/internal/services"
	"singdns-panel/internal/webassets"
)

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
			if err := os.WriteFile(path, []byte(cfgpkg.DefaultConfigTemplate), 0644); err != nil {
				log.Fatal(err)
			}
			log.Printf("wrote default config to %s", path)
			return
		case "subscription-update":
			cfg, err := cfgpkg.Load(cfgPath)
			if err != nil {
				log.Fatal(err)
			}
			systemd := services.NewSystemdService()
			singbox := services.NewSingBoxService(cfg.Services.SingBox, systemd, cfgPath)
			if err := singbox.RunScheduledSubscriptionUpdate(); err != nil {
				log.Fatal(err)
			}
			log.Printf("subscription update completed")
			return
		}
	}

	cfg, err := cfgpkg.Load(cfgPath)
	if err != nil {
		log.Fatal(err)
	}
	tplFS, err := fs.Sub(webassets.FS, "templates")
	if err != nil {
		log.Fatal(err)
	}
	tpls, err := template.ParseFS(tplFS, "*.html")
	if err != nil {
		log.Fatal(err)
	}
	staticFS, err := fs.Sub(webassets.FS, "static")
	if err != nil {
		log.Fatal(err)
	}
	systemd := services.NewSystemdService()
	app := &handlers.App{
		Config:       cfg,
		ConfigPath:   cfgPath,
		Sessions:     authpkg.NewSessionManager("singdns_session"),
		Limiter:      authpkg.NewLoginLimiter(5, 15*time.Minute),
		Templates:    tpls,
		SingBox:      services.NewSingBoxService(cfg.Services.SingBox, systemd, cfgPath),
		MosDNS:       services.NewMosDNSService(cfg.Services.MosDNS, systemd),
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
		pr.Get("/api/panel/upgrade/task", app.PanelUpgradeTaskAPI)
		pr.Post("/api/panel/upgrade", app.PanelUpgradeAPI)
		pr.Post("/api/panel/upgrade/remote", app.PanelRemoteUpgradeAPI)
		pr.Post("/api/auth/password", app.ChangePasswordAPI)
		pr.Get("/api/singbox/overview", app.SingBoxOverviewAPI)
		pr.Get("/api/singbox/status", app.SingBoxStatusAPI)
		pr.Get("/api/singbox/config", app.SingBoxConfigAPI)
		pr.Post("/api/singbox/action/{action}", app.SingBoxActionAPI)
		pr.Post("/api/singbox/config/validate", app.SingBoxConfigValidateAPI)
		pr.Post("/api/singbox/config/save", app.SingBoxConfigSaveAPI)
		pr.Get("/api/singbox/subscription", app.SingBoxSubscriptionAPI)
		pr.Post("/api/singbox/subscription", app.SingBoxSubscriptionSaveAPI)
		pr.Post("/api/singbox/subscription/update", app.SingBoxSubscriptionUpdateAPI)
		pr.Get("/api/singbox/version", app.SingBoxVersionAPI)
		pr.Post("/api/singbox/upgrade", app.SingBoxUpgradeAPI)
		pr.Get("/api/singbox/cron", app.SingBoxCronGetAPI)
		pr.Post("/api/singbox/cron", app.SingBoxCronSetAPI)
		pr.Delete("/api/singbox/cron", app.SingBoxCronDeleteAPI)
		pr.Get("/api/singbox/backups", app.SingBoxBackupsAPI)
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
