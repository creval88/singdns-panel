package services

import cfgpkg "singdns-panel/internal/config"

type MosDNSService struct {
	cfg     cfgpkg.MosDNSConfig
	systemd *SystemdService
}

func NewMosDNSService(cfg cfgpkg.MosDNSConfig, systemd *SystemdService) *MosDNSService {
	return &MosDNSService{cfg: cfg, systemd: systemd}
}

func (m *MosDNSService) Status() (*ServiceStatus, error) { return m.systemd.Status(m.cfg.ServiceName) }
func (m *MosDNSService) Action(action string) (*ServiceActionResult, error) {
	return m.systemd.Action(m.cfg.ServiceName, action)
}
func (m *MosDNSService) Logs(lines int) (string, error) {
	return m.systemd.Logs(m.cfg.ServiceName, lines)
}
func (m *MosDNSService) WebURL() string { return m.cfg.WebURL }
