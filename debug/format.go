// SPDX-License-Identifier: Apache-2.0

package debug

import (
	"fmt"
	"sort"
	"strings"
)

// FormatPausedMicroflow returns a human-readable representation of a paused microflow.
func FormatPausedMicroflow(pm *PausedMicroflow) string {
	var b strings.Builder
	fmt.Fprintf(&b, "debug_id: %s\n", pm.DebugID)
	fmt.Fprintf(&b, "  Microflow: %s at %s (object_id=%s)\n",
		pm.MicroflowName, pm.ObjectName, shortID(pm.ObjectID))
	b.WriteString(FormatVariables(pm.Variables))
	return b.String()
}

// FormatVariables returns a human-readable representation of the variable scope.
func FormatVariables(vars map[string]Variable) string {
	if len(vars) == 0 {
		return "  (no variables)\n"
	}
	names := make([]string, 0, len(vars))
	for name := range vars {
		names = append(names, name)
	}
	sort.Strings(names)

	var b strings.Builder
	for _, name := range names {
		v := vars[name]
		b.WriteString("  ")
		b.WriteString(formatVariable(name, v))
		b.WriteByte('\n')
	}
	return b.String()
}

func formatVariable(name string, v Variable) string {
	switch v.Type {
	case "object":
		state := v.State
		if state == "" {
			state = "?"
		}
		id := v.ID
		if id == "" {
			id = "?"
		}
		return fmt.Sprintf("%s: %s (id=%s, %s)", name, v.Entity, id, state)
	case "enum":
		return fmt.Sprintf("%s: enum %s = %v", name, v.EnumerationName, v.Value)
	case "boolean":
		return fmt.Sprintf("%s: boolean = %v", name, v.Value)
	default:
		if v.Value != nil {
			return fmt.Sprintf("%s: %s = %v", name, v.Type, v.Value)
		}
		return fmt.Sprintf("%s: %s", name, v.Type)
	}
}

// FormatEvents returns a human-readable representation of debugger events.
func FormatEvents(events []Event) string {
	if len(events) == 0 {
		return "No new events.\n"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d event(s):\n\n", len(events))
	for _, ev := range events {
		if ev.Type == "paused_microflow" {
			b.WriteString(FormatPausedMicroflow(&ev.Data))
			b.WriteByte('\n')
		} else {
			fmt.Fprintf(&b, "[%s] unknown event type\n", ev.Type)
		}
	}
	return b.String()
}

// FormatSessionStart returns a summary of a newly started debug session.
func FormatSessionStart(result *SessionResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Debug session started.\n")
	fmt.Fprintf(&b, "  Session:  %s\n", shortID(result.SessionToken))
	fmt.Fprintf(&b, "  Runtime:  %s\n", result.RuntimeVersion)
	fmt.Fprintf(&b, "  Project:  %s\n", shortID(result.ProjectID))
	if len(result.PausedMicroflows) > 0 {
		fmt.Fprintf(&b, "\nAlready paused (%d):\n", len(result.PausedMicroflows))
		for _, pm := range result.PausedMicroflows {
			b.WriteString(FormatPausedMicroflow(&pm))
		}
	}
	return b.String()
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}
