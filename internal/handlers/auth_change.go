package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	authpkg "singdns-panel/internal/auth"
)

func (a *App) ChangePasswordAPI(w http.ResponseWriter, r *http.Request) {
	var in struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		respondMessage(w, err, "")
		return
	}

	if !authpkg.CheckPassword(a.Config.Auth.PasswordHash, in.OldPassword) {
		respondMessage(w, fmt.Errorf("旧密码不正确"), "")
		return
	}

	if len(in.NewPassword) < 6 {
		respondMessage(w, fmt.Errorf("新密码不能少于 6 位"), "")
		return
	}

	newHash, err := authpkg.HashPassword(in.NewPassword)
	if err != nil {
		respondMessage(w, err, "")
		return
	}

	// Update in memory
	a.Config.Auth.PasswordHash = newHash

	// Save to disk
	if err := a.Config.Save(a.ConfigPath); err != nil {
		respondMessage(w, fmt.Errorf("保存配置失败: %v", err), "")
		return
	}

	a.auditFromRequest(r, "auth.change_password", nil)
	respondMessage(w, nil, "密码修改成功，请重新登录")
}
