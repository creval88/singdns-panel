package services

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"singdns-panel/internal/utils"
)

const (
	managedNetplanPath    = "/etc/netplan/99-singdns-panel.yaml"
	interfacesConfigPath  = "/etc/network/interfaces"
	resolvConfPath        = "/etc/resolv.conf"
	networkRollbackSuffix = ".singdns-panel.bak"
)

var ipv4Re = regexp.MustCompile(`^(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3}$`)

type NetworkStatus struct {
	Interface      string   `json:"interface"`
	Mode           string   `json:"mode"`
	Address        string   `json:"address"`
	Prefix         int      `json:"prefix"`
	Gateway        string   `json:"gateway"`
	DNS            []string `json:"dns"`
	DNSText        string   `json:"dns_text"`
	RuntimeDNS     []string `json:"runtime_dns"`
	RuntimeDNSText string   `json:"runtime_dns_text"`
	ConfigDNS      []string `json:"config_dns"`
	ConfigDNSText  string   `json:"config_dns_text"`
	Source         string   `json:"source"`
	Backend        string   `json:"backend"`
	LastGoodPath   string   `json:"last_good_path"`
	Summary        string   `json:"summary"`
	NetplanOK      bool     `json:"netplan_ok"`
	NetplanPath    string   `json:"netplan_path"`
	ResolvPath     string   `json:"resolv_path"`
}

type NetworkSettingsInput struct {
	Interface string   `json:"interface"`
	Mode      string   `json:"mode"`
	Address   string   `json:"address"`
	Prefix    int      `json:"prefix"`
	Gateway   string   `json:"gateway"`
	DNS       []string `json:"dns"`
}

type NetworkService struct{}

func NewNetworkService() *NetworkService { return &NetworkService{} }

func (s *NetworkService) Status() (*NetworkStatus, error) {
	st := &NetworkStatus{Mode: "unknown", NetplanPath: managedNetplanPath, ResolvPath: resolvConfPath}
	st.LastGoodPath = lastGoodPathFor(interfacesConfigPath)
	st.Interface = strings.TrimSpace(firstLine(runShellTrim("ip -4 route show default | awk 'NR==1{print $5}'")))
	if st.Interface == "" {
		st.Interface = strings.TrimSpace(firstLine(runShellTrim("ip route show default | awk 'NR==1{print $5}'")))
	}
	addr := strings.TrimSpace(firstLine(runShellTrim("ip -4 -o addr show dev " + shellArg(st.Interface) + " scope global | awk 'NR==1{print $4}'")))
	if addr != "" {
		parts := strings.SplitN(addr, "/", 2)
		st.Address = strings.TrimSpace(parts[0])
		if len(parts) == 2 {
			if p, err := strconv.Atoi(strings.TrimSpace(parts[1])); err == nil {
				st.Prefix = p
			}
		}
	}
	st.Gateway = strings.TrimSpace(firstLine(runShellTrim("ip -4 route show default | awk 'NR==1{print $3}'")))
	st.RuntimeDNS = readResolvNameservers(resolvConfPath)
	st.RuntimeDNSText = strings.Join(st.RuntimeDNS, ", ")
	st.DNS = append([]string{}, st.RuntimeDNS...)
	st.DNSText = st.RuntimeDNSText
	if networkPathExists("/usr/sbin/netplan") {
		st.NetplanOK = true
	}

	if networkPathExists(managedNetplanPath) {
		st.Backend = "netplan"
		st.LastGoodPath = lastGoodPathFor(managedNetplanPath)
		b, _ := os.ReadFile(managedNetplanPath)
		text := string(b)
		st.Source = managedNetplanPath
		if strings.Contains(text, "dhcp4: true") {
			st.Mode = "dhcp"
		}
		if strings.Contains(text, "addresses:") {
			st.Mode = "static"
		}
		if vals := yamlListAfter(text, "addresses:"); len(vals) > 0 {
			parts := strings.SplitN(vals[0], "/", 2)
			st.Address = strings.TrimSpace(parts[0])
			if len(parts) == 2 {
				if p, err := strconv.Atoi(strings.TrimSpace(parts[1])); err == nil {
					st.Prefix = p
				}
			}
		}
		if gw := yamlScalar(text, "gateway4:"); gw != "" {
			st.Gateway = gw
		}
		if dns := yamlListAfter(text, "addresses:", "nameservers:"); len(dns) > 0 {
			st.ConfigDNS = dns
			st.ConfigDNSText = strings.Join(dns, ", ")
		}
	} else if networkPathExists(interfacesConfigPath) {
		st.Backend = "ifupdown"
		if cfg, ok := parseInterfacesConfig(mustReadFile(interfacesConfigPath), st.Interface); ok {
			st.Source = interfacesConfigPath
			if cfg.Mode != "" {
				st.Mode = cfg.Mode
			}
			if cfg.Address != "" {
				st.Address = cfg.Address
			}
			if cfg.Prefix > 0 {
				st.Prefix = cfg.Prefix
			}
			if cfg.Gateway != "" {
				st.Gateway = cfg.Gateway
			}
			if len(cfg.DNS) > 0 {
				st.ConfigDNS = cfg.DNS
				st.ConfigDNSText = strings.Join(cfg.DNS, ", ")
			}
		}
	} else {
		st.Source = resolvConfPath
		st.Backend = "resolv.conf"
		st.LastGoodPath = lastGoodPathFor(resolvConfPath)
	}

	if len(st.ConfigDNS) == 0 {
		st.ConfigDNS = append([]string{}, st.RuntimeDNS...)
		st.ConfigDNSText = st.RuntimeDNSText
	}
	if st.Mode == "unknown" && st.Address != "" {
		st.Mode = "dhcp"
	}
	st.Summary = buildNetworkSummary(st)
	return st, nil
}

func (s *NetworkService) Apply(in NetworkSettingsInput) (*OperationResult, error) {
	in.Interface = strings.TrimSpace(in.Interface)
	in.Mode = strings.ToLower(strings.TrimSpace(in.Mode))
	in.Address = strings.TrimSpace(in.Address)
	in.Gateway = strings.TrimSpace(in.Gateway)
	if in.Interface == "" {
		return nil, fmt.Errorf("网卡不能为空")
	}
	if _, err := net.InterfaceByName(in.Interface); err != nil {
		return nil, fmt.Errorf("网卡不存在: %s", in.Interface)
	}
	if in.Mode != "dhcp" && in.Mode != "static" && in.Mode != "dns-only" {
		return nil, fmt.Errorf("mode 仅支持 dhcp / static / dns-only")
	}
	dns, err := normalizeDNSList(in.DNS)
	if err != nil {
		return nil, err
	}
	if in.Mode == "static" {
		if !ipv4Re.MatchString(in.Address) {
			return nil, fmt.Errorf("静态 IP 格式错误")
		}
		if in.Prefix <= 0 || in.Prefix > 32 {
			return nil, fmt.Errorf("前缀长度需在 1-32 之间")
		}
		if in.Gateway != "" && !ipv4Re.MatchString(in.Gateway) {
			return nil, fmt.Errorf("网关格式错误")
		}
	}
	if in.Mode == "dns-only" {
		if len(dns) == 0 {
			return nil, fmt.Errorf("DNS 不能为空")
		}
		if err := s.writeResolvConf(dns); err != nil {
			return nil, err
		}
		return &OperationResult{Action: "system.network.apply", Message: "系统 DNS 已更新 · DNS: " + strings.Join(dns, ", ")}, nil
	}
	if networkPathExists("/usr/sbin/netplan") {
		return s.applyNetplan(in, dns)
	}
	if networkPathExists(interfacesConfigPath) {
		return s.applyIfupdown(in, dns)
	}
	return nil, fmt.Errorf("当前系统未检测到受支持的网络配置后端（需要 netplan 或 /etc/network/interfaces）")
}

func (s *NetworkService) applyNetplan(in NetworkSettingsInput, dns []string) (*OperationResult, error) {
	content := buildNetplanYAML(in, dns)
	current := mustReadFile(managedNetplanPath)
	if normalizeConfigText(current) == normalizeConfigText(content) {
		msg := "netplan 配置未变化，已跳过 apply"
		if len(dns) > 0 {
			msg += " · DNS: " + strings.Join(dns, ", ")
		}
		return &OperationResult{Action: "system.network.apply", Message: msg}, nil
	}
	backupPath, hadOriginal, err := backupNetworkFile(managedNetplanPath)
	if err != nil {
		return nil, fmt.Errorf("创建 netplan 备份失败: %w", err)
	}
	if err := writeRootFile(managedNetplanPath, content, 0644); err != nil {
		return nil, fmt.Errorf("写入 netplan 配置失败: %w", err)
	}
	if _, err := runSudo(20*time.Second, "/usr/sbin/netplan", "generate"); err != nil {
		rbErr := restoreNetworkFile(managedNetplanPath, backupPath, hadOriginal)
		if rbErr != nil {
			return nil, fmt.Errorf("netplan generate 失败，且回滚失败: apply_err=%v rollback_err=%v", err, rbErr)
		}
		return nil, fmt.Errorf("netplan generate 失败，已自动回滚: %w", err)
	}
	if _, err := runSudo(30*time.Second, "/usr/sbin/netplan", "apply"); err != nil {
		rbErr := restoreNetworkFile(managedNetplanPath, backupPath, hadOriginal)
		if rbErr != nil {
			return nil, fmt.Errorf("netplan apply 失败，且回滚失败: apply_err=%v rollback_err=%v", err, rbErr)
		}
		if _, rbApplyErr := runSudo(30*time.Second, "/usr/sbin/netplan", "apply"); rbApplyErr != nil {
			return nil, fmt.Errorf("netplan apply 失败，配置已回滚，但回滚后的 netplan apply 失败: apply_err=%v rollback_apply_err=%v", err, rbApplyErr)
		}
		return nil, fmt.Errorf("netplan apply 失败，已自动回滚: %w", err)
	}
	_ = cleanupNetworkBackup(backupPath)
	return networkApplyResult(in, dns), nil
}

func (s *NetworkService) applyIfupdown(in NetworkSettingsInput, dns []string) (*OperationResult, error) {
	content := buildInterfacesConfig(in, dns)
	current := mustReadFile(interfacesConfigPath)
	if normalizeConfigText(current) == normalizeConfigText(content) {
		msg := "ifupdown 配置未变化，已跳过 networking 重启"
		if len(dns) > 0 {
			msg += " · DNS: " + strings.Join(dns, ", ")
		}
		return &OperationResult{Action: "system.network.apply", Message: msg}, nil
	}
	backupPath, hadOriginal, err := backupNetworkFile(interfacesConfigPath)
	if err != nil {
		return nil, fmt.Errorf("创建 interfaces 备份失败: %w", err)
	}
	if err := writeRootFile(interfacesConfigPath, content, 0644); err != nil {
		return nil, fmt.Errorf("写入 interfaces 配置失败: %w", err)
	}
	if _, err := runSudo(45*time.Second, "/bin/systemctl", "restart", "networking"); err != nil {
		rbErr := restoreNetworkFile(interfacesConfigPath, backupPath, hadOriginal)
		if rbErr != nil {
			return nil, fmt.Errorf("重启 networking 失败，且回滚失败: apply_err=%v rollback_err=%v", err, rbErr)
		}
		if _, rbRestartErr := runSudo(45*time.Second, "/bin/systemctl", "restart", "networking"); rbRestartErr != nil {
			return nil, fmt.Errorf("重启 networking 失败，配置已回滚，但回滚后的 networking 重启仍失败: apply_err=%v rollback_restart_err=%v", err, rbRestartErr)
		}
		return nil, fmt.Errorf("重启 networking 失败，已自动回滚: %w", err)
	}
	_ = cleanupNetworkBackup(backupPath)
	return networkApplyResult(in, dns), nil
}

func networkApplyResult(in NetworkSettingsInput, dns []string) *OperationResult {
	msg := "网络配置已应用"
	if in.Mode == "dhcp" {
		msg = fmt.Sprintf("已切换为 DHCP（%s）", in.Interface)
	} else {
		msg = fmt.Sprintf("已设置静态 IP：%s/%d（%s）", in.Address, in.Prefix, in.Interface)
	}
	if len(dns) > 0 {
		msg += " · DNS: " + strings.Join(dns, ", ")
	}
	return &OperationResult{Action: "system.network.apply", Message: msg}
}

func (s *NetworkService) writeResolvConf(dns []string) error {
	var b strings.Builder
	b.WriteString("# managed by singdns-panel\n")
	for _, ip := range dns {
		b.WriteString("nameserver ")
		b.WriteString(ip)
		b.WriteString("\n")
	}
	content := b.String()
	current := mustReadFile(resolvConfPath)
	if normalizeConfigText(current) == normalizeConfigText(content) {
		return nil
	}
	backupPath, hadOriginal, err := backupNetworkFile(resolvConfPath)
	if err != nil {
		return fmt.Errorf("创建 resolv.conf 备份失败: %w", err)
	}
	if err := writeRootFile(resolvConfPath, content, 0644); err != nil {
		return fmt.Errorf("写入 resolv.conf 失败: %w", err)
	}
	if _, err := runSudo(10*time.Second, "/usr/bin/test", "-s", resolvConfPath); err != nil {
		rbErr := restoreNetworkFile(resolvConfPath, backupPath, hadOriginal)
		if rbErr != nil {
			return fmt.Errorf("校验 resolv.conf 失败，且回滚失败: apply_err=%v rollback_err=%v", err, rbErr)
		}
		return fmt.Errorf("校验 resolv.conf 失败，已自动回滚: %w", err)
	}
	_ = cleanupNetworkBackup(backupPath)
	return nil
}

func buildNetplanYAML(in NetworkSettingsInput, dns []string) string {
	var b strings.Builder
	b.WriteString("network:\n")
	b.WriteString("  version: 2\n")
	b.WriteString("  renderer: networkd\n")
	b.WriteString("  ethernets:\n")
	b.WriteString("    ")
	b.WriteString(in.Interface)
	b.WriteString(":\n")
	if in.Mode == "dhcp" {
		b.WriteString("      dhcp4: true\n")
		b.WriteString("      dhcp6: false\n")
	} else {
		b.WriteString("      dhcp4: false\n")
		b.WriteString("      dhcp6: false\n")
		b.WriteString("      addresses:\n")
		b.WriteString("        - ")
		b.WriteString(in.Address)
		b.WriteString("/")
		b.WriteString(strconv.Itoa(in.Prefix))
		b.WriteString("\n")
		if in.Gateway != "" {
			b.WriteString("      gateway4: ")
			b.WriteString(in.Gateway)
			b.WriteString("\n")
		}
	}
	if len(dns) > 0 {
		b.WriteString("      nameservers:\n")
		b.WriteString("        addresses:\n")
		for _, ip := range dns {
			b.WriteString("          - ")
			b.WriteString(ip)
			b.WriteString("\n")
		}
	}
	return b.String()
}

func buildInterfacesConfig(in NetworkSettingsInput, dns []string) string {
	var b strings.Builder
	b.WriteString("source /etc/network/interfaces.d/*\n\n")
	b.WriteString("auto lo\n")
	b.WriteString("iface lo inet loopback\n\n")
	b.WriteString("auto ")
	b.WriteString(in.Interface)
	b.WriteString("\n")
	b.WriteString("iface ")
	b.WriteString(in.Interface)
	if in.Mode == "dhcp" {
		b.WriteString(" inet dhcp\n")
	} else {
		b.WriteString(" inet static\n")
		b.WriteString("    address ")
		b.WriteString(in.Address)
		b.WriteString("\n")
		if in.Prefix > 0 {
			b.WriteString("    netmask ")
			b.WriteString(prefixToNetmask(in.Prefix))
			b.WriteString("\n")
		}
		if in.Gateway != "" {
			b.WriteString("    gateway ")
			b.WriteString(in.Gateway)
			b.WriteString("\n")
		}
	}
	if len(dns) > 0 {
		b.WriteString("    dns-nameservers ")
		b.WriteString(strings.Join(dns, " "))
		b.WriteString("\n")
	}
	return b.String()
}

func buildNetworkSummary(st *NetworkStatus) string {
	parts := []string{}
	if st.Interface != "" {
		parts = append(parts, "网卡: "+st.Interface)
	}
	if st.Mode != "" {
		parts = append(parts, "模式: "+st.Mode)
	}
	if st.Address != "" {
		if st.Prefix > 0 {
			parts = append(parts, fmt.Sprintf("IP: %s/%d", st.Address, st.Prefix))
		} else {
			parts = append(parts, "IP: "+st.Address)
		}
	}
	if st.Gateway != "" {
		parts = append(parts, "网关: "+st.Gateway)
	}
	if len(st.DNS) > 0 {
		parts = append(parts, "DNS: "+strings.Join(st.DNS, ", "))
	}
	if len(parts) == 0 {
		return "未检测到网络信息"
	}
	return strings.Join(parts, " · ")
}

func readResolvNameservers(path string) []string {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	out := []string{}
	seen := map[string]struct{}{}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "nameserver ") {
			continue
		}
		ip := strings.TrimSpace(strings.TrimPrefix(line, "nameserver "))
		if ip == "" {
			continue
		}
		if _, ok := seen[ip]; ok {
			continue
		}
		seen[ip] = struct{}{}
		out = append(out, ip)
	}
	return out
}

func normalizeDNSList(in []string) ([]string, error) {
	out := []string{}
	seen := map[string]struct{}{}
	for _, item := range in {
		for _, part := range strings.FieldsFunc(item, func(r rune) bool { return r == ',' || r == '\n' || r == '\r' || r == ' ' || r == '\t' }) {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			if !ipv4Re.MatchString(part) {
				return nil, fmt.Errorf("DNS 格式错误: %s", part)
			}
			if _, ok := seen[part]; ok {
				continue
			}
			seen[part] = struct{}{}
			out = append(out, part)
		}
	}
	return out, nil
}

func writeRootFile(targetPath, content string, mode os.FileMode) error {
	tmpPath, err := writeTempContent(content)
	if err != nil {
		return err
	}
	defer os.Remove(tmpPath)
	dir := filepath.Dir(targetPath)
	if !networkPathExists(dir) {
		if _, err := runSudo(10*time.Second, "/bin/mkdir", "-p", dir); err != nil {
			return err
		}
	}
	if _, err := runSudo(10*time.Second, "/usr/bin/install", "-m", fmt.Sprintf("%o", mode), tmpPath, targetPath); err != nil {
		return err
	}
	return nil
}

func runSudo(timeout time.Duration, command string, args ...string) (*utils.CommandResult, error) {
	allArgs := []string{"-n", command}
	allArgs = append(allArgs, args...)
	return utils.Run(timeout, "sudo", allArgs...)
}

func runShellTrim(cmd string) string {
	res, err := utils.RunShell(5*time.Second, cmd)
	if err != nil || res == nil {
		return ""
	}
	if strings.TrimSpace(res.Stdout) != "" {
		return strings.TrimSpace(res.Stdout)
	}
	return strings.TrimSpace(res.Stderr)
}

func firstLine(s string) string {
	if s == "" {
		return ""
	}
	parts := strings.Split(s, "\n")
	return strings.TrimSpace(parts[0])
}

func shellArg(s string) string {
	s = strings.ReplaceAll(s, "'", "'\\''")
	return "'" + s + "'"
}

func yamlScalar(text, key string) string {
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, key) {
			return strings.TrimSpace(strings.TrimPrefix(trimmed, key))
		}
	}
	return ""
}

func yamlListAfter(text, key string, parentKeys ...string) []string {
	lines := strings.Split(text, "\n")
	activeParent := len(parentKeys) == 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if len(parentKeys) > 0 {
			for _, pk := range parentKeys {
				if trimmed == pk {
					activeParent = true
				}
			}
		}
		if !activeParent {
			continue
		}
		if trimmed == key {
			vals := []string{}
			continueList := false
			for i := range lines {
				if strings.TrimSpace(lines[i]) != key {
					continue
				}
				for j := i + 1; j < len(lines); j++ {
					t := strings.TrimSpace(lines[j])
					if t == "" {
						continue
					}
					if strings.HasPrefix(t, "-") {
						vals = append(vals, strings.TrimSpace(strings.TrimPrefix(t, "-")))
						continueList = true
						continue
					}
					if continueList {
						return vals
					}
					if !strings.HasPrefix(lines[j], " ") {
						return vals
					}
				}
				return vals
			}
		}
	}
	return nil
}

type interfacesConfig struct {
	Mode    string
	Address string
	Prefix  int
	Gateway string
	DNS     []string
}

func parseInterfacesConfig(text, iface string) (*interfacesConfig, bool) {
	iface = strings.TrimSpace(iface)
	if iface == "" {
		return nil, false
	}
	cfg := &interfacesConfig{}
	inBlock := false
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 4 && fields[0] == "iface" {
			inBlock = fields[1] == iface
			if inBlock {
				cfg.Mode = strings.ToLower(strings.TrimSpace(fields[3]))
			}
			continue
		}
		if len(fields) > 0 && (fields[0] == "auto" || fields[0] == "allow-hotplug" || fields[0] == "source" || fields[0] == "source-directory") {
			if inBlock {
				inBlock = false
			}
			continue
		}
		if !inBlock || len(fields) < 2 {
			continue
		}
		switch fields[0] {
		case "address":
			addr := strings.TrimSpace(fields[1])
			if strings.Contains(addr, "/") {
				parts := strings.SplitN(addr, "/", 2)
				cfg.Address = strings.TrimSpace(parts[0])
				if p, err := strconv.Atoi(strings.TrimSpace(parts[1])); err == nil {
					cfg.Prefix = p
				}
			} else {
				cfg.Address = addr
			}
		case "netmask":
			if cfg.Prefix == 0 {
				cfg.Prefix = netmaskToPrefix(fields[1])
			}
		case "gateway":
			cfg.Gateway = strings.TrimSpace(fields[1])
		case "dns-nameservers":
			cfg.DNS = dedupeStrings(fields[1:])
		}
	}
	if cfg.Mode == "" {
		return nil, false
	}
	return cfg, true
}

func netmaskToPrefix(mask string) int {
	ip := net.ParseIP(strings.TrimSpace(mask)).To4()
	if ip == nil {
		return 0
	}
	ones, bits := net.IPMask(ip).Size()
	if bits != 32 {
		return 0
	}
	return ones
}

func prefixToNetmask(prefix int) string {
	if prefix <= 0 || prefix > 32 {
		return "255.255.255.0"
	}
	mask := net.CIDRMask(prefix, 32)
	return net.IP(mask).String()
}

func dedupeStrings(items []string) []string {
	out := make([]string, 0, len(items))
	seen := map[string]struct{}{}
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func networkPathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func writeTempContent(content string) (string, error) {
	tmp, err := os.CreateTemp("", "singdns-network-*")
	if err != nil {
		return "", err
	}
	tmpPath := tmp.Name()
	if _, err := tmp.WriteString(content); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return "", err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return "", err
	}
	return tmpPath, nil
}

func backupNetworkFile(targetPath string) (backupPath string, hadOriginal bool, err error) {
	if !networkPathExists(targetPath) {
		return targetPath + networkRollbackSuffix, false, nil
	}
	content := mustReadFile(targetPath)
	if err := writeRootFile(lastGoodPathFor(targetPath), content, 0644); err != nil {
		return "", false, err
	}
	backupPath, err = writeTempContent(content)
	if err != nil {
		return "", false, err
	}
	return backupPath, true, nil
}

func restoreNetworkFile(targetPath, backupPath string, hadOriginal bool) error {
	if !hadOriginal {
		if !networkPathExists(targetPath) {
			return nil
		}
		_, err := runSudo(10*time.Second, "/bin/rm", "-f", targetPath)
		return err
	}
	defer os.Remove(backupPath)
	if _, err := runSudo(10*time.Second, "/usr/bin/install", "-m", "644", backupPath, targetPath); err != nil {
		return err
	}
	return nil
}

func cleanupNetworkBackup(backupPath string) error {
	if strings.TrimSpace(backupPath) == "" {
		return nil
	}
	if err := os.Remove(backupPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func lastGoodPathFor(targetPath string) string {
	return targetPath + ".singdns-panel.last-good"
}

func mustReadFile(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(b)
}

func normalizeConfigText(s string) string {
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	return strings.Join(out, "\n")
}
