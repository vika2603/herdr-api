# herdr-api

Go client for the [Herdr](https://herdr.dev) socket API, and a toolkit for
writing Herdr plugins in Go.

The wire types, the result and event decoders, and a typed wrapper for every
one of the 102 API methods are generated from the schema the herdr binary
prints, so the client tracks the server rather than a hand-written guess of
it. The transport, the plugin process environment and the manifest parser are
hand-written.

Generated against herdr 0.9.0, protocol 22.

## Install

```bash
go get github.com/vika2603/herdr-api
```

## Connect and call

The package name is `herdr`. `NewFromEnv` resolves the socket the way the
herdr CLI does: `HERDR_SOCKET_PATH` first, then the session named by
`HERDR_SESSION`, then the default session.

```go
client, err := herdr.NewFromEnv()
if err != nil {
	return err
}

pong, err := client.Ping(ctx)
if err != nil {
	return err
}
fmt.Printf("herdr %s, protocol %d\n", pong.Version, pong.Protocol)

panes, err := client.PaneList(ctx, herdr.PaneListParams{})
if err != nil {
	return err
}
for _, pane := range panes.Panes {
	fmt.Printf("%s %s\n", pane.PaneID, pane.AgentStatus)
}
```

Each method takes its own params type and returns its own result type. A
method whose params carry nothing takes none, as `Ping` does above.

Optional fields that are not strings are pointers, so that leaving one unset
is distinguishable from sending its zero value. `herdr.Ptr` supplies the
address inline, and `herdr.Value` reads one back:

```go
created, err := client.PaneSplit(ctx, herdr.PaneSplitParams{
	Direction: herdr.SplitDirectionRight,
	Cwd:       herdr.Ptr("/repo"),
})
if err != nil {
	return err
}

_, err = client.PaneSendInput(ctx, herdr.PaneSendInputParams{
	PaneID: created.Pane.PaneID,
	Text:   "go test ./...",
	Keys:   []string{"enter"},
})
```

The server reads one request per connection and closes it afterwards, so every
call dials a fresh connection. A `Client` is safe for concurrent use, performs
no I/O until its first call, and cancels an in-flight call when its context is
done.

## Errors

The server answers a failed request with a code and a message, which arrive as
`*herdr.Error`:

```go
_, err := client.PaneGet(ctx, herdr.PaneTarget{PaneID: "w9:p9"})
if herdr.IsCode(err, herdr.ErrCodePaneNotFound) {
	// the pane is gone
}

var apiErr *herdr.Error
if errors.As(err, &apiErr) {
	log.Println(apiErr.Code, apiErr.Message)
}
```

The codes herdr reports are available as `ErrCode*` constants, and comparing
`Code` against a plain string keeps working for codes a newer server adds.

## Events

`events.subscribe` is the one method that keeps its connection open. It
returns a `*Stream` whose `NextEvent` yields decoded events:

```go
stream, err := client.EventsSubscribe(ctx, herdr.EventsSubscribeParams{
	Subscriptions: []herdr.Subscription{
		herdr.PaneAgentStatusChangedSubscription{PaneID: "w1:p1"},
	},
})
if err != nil {
	return err
}
defer stream.Close()

for {
	event, err := stream.NextEvent(ctx)
	if err != nil {
		return err
	}
	if changed, ok := event.(*herdr.PaneAgentStatusChangedEvent); ok {
		fmt.Println(changed.PaneID, changed.AgentStatus)
	}
}
```

A subscription starts when the server accepts it and does not replay earlier
events. To build a complete picture, open the subscription first, buffer what
arrives, then call `SessionSnapshot` and apply the buffer on top.

## Writing a plugin

A Herdr plugin is a directory with a `herdr-plugin.toml` manifest and commands
Herdr launches. Herdr injects the invocation context into the environment,
which `plugin.Load` reads:

```go
env, err := plugin.Load()
if err != nil {
	log.Fatal(err)
}

client := env.Client()
log.Println(env.PluginID, env.Kind(), env.StateDir)
```

`Kind` reports which manifest entrypoint started the process: a startup hook,
an action, an event hook or a pane command. Durable state belongs under
`env.StateDir` and user-editable configuration under `env.ConfigDir`; the
plugin's own directory is a managed checkout when it was installed from
GitHub.

`plugin/manifest` reads and validates a manifest with the same rules herdr
applies, which is useful in a plugin's own tests and in tooling:

```go
m, warnings, err := manifest.Parse("herdr-plugin.toml")
```

Warnings are returned separately from errors, matching herdr: an event hook
naming an event that herdr never fires for hooks is a warning, a duplicate
action id is an error.

## Layout

| Path | Contents |
| --- | --- |
| `.` (package `herdr`) | Transport, plus the generated types, results, events and method wrappers |
| `plugin` | The environment Herdr injects into plugin commands |
| `plugin/manifest` | `herdr-plugin.toml` parsing and validation |
| `cmd/herdr-apigen`, `internal/gen` | The generator that produces `*_gen.go` |
| `schema` | The schema snapshot and the method-to-result table |
| `docs/design.md` | Protocol facts, generation rules and the development plan |

## Upgrading to a new herdr

```bash
just schema-update   # rewrite schema/herdr-api.schema.json from the installed herdr
just gen             # regenerate *_gen.go
just check           # build, test, lint, and verify the generated code is current
```

`schema/method-results.json` records which result type each method returns,
which the schema itself does not state. Add an entry for any new method; the
generator refuses to run while one is missing. `schema/README.md` has the
details.

Generated code ignores fields it does not know, and an unrecognised result or
event type surfaces as an error rather than a panic, so a newer server does
not break a client outright. Compare the `Protocol` in a `Ping` response with
`herdr.SchemaProtocol` to detect one.

## Status

The client, the generator and the manifest parser are complete. The plugin
runtime dispatcher, a cached live session mirror and the end-to-end suite are
in progress; see the phase 2 section of `docs/design.md`.
