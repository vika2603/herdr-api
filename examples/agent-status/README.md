# Agent Status Log

An example Herdr plugin: one Go binary serving three entrypoints through
`plugin.Run`, which selects the handler from the environment Herdr injects.

| Entrypoint | Manifest section | What it does |
| --- | --- | --- |
| Startup hook | `[[startup]]` | Empties the log, so it only ever describes the running session |
| Event hook | `[[events]] on = "pane.agent_status_changed"` | Appends one JSON record per agent status change |
| Action `show` | `[[actions]]` | Prints the recorded changes |

Records are written as JSON lines to `agent-status.jsonl` in
`HERDR_PLUGIN_STATE_DIR`, the directory Herdr gives the plugin for its own
state. Each record holds a timestamp, the workspace and pane ids, the agent
label Herdr displays, and the new status.

The action reads its invocation context through `Env.Context()`. When Herdr
invoked it from a workspace, only that workspace's records are printed;
invoked globally, it prints all of them.

## Install

The manifest declares `linux` and `macos`. On Windows, add a second entry per
command that builds and runs `herdr-agent-status.exe`.

Linking registers the plugin for your user account and enables it in every
Herdr session, so run it yourself rather than letting a tool do it:

```bash
herdr plugin link path/to/herdr-client/examples/agent-status
herdr plugin list
```

Linking runs the `[[build]]` command, which produces `herdr-agent-status` in
this directory. To remove the plugin again:

```bash
herdr plugin unlink example.agent-status
```

## Use

The startup hook runs when a Herdr session starts, so start a new session or
restart the current one after linking. Then let an agent pane change status
and print the log:

```bash
herdr plugin action invoke show --plugin example.agent-status
```

Herdr records every hook and action run, with its exit code, stdout and
stderr:

```bash
herdr plugin log list --plugin example.agent-status
```

## Exit codes

`plugin.Run` returns the exit code Herdr stores in that log: `0` on success,
`1` when a handler returned an error, and `2` when no handler ran because the
environment was not a plugin environment, its entrypoint kind was unknown, no
handler was registered for it, or the event envelope did not decode. The error
is written to stderr in every failing case.
