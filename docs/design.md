# Design

This document describes how `github.com/vika2603/herdr-client` is built and
why, and what is still missing. Every statement about the protocol is backed
by `schema/herdr-api.schema.json` (herdr 0.9.0, protocol 22), by the herdr
v0.9.0 sources, or by a test in this repository.

## What the module is

A Go client for Herdr. It carries the whole socket API, a cache that mirrors a
live session, and the pieces a Herdr plugin written in Go needs. It began as a
wrapper over the API, which is where the original name herdr-api came from,
and was renamed once it grew past that.

Most of the surface is generated from the schema the herdr binary prints, so
the client follows the server rather than a hand-written guess of it. Only
what the schema cannot express is hand-written: the transport, the session
mirror, the plugin process environment, the manifest parser, and four unions
that carry no discriminator.

## Verified protocol facts

- Transport is newline-delimited JSON over a local socket. On Unix it is a Unix
  domain socket. On Windows it is a named pipe: `HERDR_SOCKET_PATH` holds a
  file path and the pipe name is `\\.\pipe\` followed by that path.
- The server reads exactly one request line per connection, writes one
  response line, and closes the connection (`src/api/server.rs`,
  `handle_connection`). Ordinary calls therefore dial a fresh connection each
  time; connections cannot be reused or pipelined.
- `events.subscribe` keeps the connection open after answering
  `{"type":"subscription_started"}` and pushes one event per line. Writing
  anything further on that connection makes the server close it.
  `pane.graphics.stream` also keeps its connection open but is absent from the
  schema and is out of scope for this iteration.
- Pushed events use two envelopes, distinguished by the `event` field:
  - Lifecycle events: `{"event":"pane_created","data":{"type":"pane_created",...}}`.
    `event` is an `EventKind` (underscore form) and `data` is discriminated by
    `type`. This is the `event` section of the schema.
  - The three dedicated subscriptions:
    `{"event":"pane.output_matched","data":{...}}`. `event` is the dotted name
    and `data` has no discriminator. This is the `subscription_event` section.
- Error responses are `{"id":"...","error":{"code":"...","message":"..."}}`.
  When the request itself cannot be parsed, `id` is the empty string. Observed
  codes are listed under "Error codes".
- For plugin event hooks, `HERDR_PLUGIN_EVENT` is the dotted event name
  (`EventKind::dot_name`) and `HERDR_PLUGIN_EVENT_JSON` is the full
  `EventEnvelope`. `HERDR_PLUGIN_CONTEXT_JSON` is a `PluginInvocationContext`.
- The schema does not record which `ResponseResult` variant a method returns.
  That relation is maintained in `schema/method-results.json`, derived from the
  v0.9.0 handler sources.

## Layout

| Path | Contents | Written by |
| --- | --- | --- |
| `client.go` `stream.go` `dial_*.go` `socketpath.go` `errors.go` `ptr.go` | Transport, error codes, pointer helpers | hand |
| `subscribe.go` `session.go` | Typed event stream, live session mirror | hand |
| `unions_manual.go` | The four unions without a discriminator | hand |
| `*_gen.go` | Types, results, events, and a wrapper per method | generated |
| `cmd/herdr-apigen` `internal/gen` | The generator | hand |
| `internal/cmd/herdrcheck` | Drift detection against the installed herdr | hand |
| `plugin` `plugin/manifest` | Plugin environment, dispatch, manifest | hand |
| `examples/agent-status` | A worked plugin serving three entrypoint kinds | hand |
| `internal/e2e` | The suite that exercises the API against a real server | hand |
| `schema` | The snapshot, the method result table, the accepted gaps | recorded |

## Transport

```go
func New(socketPath string, opts ...Option) *Client          // no I/O
func NewFromEnv(opts ...Option) (*Client, error)
func ResolveSocketPath(session string) (string, error)
func WithDialTimeout(d time.Duration) Option
func WithRequestIDs(next func() string) Option

func (c *Client) SocketPath() string
func (c *Client) Call(ctx context.Context, method string, params, result any) error
func (c *Client) CallRaw(ctx context.Context, method string, params any) (json.RawMessage, error)
func (c *Client) OpenStream(ctx context.Context, method string, params any) (*Stream, error)
```

`Call` and `CallRaw` dial a connection, write one request line, read one
response line and close, because that is all the server allows. A `nil`
`params` is sent as `{}`. An error response becomes `*Error` carrying the
method. Cancelling the context closes the connection, which is the only way to
unblock a read on every platform, and the call reports `ctx.Err()`.

`OpenStream` keeps the connection and hands back a `*Stream` whose `Next`
decodes each pushed line into a `RawEvent`; `Ack` holds the result of the
response that opened it. Nothing is ever written to that connection again,
because the server closes a streaming connection as soon as the client sends
anything else. `Close` unblocks a waiting `Next`, which then reports
`ErrStreamClosed`.

`ResolveSocketPath` follows herdr: an explicit session name wins, then
`HERDR_SOCKET_PATH`, then `HERDR_SESSION`, then the default session. The
config directory is `$XDG_CONFIG_HOME/herdr` or the platform default. A herdr
built with debug assertions uses `herdr-dev` instead, so a resolved path only
reaches a release build; `HERDR_SOCKET_PATH` is the way to a debug one.

Optional request fields that are not strings are pointers, so that leaving one
unset differs from sending its zero value. `Ptr` and `Value` cover the 134
such fields without a named variable per field.

## Generated code

Generator inputs: `schema/herdr-api.schema.json` and
`schema/method-results.json`. All output files live in the root package and
start with `// Code generated by herdr-apigen. DO NOT EDIT.`:

| File | Content |
| --- | --- |
| `schema_gen.go` | `const SchemaProtocol uint32 = 22` |
| `types_gen.go` | Every `$defs` entry: params, info structs, enums, discriminated unions |
| `results_gen.go` | `Result` interface, the 64 result variants, `DecodeResult` |
| `events_gen.go` | `EventKind`, `Event` interface, event structs, `DecodeEvent`, `EventEnvelope` |
| `methods_gen.go` | `Method*` constants and the `(*Client)` wrappers |

`generate.go` carries
`//go:generate go run ./cmd/herdr-apigen -schema schema/herdr-api.schema.json -methods schema/method-results.json -out .`.

### Naming

- `$defs` names are used verbatim (`PaneInfo`, `PluginInvocationContext`).
  A name that appears in several schema sections must be identical after
  normalising `$ref` values to bare definition names; otherwise the generator
  fails.
- Fields: snake_case to PascalCase with Go initialisms upper-cased: `id→ID`,
  `url→URL`, `json→JSON`, `api→API`, `ttl→TTL`, `pid→PID`, `tty→TTY`,
  `ansi→ANSI`, `ui→UI`, `cli→CLI`, `os→OS`; everything else is title-cased
  per word (`cwd→Cwd`, `ms→Ms`, `px→Px`, `data_base64→DataBase64`,
  `argv0→Argv0`).
- Enums: `type AgentStatus string` with constants such as
  `AgentStatusIdle AgentStatus = "idle"`. `-`, `_` and `.` in values are word
  boundaries (`recent_unwrapped→RecentUnwrapped`, `top-left→TopLeft`,
  `antigravity_cli→AntigravityCLI`).
- `ResponseResult` variants: `<Pascal(type)>Response`, for example
  `pane_info→PaneInfoResponse`, `ok→OKResponse`, `pong→PongResponse`. The
  suffix is `Response` rather than `Result` because ten variants carry a
  payload the schema already calls `<Pascal(type)>Result`, such as
  `pane_read→PaneReadResponse` holding a `PaneReadResult`.
- `EventData` variants: `<Pascal(type)>Event`, for example
  `pane_created→PaneCreatedEvent`. Definitions in the `subscription_event`
  section with the same name and shape (`PaneAgentStatusChangedEvent`) merge
  into one type.
- `Subscription` variants: `<Pascal(type)>Subscription` with dots as word
  boundaries, for example
  `pane.agent_status_changed→PaneAgentStatusChangedSubscription`.
- Variants of other discriminated unions: `<UnionName><Pascal(tag)>`, for
  example `LayoutNodePane`, `LayoutNodeSplit`, `PaneMoveDestinationNewTab`,
  `OutputMatchRegex`, `AgentViewFilterAll`, `EventMatchPaneClosed`. The
  discriminator is the property that carries a `const` in every variant
  (usually `type`; `event` for `EventMatch`).
- Methods: `pane.graphics.set→MethodPaneGraphicsSet = "pane.graphics.set"`
  with wrapper `PaneGraphicsSet`.

### Type mapping

| Schema | Go |
| --- | --- |
| `string` | `string` |
| `integer` with `format` `uint64`/`uint32`/`uint16`/`int32` | matching Go type |
| `integer` with `format` `uint` | `uint64` |
| `integer` without format | `int64` |
| `number` | `float64` |
| `boolean` | `bool` |
| `array` | `[]T` |
| `object` with `additionalProperties` | `map[string]T`; `map[string]*T` when values are nullable |
| `true` (any value, e.g. `agent_explain.explain`) | `json.RawMessage` |
| `oneOf`/`anyOf` with a discriminator | sealed interface plus variant structs (below) |
| union without a discriminator | handwritten type, see "Handwritten complements" |

Optional and nullable fields:

| Case | Go field |
| --- | --- |
| required, not nullable | `T` |
| required, nullable | `*T` |
| optional `string` or enum, not nullable | `T` with `omitempty` |
| optional `bool`/integer/float/struct, not nullable | `*T` with `omitempty` (`strip_ansi` defaults to true; a value type could not express an explicit false) |
| optional, nullable | `*T` with `omitempty` (serde treats null and absent identically for `Option` fields, so an explicit null is never needed) |
| optional array/map | `[]T` / `map[string]T` with `omitempty` |

Discriminated unions are generated as:

```go
type LayoutNode interface{ isLayoutNode() }
type LayoutNodePane struct{ ... }                       // without the discriminator field
func (LayoutNodePane) isLayoutNode() {}
func (v LayoutNodePane) MarshalJSON() ([]byte, error)   // injects "type":"pane"
func decodeLayoutNode(data []byte) (LayoutNode, error)  // reads the discriminator, decodes the variant
```

Structs with union-typed fields (including slices and pointers of them) get an
`UnmarshalJSON` that first decodes into an auxiliary struct holding the union
fields as `json.RawMessage`, then calls the matching `decodeX` per field.

### Results and events

```go
type Result interface{ ResultType() string }          // the "type" constant; variants are <Pascal(type)>Response
func DecodeResult(raw json.RawMessage) (Result, error) // by "type"; unknown types yield *UnknownResultError

type EventKind string                                  // underscore form, constants like EventKindPaneCreated
func (k EventKind) DotName() string                    // first underscore becomes a dot, matching herdr EventKind::dot_name
type Event interface{ EventName() string }             // dotted name, e.g. "pane.created", "pane.output_matched"
type EventEnvelope struct {
    Event EventKind `json:"event"`
    Data  Event     `json:"data"`
}
func DecodeEvent(event string, data json.RawMessage) (Event, error)
func (s *Stream) NextEvent(ctx context.Context) (Event, error)   // Next followed by DecodeEvent
```

`DecodeEvent`: when `event` contains a dot, decode directly by the three
`subscription_event` names; otherwise decode the `EventData` variant selected
by `data.type`. Unknown names yield `*UnknownEventError` carrying the raw data.

### Method wrappers

The shape follows `method-results.json`:

```go
// {"result":"pane_info"}
func (c *Client) PaneGet(ctx context.Context, params PaneTarget) (*PaneInfoResponse, error)
// params of type EmptyParams or PingParams are omitted from the signature
func (c *Client) Ping(ctx context.Context) (*PongResponse, error)
// {"results":[...]} or "*"
func (c *Client) PluginPaneOpen(ctx context.Context, params PluginPaneOpenParams) (Result, error)
// {"stream":"subscription_started"}
func (c *Client) EventsSubscribe(ctx context.Context, params EventsSubscribeParams) (*Stream, error)
```

Single-result wrappers are implemented uniformly as `CallRaw` →
`DecodeResult` → type assertion; a failed assertion returns
`*UnexpectedResultError{Method, Want, Got}`.

### Handwritten complements (`unions_manual.go`)

Unions without a discriminator are handwritten types implementing
`MarshalJSON`/`UnmarshalJSON`; the generator skips these definitions via its
configuration and references the handwritten names: `PopupSize` (integer or a
`"80%"` string), `AgentViewValue` (string | bool | uint64 | `{"context":...}`),
`AgentViewField` and `AgentViewSortField` (built-in enum | `{"token":...}`).

### Generator (`cmd/herdr-apigen`, `internal/gen`)

- Implements only the JSON Schema subset the schema uses: `type` (string or
  array), `properties`, `required`, `$ref`, `oneOf`/`anyOf`, `enum`, `const`,
  `items`, `additionalProperties`, `format`, `description`, `default`. Any
  other keyword is an error, never silently ignored.
- Output is deterministically ordered and formatted with `go/format`.
  `description` values become godoc.
- Validation: `method-results.json` must cover every method in the schema and
  must not reference unknown methods or result types.
- Tests: unit tests for the naming and mapping rules; a golden test on the real
  schema; round-trip decoding tests in the root package using responses
  captured from a live server under `testdata/`.
- No third-party dependencies.

## Graphics streaming

`pane.graphics.stream` keeps its connection open and sends binary frames, so
`graphics.go` implements it by hand on the transport's own helpers:

```go
func (c *Client) PaneGraphicsStream(ctx context.Context, params PaneGraphicsStreamParams) (*GraphicsStream, error)
func (s *GraphicsStream) SendFrame(ctx context.Context, frame GraphicsFrame) error
func (s *GraphicsStream) SendFileFrame(ctx context.Context, frame GraphicsFileFrame) (*PaneGraphicsFrameAckResponse, error)
func (s *GraphicsStream) Wait(ctx context.Context) error
func (s *GraphicsStream) Close() error
```

An inline frame is one JSON header line followed by exactly `data_length`
raw bytes and draws no reply, which is why `SendFrame` returns only an error.
A file frame names an immutable file the terminal reads itself, and the
server answers it with `pane_graphics_frame_ack` once the terminal has
accepted it. That variant is the only one in `method-results.json` that no
method returns, which is consistent with it belonging here. Closing the
connection clears the layer, and the server ends the stream on an error
response or on a frame that stalls.

`OpenStream` cannot serve this: it hands the connection to a reader goroutine
that never writes, and a frame stream has to keep writing. The open sequence
is therefore repeated in `graphics.go` over the same package helpers.

What the fake server proves is the framing: two frames in sequence parse only
if the first body was consumed whole. Against a live server only the entry
point was checked, read-only, by opening a stream for a pane id that cannot
exist and getting `pane_not_found`, which shows the method is accepted and
reaches its handler. Frame acceptance, acknowledgements, timeouts and the
error codes are backed by the herdr sources rather than by execution, because
exercising them means drawing into a real pane.

## Session mirror

```go
func (c *Client) Subscribe(ctx context.Context, subs ...Subscription) (*EventStream, error)
func (s *EventStream) Next(ctx context.Context) (Event, error)

func MirrorSubscriptions() []Subscription
func OpenSession(ctx context.Context, c *Client, subs ...Subscription) (*Session, error)
func (s *Session) Workspaces() []WorkspaceInfo
func (s *Session) Tabs() []TabInfo
func (s *Session) Pane(paneID string) (PaneInfo, bool)
func (s *Session) Agents() []AgentInfo
func (s *Session) Layout(tabID string) (PaneLayoutSnapshot, bool)
func (s *Session) Next(ctx context.Context) (Event, error)
func (s *Session) Close() error
```

A subscription starts when the server accepts it and never replays what came
before, so a client that wants complete state has to subscribe first and take
the snapshot second. `OpenSession` does exactly that: it subscribes, buffers
what arrives, calls `session.snapshot`, installs it, applies the buffer in
order and then keeps streaming. Applying an event twice has to be safe for
that to work, which it is because every handler assigns state rather than
adjusting it.

The cache advances only as `Next` delivers, so reading an accessor after
`Next` shows the state that event produced. Accessors copy what they return
and the mirror is safe for concurrent readers.

A server restart, which is what live handoff does, ends the stream.`Session`
reconnects, bootstraps again, and reports the gap as one `*ResyncEvent` so a
caller can drop anything it derived from the older state. Backoff is bounded
by the context. A response that the server refuses, rather than a connection
that failed, is not retried.

Two behaviours were settled by experiment rather than by reading the schema.
Focus is exclusive across the session and herdr emits the whole chain, so
focusing a workspace also produces `tab.focused` and `pane.focused`; the
mirror can treat focus as a single flag without lagging. And `released` on
`pane.agent_detected` means the agent handed the pane back to the shell, which
`src/events.rs` calls `AppEvent::HookAgentReleased`, so the mirror drops the
agent and keeps its final status.

## Package `plugin`

```go
func Load() (*Env, error)
func LoadFrom(lookup func(string) (string, bool)) (*Env, error)
func (e *Env) Kind() EntryKind
func (e *Env) Client(opts ...herdr.Option) *herdr.Client
func (e *Env) Context() (*herdr.PluginInvocationContext, error)
func (e *Env) EventEnvelope() (*herdr.EventEnvelope, error)

type Handlers struct {
    Startup func(context.Context, *Env) error
    Action  func(context.Context, *Env, string) error
    Event   func(context.Context, *Env, *herdr.EventEnvelope) error
    Pane    func(context.Context, *Env, string) error
}
func Run(ctx context.Context, h Handlers) int
```

`Env` is the environment Herdr injects into a plugin command. `Kind` reports
which manifest entrypoint started the process: a startup hook sets
`HERDR_PLUGIN_EVENT` to the literal `startup`, an event hook sets it to a
dotted event name, an action sets `HERDR_PLUGIN_ACTION_ID`, and a pane command
sets `HERDR_PLUGIN_ENTRYPOINT_ID`.

`ShutdownContext` cancels a context when Herdr asks the process to stop, which
a pane entrypoint needs because it runs until the user closes the pane and
nothing else tells it to finish. Closing a pane delivers SIGHUP and then
SIGTERM to the process in it. That was measured against a running server, by
opening a pane on a trapping script through `layout.apply` and closing it with
`pane.close`, not inferred: herdr has no explicit kill on that path, and the
plan had wrongly guessed SIGINT. `Run` installs nothing itself, so a plugin
opts in by passing the context.

`Run` dispatches by kind and returns a process exit code: `ExitOK` when the
handler returned nil, `ExitHandlerError` when it returned an error, and
`ExitRuntimeError` when no handler ran at all. Herdr records the exit status
in its plugin command log, so those two failures are worth telling apart. A
missing handler for the invoked kind is a configuration error rather than a
silent success. Errors are written to stderr, which Herdr captures in the same
log.

## Package `plugin/manifest`

`Parse`, `Decode` and `Validate` mirror what herdr does with
`herdr-plugin.toml`, down to the error codes, the 120-character id limit and
the rule that trims surrounding whitespace before judging a value. Warnings
come back separately from errors, as they do in herdr: an unknown event name
is a warning, a duplicate action id is an error.

The event names a hook may reference are the 22 in `PLUGIN_HOOK_EVENT_KINDS`,
not all 26 `EventKind` values. Herdr excludes `workspace.metadata_updated`,
`pane.updated`, `pane.output_changed` and `layout.updated` and never fires a
hook for them, so naming one is a warning here too.

`github.com/BurntSushi/toml` is the module's only third-party dependency, and
only this package uses it.

## Error codes

`errors.go` names the 35 codes seen so far: those read out of `encode_error`
callers in the herdr sources, and 13 more that `internal/e2e` met while
exercising the methods that report them. `IsCode` matches through wrapping,
and comparing `Code` against a plain string keeps working for a code a newer
server adds.

## Testing

Unit tests cover the transport against a fake server that behaves exactly as
herdr does, the generator's rules and its output, the mirror's bootstrap
ordering and reconnection, the plugin environment and dispatch, and manifest
validation with a fixture per rule. Decoding is also checked against responses
captured from a real server, sanitised, under `testdata`.

`internal/e2e` runs behind the `e2e` build tag against a server it starts
itself: `herdr --session <name> server` under a temporary `XDG_CONFIG_HOME`,
stopped in cleanup. It never touches the caller's session. Its purpose is to
prove `schema/method-results.json`, which the schema does not state and which
was derived by reading herdr's handlers: every reachable method is called
through its generated wrapper and its result type asserted, so a wrong mapping
fails as a decode or assertion error. It currently exercises 90 of the 102
methods with no disagreements, and the coverage list is checked against the
schema so a method can neither disappear nor go unexplained unnoticed.

The eight plugin methods are among them. Linking a fixture manifest written
inside the harness's own temporary directory reaches the registry and the
plugin pane methods, because the registry follows `XDG_CONFIG_HOME`; the
suite asserts that the caller's real registry is unchanged across the run,
comparing contents rather than modification time, because a live server
rewrites that file with identical contents.

`just check` runs build, tests, lint and the generated-code check. `just e2e`
runs the suite above. `just herdr-check` reports drift from the snapshot.

## Known gaps

`pane.graphics.stream` is the one method the server accepts that the schema
does not declare, so no wrapper is generated for it. Comparing the method list
the server reports in an `invalid_request` error against the schema snapshot
of herdr 0.9.0 shows that single difference; every other method the server
accepts is generated. The method is absent from the schema because its framing
is not newline-delimited JSON: after the server acknowledges the request, the
client sends one JSON header followed by exactly `data_length` raw bytes per
frame. Supporting it means a hand-written streaming type next to the
transport, not a generated wrapper. See "Not built yet".

Rerun that comparison after a schema refresh: a method that appears in the
error list but not in the snapshot is a method this module cannot reach.

## Where the server is narrower than the schema

The schema states what a request may contain, not what the server accepts, so
two methods take arguments the schema permits and herdr 0.9.0 refuses.
`internal/e2e` found both.

`events.wait` accepts every `EventMatch` variant in the schema, but 0.9.0
matches only pane agent status; any other variant returns
`unsupported_event_wait_match`. Wait on other events with `events.subscribe`
instead.

`worktree.create` without a `path` puts the checkout under the calling user's
home, at `~/.herdr/worktrees/<repo>/<branch>`, not relative to `cwd`. Pass an
explicit `path` when the location matters, which is what the e2e suite does so
that it stays inside its temporary directory.

## Following a new herdr release

Three kinds of change arrive with a release, and they are found in different
ways. Run `just herdr-check` first: it compares the installed binary's schema
and the running server against the snapshot, and exits non-zero on any
difference that `schema/known-gaps.json` does not already account for.

**Changes the schema describes** are handled by regenerating. `just
schema-update && just gen && just check` rewrites the snapshot and the
generated code. The generator fails when a method in the schema has no entry
in `schema/method-results.json` and when that file names a method or result
type that no longer exists, so an added or removed method cannot pass
silently. A changed field type becomes a compile error in whatever uses it. A
new enum value simply appears; decoding an unknown one still works because the
enums are strings.

**Which result type a method returns** is not in the schema, so a change there
would leave `method-results.json` quietly wrong. `internal/e2e` is the guard:
it calls each reachable method through its generated wrapper and asserts on the
decoded result type, so a changed mapping fails as a decode or assertion error.
Run `just e2e` after regenerating.

**Behaviour the schema does not describe** is the part with no automatic
guard. Each item below was read out of the herdr sources at v0.9.0 and has to
be re-read when the version this module targets changes. The file is the place
to look, not a guarantee it still exists.

| Fact | Where it came from |
| --- | --- |
| One request per connection; only `events.subscribe` keeps the connection open | `src/api/server.rs`, `handle_connection` and `stream_subscriptions` |
| Socket path resolution and the session name rules | `src/session.rs`, `api_socket_path_for` and `validate_name` |
| The config directory chain, including `herdr-dev` for debug builds | `src/config/io.rs`, `config_dir`, `platform_config_dir` and `app_dir_name` |
| The environment injected into plugin commands | `src/app/api/plugins/runtime.rs` and `src/app/api/plugins/panes.rs` |
| Manifest validation rules, limits and error codes | `src/app/api/plugins/manifest.rs` |
| The 22 events a manifest hook may name, narrower than the 26 `EventKind` values | `src/api/schema/events.rs`, `PLUGIN_HOOK_EVENT_KINDS` |
| Popup size parsing, integer or percentage | `src/popup_size.rs` |
| `released` on `pane.agent_detected` meaning the agent handed the pane back | `src/events.rs`, `AppEvent::HookAgentReleased` |
| The set of error codes | `encode_error` callers across `src/app/api/` |

The last column is why `schema/README.md` records the version a snapshot was
taken from: an upgrade means re-reading those files at the new tag, not
guessing from behaviour.

## The plugin authoring layer

Writing `examples/agent-status`, the first real plugin on this module, showed
what the library still left to the author. Each piece below closes a gap that
example had hand-rolled, which is why the rewrite onto the layer cut its
`main.go` from 201 lines to 112 and its manifest test from 56 to 13.

### Registering by id instead of switching on one

```go
p := plugin.New()
p.Startup(onStartup)
p.Action("show", onShow)
p.Pane("board", onBoard)
plugin.OnEvent(p, onStatusChanged)   // func(context.Context, *plugin.Env, *herdr.PaneAgentStatusChangedEvent) error
os.Exit(p.Run(ctx))
```

`OnEvent` is a free function rather than a method because it takes a type
parameter: the generated event types implement `EventName`, so the name comes
from the handler's own argument and the author never writes the string. A
payload shared with a subscription-only event is rejected at registration,
since the name would be ambiguous.

Registration mistakes panic rather than surfacing at dispatch: a nil handler,
an empty id, or a second registration for an id already taken. Registration is
a program's static description of itself, and a silent overwrite would make
`CheckManifest` agree with a manifest the binary does not serve. `Handlers`
and the original `Run` still work, on the same dispatch.

### Checking the manifest against the code

```go
func TestManifest(t *testing.T) {
    plugintest.CheckManifest(t, "herdr-plugin.toml", newPlugin())
}
```

Herdr validates the manifest, but nothing tied its ids to the handlers a
binary serves, so a renamed action failed at invocation time rather than in a
test. The registry knows every id, so one call reports an id declared with no
handler, a handler with no manifest entry, and an `[[events]] on` value herdr
never fires a hook for.

### Testing a handler without Herdr

```go
env := plugintest.Env(plugintest.Action("show"), plugintest.Workspace("w1"))
```

`plugin/plugintest` builds an `Env` directly, with options for the entrypoint
kind, the invocation context, the event payload and the two directories, so a
handler test sets no environment variables. It is a separate package so that
importing it cannot pull test-only code into a plugin binary.

### Reading the context without pointer checks

`Env.Invocation` returns the invocation context with its optional fields
flattened to values: a field Herdr did not send reads as the empty string.
It reports no error, because an entrypoint invoked without a context and a
context that fails to decode leave a plugin reading one field with nothing
different to do; `Env.Context` keeps the pointer form for when the difference
matters. `Worktree` stays a pointer, having no useful empty value.

### Owning state without the file plumbing

`StatePath`, `ConfigPath`, `ReadState`, `WriteState`, their JSON forms and
`AppendStateJSONL` share one write path: a temporary file in the same
directory, then a rename, so a crash mid-write cannot truncate what was there.
A name that would escape the directory is rejected. That atomicity is the
reason this belongs in the library rather than in each plugin.

### What is deliberately not included

No wrapper for multi-step flows such as "split a pane, run a command, wait
for output". Those are two or three generated calls and the useful shape
differs per plugin; a wrapper would guess wrong and hide the calls that
matter. No logging helper either: Herdr already captures stdout and stderr
into its command log, so the standard library is enough.

## Not built yet

**The last 12 methods.** `agent.start`, `agent.prompt` and `agent.send_keys`
need a real agent process in the pane; a machine with a supported agent CLI
could cover them and one without would skip. `client_shell.surface.set`,
`command.invoke`, `popup.close` and `pane.graphics.info` need an attached
client or the client shell endpoint. `product_announcement.dismiss` and
`release_notes.dismiss` need state a fresh server does not have.
`integration.install` and `integration.uninstall` write outside the
temporary config home, into the user's own agent configuration.
`server.live_handoff` would take down the suite's own server.

**A tagged release.** There is none, so `go get` resolves a pseudo-version of
the latest commit.
