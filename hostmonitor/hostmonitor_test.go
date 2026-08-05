package hostmonitor

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"testing"
	"time"
)

func TestLinuxParsersAndRates(t *testing.T) {
	cpu, err := parseCPUStat("cpu  100 2 30 400 10 3 4 1\ncpu0 1 2 3 4")
	if err != nil || cpu.TotalTicks() != 550 {
		t.Fatalf("cpu parse: %#v err=%v", cpu, err)
	}
	usage := cpuUsage(cpu, CPUStats{User: 120, Nice: 2, System: 40, Idle: 450, IOWait: 10, IRQ: 3, SoftIRQ: 4, Steal: 1})
	if usage <= 0 || usage >= 100 {
		t.Fatalf("cpu usage out of range: %f", usage)
	}
	memory, err := parseMemInfo("MemTotal: 1024 kB\nMemAvailable: 256 kB\nSwapTotal: 128 kB\nSwapFree: 32 kB\n")
	if err != nil || memory.UsedBytes != 768*1024 || memory.SwapUsedBytes != 96*1024 {
		t.Fatalf("memory parse: %#v err=%v", memory, err)
	}
	load, err := parseLoadAverage("0.10 0.20 0.30 1/100 42")
	if err != nil || load.Five != 0.20 {
		t.Fatalf("load parse: %#v err=%v", load, err)
	}
	uptime, err := parseUptime("12.5 1.0")
	if err != nil || uptime != 12500*time.Millisecond {
		t.Fatalf("uptime parse: %s err=%v", uptime, err)
	}
	network, err := parseNetworkDevices("Inter-| Receive | Transmit\n eth0: 100 0 0 0 0 0 0 0 200 0 0 0 0 0 0 0\n")
	if err != nil {
		t.Fatal(err)
	}
	applyNetworkRates(network, map[string]NetworkInterface{"eth0": {Name: "eth0", ReceiveBytes: 40, TransmitBytes: 80}}, 2*time.Second)
	if network["eth0"].ReceiveRate != 30 || network["eth0"].TransmitRate != 60 {
		t.Fatalf("network rates: %#v", network["eth0"])
	}
}

func TestLocalLinuxSourceCollectsRealKernelSnapshot(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux integration test")
	}
	source := NewLocalLinuxSource()
	first, err := source.Sample(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if first.Identity.Hostname == "" || first.Identity.LogicalCPUs <= 0 || first.CPU.TotalTicks() == 0 || first.Memory.TotalBytes == 0 || len(first.Network) == 0 || first.Uptime <= 0 {
		t.Fatalf("incomplete real host snapshot: %#v", first)
	}
	time.Sleep(20 * time.Millisecond)
	second, err := source.Sample(context.Background(), &first)
	if err != nil {
		t.Fatal(err)
	}
	if second.CollectedAt.Before(first.CollectedAt) || second.CPU.UsagePercent < 0 || second.CPU.UsagePercent > 100 {
		t.Fatalf("invalid second snapshot: %#v", second)
	}
}

type fakeSource struct {
	mu      sync.Mutex
	calls   int
	closed  bool
	failure bool
}

func (s *fakeSource) Sample(ctx context.Context, previous *HostSnapshot) (HostSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failure {
		return HostSnapshot{}, errors.New("sample failed")
	}
	s.calls++
	return HostSnapshot{CollectedAt: time.Now(), CPU: CPUStats{User: uint64(s.calls), Idle: 10}, Network: map[string]NetworkInterface{}}, nil
}

func (s *fakeSource) Close() error {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	return nil
}

func TestSessionPublishesRevisionsAndStopsCleanly(t *testing.T) {
	source := &fakeSource{}
	session := NewSession("runtime-1", source, SamplingPolicy{Interval: 5 * time.Millisecond})
	if err := session.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := session.Start(context.Background()); err == nil {
		t.Fatal("second start unexpectedly succeeded")
	}
	deadline := time.After(time.Second)
	var update Update
	for update.Snapshot.Revision < 2 {
		select {
		case update = <-session.Updates():
		case <-deadline:
			t.Fatal("timed out waiting for monitor revisions")
		}
	}
	if update.SessionID != "runtime-1" || session.Snapshot().Revision < 2 {
		t.Fatalf("invalid monitor update: %#v", update)
	}
	session.Stop()
	session.Stop()
	session.Wait()
	if session.State().Phase != PhaseStopped {
		t.Fatalf("session did not stop: %#v", session.State())
	}
	source.mu.Lock()
	closed := source.closed
	source.mu.Unlock()
	if !closed {
		t.Fatal("source was not closed")
	}
}

func TestSessionPublishesDegradedStateWithoutDiscardingLastSnapshot(t *testing.T) {
	source := &fakeSource{}
	session := NewSession("runtime-degraded", source, SamplingPolicy{Interval: 5 * time.Millisecond})
	if err := session.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer func() {
		session.Stop()
		session.Wait()
	}()

	deadline := time.After(time.Second)
	for session.Snapshot().Revision == 0 {
		select {
		case <-session.Updates():
		case <-deadline:
			t.Fatal("timed out waiting for initial snapshot")
		}
	}
	lastRevision := session.Snapshot().Revision
	source.mu.Lock()
	source.failure = true
	source.mu.Unlock()

	for {
		select {
		case update := <-session.Updates():
			if update.State.Phase != PhaseDegraded {
				continue
			}
			if update.Snapshot.Revision != lastRevision || update.State.Failures == 0 || update.State.LastError == "" {
				t.Fatalf("invalid degraded update: %#v", update)
			}
			return
		case <-deadline:
			t.Fatal("timed out waiting for degraded update")
		}
	}
}
