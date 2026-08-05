package guis

import (
	"testing"
	"time"

	"github.com/0xdevelop/NBTerminal/hostmonitor"
)

func TestMonitorPresentationHelpers(t *testing.T) {
	if got := clampRatio(-1); got != 0 {
		t.Fatalf("negative ratio = %f", got)
	}
	if got := clampRatio(1.5); got != 1 {
		t.Fatalf("overflow ratio = %f", got)
	}
	if got := formatMonitorBytes(1536); got != "1.5 KiB" {
		t.Fatalf("formatted bytes = %q", got)
	}
	if got := formatMonitorUptime(49*time.Hour + 3*time.Minute); got != "2d 01h 03m" {
		t.Fatalf("formatted uptime = %q", got)
	}
}

func TestPrimaryMonitorNetworkPrefersNonLoopbackThenTraffic(t *testing.T) {
	network, ok := primaryMonitorNetwork(map[string]hostmonitor.NetworkInterface{
		"lo":   {Name: "lo", ReceiveRate: 9999, TransmitRate: 9999},
		"eth0": {Name: "eth0", ReceiveRate: 10, TransmitRate: 20},
		"eth1": {Name: "eth1", ReceiveRate: 30, TransmitRate: 40},
	})
	if !ok || network.Name != "eth1" {
		t.Fatalf("primary interface = %#v ok=%t", network, ok)
	}
	if _, ok := primaryMonitorNetwork(nil); ok {
		t.Fatal("empty network unexpectedly returned an interface")
	}
}
