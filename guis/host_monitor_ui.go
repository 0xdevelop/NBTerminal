package guis

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/0xdevelop/NBTerminal/hostmonitor"
	"github.com/0xdevelop/fltk2go/fltk_bridge"
	"github.com/0xdevelop/fltk2go/uikit"
	"github.com/0xdevelop/fltk2go/uikit/progress"
)

func (a *finalShellApp) buildMonitorSidebar(parent *uikit.UIGroup, margin, width int) {
	if a == nil || parent == nil {
		return
	}
	panel := uikit.NewUIGroup(rect(margin, 72, width, 786))
	panel.SetBackgroundColor(uint(tokenColor(modernTheme.card)))
	panel.SetAutomationID("monitor.sidebar")
	panel.Raw().Resizable(panel.Raw())

	a.monitorTitle = sectionTitle(margin+18, 86, width-36, 24, tr("monitor.title"))
	styleDynamicLabel(a.monitorTitle)
	a.monitorTitle.View().SetAutomationID("monitor.host")
	panel.AddSubview(a.monitorTitle)
	a.monitorStatus = mutedLabel(margin+18, 116, width-36, 22, tr("monitor.starting"))
	styleDynamicLabel(a.monitorStatus)
	a.monitorStatus.View().SetAutomationID("monitor.status")
	panel.AddSubview(a.monitorStatus)

	panel.AddSubview(sectionTitle(margin+18, 158, width-36, 22, tr("monitor.overview")))
	a.monitorUptime = mutedLabel(margin+18, 190, width-36, 20, trf("monitor.uptime", "—"))
	styleDynamicLabel(a.monitorUptime)
	a.monitorUptime.View().SetAutomationID("monitor.uptime")
	panel.AddSubview(a.monitorUptime)
	a.monitorLoad = mutedLabel(margin+18, 216, width-36, 20, trf("monitor.load", "—"))
	styleDynamicLabel(a.monitorLoad)
	a.monitorLoad.View().SetAutomationID("monitor.load")
	panel.AddSubview(a.monitorLoad)

	a.monitorCPU = label(margin+18, 260, width-36, 22, trf("monitor.cpu", 0.0))
	styleDynamicLabel(a.monitorCPU)
	a.monitorCPU.View().SetAutomationID("monitor.cpu")
	panel.AddSubview(a.monitorCPU)
	progressStyle := progress.DefaultProgressStyle()
	progressStyle.Color = uint(tokenColor(modernTheme.elevated))
	progressStyle.SelectionColor = uint(tokenColor(modernTheme.primary))
	a.monitorCPUBar = progress.NewUIProgressWithOptions(rect(margin+18, 288, width-36, 10), progressStyle)
	a.monitorCPUBar.View().SetAutomationID("monitor.cpu_progress")
	panel.AddSubview(a.monitorCPUBar)

	a.monitorMemory = label(margin+18, 320, width-36, 22, trf("monitor.memory", "—", "—", 0.0))
	styleDynamicLabel(a.monitorMemory)
	a.monitorMemory.View().SetAutomationID("monitor.memory")
	panel.AddSubview(a.monitorMemory)
	a.monitorMemBar = progress.NewUIProgressWithOptions(rect(margin+18, 348, width-36, 10), progressStyle)
	a.monitorMemBar.View().SetAutomationID("monitor.memory_progress")
	panel.AddSubview(a.monitorMemBar)

	panel.AddSubview(sectionTitle(margin+18, 396, width-36, 22, tr("monitor.network_title")))
	a.monitorNetwork = mutedLabel(margin+18, 428, width-36, 52, tr("monitor.network_waiting"))
	styleDynamicLabel(a.monitorNetwork)
	a.monitorNetwork.View().SetAutomationID("monitor.network")
	panel.AddSubview(a.monitorNetwork)

	panel.AddSubview(mutedLabel(margin+18, 750, width-36, 42, tr("monitor.details_hint")))
	a.monitorPanel = panel
	parent.AddSubview(panel)
	panel.Raw().Hide()
}

func (a *finalShellApp) startMonitorForSession(state terminalTabState) {
	if a == nil || state.ID == "" || state.Profile.Type != connectionTypeLocal {
		return
	}
	monitor := hostmonitor.NewSession(hostmonitor.SessionID(state.ID), hostmonitor.NewLocalLinuxSource(), hostmonitor.SamplingPolicy{Interval: time.Second})
	a.monitorMu.Lock()
	if a.monitors == nil {
		a.monitors = make(map[string]*hostmonitor.Session)
	}
	if existing := a.monitors[state.ID]; existing != nil {
		a.monitorMu.Unlock()
		return
	}
	a.monitors[state.ID] = monitor
	a.monitorMu.Unlock()
	if err := monitor.Start(context.Background()); err != nil {
		a.monitorMu.Lock()
		delete(a.monitors, state.ID)
		a.monitorMu.Unlock()
		return
	}
	go func(runtimeID string, session *hostmonitor.Session) {
		for update := range session.Updates() {
			current := update
			fltk_bridge.Awake(func() {
				if a.monitorIsCurrent(runtimeID, session) && a.activeSessionID() == runtimeID {
					a.applyMonitorUpdate(current)
				}
			})
		}
	}(state.ID, monitor)
}

func (a *finalShellApp) monitorIsCurrent(runtimeID string, monitor *hostmonitor.Session) bool {
	a.monitorMu.RLock()
	defer a.monitorMu.RUnlock()
	return a.monitors[runtimeID] == monitor
}

func (a *finalShellApp) stopMonitorForSession(runtimeID string) {
	if a == nil || runtimeID == "" {
		return
	}
	a.monitorMu.Lock()
	monitor := a.monitors[runtimeID]
	delete(a.monitors, runtimeID)
	a.monitorMu.Unlock()
	if monitor != nil {
		monitor.Stop()
		monitor.Wait()
	}
}

func (a *finalShellApp) stopAllMonitors() {
	if a == nil {
		return
	}
	a.monitorMu.Lock()
	monitors := make([]*hostmonitor.Session, 0, len(a.monitors))
	for id, monitor := range a.monitors {
		if monitor != nil {
			monitors = append(monitors, monitor)
		}
		delete(a.monitors, id)
	}
	a.monitorMu.Unlock()
	for _, monitor := range monitors {
		monitor.Stop()
	}
	for _, monitor := range monitors {
		monitor.Wait()
	}
}

func (a *finalShellApp) renderMonitorSidebar(state terminalTabState, ok bool) {
	if a == nil || a.monitorPanel == nil || a.monitorPanel.Raw() == nil {
		return
	}
	if !ok {
		if a.quickPanel != nil && a.quickPanel.Raw() != nil {
			a.quickPanel.Raw().Show()
		}
		a.monitorPanel.Raw().Hide()
		return
	}
	if a.quickPanel != nil && a.quickPanel.Raw() != nil {
		a.quickPanel.Raw().Hide()
	}
	a.monitorPanel.Raw().Show()
	if a.monitorTitle != nil {
		a.monitorTitle.SetText(state.Profile.Name)
	}
	if state.Profile.Type != connectionTypeLocal {
		a.resetMonitorValues(tr("monitor.ssh_pending"))
		return
	}
	a.monitorMu.RLock()
	monitor := a.monitors[state.ID]
	a.monitorMu.RUnlock()
	if monitor == nil {
		a.resetMonitorValues(tr("monitor.starting"))
		return
	}
	snapshot := monitor.Snapshot()
	if snapshot.Revision == 0 {
		a.resetMonitorValues(tr("monitor.starting"))
		return
	}
	a.applyMonitorUpdate(hostmonitor.Update{SessionID: hostmonitor.SessionID(state.ID), State: monitor.State(), Snapshot: snapshot})
}

func (a *finalShellApp) resetMonitorValues(status string) {
	if a.monitorStatus != nil {
		a.monitorStatus.SetText(status)
	}
	if a.monitorUptime != nil {
		a.monitorUptime.SetText(trf("monitor.uptime", "—"))
	}
	if a.monitorLoad != nil {
		a.monitorLoad.SetText(trf("monitor.load", "—"))
	}
	if a.monitorCPU != nil {
		a.monitorCPU.SetText(trf("monitor.cpu", 0.0))
	}
	if a.monitorMemory != nil {
		a.monitorMemory.SetText(trf("monitor.memory", "—", "—", 0.0))
	}
	if a.monitorNetwork != nil {
		a.monitorNetwork.SetText(tr("monitor.network_waiting"))
	}
	if a.monitorCPUBar != nil {
		a.monitorCPUBar.SetProgress(0)
	}
	if a.monitorMemBar != nil {
		a.monitorMemBar.SetProgress(0)
	}
}

func (a *finalShellApp) applyMonitorUpdate(update hostmonitor.Update) {
	snapshot := update.Snapshot
	if a.monitorStatus != nil {
		status := tr("monitor.online")
		if update.State.Phase == hostmonitor.PhaseDegraded {
			status = tr("monitor.degraded")
		}
		a.monitorStatus.SetText(status)
	}
	if a.monitorUptime != nil {
		a.monitorUptime.SetText(trf("monitor.uptime", formatMonitorUptime(snapshot.Uptime)))
	}
	if a.monitorLoad != nil {
		a.monitorLoad.SetText(trf("monitor.load", fmt.Sprintf("%.2f  %.2f  %.2f", snapshot.Load.One, snapshot.Load.Five, snapshot.Load.Fifteen)))
	}
	cpuRatio := clampRatio(snapshot.CPU.UsagePercent / 100)
	if a.monitorCPU != nil {
		a.monitorCPU.SetText(trf("monitor.cpu", snapshot.CPU.UsagePercent))
	}
	if a.monitorCPUBar != nil {
		a.monitorCPUBar.SetProgress(cpuRatio)
	}
	memoryRatio := 0.0
	if snapshot.Memory.TotalBytes > 0 {
		memoryRatio = float64(snapshot.Memory.UsedBytes) / float64(snapshot.Memory.TotalBytes)
	}
	if a.monitorMemory != nil {
		a.monitorMemory.SetText(trf("monitor.memory", formatMonitorBytes(snapshot.Memory.UsedBytes), formatMonitorBytes(snapshot.Memory.TotalBytes), memoryRatio*100))
	}
	if a.monitorMemBar != nil {
		a.monitorMemBar.SetProgress(clampRatio(memoryRatio))
	}
	if a.monitorNetwork != nil {
		if network, ok := primaryMonitorNetwork(snapshot.Network); ok {
			a.monitorNetwork.SetText(trf("monitor.network", network.Name, formatMonitorRate(network.ReceiveRate), formatMonitorRate(network.TransmitRate)))
		} else {
			a.monitorNetwork.SetText(tr("monitor.network_waiting"))
		}
	}
}

func clampRatio(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func primaryMonitorNetwork(network map[string]hostmonitor.NetworkInterface) (hostmonitor.NetworkInterface, bool) {
	if len(network) == 0 {
		return hostmonitor.NetworkInterface{}, false
	}
	rows := make([]hostmonitor.NetworkInterface, 0, len(network))
	for _, item := range network {
		rows = append(rows, item)
	}
	sort.Slice(rows, func(i, j int) bool {
		leftLoopback, rightLoopback := rows[i].Name == "lo", rows[j].Name == "lo"
		if leftLoopback != rightLoopback {
			return !leftLoopback
		}
		left := rows[i].ReceiveRate + rows[i].TransmitRate
		right := rows[j].ReceiveRate + rows[j].TransmitRate
		if left != right {
			return left > right
		}
		return rows[i].Name < rows[j].Name
	})
	return rows[0], true
}

func formatMonitorBytes(value uint64) string {
	const unit = 1024
	if value < unit {
		return fmt.Sprintf("%d B", value)
	}
	div, exp := uint64(unit), 0
	for n := value / unit; n >= unit && exp < 4; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(value)/float64(div), "KMGTPE"[exp])
}

func formatMonitorRate(value float64) string {
	if value < 0 {
		value = 0
	}
	return formatMonitorBytes(uint64(value)) + "/s"
}

func formatMonitorUptime(value time.Duration) string {
	if value < 0 {
		value = 0
	}
	days := int(value / (24 * time.Hour))
	value %= 24 * time.Hour
	hours := int(value / time.Hour)
	minutes := int((value % time.Hour) / time.Minute)
	if days > 0 {
		return fmt.Sprintf("%dd %02dh %02dm", days, hours, minutes)
	}
	return fmt.Sprintf("%02dh %02dm", hours, minutes)
}
