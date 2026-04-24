// SPDX-License-Identifier: Apache-2.0

package debug

import (
	"errors"
	"fmt"
	"sort"
	"time"
)

// TraceOptions configures a trace run.
type TraceOptions struct {
	MaxSteps    int
	Deep        bool
	PollTimeout time.Duration
}

// TraceStep records one step in the trace.
type TraceStep struct {
	Number        int
	ObjectID      string
	ObjectName    string
	MicroflowName string
	Diffs         []VarDiff
	FrameChange   FrameChange
}

// FrameChange describes a microflow frame transition.
type FrameChange int

const (
	FrameNone    FrameChange = iota
	FrameEntered             // stepped into a sub-microflow
	FrameReturned            // returned to parent microflow
)

// TraceResult holds the complete trace output.
type TraceResult struct {
	DebugID        string
	MicroflowName  string
	Steps          []TraceStep
	Reason         string // "completed", "max steps reached", "breakpoint hit", "error"
	Error          error
}

// VarDiff describes a single variable change between two snapshots.
type VarDiff struct {
	Name   string
	Change string // "+", "~", "-"
	Old    string
	New    string
}

// Trace steps through a paused microflow, collecting variable diffs at each action.
// initialState provides the starting position (from breakpoint hit or start_session).
func Trace(client *Client, sessionToken, debugID string, initialState *PausedMicroflow, opts TraceOptions) *TraceResult {
	if opts.MaxSteps <= 0 {
		opts.MaxSteps = 20
	}
	if opts.PollTimeout <= 0 {
		opts.PollTimeout = 5 * time.Second
	}

	result := &TraceResult{DebugID: debugID}

	type frame struct {
		microflowName string
		snapshot      map[string]Variable
	}
	var stack []frame

	var prevSnapshot map[string]Variable
	var prevMicroflow string

	if initialState != nil {
		prevMicroflow = initialState.MicroflowName
		prevSnapshot = initialState.Variables
		result.MicroflowName = initialState.MicroflowName
	}

	var lastWasEnd bool

	for i := 1; i <= opts.MaxSteps; i++ {
		var err error
		if opts.Deep {
			err = client.StepInto(sessionToken, debugID)
		} else {
			err = client.StepOver(sessionToken, debugID)
		}
		if err != nil {
			var de *DebuggerError
			if errors.As(err, &de) && de.IsNotFound() {
				result.Reason = "completed"
				return result
			}
			if lastWasEnd {
				result.Reason = "completed"
				return result
			}
			result.Reason = "error"
			result.Error = fmt.Errorf("step %d: %w", i, err)
			return result
		}

		poll, err := client.PollEvents(sessionToken, opts.PollTimeout)
		if err != nil {
			if lastWasEnd {
				result.Reason = "completed"
				return result
			}
			result.Reason = "error"
			result.Error = fmt.Errorf("poll after step %d: %w", i, err)
			return result
		}

		var pm *PausedMicroflow
		for _, ev := range poll.Events {
			if ev.Type == "paused_microflow" {
				data := ev.Data
				pm = &data
				break
			}
		}

		if pm == nil {
			result.Reason = "completed"
			return result
		}

		if pm.DebugID != debugID {
			result.Reason = "breakpoint hit"
			return result
		}

		if result.MicroflowName == "" {
			result.MicroflowName = pm.MicroflowName
		}

		step := TraceStep{
			Number:        i,
			ObjectID:      pm.ObjectID,
			ObjectName:    pm.ObjectName,
			MicroflowName: pm.MicroflowName,
		}

		// Frame change detection — applies to both step-over and step-into
		if prevMicroflow != "" && pm.MicroflowName != prevMicroflow {
			if len(stack) > 0 && stack[len(stack)-1].microflowName == pm.MicroflowName {
				// Returned to parent frame — diff against the pre-call snapshot
				step.FrameChange = FrameReturned
				parent := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				step.Diffs = DiffVariables(parent.snapshot, pm.Variables)
			} else {
				// Entered a sub-microflow — save current frame
				stack = append(stack, frame{
					microflowName: prevMicroflow,
					snapshot:      prevSnapshot,
				})
				step.FrameChange = FrameEntered
				step.Diffs = nil
			}
		} else if prevSnapshot == nil {
			step.Diffs = DiffVariables(nil, pm.Variables)
		} else {
			step.Diffs = DiffVariables(prevSnapshot, pm.Variables)
		}

		prevSnapshot = pm.Variables
		prevMicroflow = pm.MicroflowName
		lastWasEnd = pm.ObjectName == "End"

		result.Steps = append(result.Steps, step)
	}

	result.Reason = "max steps reached"
	return result
}

// DiffVariables computes the differences between two variable snapshots.
func DiffVariables(prev, curr map[string]Variable) []VarDiff {
	var diffs []VarDiff

	currKeys := make([]string, 0, len(curr))
	for k := range curr {
		currKeys = append(currKeys, k)
	}
	sort.Strings(currKeys)

	prevSet := make(map[string]bool, len(prev))
	for k := range prev {
		prevSet[k] = true
	}

	for _, name := range currKeys {
		cv := curr[name]
		if pv, ok := prev[name]; ok {
			oldStr := formatVariable(name, pv)
			newStr := formatVariable(name, cv)
			if oldStr != newStr {
				diffs = append(diffs, VarDiff{Name: name, Change: "~", Old: oldStr, New: newStr})
			}
			delete(prevSet, name)
		} else {
			diffs = append(diffs, VarDiff{Name: name, Change: "+", New: formatVariable(name, cv)})
		}
	}

	removed := make([]string, 0, len(prevSet))
	for k := range prevSet {
		removed = append(removed, k)
	}
	sort.Strings(removed)
	for _, name := range removed {
		diffs = append(diffs, VarDiff{Name: name, Change: "-", Old: formatVariable(name, prev[name])})
	}

	return diffs
}
