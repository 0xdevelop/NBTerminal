package guis

import (
	"context"
	"errors"
	"strings"
	"sync"

	"github.com/0xdevelop/NBTerminal/terminal"
)

var (
	errInteractiveRuntimeExists   = errors.New("interactive runtime already exists")
	errInteractiveRuntimeNotFound = errors.New("interactive runtime not found")
)

type interactiveRuntime struct {
	session terminal.InteractiveSession
	cancel  context.CancelFunc
}

// interactiveRuntimeRegistry gives each terminal tab exclusive ownership of one
// long-lived transport. Routing is always by opaque runtime ID, never by the
// currently selected profile or tab index, so background sessions cannot consume
// another tab's keyboard, resize, interrupt, or close events.
type interactiveRuntimeRegistry struct {
	mu       sync.RWMutex
	runtimes map[string]interactiveRuntime
}

func newInteractiveRuntimeRegistry() *interactiveRuntimeRegistry {
	return &interactiveRuntimeRegistry{runtimes: make(map[string]interactiveRuntime)}
}

func (r *interactiveRuntimeRegistry) Start(parent context.Context, id string, session terminal.InteractiveSession, size terminal.TerminalSize) error {
	id = strings.TrimSpace(id)
	if parent == nil {
		parent = context.Background()
	}
	if r == nil || id == "" || session == nil {
		return errInteractiveRuntimeNotFound
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.runtimes[id]; exists {
		return errInteractiveRuntimeExists
	}
	ctx, cancel := context.WithCancel(parent)
	if err := session.Start(ctx, size); err != nil {
		cancel()
		return err
	}
	r.runtimes[id] = interactiveRuntime{session: session, cancel: cancel}
	return nil
}

// Replace starts a successor transport before atomically publishing it under
// the same runtime ID. A failed replacement leaves the current terminal usable;
// once the swap succeeds, the superseded transport is cancelled and closed.
func (r *interactiveRuntimeRegistry) Replace(parent context.Context, id string, session terminal.InteractiveSession, size terminal.TerminalSize) error {
	id = strings.TrimSpace(id)
	if parent == nil {
		parent = context.Background()
	}
	if r == nil || id == "" || session == nil {
		return errInteractiveRuntimeNotFound
	}
	r.mu.RLock()
	previous, exists := r.runtimes[id]
	r.mu.RUnlock()
	if !exists {
		return errInteractiveRuntimeNotFound
	}

	ctx, cancel := context.WithCancel(parent)
	if err := session.Start(ctx, size); err != nil {
		cancel()
		return err
	}
	r.mu.Lock()
	current, exists := r.runtimes[id]
	if !exists || current.session != previous.session {
		r.mu.Unlock()
		cancel()
		_ = session.Close()
		return errInteractiveRuntimeExists
	}
	r.runtimes[id] = interactiveRuntime{session: session, cancel: cancel}
	r.mu.Unlock()

	previous.cancel()
	_ = previous.session.Close()
	return nil
}

func (r *interactiveRuntimeRegistry) Session(id string) (terminal.InteractiveSession, bool) {
	if r == nil {
		return nil, false
	}
	r.mu.RLock()
	runtime, ok := r.runtimes[strings.TrimSpace(id)]
	r.mu.RUnlock()
	return runtime.session, ok
}

func (r *interactiveRuntimeRegistry) Has(id string) bool {
	_, ok := r.Session(id)
	return ok
}

func (r *interactiveRuntimeRegistry) WriteInput(id string, data []byte) error {
	session, ok := r.Session(id)
	if !ok {
		return errInteractiveRuntimeNotFound
	}
	return session.WriteInput(data)
}

func (r *interactiveRuntimeRegistry) Resize(id string, size terminal.TerminalSize) error {
	session, ok := r.Session(id)
	if !ok {
		return errInteractiveRuntimeNotFound
	}
	return session.Resize(size)
}

func (r *interactiveRuntimeRegistry) Interrupt(id string) error {
	session, ok := r.Session(id)
	if !ok {
		return errInteractiveRuntimeNotFound
	}
	return session.Interrupt()
}

func (r *interactiveRuntimeRegistry) Close(id string) bool {
	if r == nil {
		return false
	}
	id = strings.TrimSpace(id)
	r.mu.Lock()
	runtime, ok := r.runtimes[id]
	if ok {
		delete(r.runtimes, id)
	}
	r.mu.Unlock()
	if !ok {
		return false
	}
	runtime.cancel()
	_ = runtime.session.Close()
	return true
}

// Release removes a transport only if it is still the same instance. This
// prevents a delayed Wait completion from deleting a newer runtime with the same
// ID during teardown/reconstruction.
func (r *interactiveRuntimeRegistry) Release(id string, session terminal.InteractiveSession) bool {
	if r == nil || session == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	runtime, ok := r.runtimes[strings.TrimSpace(id)]
	if !ok || runtime.session != session {
		return false
	}
	delete(r.runtimes, strings.TrimSpace(id))
	runtime.cancel()
	return true
}

func (r *interactiveRuntimeRegistry) CloseAll() {
	if r == nil {
		return
	}
	r.mu.Lock()
	runtimes := r.runtimes
	r.runtimes = make(map[string]interactiveRuntime)
	r.mu.Unlock()
	for _, runtime := range runtimes {
		runtime.cancel()
		_ = runtime.session.Close()
	}
}
