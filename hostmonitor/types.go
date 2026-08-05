package hostmonitor

import "time"

type SessionID string

type Phase string

const (
	PhaseNew      Phase = "new"
	PhaseRunning  Phase = "running"
	PhaseDegraded Phase = "degraded"
	PhaseStopped  Phase = "stopped"
)

type HostIdentity struct {
	OSName       string
	OSVersion    string
	Kernel       string
	Architecture string
	Hostname     string
	CPUModel     string
	LogicalCPUs  int
}

type CPUStats struct {
	User, Nice, System, Idle, IOWait, IRQ, SoftIRQ, Steal uint64
	UsagePercent                                          float64
}

func (s CPUStats) TotalTicks() uint64 {
	return s.User + s.Nice + s.System + s.Idle + s.IOWait + s.IRQ + s.SoftIRQ + s.Steal
}

func (s CPUStats) IdleTicks() uint64 { return s.Idle + s.IOWait }

type MemoryStats struct {
	TotalBytes     uint64
	AvailableBytes uint64
	UsedBytes      uint64
	SwapTotalBytes uint64
	SwapFreeBytes  uint64
	SwapUsedBytes  uint64
}

type LoadStats struct {
	One, Five, Fifteen float64
}

type NetworkInterface struct {
	Name          string
	ReceiveBytes  uint64
	TransmitBytes uint64
	ReceiveRate   float64
	TransmitRate  float64
}

type HostSnapshot struct {
	Revision    uint64
	CollectedAt time.Time
	Identity    HostIdentity
	CPU         CPUStats
	Memory      MemoryStats
	Load        LoadStats
	Uptime      time.Duration
	Network     map[string]NetworkInterface
}

type MonitorState struct {
	Phase       Phase
	StartedAt   time.Time
	LastSuccess time.Time
	LastError   string
	Failures    uint32
}

type Update struct {
	SessionID SessionID
	State     MonitorState
	Snapshot  HostSnapshot
}

func cloneSnapshot(in HostSnapshot) HostSnapshot {
	out := in
	if in.Network != nil {
		out.Network = make(map[string]NetworkInterface, len(in.Network))
		for key, value := range in.Network {
			out.Network[key] = value
		}
	}
	return out
}
