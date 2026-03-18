package handlers

import (
	"net/http"

	authpkg "singdns-panel/internal/auth"
)

func (a *App) LoginPage(w http.ResponseWriter, r *http.Request) {
	token, _ := authpkg.NewCSRFToken()
	authpkg.SetCSRFCookie(w, token)
	a.render(w, "login.html", map[string]any{"Title": "登录"})
}

func (a *App) LoginPost(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	ip := r.RemoteAddr
	if !a.Limiter.Allow(ip) {
		a.render(w, "login.html", map[string]any{"Title": "登录", "Error": "尝试过多，请稍后再试"})
		return
	}
	username := r.FormValue("username")
	password := r.FormValue("password")
	if username != a.Config.Auth.Username || !authpkg.CheckPassword(a.Config.Auth.PasswordHash, password) {
		a.Limiter.Fail(ip)
		a.render(w, "login.html", map[string]any{"Title": "登录", "Error": "用户名或密码错误"})
		return
	}
	a.Limiter.Reset(ip)
	if err := a.Sessions.Create(w, username); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/", http.StatusFound)
}

func (a *App) Logout(w http.ResponseWriter, r *http.Request) {
	a.Sessions.Destroy(w, r)
	http.Redirect(w, r, "/login", http.StatusFound)
}
