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
	Installed bool           `json:"installed"`
	Version   string         `json:"version"`
	Status    *ServiceStatus `json:"status,omitempty"`
	Detail    string         `json:"detail"`
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

func runInstallShell(timeout time.Duration, step, command string) error {
	res, err := utils.RunShell(timeout, command)
	if err != nil {
		return commandError(step, err, res)
	}
	return nil
}

func (s *SingBoxService) InstallComponentStatus() *InstallComponentStatus {
	binPath := resolveSingBoxBinPath(s.cfg.BinPath)
	out := &InstallComponentStatus{Installed: fileExists(binPath)}
	if ver, err := s.Version(); err == nil {
		out.Version = strings.TrimSpace(ver)
		if out.Version != "" {
			out.Installed = true
		}
	}
	if st, err := s.Status(); err == nil {
		out.Status = st
	}
	if out.Installed {
		out.Detail = fmt.Sprintf("已检测到 sing-box（%s）", binPath)
	} else {
		out.Detail = "未检测到 sing-box"
	}
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
	content := "net.ipv4.ip_forward=1\nnet.ipv6.conf.all.forwarding=1\n"
	tmp, err := os.CreateTemp("", "singdns-ipforward-*.conf")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.WriteString(content); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if _, err := utils.Run(10*time.Second, "sudo", "mkdir", "-p", "/etc/sysctl.d"); err != nil {
		return err
	}
	if _, err := utils.Run(10*time.Second, "sudo", "install", "-m", "644", tmpPath, "/etc/sysctl.d/99-ipforward.conf"); err != nil {
		return err
	}
	if _, err := utils.Run(10*time.Second, "sudo", "sysctl", "-w", "net.ipv4.ip_forward=1"); err != nil {
		return err
	}
	if _, err := utils.Run(10*time.Second, "sudo", "sysctl", "-w", "net.ipv6.conf.all.forwarding=1"); err != nil {
		return err
	}
	if _, err := utils.Run(10*time.Second, "sudo", "sysctl", "--system"); err != nil {
		return err
	}
	return nil
}

func (s *SingBoxService) InstallOfficial(enableIPForward bool) (*OperationResult, error) {
	cmd := `set -e
export DEBIAN_FRONTEND=noninteractive
if ! command -v curl >/dev/null 2>&1; then
  sudo apt-get update
  sudo apt-get install -y curl ca-certificates
fi
curl -fsSL https://sing-box.app/install.sh | sudo bash
sudo systemctl enable sing-box
sudo systemctl restart sing-box || sudo systemctl start sing-box
`
	if err := runInstallShell(12*time.Minute, "安装 sing-box 失败", cmd); err != nil {
		return nil, err
	}
	if enableIPForward {
		if err := s.EnableIPForward(); err != nil {
			return nil, fmt.Errorf("sing-box 已安装，但开启 IP 转发失败: %w", err)
		}
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

	cmd := `set -e
export DEBIAN_FRONTEND=noninteractive
sudo apt-get update
sudo apt-get install -y wget unzip curl lsof tar ca-certificates rsync || sudo apt-get install -y wget unzip curl lsof tar ca-certificates

BIN_PATH=/cus/bin
CONFIG_PATH=/cus/mosdns
MOSDNS_BIN=$BIN_PATH/mosdns
SERVICE_FILE=/etc/systemd/system/mosdns.service
CONFIG_BASE_URL=https://raw.githubusercontent.com/yyysuo/firetv/refs/heads/master/mosdnsconfigupdate/mosdns1225all.zip
CONFIG_UPDATE_URL=https://raw.githubusercontent.com/yyysuo/firetv/refs/heads/master/mosdnsconfigupdate/mosdns20251225allup.zip
TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT

ARCH_RAW=$(uname -m)
case "$ARCH_RAW" in
  x86_64) ARCH_STR=amd64 ;;
  aarch64) ARCH_STR=arm64 ;;
  *) echo "unsupported arch: $ARCH_RAW" >&2; exit 1 ;;
esac

if command -v ss >/dev/null 2>&1 && ss -lntup 2>/dev/null | grep -q ':53 '; then
  if systemctl is-active --quiet systemd-resolved 2>/dev/null || systemctl is-enabled --quiet systemd-resolved 2>/dev/null; then
    sudo systemctl stop systemd-resolved || true
    sudo systemctl disable systemd-resolved || true
    printf 'nameserver 223.5.5.5\n' | sudo tee /etc/resolv.conf >/dev/null
  else
    lsof -i :53 || true
  fi
fi

TAG_VERSION=$(curl -fsSL https://api.github.com/repos/yyysuo/mosdns/releases/latest | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n1)
if [ -z "$TAG_VERSION" ]; then
  echo 'failed to resolve latest mosdns release tag' >&2
  exit 1
fi
DOWNLOAD_URL="https://github.com/yyysuo/mosdns/releases/download/${TAG_VERSION}/mosdns-linux-${ARCH_STR}.zip"

sudo systemctl stop mosdns || true
sudo mkdir -p "$BIN_PATH" "$CONFIG_PATH"

curl -fL "$DOWNLOAD_URL" -o "$TMP_DIR/mosdns.zip"
unzip -oq "$TMP_DIR/mosdns.zip" -d "$TMP_DIR/mosdns_extract"
MOSDNS_SRC=$(find "$TMP_DIR/mosdns_extract" -type f -name mosdns | head -n1)
if [ -z "$MOSDNS_SRC" ]; then
  echo 'mosdns binary not found in release zip' >&2
  exit 1
fi
sudo install -m 755 "$MOSDNS_SRC" "$MOSDNS_BIN"

curl -fL "$CONFIG_BASE_URL" -o "$TMP_DIR/mosdns_base.zip"
unzip -oq "$TMP_DIR/mosdns_base.zip" -d "$TMP_DIR/mosdns_base"
if command -v rsync >/dev/null 2>&1; then
  sudo rsync -a "$TMP_DIR/mosdns_base"/ "$CONFIG_PATH"/
else
  sudo cp -a "$TMP_DIR/mosdns_base"/. "$CONFIG_PATH"/
fi

if curl -fsSL "$CONFIG_UPDATE_URL" -o "$TMP_DIR/mosdns_update.zip"; then
  unzip -oq "$TMP_DIR/mosdns_update.zip" -d "$TMP_DIR/mosdns_update"
  if command -v rsync >/dev/null 2>&1; then
    sudo rsync -a "$TMP_DIR/mosdns_update"/ "$CONFIG_PATH"/
  else
    sudo cp -a "$TMP_DIR/mosdns_update"/. "$CONFIG_PATH"/
  fi
fi

sudo chmod -R 777 "$CONFIG_PATH"
cat <<'UNIT' | sudo tee "$SERVICE_FILE" >/dev/null
[Unit]
Description=MosDNS Service
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/cus/bin/mosdns start -c /cus/mosdns/config_custom.yaml -d /cus/mosdns
Restart=always
RestartSec=3

[Install]
WantedBy=multi-user.target
UNIT

sudo systemctl daemon-reload
sudo systemctl enable mosdns
sudo systemctl restart mosdns || sudo systemctl start mosdns
`
	if err := runInstallShell(15*time.Minute, "安装 mosdns 失败", cmd); err != nil {
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
