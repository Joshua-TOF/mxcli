# Debug Microflow

Debug a running Mendix application by setting breakpoints, stepping through microflows, and inspecting variables.

## Prerequisites

- The Mendix app must be **running** (locally via Studio Pro, Docker, or in the cloud)
- You need the **debugger password** (see below for how to find it)

### Connecting to local Studio Pro

1. Run the app in Studio Pro (F5 or Run Locally)
2. Set the M2EE log level to **Trace** in the Studio Pro console
3. Start the debugger: in Studio Pro, go to **Run → Start Debugger** and select **Local**
4. Find the password in the Studio Pro console — look for the M2EE log node message:
   `Handling adminaction 'enable_debugger' with params '{"password":"<UUID>"}'`
5. The app URL is `http://localhost:8080` and the password is the UUID from the log message

```bash
mxcli debug start -p app.mpr --url http://localhost:8080 --password <UUID-from-console>
```

### Connecting to local Docker

Zero-config — the debugger password is auto-resolved from `.docker/.env`:

```bash
mxcli debug start -p app.mpr
```

### Connecting to a cloud app

Use the debugger password from Mendix Cloud portal settings:

```bash
mxcli debug start -p app.mpr --url https://myapp.mendixcloud.com --password <password>
```

## Quick Start

```bash
# Start debug session (local Docker — zero config)
mxcli debug start -p app.mpr

# Or connect to a cloud app
mxcli debug start -p app.mpr --url https://myapp.mendixcloud.com --password <pw>

# See actions with their IDs
mxcli describe microflow Module.MyMicroflow --ids -p app.mpr

# Set breakpoint by action name
mxcli debug breakpoint add -p app.mpr Module.MyMicroflow --action "Commit"

# Or by action index (1-based, from describe --ids output)
mxcli debug breakpoint add -p app.mpr Module.MyMicroflow --action "#3"

# Wait for breakpoint hit (ask user to trigger the action in the app)
mxcli debug poll -p app.mpr

# Step through
mxcli debug step-over -p app.mpr <debug-id>
mxcli debug poll -p app.mpr

# Continue execution
mxcli debug continue -p app.mpr <debug-id>

# Stop debugging
mxcli debug stop -p app.mpr
```

## Debugging Workflow

1. Start a debug session with `debug start`
2. Use `describe microflow --ids` to see actions and their internal object IDs
3. Set breakpoints with `debug breakpoint add` (by action name, type, or #index)
4. Ask the user to trigger the microflow in the Mendix app
5. Use `debug poll` to wait for the breakpoint hit — inspect variables in the output
6. Step through with `step-over`, `step-into`, or `step-out`
7. After each step, `poll` again to see updated position and variables
8. `continue` to resume or `stop` to end the session

## All Commands

```bash
mxcli debug start -p app.mpr [--url <url>] [--password <pw>]
mxcli debug stop -p app.mpr
mxcli debug status -p app.mpr

mxcli debug breakpoint add -p app.mpr <microflow> --action <name-or-index> [--condition <expr>]
mxcli debug breakpoint add -p app.mpr <microflow> --object-id <uuid>
mxcli debug breakpoint remove -p app.mpr <object-id>

mxcli debug poll -p app.mpr [--timeout <seconds>]
mxcli debug continue -p app.mpr [<debug-id>]
mxcli debug step-over -p app.mpr <debug-id>
mxcli debug step-into -p app.mpr <debug-id>
mxcli debug step-out -p app.mpr <debug-id>
```

## Tips

- `poll --timeout 0` returns immediately without waiting
- `continue` without a debug-id continues all paused microflows
- Conditional breakpoints: `--condition "$Client/Naam = 'test'"`
- Session state is stored in `.mxcli/debug-session.json` — survives between commands
- If session disconnects, just run `debug start` again
