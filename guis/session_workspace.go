package guis

import (
	"fmt"
	"strings"
)

type sessionStatus string

const (
	sessionIdle      sessionStatus = "idle"
	sessionRunning   sessionStatus = "running"
	sessionSucceeded sessionStatus = "succeeded"
	sessionFailed    sessionStatus = "failed"
	sessionStopped   sessionStatus = "stopped"
)

type terminalTabState struct {
	ID             string
	ProfileID      string
	InstanceNumber int
	Profile        connectionProfile
	CommandDraft   string
	Output         string
	Status         sessionStatus
	RunID          string
}

type sessionWorkspace struct {
	tabs             []terminalTabState
	activeIndex      int
	profileSelection string
	nextRuntimeID    uint64
}

func newSessionWorkspace() *sessionWorkspace {
	return &sessionWorkspace{activeIndex: -1}
}

func (w *sessionWorkspace) Tabs() []terminalTabState {
	if w == nil {
		return nil
	}
	return append([]terminalTabState(nil), w.tabs...)
}

func (w *sessionWorkspace) ActiveIndex() int {
	if w == nil {
		return -1
	}
	return w.activeIndex
}

func (w *sessionWorkspace) Active() (terminalTabState, bool) {
	if w == nil || w.activeIndex < 0 || w.activeIndex >= len(w.tabs) {
		return terminalTabState{}, false
	}
	return w.tabs[w.activeIndex], true
}

func (w *sessionWorkspace) Open(profile connectionProfile) (int, bool) {
	if w == nil || strings.TrimSpace(profile.ID) == "" {
		return -1, false
	}
	instanceNumber := 1
	for index := range w.tabs {
		if w.tabs[index].ProfileID == profile.ID && w.tabs[index].InstanceNumber >= instanceNumber {
			instanceNumber = w.tabs[index].InstanceNumber + 1
		}
	}
	w.nextRuntimeID++
	state := terminalTabState{
		ID:             fmt.Sprintf("runtime-%d", w.nextRuntimeID),
		ProfileID:      profile.ID,
		InstanceNumber: instanceNumber,
		Profile:        profile,
		Status:         sessionIdle,
		Output:         terminalWelcomeText(),
	}
	w.tabs = append(w.tabs, state)
	w.activeIndex = len(w.tabs) - 1
	return w.activeIndex, true
}

func (w *sessionWorkspace) Select(index int) bool {
	if w == nil || index < 0 || index >= len(w.tabs) {
		return false
	}
	w.activeIndex = index
	return true
}

func (w *sessionWorkspace) SetProfileSelection(profileID string) {
	if w != nil {
		w.profileSelection = strings.TrimSpace(profileID)
	}
}

func (w *sessionWorkspace) ProfileSelection() string {
	if w == nil {
		return ""
	}
	return w.profileSelection
}

func (w *sessionWorkspace) SetActiveDraft(draft string) bool {
	if w == nil || w.activeIndex < 0 || w.activeIndex >= len(w.tabs) {
		return false
	}
	w.tabs[w.activeIndex].CommandDraft = draft
	return true
}

func (w *sessionWorkspace) SetActiveOutput(output string) bool {
	if w == nil || w.activeIndex < 0 || w.activeIndex >= len(w.tabs) {
		return false
	}
	w.tabs[w.activeIndex].Output = output
	return true
}

func (w *sessionWorkspace) AppendActiveOutput(output string) bool {
	if w == nil || w.activeIndex < 0 || w.activeIndex >= len(w.tabs) {
		return false
	}
	w.tabs[w.activeIndex].Output += output
	return true
}

func (w *sessionWorkspace) AppendOutput(id, output string) bool {
	if w == nil {
		return false
	}
	for index := range w.tabs {
		if w.tabs[index].ID == id {
			w.tabs[index].Output += output
			return true
		}
	}
	return false
}

func (w *sessionWorkspace) BeginRun(runID string) bool {
	if w == nil || w.activeIndex < 0 || w.activeIndex >= len(w.tabs) || strings.TrimSpace(runID) == "" {
		return false
	}
	state := &w.tabs[w.activeIndex]
	if state.Status == sessionRunning {
		return false
	}
	state.Status = sessionRunning
	state.RunID = runID
	return true
}

func (w *sessionWorkspace) FinishRun(runID string, status sessionStatus) bool {
	if w == nil || strings.TrimSpace(runID) == "" {
		return false
	}
	for index := range w.tabs {
		state := &w.tabs[index]
		if state.RunID != runID || state.Status != sessionRunning {
			continue
		}
		state.Status = status
		state.RunID = ""
		return true
	}
	return false
}

func (w *sessionWorkspace) Close(index int) bool {
	if w == nil || index < 0 || index >= len(w.tabs) || w.tabs[index].Status == sessionRunning {
		return false
	}
	wasActive := index == w.activeIndex
	w.tabs = append(w.tabs[:index], w.tabs[index+1:]...)
	if len(w.tabs) == 0 {
		w.activeIndex = -1
		return true
	}
	if wasActive {
		if index >= len(w.tabs) {
			index = len(w.tabs) - 1
		}
		w.activeIndex = index
	} else if index < w.activeIndex {
		w.activeIndex--
	}
	return true
}
