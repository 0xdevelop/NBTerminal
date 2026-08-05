package hostmonitor

import (
	"context"
	"errors"
	"sync"
	"time"
)

type SamplingPolicy struct {
	Interval time.Duration
}

func (p SamplingPolicy) normalized() SamplingPolicy {
	if p.Interval <= 0 {
		p.Interval = time.Second
	}
	return p
}

type Session struct {
	id     SessionID
	source Source
	policy SamplingPolicy

	mu       sync.RWMutex
	state    MonitorState
	snapshot HostSnapshot
	started  bool
	cancel   context.CancelFunc
	done     chan struct{}
	updates  chan Update
}

func NewSession(id SessionID, source Source, policy SamplingPolicy) *Session {
	return &Session{id: id, source: source, policy: policy.normalized(), done: make(chan struct{}), updates: make(chan Update, 1), state: MonitorState{Phase: PhaseNew}}
}

func (s *Session) Start(parent context.Context) error {
	if s == nil || s.source == nil || s.id == "" {
		return errors.New("invalid host monitor session")
	}
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return errors.New("host monitor session already started")
	}
	s.started = true
	ctx, cancel := context.WithCancel(parent)
	s.cancel = cancel
	s.state.Phase = PhaseRunning
	s.state.StartedAt = time.Now()
	s.mu.Unlock()
	go s.run(ctx)
	return nil
}

func (s *Session) run(ctx context.Context) {
	defer close(s.done)
	defer close(s.updates)
	defer s.source.Close()
	s.collect(ctx)
	ticker := time.NewTicker(s.policy.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			s.mu.Lock()
			s.state.Phase = PhaseStopped
			s.mu.Unlock()
			return
		case <-ticker.C:
			s.collect(ctx)
		}
	}
}

func (s *Session) collect(ctx context.Context) {
	s.mu.RLock()
	previous := cloneSnapshot(s.snapshot)
	hasPrevious := previous.Revision > 0
	s.mu.RUnlock()
	var previousPtr *HostSnapshot
	if hasPrevious {
		previousPtr = &previous
	}
	next, err := s.source.Sample(ctx, previousPtr)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		s.mu.Lock()
		s.state.Phase = PhaseDegraded
		s.state.LastError = err.Error()
		s.state.Failures++
		update := Update{SessionID: s.id, State: s.state, Snapshot: cloneSnapshot(s.snapshot)}
		s.mu.Unlock()
		s.publish(update)
		return
	}
	s.mu.Lock()
	next.Revision = s.snapshot.Revision + 1
	s.snapshot = cloneSnapshot(next)
	s.state.Phase = PhaseRunning
	s.state.LastSuccess = next.CollectedAt
	s.state.LastError = ""
	s.state.Failures = 0
	update := Update{SessionID: s.id, State: s.state, Snapshot: cloneSnapshot(s.snapshot)}
	s.mu.Unlock()
	s.publish(update)
}

func (s *Session) publish(update Update) {
	select {
	case s.updates <- update:
		return
	default:
	}
	select {
	case <-s.updates:
	default:
	}
	select {
	case s.updates <- update:
	default:
	}
}

func (s *Session) Stop() {
	if s == nil {
		return
	}
	s.mu.RLock()
	cancel := s.cancel
	s.mu.RUnlock()
	if cancel != nil {
		cancel()
	}
}

func (s *Session) Wait() {
	if s == nil {
		return
	}
	s.mu.RLock()
	started := s.started
	done := s.done
	s.mu.RUnlock()
	if started {
		<-done
	}
}

func (s *Session) Updates() <-chan Update {
	if s == nil {
		return nil
	}
	return s.updates
}

func (s *Session) Snapshot() HostSnapshot {
	if s == nil {
		return HostSnapshot{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneSnapshot(s.snapshot)
}

func (s *Session) State() MonitorState {
	if s == nil {
		return MonitorState{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state
}
