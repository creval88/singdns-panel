package services

import (
	"strings"
	"sync"

	cfgpkg "singdns-panel/internal/config"
)

type MosDNSService struct {
	mu      sync.RWMutex
	cfg     cfgpkg.MosDNSConfig
	systemd *SystemdService
}

func NewMosDNSService(cfg cfgpkg.MosDNSConfig, systemd *SystemdService) *MosDNSService {
	return &MosDNSService{cfg: cfg, systemd: systemd}
}

func (m *MosDNSService) Status() (*ServiceStatus, error) { return m.systemd.Status(m.serviceName()) }
func (m *MosDNSService) Action(action string) (*ServiceActionResult, error) {
	return m.systemd.Action(m.serviceName(), action)
}
func (m *MosDNSService) Logs(lines int) (string, error) {
	return m.systemd.Logs(m.serviceName(), lines)
}

func (m *MosDNSService) WebURL() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return strings.TrimSpace(m.cfg.WebURL)
}

func (m *MosDNSService) UpdateConfig(cfg cfgpkg.MosDNSConfig) {
	m.mu.Lock()
	m.cfg = cfg
	m.mu.Unlock()
}

func (m *MosDNSService) serviceName() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.cfg.ServiceName
}
