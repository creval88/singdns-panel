package services

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	cfgpkg "singdns-panel/internal/config"
	"singdns-panel/internal/utils"
)

type InstallComponentStatus struct {
	Installed         bool           `json:"installed"`
	Version           string         `json:"version"`
	Status            *ServiceStatus `json:"status,omitempty"`
	Detail            string         `json:"detail"`
	ConfiguredBinary  string         `json:"configured_binary,omitempty"`
	ServiceBinary     string         `json:"service_binary,omitempty"`
	RunningBinary     string         `json:"running_binary,omitempty"`
	CandidateBinaries []string       `json:"candidate_binaries,omitempty"`
	PathConsistent    bool           `json:"path_consistent"`
}

type SystemInstallStatus struct {
	SingBox   *InstallComponentStatus `json:"singbox"`
	MosDNS    *InstallComponentStatus `json:"mosdns"`
	IPForward *IPForwardStatus        `json:"ip_forward,omitempty"`
	Arch      string                  `json:"arch"`
}

func normalizeInstallArch(a string) string {
	a = strings.ToLower(strings.TrimSpace(a))
	switch a {
	case "x86_64", "x64":
		return "amd64"
	case "aarch64":
		return "arm64"
	case "arm64", "amd64":
		return a
	default:
		return a
	}
}

func trimCommandOutput(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	lines := strings.Split(s, "\n")
	if len(lines) > 0 {
		return strings.TrimSpace(lines[0])
	}
	return s
}

func fileExists(path string) bool {
	st, err := os.Stat(strings.TrimSpace(path))
	return err == nil && !st.IsDir()
}

func commandError(step string, err error, res *utils.CommandResult) error {
	parts := []string{strings.TrimSpace(step)}
	if err != nil {
		parts = append(parts, err.Error())
	}
	if res != nil {
		if s := strings.TrimSpace(res.Stdout); s != "" {
			parts = append(parts, "stdout: "+s)
		}
		if s := strings.TrimSpace(res.Stderr); s != "" {
			parts = append(parts, "stderr: "+s)
		}
	}
	return fmt.Errorf(strings.Join(parts, " | "))
}

func runRootHelper(timeout time.Duration, step, helper string, args ...string) error {
	allArgs := append([]string{"-n", helper}, args...)
	res, err := utils.Run(timeout, "sudo", allArgs...)
	if err != nil {
		return commandError(step, err, res)
	}
	return nil
}

func (s *SingBoxService) InstallComponentStatus() *InstallComponentStatus {
	binPath := resolveSingBoxBinPath(s.cfg.BinPath)
	out := &InstallComponentStatus{Installed: fileExists(binPath), ConfiguredBinary: strings.TrimSpace(s.cfg.BinPath)}
	if ver, err := s.Version(); err == nil {
		out.Version = strings.TrimSpace(ver)
		if out.Version != "" {
			out.Installed = true
		}
	}
	if st, err := s.Status(); err == nil {
		out.Status = st
	}

	if serviceBinary, err := s.systemd.ExecStartBinary(s.cfg.ServiceName); err == nil {
		out.ServiceBinary = strings.TrimSpace(serviceBinary)
	}
	if runningBinary, err := s.runningBinaryPath(); err == nil {
		out.RunningBinary = strings.TrimSpace(runningBinary)
	}
	out.CandidateBinaries = s.CandidateBinaryPaths()

	pathSet := make(map[string]struct{})
	for _, candidate := range []string{out.ConfiguredBinary, out.ServiceBinary, out.RunningBinary} {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		pathSet[candidate] = struct{}{}
	}
	out.PathConsistent = len(pathSet) <= 1

	detailParts := []string{}
	if out.Installed {
		detailParts = append(detailParts, fmt.Sprintf("已检测到 sing-box（%s）", binPath))
	} else {
		detailParts = append(detailParts, "未检测到 sing-box")
	}
	if out.ConfiguredBinary != "" {
		detailParts = append(detailParts, "面板配置="+out.ConfiguredBinary)
	}
	if out.ServiceBinary != "" {
		detailParts = append(detailParts, "systemd="+out.ServiceBinary)
	}
	if out.RunningBinary != "" {
		detailParts = append(detailParts, "运行中="+out.RunningBinary)
	}
	if !out.PathConsistent {
		detailParts = append(detailParts, "检测到内核路径不一致，请统一面板配置与 systemd 的可执行文件路径")
	}
	out.Detail = strings.Join(detailParts, "；")
	return out
}

func (m *MosDNSService) InstallComponentStatus() *InstallComponentStatus {
	m.mu.RLock()
	cfg := m.cfg
	m.mu.RUnlock()

	out := &InstallComponentStatus{Installed: fileExists("/cus/bin/mosdns")}
	if out.Installed {
		if res, err := utils.Run(10*time.Second, "/cus/bin/mosdns", "version"); err == nil {
			out.Version = trimCommandOutput(res.Stdout)
		}
	}
	if st, err := m.Status(); err == nil {
		out.Status = st
		if st.Active || strings.TrimSpace(st.ActiveState) != "" {
			out.Installed = true
		}
	}
	if strings.TrimSpace(cfg.WebURL) != "" {
		if out.Detail == "" {
			out.Detail = "已配置 Web 面板地址"
		}
	}
	if out.Detail == "" {
		if out.Installed {
			out.Detail = "已检测到 mosdns"
		} else {
			out.Detail = "未检测到 mosdns"
		}
	}
	return out
}

func CollectSystemInstallStatus(singbox *SingBoxService, mosdns *MosDNSService) *SystemInstallStatus {
	out := &SystemInstallStatus{
		SingBox: singbox.InstallComponentStatus(),
		MosDNS:  mosdns.InstallComponentStatus(),
		Arch:    "linux/" + normalizeInstallArch(runtime.GOARCH),
	}
	if ipf, err := singbox.IPForwardStatus(); err == nil {
		out.IPForward = ipf
	}
	return out
}

func (s *SingBoxService) EnableIPForward() error {
	return runRootHelper(30*time.Second, "开启 IP 转发失败", "/usr/local/bin/singdns-panel-enable-ip-forward.sh")
}

func (s *SingBoxService) InstallOfficial(enableIPForward bool) (*OperationResult, error) {
	enable := "0"
	if enableIPForward {
		enable = "1"
	}
	if err := runRootHelper(12*time.Minute, "安装 sing-box 失败", "/usr/local/bin/singdns-panel-install-singbox.sh", enable); err != nil {
		return nil, err
	}
	msg := "已按官方方式安装 sing-box"
	if enableIPForward {
		msg += "，并已开启 IP 转发"
	}
	return &OperationResult{Action: "system.install.singbox", Message: msg}, nil
}

func (m *MosDNSService) InstallFromReference() (*OperationResult, error) {
	m.mu.RLock()
	cfg := m.cfg
	m.mu.RUnlock()

	if err := runRootHelper(15*time.Minute, "安装 mosdns 失败", "/usr/local/bin/singdns-panel-install-mosdns.sh"); err != nil {
		return nil, err
	}

	msg := "已按参考逻辑安装/更新 mosdns"
	if strings.TrimSpace(cfg.WebURL) == "" {
		msg += "，如需嵌入面板请补充 Web URL"
	}
	return &OperationResult{Action: "system.install.mosdns", Message: msg}, nil
}

func EnsureMosDNSInstallDefaults(cfg *cfgpkg.MosDNSConfig) {
	if cfg == nil {
		return
	}
}

// 离线安装（上传包）
func (m *MosDNSService) InstallFromUploaded(corePath, cfgZipPath, originalName string) (*OperationResult, error) {
	if err := runRootHelper(10*time.Minute, "离线安装 mosdns 失败", "/usr/local/bin/singdns-panel-install-mosdns-upload.sh", corePath, cfgZipPath, originalName); err != nil {
		return nil, err
	}
	return &OperationResult{Action: "system.install.mosdns.upload", Message: "已通过上传包安装/更新 mosdns"}, nil
}
