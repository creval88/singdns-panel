package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	svc "singdns-panel/internal/services"
)

func (a *App) ConfigCenterPage(w http.ResponseWriter, r *http.Request) {
	overview, _ := a.SingBox.ConfigCenterOverview()
	draft, _ := a.SingBox.ConfigCenterDraftFromCurrent()
	validation := &svc.ConfigCenterValidation{OK: true, Errors: []string{}, Warnings: []string{}, CanApply: true}
	if draft != nil {
		validation = svc.ValidateConfigCenterDraft(draft)
	}
	a.render(w, "config_center.html", map[string]any{
		"Title":           "Config Center",
		"ActiveNav":       "config-center",
		"PageTitle":       "配置中心（预览版）",
		"Eyebrow":         "Config",
		"SidebarSubtitle": "sing-box 结构化配置控制台",
		"Overview":        overview,
		"Draft":           draft,
		"Validation":      validation,
	})
}

func (a *App) ConfigCenterOverviewAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	overview, err := a.SingBox.ConfigCenterOverview()
	if err != nil {
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": err.Error()})
		return
	}
	_ = json.NewEncoder(w).Encode(overview)
}

func (a *App) ConfigCenterDraftAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	draft, err := a.SingBox.ConfigCenterDraftFromCurrent()
	if err != nil {
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": err.Error()})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "draft": draft})
}

func (a *App) ConfigCenterValidateAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var in struct {
		Config string                 `json:"config"`
		Draft  *svc.ConfigCenterDraft `json:"draft"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	if in.Draft != nil {
		content, err := a.SingBox.BuildConfigCenterContentFromDraft(in.Draft)
		if err != nil {
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": err.Error()})
			return
		}
		in.Config = content
	}
	if strings.TrimSpace(in.Config) == "" {
		in.Config, _ = a.SingBox.ReadConfig()
	}
	result, err := a.SingBox.ValidateConfigCenterContent(in.Config)
	if err != nil {
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": err.Error()})
		return
	}
	_ = json.NewEncoder(w).Encode(result)
}

func (a *App) ConfigCenterSaveAPI(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Draft *svc.ConfigCenterDraft `json:"draft"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		respondMessage(w, err, "")
		return
	}
	if in.Draft == nil {
		respondMessage(w, http.ErrBodyNotAllowed, "")
		return
	}
	res, err := a.SingBox.SaveConfigCenterDraftDetailed(in.Draft)
	if err != nil {
		a.auditFromRequest(r, "singbox.config_center.save", err)
		respondMessage(w, err, "")
		return
	}
	a.auditMessageFromRequest(r, "singbox.config_center.save", res.AuditText())
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":          true,
		"message":     res.Message,
		"backup_name": res.BackupName,
		"bytes":       res.Bytes,
		"risk":        res.Risk,
		"validation":  res.Validation,
		"summary":     res.Summary,
	})
}
