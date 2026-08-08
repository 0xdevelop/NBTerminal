package guis

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/0xdevelop/NBTerminal/terminal"
	"github.com/0xdevelop/fltk2go/fltk_bridge"
	"github.com/0xdevelop/fltk2go/uikit"
	"github.com/george012/gtbox/gtbox_log"
)

const (
	defaultTerminalColumns = 80
	defaultTerminalRows    = 24
)

type sshHostKeyPrompt struct {
	Title, Message, Cancel, Trust string
}

func unknownSSHHostKeyPrompt(fingerprint string) sshHostKeyPrompt {
	return sshHostKeyPrompt{
		Title:   tr("ssh.host_key.unknown_title"),
		Message: trf("ssh.host_key.unknown_message", fingerprint),
		Cancel:  tr("button.cancel"),
		Trust:   tr("ssh.host_key.trust"),
	}
}

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

// startInteractiveSession joins the native terminal renderer with a long-lived
// local or SSH PTY at the product boundary. The reusable widget owns VT/input
// translation, while the transport remains owned by the opaque runtime tab ID.
func (a *finalShellApp) startInteractiveSession(state terminalTabState) error {
	if a == nil || strings.TrimSpace(state.ID) == "" {
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
	var transport terminal.InteractiveSession
	switch state.Profile.Type {
	case connectionTypeLocal:
		transport = terminal.NewLocalPTYSession(conn)
	case connectionTypeSSH:
		if a.sshHostKeys == nil {
			return terminal.ErrSSHHostKeyVerifierRequired
		}
		conn.HostKeyCallback = a.sshHostKeys.Callback()
		transport = terminal.NewSSHPTYSession(conn)
	default:
		return nil
	}
	if err := a.interactive.Start(context.Background(), state.ID, transport, a.activeTerminalSize()); err != nil {
		var hostKeyErr *terminal.SSHHostKeyError
		if !errors.As(err, &hostKeyErr) {
			return err
		}
		if hostKeyErr.Kind() == terminal.SSHHostKeyChanged {
			uikit.Alert(tr("ssh.host_key.changed_title"), tr("ssh.host_key.changed_message"))
			return err
		}
		prompt := unknownSSHHostKeyPrompt(hostKeyErr.Fingerprint())
		if uikit.TitledChoice(prompt.Title, prompt.Message, prompt.Cancel, prompt.Trust) != 1 {
			return err
		}
		if err := a.sshHostKeys.Trust(hostKeyErr); err != nil {
			return err
		}
		if err := a.interactive.Start(context.Background(), state.ID, transport, a.activeTerminalSize()); err != nil {
			return err
		}
	}

	// PTYs may emit hundreds of chunks between FLTK frames. Coalesce them behind
	// one pending Awake callback and publish shell completion only after the final
	// bytes have reached the native VT parser. This preserves stream order without
	// flooding the GUI event queue during high-volume local or SSH output.
	outputBatcher := newGUIOutputBatcher(func(fn func()) {
		fltk_bridge.Awake(fn)
	}, func(text string) { a.appendSessionOutput(state.ID, text) })
	go drainInteractiveOutput(transport, outputBatcher, func(err error) {
		if !a.interactive.Release(state.ID, transport) {
			return
		}
		if err != nil {
			a.appendSessionOutput(state.ID, trf("output.session_shell_exited_error", err.Error()))
		} else {
			a.appendSessionOutput(state.ID, tr("output.session_shell_exited"))
		}
		if a.activeSessionID() == state.ID {
			a.setStatus(tr("status.session_shell_exited"))
			a.updateCommandControls()
		}
	})
	return nil
}

// drainInteractiveOutput keeps arbitrary PTY byte chunks ordered and waits for
// transport completion before enqueueing the GUI-visible exit transition. The
// batcher owns GUI-thread scheduling, so this function is transport-only and
// directly testable without a display server.
func drainInteractiveOutput(session terminal.InteractiveSession, batcher *guiOutputBatcher, onExit func(error)) {
	if session == nil || batcher == nil {
		return
	}
	for chunk := range session.Output() {
		if len(chunk) > 0 {
			batcher.Enqueue(string(chunk))
		}
	}
	err := session.Wait()
	batcher.AfterFlush(func() {
		if onExit != nil {
			onExit(err)
		}
	})
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
	if !ok || (state.Profile.Type != connectionTypeLocal && state.Profile.Type != connectionTypeSSH) {
		return false
	}
	if err := a.startInteractiveSession(state); err != nil {
		a.appendSessionOutput(state.ID, trf("output.session_shell_start_failed", err.Error()))
		a.setStatus(tr("status.session_shell_start_failed"))
		a.showTopNotice(tr("notice.session_shell_start_failed.title"), err.Error(), true)
		return true
	}
	if err := a.interactive.WriteInput(state.ID, []byte(command+"\r")); err != nil {
		a.appendSessionOutput(state.ID, trf("output.shell_input_failed", err.Error()))
		a.setStatus(tr("status.shell_input_failed"))
		return true
	}
	if a.history != nil {
		connectionType := terminal.ConnectionTypeSSH
		if state.Profile.Type == connectionTypeLocal {
			connectionType = terminal.ConnectionTypeLocal
		}
		if err := a.history.Append(terminal.HistoryEntry{
			Time:           time.Now().UTC(),
			ConnectionID:   state.Profile.ID,
			ConnectionName: state.Profile.Name,
			ConnectionType: connectionType,
			Command:        command,
			Interactive:    true,
		}); err != nil {
			gtbox_log.LogErrorf("persist interactive command history failed: %s", err.Error())
		}
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
	a.setStatus(tr("status.session_interrupt_sent"))
	return true
}
