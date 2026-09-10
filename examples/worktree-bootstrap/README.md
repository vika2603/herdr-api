# Worktree Bootstrap

An example Herdr plugin: one action turns a gesture into a ready workspace. It
creates a git worktree for the branch an issue names, arranges two panes in
the new checkout, starts a coding agent in one of them and sends that agent
its first prompt.

| Entrypoint | Manifest section | How it is reached |
| --- | --- | --- |
| Action `bootstrap` | `[[actions]] contexts = ["selection"]` | Invoked on a selected issue URL |
| Link handler `issue` | `[[link_handlers]] action = "bootstrap"` | Clicking a matching link in any pane |

A link handler owns no command of its own: it names an action of the same
plugin, and Herdr runs that action with `HERDR_PLUGIN_CLICKED_URL` and
`HERDR_PLUGIN_LINK_HANDLER_ID` set. So one handler serves both ways in and
tells them apart by `Env.LinkHandlerID`:

```go
func clickedURL(env *plugin.Env) string {
	if env.LinkHandlerID != "" {
		return env.ClickedURL
	}
	return env.Invocation().SelectedText
}
```

The pattern Herdr matches against a clicked URL lives in the manifest, and
`main.go` holds the same pattern because it parses the URL itself for the
issue number. `TestLinkPatternMatchesTheManifest` compares the two, so the two
copies of one decision cannot drift apart.

## The sequence

`newPlan` turns the URL and the configuration into the requests to make, and
`bootstrap` makes them, taking every id from the response that carries it. The
decision is therefore testable without a server, and each call names what the
previous one returned rather than a guess:

| Call | Ids it needs | What it answers with |
| --- | --- | --- |
| `worktree.create` | the invoking workspace, which names the repository | the new workspace, its tab, and the checkout path |
| `workspace.report_metadata` | the new workspace id | — |
| `layout.apply` | the tab the worktree opened, the checkout path for both panes | the applied layout, with a pane id per pane |
| `agent.start` | the pane id of the agent pane | the started agent, in a pane |
| `agent.prompt` | that pane as the target | — |

Two of those ids are worth spelling out. `worktree.create` takes the workspace
the action was invoked from, because that is how it finds the repository to
branch from; without it the server resolves a repository from its own working
directory. And a new pane does not inherit the working directory of whatever
created it, so every pane in the arrangement names the checkout explicitly.

The metadata is reported before the panes exist, so a workspace whose agent
failed to start still says which issue it was opened for. `source` names this
plugin as the authority for the tokens it reports. The request also takes a
`ttl_ms`, which this example leaves out: the issue a workspace was opened for
does not expire.

## Configuration

`HERDR_PLUGIN_CONFIG_DIR` is the directory Herdr gives the plugin for
user-editable configuration. This plugin reads `config.json` from it through
`Env.ConfigPath`, and both fields have working defaults, so an unconfigured
plugin still bootstraps:

```json
{
  "agent_kind": "codex",
  "prompt": "Read {{url}} and propose a plan before you change anything."
}
```

`agent_kind` is a Herdr agent kind, as `herdr agent start --kind` takes, and
`{{url}}` in the prompt is replaced with the clicked URL.

## Install

The manifest declares `linux` and `macos`. On Windows, add a second entry per
command that builds and runs `herdr-worktree-bootstrap.exe`.

`herdr plugin link` registers a directory but does not run its `[[build]]`
command; only `herdr plugin install` does, and that clones from GitHub. Build
the binary first, then link:

```bash
cd path/to/herdr-client/examples/worktree-bootstrap
go build -o herdr-worktree-bootstrap .
herdr plugin link .
herdr plugin list
```

To remove the plugin again:

```bash
herdr plugin unlink example.worktree-bootstrap
```

## Use

Click an issue or pull request link in any pane and choose the handler, or
select such a URL and invoke the action:

```bash
herdr plugin action invoke bootstrap --plugin example.worktree-bootstrap
```

Invoked that way the action has no clicked URL and no selection, so it reports
that what it was given is not an issue URL. The repository must also be one
Herdr already trusts: the plugin does not pass `trust_repository`, so on an
untrusted repository `worktree.create` reports that rather than trusting it on
the user's behalf.

Herdr records every action run, with its exit code, stdout and stderr:

```bash
herdr plugin log list --plugin example.worktree-bootstrap
```
