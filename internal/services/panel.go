package services

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	cfgpkg "singdns-panel/internal/config"
	"singdns-panel/internal/utils"
)

const (
	maxUpgradeArchiveBytes = 500 * 1024 * 1024 // 500MB
	maxManifestBytes       = 2 * 1024 * 1024   // 2MB
)

type PanelService struct {
	version string
	cfg     cfgpkg.PanelUpdateConfig

	mu    sync.RWMutex
	tasks map[string]*UpgradeTask
}

type PanelReleaseInfo struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	Version     string `json:"version"`
	HasUpgrade  bool   `json:"has_upgrade"`
	HasBinary   bool   `json:"has_binary"`
	HasScript   bool   `json:"has_script"`
	ModTime     string `json:"mod_time"`
	InstallHint string `json:"install_hint"`
}

type RemoteReleaseInfo struct {
	Version     string `json:"version"`
	URL         string `json:"url"`
	SHA256      string `json:"sha256,omitempty"`
	Channel     string `json:"channel"`
	Arch        string `json:"arch"`
	ManifestURL string `json:"manifest_url"`
}

type UpgradeTask struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	Status      string `json:"status"` // pending|running|success|failed
	Message     string `json:"message"`
	Version     string `json:"version,omitempty"`
	ReleasePath string `json:"release_path,omitempty"`
	StartedAt   string `json:"started_at,omitempty"`
	FinishedAt  string `json:"finished_at,omitempty"`
	Error       string `json:"error,omitempty"`
}

func NewPanelService(version string, cfg cfgpkg.PanelUpdateConfig) *PanelService {
	return &PanelService{version: version, cfg: cfg, tasks: map[string]*UpgradeTask{}}
}

func (p *PanelService) CurrentVersion() string {
	if strings.TrimSpace(p.version) == "" {
		return "dev"
	}
	return p.version
}

func (p *PanelService) Configured() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return strings.TrimSpace(p.cfg.ReleaseDir) != "" || strings.TrimSpace(p.cfg.UpgradeCommand) != ""
}

func (p *PanelService) ConfigSnapshot() cfgpkg.PanelUpdateConfig {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.cfg
}

func (p *PanelService) UpdateConfig(cfg cfgpkg.PanelUpdateConfig) {
	p.mu.Lock()
	p.cfg = cfg
	p.mu.Unlock()
}

func (p *PanelService) LatestLocalRelease() (*PanelReleaseInfo, error) {
	p.mu.RLock()
	releaseDir := strings.TrimSpace(p.cfg.ReleaseDir)
	p.mu.RUnlock()
	if releaseDir == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(releaseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var dirs []os.DirEntry
	for _, e := range entries {
		if e.IsDir() {
			dirs = append(dirs, e)
		}
	}
	sort.Slice(dirs, func(i, j int) bool {
		ai, _ := dirs[i].Info()
		aj, _ := dirs[j].Info()
		if ai == nil || aj == nil {
			return dirs[i].Name() > dirs[j].Name()
		}
		return ai.ModTime().After(aj.ModTime())
	})
	for _, dir := range dirs {
		full := filepath.Join(releaseDir, dir.Name())
		info, err := p.inspectRelease(full, dir.Name())
		if err == nil {
			return info, nil
		}
	}
	return nil, nil
}

func (p *PanelService) inspectRelease(path, name string) (*PanelReleaseInfo, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	binPath := filepath.Join(path, "bin", "singdns-panel")
	scriptPath := filepath.Join(path, "upgrade.sh")
	_, binErr := os.Stat(binPath)
	_, scriptErr := os.Stat(scriptPath)
	version := name
	if b, err := os.ReadFile(filepath.Join(path, "VERSION")); err == nil {
		if v := strings.TrimSpace(string(b)); v != "" {
			version = v
		}
	}
	return &PanelReleaseInfo{
		Name:        name,
		Path:        path,
		Version:     version,
		HasUpgrade:  binErr == nil && scriptErr == nil,
		HasBinary:   binErr == nil,
		HasScript:   scriptErr == nil,
		ModTime:     st.ModTime().Format("2006-01-02 15:04:05"),
		InstallHint: fmt.Sprintf("将发布包解压到 %s/<版本目录>/ 后，即可在面板内检测到升级候选。", strings.TrimRight(filepath.Clean(filepath.Dir(path)), "/")),
	}, nil
}

func (p *PanelService) Upgrade() error {
	cmd := strings.TrimSpace(p.cfg.UpgradeCommand)
	if cmd != "" {
		_, err := utils.RunShell(10*time.Minute, cmd)
		return err
	}
	rel, err := p.LatestLocalRelease()
	if err != nil {
		return err
	}
	if rel == nil {
		return fmt.Errorf("未发现可用的本地升级包")
	}
	if !rel.HasBinary || !rel.HasScript {
		return fmt.Errorf("升级包不完整，需要包含 bin/singdns-panel 和 upgrade.sh")
	}
	return p.upgradeFromRelease(rel)
}

func (p *PanelService) upgradeFromRelease(rel *PanelReleaseInfo) error {
	upgradeScript := filepath.Join(rel.Path, "upgrade.sh")
	if _, err := os.Stat(upgradeScript); err != nil {
		return fmt.Errorf("升级包缺少脚本: %s", upgradeScript)
	}

	if _, err := utils.Run(5*time.Minute, "sudo", "bash", upgradeScript); err != nil {
		return fmt.Errorf("执行升级脚本失败: %w", err)
	}
	if _, err := utils.Run(20*time.Second, "sudo", "systemctl", "is-active", "--quiet", "singdns-panel"); err != nil {
		return fmt.Errorf("升级后服务未正常运行: %w", err)
	}
	return nil
}

func (p *PanelService) ResolveRemoteRelease() (*RemoteReleaseInfo, error) {
	base := strings.TrimSpace(p.cfg.BaseURL)
	if base == "" {
		return nil, fmt.Errorf("未配置 panel_update.base_url")
	}
	manifestURL := base
	if !strings.HasSuffix(strings.ToLower(base), ".json") {
		manifestURL = strings.TrimRight(base, "/") + "/latest.json"
	}

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Get(manifestURL)
	if err != nil {
		return nil, fmt.Errorf("获取更新清单失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("获取更新清单失败，HTTP状态码: %d", resp.StatusCode)
	}

	var data map[string]any
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxManifestBytes)).Decode(&data); err != nil {
		return nil, fmt.Errorf("解析更新清单失败: %w", err)
	}

	channel := strings.TrimSpace(p.cfg.Channel)
	if channel == "" {
		channel = "stable"
	}
	arch := strings.TrimSpace(p.cfg.Arch)
	if arch == "" {
		arch = normalizeArch(runtime.GOARCH)
	}

	pkg := pickRemotePackage(data, channel, arch)
	if pkg == nil {
		return nil, fmt.Errorf("更新清单中未找到 channel=%s arch=%s 的发布包", channel, arch)
	}
	if strings.TrimSpace(pkg.URL) == "" {
		return nil, errors.New("更新清单缺少下载 URL")
	}
	pkg.Channel = channel
	pkg.Arch = arch
	pkg.ManifestURL = manifestURL
	return pkg, nil
}

func pickRemotePackage(data map[string]any, channel, arch string) *RemoteReleaseInfo {
	// 1) channels.{channel}.{arch}
	if channels, ok := asMap(data["channels"]); ok {
		if ch, ok := asMap(channels[channel]); ok {
			if pkg := pickFromArchNode(ch, arch); pkg != nil {
				return pkg
			}
		}
	}
	// 2) {channel:{arch:{...}}}
	if ch, ok := asMap(data[channel]); ok {
		if pkg := pickFromArchNode(ch, arch); pkg != nil {
			return pkg
		}
	}
	// 3) 顶层直接 {amd64:{...},arm64:{...}}
	if pkg := pickFromArchNode(data, arch); pkg != nil {
		return pkg
	}
	// 4) 顶层单包
	if pkg := parseRemoteReleaseInfo(data); pkg != nil {
		return pkg
	}
	return nil
}

func pickFromArchNode(node map[string]any, arch string) *RemoteReleaseInfo {
	if archNode, ok := asMap(node[arch]); ok {
		if pkg := parseRemoteReleaseInfo(archNode); pkg != nil {
			return pkg
		}
	}
	if pkgs, ok := asMap(node["packages"]); ok {
		if archNode, ok := asMap(pkgs[arch]); ok {
			if pkg := parseRemoteReleaseInfo(archNode); pkg != nil {
				return pkg
			}
		}
	}
	return nil
}

func parseRemoteReleaseInfo(m map[string]any) *RemoteReleaseInfo {
	url := asString(m["url"])
	if strings.TrimSpace(url) == "" {
		return nil
	}
	sha := asString(m["sha256"])
	if sha == "" {
		sha = asString(m["sha"])
	}
	return &RemoteReleaseInfo{
		Version: asString(m["version"]),
		URL:     url,
		SHA256:  sha,
	}
}

func normalizeArch(a string) string {
	switch strings.ToLower(strings.TrimSpace(a)) {
	case "x86_64", "x64":
		return "amd64"
	case "aarch64":
		return "arm64"
	default:
		return strings.ToLower(strings.TrimSpace(a))
	}
}

func asMap(v any) (map[string]any, bool) {
	m, ok := v.(map[string]any)
	return m, ok
}

func asString(v any) string {
	s, _ := v.(string)
	return strings.TrimSpace(s)
}

func (p *PanelService) DownloadAndExtract(downloadURL string) (*PanelReleaseInfo, error) {
	return p.DownloadAndExtractWithSHA(downloadURL, "")
}

func (p *PanelService) DownloadAndExtractWithSHA(downloadURL, expectedSHA string) (*PanelReleaseInfo, error) {
	p.mu.RLock()
	releaseDir := strings.TrimSpace(p.cfg.ReleaseDir)
	p.mu.RUnlock()
	if releaseDir == "" {
		return nil, fmt.Errorf("未配置面板升级目录(release_dir)，无法执行远程下载")
	}
	if err := os.MkdirAll(releaseDir, 0755); err != nil {
		return nil, fmt.Errorf("创建升级目录失败: %v", err)
	}

	fail := func(format string, args ...any) (*PanelReleaseInfo, error) {
		msg := fmt.Sprintf(format, args...)
		logPath := p.writeUpgradeLog(msg)
		if logPath == "" {
			return nil, errors.New(msg)
		}
		return nil, fmt.Errorf("%s（详情日志: %s）", msg, logPath)
	}

	tmpTar, err := os.CreateTemp("", "singdns-panel-update-*.tar.gz")
	if err != nil {
		return fail("创建临时文件失败: %v", err)
	}
	tmpTarPath := tmpTar.Name()
	defer os.Remove(tmpTarPath)

	hashWriter := sha256.New()
	writer := io.MultiWriter(tmpTar, hashWriter)

	if strings.HasPrefix(downloadURL, "file://") {
		srcPath := strings.TrimPrefix(downloadURL, "file://")
		src, err := os.Open(srcPath)
		if err != nil {
			tmpTar.Close()
			return fail("打开本地升级包失败: %v", err)
		}
		defer src.Close()
		n, err := io.Copy(writer, io.LimitReader(src, maxUpgradeArchiveBytes+1))
		tmpTar.Close()
		if err != nil {
			return fail("复制本地文件失败: %v", err)
		}
		if n > maxUpgradeArchiveBytes {
			return fail("升级包超过大小限制（>%dMB）", maxUpgradeArchiveBytes/1024/1024)
		}
	} else {
		if !strings.HasSuffix(strings.ToLower(downloadURL), ".tar.gz") {
			return fail("下载链接必须是 .tar.gz 归档")
		}
		client := &http.Client{Timeout: 5 * time.Minute}
		resp, err := client.Get(downloadURL)
		if err != nil {
			tmpTar.Close()
			return fail("下载失败: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			tmpTar.Close()
			return fail("下载失败，HTTP状态码: %d", resp.StatusCode)
		}
		if ct := strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Type"))); ct != "" {
			if !(strings.Contains(ct, "gzip") || strings.Contains(ct, "octet-stream") || strings.Contains(ct, "x-tar") || strings.Contains(ct, "tar")) {
				tmpTar.Close()
				return fail("响应类型异常: %s", ct)
			}
		}
		if resp.ContentLength > maxUpgradeArchiveBytes {
			tmpTar.Close()
			return fail("升级包超过大小限制（>%dMB）", maxUpgradeArchiveBytes/1024/1024)
		}
		n, err := io.Copy(writer, io.LimitReader(resp.Body, maxUpgradeArchiveBytes+1))
		tmpTar.Close()
		if err != nil {
			return fail("写入临时文件失败: %v", err)
		}
		if n > maxUpgradeArchiveBytes {
			return fail("升级包超过大小限制（>%dMB）", maxUpgradeArchiveBytes/1024/1024)
		}
	}

	actualSHA := strings.ToLower(hex.EncodeToString(hashWriter.Sum(nil)))
	expect := strings.ToLower(strings.TrimSpace(expectedSHA))
	if expect != "" && expect != actualSHA {
		return fail("SHA256 校验失败: expected=%s actual=%s", expect, actualSHA)
	}

	dirName := fmt.Sprintf("remote_%d", time.Now().Unix())
	targetDir := filepath.Join(releaseDir, dirName)
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fail("创建解压目录失败: %v", err)
	}

	if _, err := utils.Run(2*time.Minute, "tar", "-xzf", tmpTarPath, "-C", targetDir, "--strip-components=1"); err != nil {
		_ = os.RemoveAll(targetDir)
		return fail("解压失败: %v", err)
	}

	info, err := p.inspectRelease(targetDir, dirName)
	if err != nil {
		_ = os.RemoveAll(targetDir)
		return fail("校验升级包失败: %v", err)
	}
	if !info.HasBinary || !info.HasScript {
		_ = os.RemoveAll(targetDir)
		return fail("下载的升级包不完整，必须包含 bin/singdns-panel 和 upgrade.sh")
	}

	return info, nil
}

func (p *PanelService) writeUpgradeLog(content string) string {
	p.mu.RLock()
	releaseDir := strings.TrimSpace(p.cfg.ReleaseDir)
	p.mu.RUnlock()
	if releaseDir == "" {
		return ""
	}
	if err := os.MkdirAll(releaseDir, 0755); err != nil {
		return ""
	}
	path := filepath.Join(releaseDir, fmt.Sprintf("upgrade-%s.log", time.Now().Format("20060102-150405")))
	_ = os.WriteFile(path, []byte(time.Now().Format(time.RFC3339)+"\n"+content+"\n"), 0644)
	return path
}

func (p *PanelService) NewTask(kind, version, releasePath string) *UpgradeTask {
	t := &UpgradeTask{
		ID:          fmt.Sprintf("upg-%d", time.Now().UnixNano()),
		Kind:        kind,
		Status:      "pending",
		Message:     "任务已创建",
		Version:     strings.TrimSpace(version),
		ReleasePath: strings.TrimSpace(releasePath),
		StartedAt:   time.Now().Format(time.RFC3339),
	}
	p.mu.Lock()
	p.tasks[t.ID] = t
	p.mu.Unlock()
	return cloneTask(t)
}

func (p *PanelService) Task(taskID string) *UpgradeTask {
	p.mu.RLock()
	t, ok := p.tasks[strings.TrimSpace(taskID)]
	p.mu.RUnlock()
	if !ok {
		return nil
	}
	return cloneTask(t)
}

func (p *PanelService) MarkTaskRunning(taskID, msg string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if t, ok := p.tasks[taskID]; ok {
		t.Status = "running"
		if strings.TrimSpace(msg) != "" {
			t.Message = msg
		}
	}
}

func (p *PanelService) MarkTaskSuccess(taskID, msg string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if t, ok := p.tasks[taskID]; ok {
		t.Status = "success"
		if strings.TrimSpace(msg) != "" {
			t.Message = msg
		} else {
			t.Message = "升级成功"
		}
		t.FinishedAt = time.Now().Format(time.RFC3339)
		t.Error = ""
	}
}

func (p *PanelService) MarkTaskFailed(taskID string, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if t, ok := p.tasks[taskID]; ok {
		t.Status = "failed"
		t.FinishedAt = time.Now().Format(time.RFC3339)
		if err != nil {
			t.Error = err.Error()
			t.Message = "升级失败"
		} else {
			t.Message = "升级失败"
		}
	}
}

func cloneTask(t *UpgradeTask) *UpgradeTask {
	if t == nil {
		return nil
	}
	cp := *t
	return &cp
}
