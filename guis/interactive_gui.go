package guis

import (
	"context"
	"errors"
	"strings"

	"github.com/0xdevelop/NBTerminal/terminal"
	"github.com/0xdevelop/fltk2go/fltk_bridge"
	"github.com/0xdevelop/fltk2go/uikit"
	"github.com/george012/gtbox/gtbox_log"
)

const (
	defaultTerminalColumns = 80
	defaultTerminalRows    = 24
)

func transportTerminalSize(size uikit.TerminalSize) terminal.TerminalSize {
	columns, rows := size.Columns, size.Rows
	if columns <= 0 {
		columns = defaultTerminalColumns
	}
	if rows <= 0 {
		rows = defaultTerminalRows
	}
	if columns > 65535 {
		columns = 65535
	}
	if rows > 65535 {
		rows = 65535
	}
	return terminal.TerminalSize{Columns: uint16(columns), Rows: uint16(rows)}
}

func (a *finalShellApp) activeTerminalSize() terminal.TerminalSize {
	if a != nil && a.output != nil {
		return transportTerminalSize(a.output.Size())
	}
	return terminal.TerminalSize{Columns: defaultTerminalColumns, Rows: defaultTerminalRows}
}

// startInteractiveLocalSession joins the native terminal renderer and the local
// PTY transport at the product boundary. The reusable widget still owns VT/input
// translation, while the transport remains owned by the opaque runtime tab ID.
func (a *finalShellApp) startInteractiveLocalSession(state terminalTabState) error {
	if a == nil || state.Profile.Type != connectionTypeLocal || strings.TrimSpace(state.ID) == "" {
		return nil
	}
	if a.interactive == nil {
		a.interactive = newInteractiveRuntimeRegistry()
	}
	if a.interactive.Has(state.ID) {
		return nil
	}
	conn, err := profileToConnection(state.Profile)
	if err != nil {
		return err
	}
	transport := terminal.NewLocalPTYSession(conn)
	if err := a.interactive.Start(context.Background(), state.ID, transport, a.activeTerminalSize()); err != nil {
		return err
	}

	go func(sessionID string, session terminal.InteractiveSession) {
		for chunk := range session.Output() {
			data := append([]byte(nil), chunk...)
			fltk_bridge.Awake(func() { a.appendSessionOutput(sessionID, string(data)) })
		}
	}(state.ID, transport)
	go func(sessionID string, session terminal.InteractiveSession) {
		err := session.Wait()
		if !a.interactive.Release(sessionID, session) {
			return
		}
		fltk_bridge.Awake(func() {
			if err != nil {
				a.appendSessionOutput(sessionID, trf("output.shell_exited_error", err.Error()))
			} else {
				a.appendSessionOutput(sessionID, tr("output.shell_exited"))
			}
			if a.activeSessionID() == sessionID {
				a.setStatus(tr("status.shell_exited"))
				a.updateCommandControls()
			}
		})
	}(state.ID, transport)
	return nil
}

func (a *finalShellApp) writeActiveTerminalInput(data []byte) {
	if a == nil || len(data) == 0 || a.interactive == nil {
		return
	}
	sessionID := a.activeSessionID()
	if sessionID == "" || !a.interactive.Has(sessionID) {
		return
	}
	if err := a.interactive.WriteInput(sessionID, data); err != nil {
		gtbox_log.LogErrorf("write interactive terminal input failed: %s", err.Error())
		a.setStatus(tr("status.shell_input_failed"))
	}
}

func (a *finalShellApp) terminalViewResized(size uikit.TerminalSize) {
	if a == nil || size.Columns <= 0 || size.Rows <= 0 {
		return
	}
	sessionID := a.activeSessionID()
	if a.interactive != nil && a.interactive.Has(sessionID) {
		a.terminalColumns = size.Columns
		if err := a.interactive.Resize(sessionID, transportTerminalSize(size)); err != nil && !errors.Is(err, errInteractiveRuntimeNotFound) {
			gtbox_log.LogErrorf("resize interactive terminal failed: %s", err.Error())
		}
		return
	}
	a.reflowTerminalOutput(size)
}

func (a *finalShellApp) runInteractiveCommand(command string) bool {
	if a == nil || a.sessions == nil {
		return false
	}
	state, ok := a.sessions.Active()
	if !ok || state.Profile.Type != connectionTypeLocal {
		return false
	}
	if err := a.startInteractiveLocalSession(state); err != nil {
		a.appendSessionOutput(state.ID, trf("output.shell_start_failed", err.Error()))
		a.setStatus(tr("status.shell_start_failed"))
		a.showTopNotice(tr("notice.shell_start_failed.title"), err.Error(), true)
		return true
	}
	if err := a.interactive.WriteInput(state.ID, []byte(command+"\r")); err != nil {
		a.appendSessionOutput(state.ID, trf("output.shell_input_failed", err.Error()))
		a.setStatus(tr("status.shell_input_failed"))
		return true
	}
	if a.cmdInput != nil {
		a.cmdInput.SetText("")
		a.sessions.SetActiveDraft("")
	}
	if a.output != nil && a.output.Raw() != nil {
		a.output.Raw().TakeFocus()
	}
	return true
}

func (a *finalShellApp) configureActiveTerminalMode(state terminalTabState, ok bool) {
	if a == nil || a.output == nil || a.output.Raw() == nil {
		return
	}
	interactive := ok && a.interactive != nil && a.interactive.Has(state.ID)
	if interactive {
		a.output.Raw().SetHorizontalScrollbar(fltk_bridge.TerminalScrollbarOff)
	} else {
		a.output.Raw().SetHorizontalScrollbar(fltk_bridge.TerminalScrollbarAuto)
	}
}

func (a *finalShellApp) interruptInteractiveSession(sessionID string) bool {
	if a == nil || a.interactive == nil || !a.interactive.Has(sessionID) {
		return false
	}
	if err := a.interactive.Interrupt(sessionID); err != nil {
		a.appendSessionOutput(sessionID, trf("output.shell_input_failed", err.Error()))
		a.setStatus(tr("status.shell_input_failed"))
		return true
	}
	a.setStatus(tr("status.interrupt_sent"))
	return true
}
