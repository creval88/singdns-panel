package services

import (
	"fmt"
	"strings"
	"time"

	"singdns-panel/internal/utils"
)

type ServiceStatus struct {
	Name        string `json:"name"`
	Active      bool   `json:"active"`
	Enabled     bool   `json:"enabled"`
	ActiveState string `json:"active_state"`
	SubState    string `json:"sub_state"`
	Summary     string `json:"summary"`
}

type ServiceActionResult struct {
	Service string `json:"service"`
	Action  string `json:"action"`
	Message string `json:"message"`
}

func (r ServiceActionResult) AuditText() string {
	msg := strings.TrimSpace(r.Message)
	if msg == "" {
		return fmt.Sprintf("%s %s", r.Service, r.Action)
	}
	return msg
}

type SystemdService struct{}

func NewSystemdService() *SystemdService { return &SystemdService{} }

func (s *SystemdService) Status(name string) (*ServiceStatus, error) {
	res, err := utils.Run(5*time.Second, "systemctl", "show", name, "--no-page", "--property=ActiveState,SubState,UnitFileState,Description")
	if err != nil {
		return nil, err
	}
	st := &ServiceStatus{Name: name}
	for _, line := range strings.Split(res.Stdout, "\n") {
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		switch parts[0] {
		case "ActiveState":
			st.ActiveState = parts[1]
			st.Active = parts[1] == "active"
		case "SubState":
			st.SubState = parts[1]
		case "UnitFileState":
			st.Enabled = parts[1] == "enabled"
		case "Description":
			st.Summary = parts[1]
		}
	}
	return st, nil
}

func (s *SystemdService) Action(name, action string) (*ServiceActionResult, error) {
	action = strings.TrimSpace(action)
	switch action {
	case "start", "stop", "restart", "enable", "disable":
	default:
		return nil, fmt.Errorf("unsupported action: %s", action)
	}
	_, err := utils.Run(10*time.Second, "sudo", "systemctl", action, name)
	if err != nil {
		return nil, err
	}
	return &ServiceActionResult{Service: name, Action: action, Message: fmt.Sprintf("服务 %s 已%s", name, systemActionText(action))}, nil
}

func (s *SystemdService) Logs(name string, lines int) (string, error) {
	if lines <= 0 {
		lines = 100
	}
	res, err := utils.Run(10*time.Second, "sudo", "journalctl", "-u", name, "-n", fmt.Sprintf("%d", lines), "--no-pager")
	if err != nil {
		return res.Stdout + res.Stderr, err
	}
	return res.Stdout, nil
}

func systemActionText(action string) string {
	switch action {
	case "start":
		return "启动"
	case "stop":
		return "停止"
	case "restart":
		return "重启"
	case "enable":
		return "开启自启"
	case "disable":
		return "关闭自启"
	default:
		return action
	}
}
