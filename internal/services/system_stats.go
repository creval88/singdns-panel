package services

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type HostStats struct {
	CPUPercent string `json:"cpu_percent"`
	MemUsed    string `json:"mem_used"`
	MemTotal   string `json:"mem_total"`
	MemPercent string `json:"mem_percent"`
}

var cpuSampler struct {
	sync.Mutex
	initialized bool
	lastIdle    uint64
	lastTotal   uint64
	lastPct     float64
	lastAt      time.Time
}

func ReadHostStats() (*HostStats, error) {
	cpuPct, err := readCPUPercent()
	if err != nil {
		return nil, err
	}
	memUsed, memTotal, memPct, err := readMemStat()
	if err != nil {
		return nil, err
	}
	return &HostStats{
		CPUPercent: fmt.Sprintf("%.1f%%", cpuPct),
		MemUsed:    humanizeMB(memUsed),
		MemTotal:   humanizeMB(memTotal),
		MemPercent: fmt.Sprintf("%.1f%%", memPct),
	}, nil
}

func readCPUPercent() (float64, error) {
	idle, total, err := readCPUStat()
	if err != nil {
		return 0, err
	}
	cpuSampler.Lock()
	defer cpuSampler.Unlock()
	now := time.Now()
	if !cpuSampler.initialized {
		cpuSampler.initialized = true
		cpuSampler.lastIdle = idle
		cpuSampler.lastTotal = total
		cpuSampler.lastPct = 0
		cpuSampler.lastAt = now
		return 0, nil
	}
	deltaTotal := total - cpuSampler.lastTotal
	deltaIdle := idle - cpuSampler.lastIdle
	pct := cpuSampler.lastPct
	if deltaTotal > 0 {
		pct = float64(deltaTotal-deltaIdle) * 100 / float64(deltaTotal)
	}
	cpuSampler.lastIdle = idle
	cpuSampler.lastTotal = total
	cpuSampler.lastPct = pct
	cpuSampler.lastAt = now
	return pct, nil
}

func readCPUStat() (idle uint64, total uint64, err error) {
	b, err := os.ReadFile("/proc/stat")
	if err != nil {
		return
	}
	line := strings.Split(string(b), "\n")[0]
	parts := strings.Fields(line)
	for i, p := range parts[1:] {
		v, e := strconv.ParseUint(p, 10, 64)
		if e != nil {
			err = e
			return
		}
		total += v
		if i == 3 || i == 4 {
			idle += v
		}
	}
	return
}

func readMemStat() (usedMB, totalMB uint64, pct float64, err error) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return
	}
	defer f.Close()
	vals := map[string]uint64{}
	s := bufio.NewScanner(f)
	for s.Scan() {
		parts := strings.Fields(strings.TrimSuffix(s.Text(), ":"))
		if len(parts) < 2 {
			continue
		}
		key := strings.TrimSuffix(parts[0], ":")
		v, e := strconv.ParseUint(parts[1], 10, 64)
		if e == nil {
			vals[key] = v / 1024
		}
	}
	totalMB = vals["MemTotal"]
	avail := vals["MemAvailable"]
	usedMB = totalMB - avail
	if totalMB > 0 {
		pct = float64(usedMB) * 100 / float64(totalMB)
	}
	return
}

func humanizeMB(v uint64) string {
	if v >= 1024 {
		return fmt.Sprintf("%.1f GB", float64(v)/1024)
	}
	return fmt.Sprintf("%d MB", v)
}
