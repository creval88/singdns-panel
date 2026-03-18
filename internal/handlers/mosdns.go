package handlers

import "net/http"

func (a *App) MosDNSPage(w http.ResponseWriter, r *http.Request) {
	status, _ := a.MosDNS.Status()
	a.render(w, "mosdns.html", map[string]any{
		"Title":           "MosDNS",
		"ActiveNav":       "mosdns",
		"PageTitle":       "MosDNS 管理",
		"Eyebrow":         "Service",
		"SidebarSubtitle": "sing-box / mosdns 控制台",
		"Status":          status,
		"WebURL":          a.MosDNS.WebURL(),
	})
}

func (a *App) MosDNSStatusAPI(w http.ResponseWriter, r *http.Request) {
	a.serviceStatusAPI(w, a.MosDNS)
}

func (a *App) MosDNSActionAPI(w http.ResponseWriter, r *http.Request) {
	a.serviceActionAPI(w, r, "mosdns.action.", a.MosDNS, "MosDNS 已操作")
}
