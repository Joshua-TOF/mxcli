// SPDX-License-Identifier: Apache-2.0

package debug

import (
	"fmt"
	"strings"
)

// FormatTrace returns a human-readable diff-based trace output.
func FormatTrace(r *TraceResult) string {
	var b strings.Builder

	name := r.MicroflowName
	if name == "" {
		name = "(unknown)"
	}
	fmt.Fprintf(&b, "Trace: %s (%d steps)\n", name, len(r.Steps))
	fmt.Fprintf(&b, "debug_id: %s\n\n", r.DebugID)

	for _, step := range r.Steps {
		switch step.FrameChange {
		case FrameEntered:
			fmt.Fprintf(&b, "Step %d: → stepped into %s\n", step.Number, step.MicroflowName)
			if step.Diffs == nil {
				fmt.Fprintf(&b, "  (new scope)\n")
			}
		case FrameReturned:
			fmt.Fprintf(&b, "Step %d: ← stepped out to %s\n", step.Number, step.MicroflowName)
			formatDiffs(&b, step.Diffs)
		default:
			fmt.Fprintf(&b, "Step %d: %s at %s\n", step.Number, step.ObjectName, shortID(step.ObjectID))
			formatDiffs(&b, step.Diffs)
		}
		b.WriteByte('\n')
	}

	switch r.Reason {
	case "completed":
		fmt.Fprintf(&b, "Completed: %d steps (microflow returned)\n", len(r.Steps))
	case "max steps reached":
		fmt.Fprintf(&b, "Stopped: max steps reached (%d), microflow still running\n", len(r.Steps))
	case "breakpoint hit":
		fmt.Fprintf(&b, "Stopped: another breakpoint was hit after %d steps\n", len(r.Steps))
	case "error":
		fmt.Fprintf(&b, "Error after %d steps: %v\n", len(r.Steps), r.Error)
	}

	return b.String()
}

func formatDiffs(b *strings.Builder, diffs []VarDiff) {
	if len(diffs) == 0 {
		b.WriteString("  (no variable changes)\n")
		return
	}
	for _, d := range diffs {
		switch d.Change {
		case "+":
			fmt.Fprintf(b, "  + %s\n", d.New)
		case "-":
			fmt.Fprintf(b, "  - %s\n", d.Old)
		case "~":
			fmt.Fprintf(b, "  ~ %s\n", d.New)
		}
	}
}
