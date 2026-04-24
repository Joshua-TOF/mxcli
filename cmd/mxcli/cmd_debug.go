// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mendixlabs/mxcli/debug"
	"github.com/mendixlabs/mxcli/sdk/mpr"
	"github.com/spf13/cobra"
)

var debugCmd = &cobra.Command{
	Use:   "debug",
	Short: "Debug a running Mendix application",
	Long: `Debug a running Mendix application by setting breakpoints, stepping
through microflows, and inspecting variables.

For local Docker projects, connection details are auto-resolved from .docker/.env.
For cloud apps, pass --url and --password explicitly.

Examples:
  # Local Docker project (zero-config)
  mxcli debug start -p app.mpr

  # Cloud app
  mxcli debug start -p app.mpr --url https://myapp.mendixcloud.com --password secret

  # Set breakpoint and debug
  mxcli debug breakpoint add -p app.mpr Module.MyMicroflow --action "Commit"
  mxcli debug poll -p app.mpr
  mxcli debug step-over -p app.mpr <debug-id>
  mxcli debug continue -p app.mpr
  mxcli debug stop -p app.mpr
`,
}

var debugStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start a debugger session",
	Run: func(cmd *cobra.Command, args []string) {
		projectDir := resolveProjectDir(cmd)
		appURL, password := resolveDebugConnection(cmd, projectDir)

		client := debug.NewClient(appURL, password)
		result, err := client.StartSession()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		session := &debug.Session{
			SessionToken:   result.SessionToken,
			AppURL:         appURL,
			AuthHeader:     base64.StdEncoding.EncodeToString([]byte(password)),
			RuntimeVersion: result.RuntimeVersion,
			ProjectID:      result.ProjectID,
			StartedAt:      time.Now(),
		}
		if err := debug.SaveSession(projectDir, session); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not save session: %v\n", err)
		}

		fmt.Print(debug.FormatSessionStart(result))
	},
}

var debugStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the debugger session",
	Run: func(cmd *cobra.Command, args []string) {
		projectDir := resolveProjectDir(cmd)
		session, client := loadSessionAndClient(projectDir)

		if err := client.StopSession(session.SessionToken); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
		}
		if err := debug.ClearSession(projectDir); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
		}
		fmt.Println("Debug session stopped.")
	},
}

var debugStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show debugger session status",
	Run: func(cmd *cobra.Command, args []string) {
		projectDir := resolveProjectDir(cmd)
		session, err := debug.LoadSession(projectDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Session:  %s\n", session.SessionToken[:8])
		fmt.Printf("URL:      %s\n", session.AppURL)
		fmt.Printf("Runtime:  %s\n", session.RuntimeVersion)
		fmt.Printf("Started:  %s\n", session.StartedAt.Format(time.RFC3339))
	},
}

var debugBreakpointCmd = &cobra.Command{
	Use:   "breakpoint",
	Short: "Manage breakpoints",
}

var debugBreakpointAddCmd = &cobra.Command{
	Use:   "add <microflow>",
	Short: "Add a breakpoint to a microflow action",
	Long: `Set a breakpoint on a specific action within a microflow.

Specify the target action with --action (name, type, or #index) or --object-id (UUID).
Use 'mxcli describe microflow <name> --ids' to see available actions and their IDs.

Examples:
  mxcli debug breakpoint add -p app.mpr Module.MyMicroflow --action "Commit"
  mxcli debug breakpoint add -p app.mpr Module.MyMicroflow --action "#3"
  mxcli debug breakpoint add -p app.mpr Module.MyMicroflow --object-id 5499b05e-a4d5-49a6-8737-e4ddb08e2248
  mxcli debug breakpoint add -p app.mpr Module.MyMicroflow --action "Commit" --condition "$Client/Naam = 'test'"
`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		projectDir := resolveProjectDir(cmd)
		session, client := loadSessionAndClient(projectDir)

		microflowName := args[0]
		objectID, _ := cmd.Flags().GetString("object-id")
		actionQuery, _ := cmd.Flags().GetString("action")
		condition, _ := cmd.Flags().GetString("condition")

		if objectID == "" && actionQuery == "" {
			fmt.Fprintln(os.Stderr, "Error: provide --action or --object-id")
			os.Exit(1)
		}

		if objectID == "" {
			projectPath, _ := cmd.Root().Flags().GetString("project")
			if projectPath == "" {
				fmt.Fprintln(os.Stderr, "Error: --project (-p) is required for action resolution")
				os.Exit(1)
			}
			resolved, err := resolveBreakpointFromProject(projectPath, microflowName, actionQuery)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			objectID = string(resolved)
		}

		if err := client.AddBreakpoint(session.SessionToken, microflowName, objectID, condition); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Breakpoint set on %s at object %s\n", microflowName, objectID[:8])
	},
}

var debugBreakpointRemoveCmd = &cobra.Command{
	Use:   "remove <object-id>",
	Short: "Remove a breakpoint",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		projectDir := resolveProjectDir(cmd)
		session, client := loadSessionAndClient(projectDir)

		if err := client.RemoveBreakpoint(session.SessionToken, args[0]); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Breakpoint removed: %s\n", args[0][:8])
	},
}

var debugPollCmd = &cobra.Command{
	Use:   "poll",
	Short: "Poll for debugger events (breakpoint hits, step results)",
	Run: func(cmd *cobra.Command, args []string) {
		projectDir := resolveProjectDir(cmd)
		session, client := loadSessionAndClient(projectDir)

		timeout, _ := cmd.Flags().GetInt("timeout")
		result, err := client.PollEvents(session.SessionToken, time.Duration(timeout)*time.Second)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Print(debug.FormatEvents(result.Events))
	},
}

var debugContinueCmd = &cobra.Command{
	Use:   "continue [debug-id]",
	Short: "Continue execution (one microflow or all)",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		projectDir := resolveProjectDir(cmd)
		session, client := loadSessionAndClient(projectDir)

		if len(args) == 1 {
			if err := client.Continue(session.SessionToken, args[0]); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("Continued microflow %s\n", args[0][:8])
		} else {
			if err := client.ContinueAll(session.SessionToken); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			fmt.Println("Continued all paused microflows.")
		}
	},
}

var debugStepOverCmd = &cobra.Command{
	Use:   "step-over <debug-id>",
	Short: "Step to the next action",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		projectDir := resolveProjectDir(cmd)
		session, client := loadSessionAndClient(projectDir)

		if err := client.StepOver(session.SessionToken, args[0]); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Stepped over in %s. Run 'debug poll' to see new position.\n", args[0][:8])
	},
}

var debugStepIntoCmd = &cobra.Command{
	Use:   "step-into <debug-id>",
	Short: "Step into a sub-microflow or loop",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		projectDir := resolveProjectDir(cmd)
		session, client := loadSessionAndClient(projectDir)

		if err := client.StepInto(session.SessionToken, args[0]); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Stepped into from %s. Run 'debug poll' to see new position.\n", args[0][:8])
	},
}

var debugStepOutCmd = &cobra.Command{
	Use:   "step-out <debug-id>",
	Short: "Step out of current sub-microflow or loop",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		projectDir := resolveProjectDir(cmd)
		session, client := loadSessionAndClient(projectDir)

		if err := client.StepOut(session.SessionToken, args[0]); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Stepped out from %s. Run 'debug poll' to see new position.\n", args[0][:8])
	},
}

var debugTraceCmd = &cobra.Command{
	Use:   "trace <debug-id>",
	Short: "Step through N actions and show variable diffs",
	Long: `Automatically step through a paused microflow and collect variable changes
at each action. Outputs a compact diff-based trace instead of requiring
individual step-over + poll calls.

Examples:
  mxcli debug trace -p app.mpr <debug-id>
  mxcli debug trace -p app.mpr <debug-id> --steps 10
  mxcli debug trace -p app.mpr <debug-id> --deep
`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		projectDir := resolveProjectDir(cmd)
		session, client := loadSessionAndClient(projectDir)

		debugID := args[0]
		steps, _ := cmd.Flags().GetInt("steps")
		deep, _ := cmd.Flags().GetBool("deep")

		// Get the initial paused state via start_session (re-returns paused_microflows).
		sessionResult, err := client.StartSession()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error getting initial state: %v\n", err)
			os.Exit(1)
		}
		// Update session token (start_session creates a new one)
		session.SessionToken = sessionResult.SessionToken
		if err := debug.SaveSession(projectDir, session); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not update session: %v\n", err)
		}

		var initialState *debug.PausedMicroflow
		for i := range sessionResult.PausedMicroflows {
			if sessionResult.PausedMicroflows[i].DebugID == debugID {
				initialState = &sessionResult.PausedMicroflows[i]
				break
			}
		}

		result := debug.Trace(client, session.SessionToken, debugID, initialState, debug.TraceOptions{
			MaxSteps:    steps,
			Deep:        deep,
			PollTimeout: 5 * time.Second,
		})

		fmt.Print(debug.FormatTrace(result))
		if result.Error != nil {
			os.Exit(1)
		}
	},
}

// resolveProjectDir finds the directory containing the .mpr file.
func resolveProjectDir(cmd *cobra.Command) string {
	projectPath, _ := cmd.Root().Flags().GetString("project")
	if projectPath != "" {
		return filepath.Dir(projectPath)
	}
	dir, _ := os.Getwd()
	return dir
}

// resolveDebugConnection determines the app URL and debugger password.
// Resolution order: explicit flags → env vars → .docker/.env → defaults.
func resolveDebugConnection(cmd *cobra.Command, projectDir string) (string, string) {
	appURL, _ := cmd.Flags().GetString("url")
	password, _ := cmd.Flags().GetString("password")

	if appURL == "" {
		appURL = os.Getenv("MENDIX_APP_URL")
	}
	if password == "" {
		password = os.Getenv("RUNTIME_DEBUGGER_PASSWORD")
	}

	if appURL == "" || password == "" {
		envVars := loadDockerEnv(projectDir)
		if appURL == "" {
			port := envVars["APP_PORT"]
			if port == "" {
				port = "8080"
			}
			appURL = "http://localhost:" + port
		}
		if password == "" {
			password = envVars["M2EE_ADMIN_PASS"]
			if password == "" {
				password = "AdminPassword1!"
			}
		}
	}

	return appURL, password
}

// loadDockerEnv reads .docker/.env from the project directory.
func loadDockerEnv(projectDir string) map[string]string {
	envPath := filepath.Join(projectDir, ".docker", ".env")
	data, err := os.ReadFile(envPath)
	if err != nil {
		return nil
	}
	result := make(map[string]string)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if k, v, ok := strings.Cut(line, "="); ok {
			result[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	return result
}

// loadSessionAndClient loads the session and creates a client. Exits on error.
func loadSessionAndClient(projectDir string) (*debug.Session, *debug.Client) {
	session, err := debug.LoadSession(projectDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	return session, debug.ClientFromSession(session)
}

// resolveBreakpointFromProject opens the .mpr via an executor, finds the
// microflow by qualified name, and resolves the action query to an object_id.
func resolveBreakpointFromProject(projectPath, microflowName, actionQuery string) (string, error) {
	parts := strings.SplitN(microflowName, ".", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("microflow name must be qualified: Module.Name")
	}

	reader, err := mpr.Open(projectPath)
	if err != nil {
		return "", fmt.Errorf("open project: %w", err)
	}
	defer reader.Close()

	modules, err := reader.ListModules()
	if err != nil {
		return "", fmt.Errorf("list modules: %w", err)
	}
	moduleIDs := make(map[string]string) // id → name
	for _, m := range modules {
		moduleIDs[string(m.ID)] = m.Name
	}

	mfs, err := reader.ListMicroflows()
	if err != nil {
		return "", fmt.Errorf("list microflows: %w", err)
	}

	// Build container→parent map from folders so we can walk up to find modules.
	folders, err := reader.ListFolders()
	if err != nil {
		return "", fmt.Errorf("list folders: %w", err)
	}
	containerParent := make(map[string]string) // id → parent container id
	for _, f := range folders {
		containerParent[string(f.ID)] = string(f.ContainerID)
	}

	findModule := func(containerID string) string {
		current := containerID
		for i := 0; i < 30; i++ {
			if name, ok := moduleIDs[current]; ok {
				return name
			}
			parent, ok := containerParent[current]
			if !ok || parent == "" {
				break
			}
			current = parent
		}
		return ""
	}

	for _, mf := range mfs {
		modName := findModule(string(mf.ContainerID))
		if modName == parts[0] && mf.Name == parts[1] {
			id, err := debug.ResolveActionID(mf, actionQuery)
			if err != nil {
				return "", err
			}
			return string(id), nil
		}
	}

	return "", fmt.Errorf("microflow %s not found", microflowName)
}

func init() {
	// Start command flags
	debugStartCmd.Flags().String("url", "", "Mendix app URL (auto-resolved from .docker/.env for local projects)")
	debugStartCmd.Flags().String("password", "", "Debugger password (auto-resolved from .docker/.env for local projects)")

	// Breakpoint add flags
	debugBreakpointAddCmd.Flags().String("action", "", "Action to break on (caption, variable name, type, or #index)")
	debugBreakpointAddCmd.Flags().String("object-id", "", "Direct object UUID (from describe --ids)")
	debugBreakpointAddCmd.Flags().String("condition", "", "Mendix expression condition (e.g., \"$Client/Naam = 'test'\")")

	// Poll flags
	debugPollCmd.Flags().Int("timeout", 25, "Poll timeout in seconds (0 for immediate)")

	// Trace flags
	debugTraceCmd.Flags().Int("steps", 20, "Maximum number of steps to trace")
	debugTraceCmd.Flags().Bool("deep", false, "Use step-into instead of step-over (follow sub-microflows)")

	// Register subcommands
	debugBreakpointCmd.AddCommand(debugBreakpointAddCmd)
	debugBreakpointCmd.AddCommand(debugBreakpointRemoveCmd)

	debugCmd.AddCommand(debugStartCmd)
	debugCmd.AddCommand(debugStopCmd)
	debugCmd.AddCommand(debugStatusCmd)
	debugCmd.AddCommand(debugBreakpointCmd)
	debugCmd.AddCommand(debugPollCmd)
	debugCmd.AddCommand(debugContinueCmd)
	debugCmd.AddCommand(debugStepOverCmd)
	debugCmd.AddCommand(debugStepIntoCmd)
	debugCmd.AddCommand(debugStepOutCmd)
	debugCmd.AddCommand(debugTraceCmd)

	rootCmd.AddCommand(debugCmd)
}
