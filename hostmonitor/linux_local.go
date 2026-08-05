package hostmonitor

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type Source interface {
	Sample(context.Context, *HostSnapshot) (HostSnapshot, error)
	Close() error
}

type LocalLinuxSource struct {
	procRoot string
	etcRoot  string
	now      func() time.Time
}

func NewLocalLinuxSource() *LocalLinuxSource {
	return &LocalLinuxSource{procRoot: "/proc", etcRoot: "/etc", now: time.Now}
}

func (s *LocalLinuxSource) Close() error { return nil }

func (s *LocalLinuxSource) Sample(ctx context.Context, previous *HostSnapshot) (HostSnapshot, error) {
	if runtime.GOOS != "linux" {
		return HostSnapshot{}, errors.New("local host monitor currently requires Linux")
	}
	if err := ctx.Err(); err != nil {
		return HostSnapshot{}, err
	}
	now := s.now()
	identity, err := s.identity(ctx)
	if err != nil {
		return HostSnapshot{}, err
	}
	cpuText, err := os.ReadFile(s.procRoot + "/stat")
	if err != nil {
		return HostSnapshot{}, fmt.Errorf("read proc stat: %w", err)
	}
	cpu, err := parseCPUStat(string(cpuText))
	if err != nil {
		return HostSnapshot{}, err
	}
	memText, err := os.ReadFile(s.procRoot + "/meminfo")
	if err != nil {
		return HostSnapshot{}, fmt.Errorf("read proc meminfo: %w", err)
	}
	memory, err := parseMemInfo(string(memText))
	if err != nil {
		return HostSnapshot{}, err
	}
	loadText, err := os.ReadFile(s.procRoot + "/loadavg")
	if err != nil {
		return HostSnapshot{}, fmt.Errorf("read proc loadavg: %w", err)
	}
	load, err := parseLoadAverage(string(loadText))
	if err != nil {
		return HostSnapshot{}, err
	}
	uptimeText, err := os.ReadFile(s.procRoot + "/uptime")
	if err != nil {
		return HostSnapshot{}, fmt.Errorf("read proc uptime: %w", err)
	}
	uptime, err := parseUptime(string(uptimeText))
	if err != nil {
		return HostSnapshot{}, err
	}
	netText, err := os.ReadFile(s.procRoot + "/net/dev")
	if err != nil {
		return HostSnapshot{}, fmt.Errorf("read proc net dev: %w", err)
	}
	network, err := parseNetworkDevices(string(netText))
	if err != nil {
		return HostSnapshot{}, err
	}
	if previous != nil {
		cpu.UsagePercent = cpuUsage(previous.CPU, cpu)
		applyNetworkRates(network, previous.Network, now.Sub(previous.CollectedAt))
	}
	return HostSnapshot{
		CollectedAt: now,
		Identity:    identity,
		CPU:         cpu,
		Memory:      memory,
		Load:        load,
		Uptime:      uptime,
		Network:     network,
	}, nil
}

func (s *LocalLinuxSource) identity(ctx context.Context) (HostIdentity, error) {
	if err := ctx.Err(); err != nil {
		return HostIdentity{}, err
	}
	identity := HostIdentity{Architecture: runtime.GOARCH, LogicalCPUs: runtime.NumCPU()}
	identity.Hostname, _ = os.Hostname()
	if content, err := os.ReadFile(s.etcRoot + "/os-release"); err == nil {
		values := parseKeyValues(string(content))
		identity.OSName = values["NAME"]
		identity.OSVersion = values["VERSION_ID"]
	}
	if content, err := os.ReadFile(s.procRoot + "/sys/kernel/osrelease"); err == nil {
		identity.Kernel = strings.TrimSpace(string(content))
	}
	if content, err := os.ReadFile(s.procRoot + "/cpuinfo"); err == nil {
		identity.CPUModel = parseCPUModel(string(content))
	}
	return identity, nil
}

func parseCPUStat(text string) (CPUStats, error) {
	for _, line := range strings.Split(text, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 || fields[0] != "cpu" {
			continue
		}
		values := make([]uint64, 8)
		for i := 0; i < len(values) && i+1 < len(fields); i++ {
			value, err := strconv.ParseUint(fields[i+1], 10, 64)
			if err != nil {
				return CPUStats{}, fmt.Errorf("parse proc stat field %d: %w", i, err)
			}
			values[i] = value
		}
		return CPUStats{User: values[0], Nice: values[1], System: values[2], Idle: values[3], IOWait: values[4], IRQ: values[5], SoftIRQ: values[6], Steal: values[7]}, nil
	}
	return CPUStats{}, errors.New("aggregate cpu row not found")
}

func cpuUsage(previous, current CPUStats) float64 {
	if current.TotalTicks() <= previous.TotalTicks() || current.IdleTicks() < previous.IdleTicks() {
		return 0
	}
	total := current.TotalTicks() - previous.TotalTicks()
	idle := current.IdleTicks() - previous.IdleTicks()
	if total == 0 || idle > total {
		return 0
	}
	return float64(total-idle) * 100 / float64(total)
}

func parseMemInfo(text string) (MemoryStats, error) {
	values := map[string]uint64{}
	scanner := bufio.NewScanner(strings.NewReader(text))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}
		value, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			continue
		}
		values[strings.TrimSuffix(fields[0], ":")] = value * 1024
	}
	if err := scanner.Err(); err != nil {
		return MemoryStats{}, err
	}
	total := values["MemTotal"]
	available := values["MemAvailable"]
	if total == 0 {
		return MemoryStats{}, errors.New("MemTotal not found")
	}
	if available > total {
		available = total
	}
	swapTotal, swapFree := values["SwapTotal"], values["SwapFree"]
	if swapFree > swapTotal {
		swapFree = swapTotal
	}
	return MemoryStats{TotalBytes: total, AvailableBytes: available, UsedBytes: total - available, SwapTotalBytes: swapTotal, SwapFreeBytes: swapFree, SwapUsedBytes: swapTotal - swapFree}, nil
}

func parseLoadAverage(text string) (LoadStats, error) {
	fields := strings.Fields(text)
	if len(fields) < 3 {
		return LoadStats{}, errors.New("invalid loadavg")
	}
	values := make([]float64, 3)
	for i := range values {
		value, err := strconv.ParseFloat(fields[i], 64)
		if err != nil {
			return LoadStats{}, fmt.Errorf("parse loadavg field %d: %w", i, err)
		}
		values[i] = value
	}
	return LoadStats{One: values[0], Five: values[1], Fifteen: values[2]}, nil
}

func parseUptime(text string) (time.Duration, error) {
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return 0, errors.New("invalid uptime")
	}
	seconds, err := strconv.ParseFloat(fields[0], 64)
	if err != nil || seconds < 0 {
		return 0, errors.New("invalid uptime seconds")
	}
	return time.Duration(seconds * float64(time.Second)), nil
}

func parseNetworkDevices(text string) (map[string]NetworkInterface, error) {
	result := map[string]NetworkInterface{}
	for _, line := range strings.Split(text, "\n") {
		left, right, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		name := strings.TrimSpace(left)
		fields := strings.Fields(right)
		if name == "" || len(fields) < 9 {
			continue
		}
		receive, errReceive := strconv.ParseUint(fields[0], 10, 64)
		transmit, errTransmit := strconv.ParseUint(fields[8], 10, 64)
		if errReceive != nil || errTransmit != nil {
			return nil, fmt.Errorf("parse network counters for %s", name)
		}
		result[name] = NetworkInterface{Name: name, ReceiveBytes: receive, TransmitBytes: transmit}
	}
	if len(result) == 0 {
		return nil, errors.New("network interfaces not found")
	}
	return result, nil
}

func applyNetworkRates(current, previous map[string]NetworkInterface, elapsed time.Duration) {
	seconds := elapsed.Seconds()
	if seconds <= 0 {
		return
	}
	for name, value := range current {
		old, ok := previous[name]
		if !ok || value.ReceiveBytes < old.ReceiveBytes || value.TransmitBytes < old.TransmitBytes {
			continue
		}
		value.ReceiveRate = float64(value.ReceiveBytes-old.ReceiveBytes) / seconds
		value.TransmitRate = float64(value.TransmitBytes-old.TransmitBytes) / seconds
		current[name] = value
	}
}

func parseKeyValues(text string) map[string]string {
	result := map[string]string{}
	for _, line := range strings.Split(text, "\n") {
		key, value, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(key) == "" {
			continue
		}
		result[strings.TrimSpace(key)] = strings.Trim(strings.TrimSpace(value), "\"")
	}
	return result
}

func parseCPUModel(text string) string {
	for _, line := range strings.Split(text, "\n") {
		key, value, ok := strings.Cut(line, ":")
		if ok && (strings.TrimSpace(key) == "model name" || strings.TrimSpace(key) == "Hardware") {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
