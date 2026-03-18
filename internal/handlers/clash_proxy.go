package handlers

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// clashAPIClient 返回一个指向本地 Clash API 的 HTTP client
func (a *App) clashAPIClient() *http.Client {
	return &http.Client{Timeout: 10 * time.Second}
}

// clashAPIBase 从配置中获取 Clash API 地址（host:port）
func (a *App) clashAPIBase(r *http.Request) (string, string, error) {
	info, err := a.SingBox.ClashAPIInfo(r.Host)
	if err != nil || !info.Enabled {
		return "", "", fmt.Errorf("clash api not available")
	}
	return fmt.Sprintf("http://127.0.0.1:%s", info.Port), info.Secret, nil
}

// ClashProxyAPI 透传 Clash API 请求（反向代理，解决浏览器跨域）
// 路由: GET/POST/PUT/DELETE /api/clash/*
func (a *App) ClashProxyAPI(w http.ResponseWriter, r *http.Request) {
	base, secret, err := a.clashAPIBase(r)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"clash api not configured or not enabled"}`))
		return
	}

	// 取出 /api/clash/ 后面的路径
	subPath := strings.TrimPrefix(r.URL.Path, "/api/clash")
	if subPath == "" {
		subPath = "/"
	}
	targetURL := base + subPath
	if r.URL.RawQuery != "" {
		targetURL += "?" + r.URL.RawQuery
	}

	proxyReq, err := http.NewRequestWithContext(r.Context(), r.Method, targetURL, r.Body)
	if err != nil {
		http.Error(w, `{"error":"failed to build request"}`, http.StatusInternalServerError)
		return
	}
	if ct := r.Header.Get("Content-Type"); ct != "" {
		proxyReq.Header.Set("Content-Type", ct)
	}
	if secret != "" {
		proxyReq.Header.Set("Authorization", "Bearer "+secret)
	}

	resp, err := a.clashAPIClient().Do(proxyReq)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = fmt.Fprintf(w, `{"error":"clash api request failed: %s"}`, err.Error())
		return
	}
	defer resp.Body.Close()

	// 透传响应头和状态码
	for k, vs := range resp.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}
