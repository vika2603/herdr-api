# Module design

This document is the architecture contract for `github.com/vika2603/herdr-client`
and the interface agreement between the parts that are developed in parallel.
Every statement about the protocol is backed by `schema/herdr-client.schema.json`
(herdr 0.9.0, protocol 22) or by the herdr v0.9.0 sources.

## Goal

Provide a Go client for Herdr: the socket API, a live mirror of the session,
and the runtime helpers a Herdr plugin written in Go needs. The module started
as an API wrapper and was renamed from herdr-api to herdr-client once it grew
past that. Types and method wrappers are generated from the official
schema. Handwritten code covers only what the schema cannot express: the
transport, the plugin process environment, and a few unions without a
discriminator field.

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

## Layout and ownership

| Path | Content | Phase 1 branch |
| --- | --- | --- |
| `client.go` `stream.go` `dial_unix.go` `dial_windows.go` `socketpath.go` `errors.go` and their tests | Handwritten transport | `feat/transport` |
| `*_gen.go` `unions_manual.go` `generate.go`, tests for generated types, `testdata/` | Generated code and its handwritten complements | `feat/gen` |
| `cmd/herdr-clientgen/` `internal/gen/` | Generator | `feat/gen` |
| `schema/` | Schema snapshot, method result table | `feat/gen` (may only correct `method-results.json`) |
| `plugin/` `plugin/manifest/` | Plugin runtime, manifest parsing | `feat/plugin` |
| `internal/e2e/` | Tests against a live Herdr session (build tag `e2e`) | Phase 2 |
| `examples/` | Example plugins | Phase 2 |

File ownership does not overlap. The generator branch must not modify the
transport files; the transport branch must not depend on any generated type.
Generated code depends only on the transport signatures frozen below.

## Root package `herdr`: transport API (frozen)

```go
type Client struct{ /* private */ }
type Option func(*Client)

func New(socketPath string, opts ...Option) *Client          // no I/O
func NewFromEnv(opts ...Option) (*Client, error)             // see ResolveSocketPath("")
func ResolveSocketPath(session string) (string, error)
func WithDialTimeout(d time.Duration) Option
func WithRequestIDs(next func() string) Option

func (c *Client) SocketPath() string
func (c *Client) Call(ctx context.Context, method string, params, result any) error
func (c *Client) CallRaw(ctx context.Context, method string, params any) (json.RawMessage, error)
func (c *Client) OpenStream(ctx context.Context, method string, params any) (*Stream, error)

type Stream struct{ /* private */ }
type RawEvent struct {
    Event string          `json:"event"`
    Data  json.RawMessage `json:"data"`
}
func (s *Stream) Ack() json.RawMessage                       // result object of the opening response
func (s *Stream) Next(ctx context.Context) (*RawEvent, error)
func (s *Stream) Close() error

type Error struct{ Method, Code, Message string }
func (e *Error) Error() string
func IsCode(err error, code string) bool
var ErrStreamClosed error
```

Behaviour:

- `Call`/`CallRaw`: dial, write one line `{"id","method","params"}` (a `nil`
  `params` is sent as `{}`), read one line. If the `error` field is present
  return `*Error` with `Method` set; otherwise decode `result` into `result`
  (discard when `nil`). Context cancellation and deadlines unblock the call by
  closing the connection and are reported as `ctx.Err()`.
- `ResolveSocketPath(session)`: with a non-empty `session` return
  `<config>/sessions/<session>/herdr.sock`; otherwise consult
  `HERDR_SOCKET_PATH`, then `HERDR_SESSION`, then the default
  `<config>/herdr.sock`. `<config>` is `$XDG_CONFIG_HOME/herdr` or
  `~/.config/herdr`; on Windows follow `src/server/socket_paths.rs` in herdr.
- `OpenStream`: send the request and read the first line. On `error` return
  `*Error`. On success keep `result` for `Ack()`, then `Next` decodes each
  further line as a `RawEvent`. After the connection closes, `Next` returns
  `ErrStreamClosed` (possibly wrapping the underlying error).
- Default request ids look like `herdr-go-<sequence>`.
- The transport has no third-party dependencies. Windows named pipes are opened
  with `os.OpenFile` on `\\.\pipe\` + path.

## Root package `herdr`: generated code contract

Generator inputs: `schema/herdr-client.schema.json` and
`schema/method-results.json`. All output files live in the root package and
start with `// Code generated by herdr-clientgen. DO NOT EDIT.`:

| File | Content |
| --- | --- |
| `schema_gen.go` | `const SchemaProtocol uint32 = 22` |
| `types_gen.go` | Every `$defs` entry: params, info structs, enums, discriminated unions |
| `results_gen.go` | `Result` interface, the 64 result variants, `DecodeResult` |
| `events_gen.go` | `EventKind`, `Event` interface, event structs, `DecodeEvent`, `EventEnvelope` |
| `methods_gen.go` | `Method*` constants and the `(*Client)` wrappers |

`generate.go` carries
`//go:generate go run ./cmd/herdr-clientgen -schema schema/herdr-client.schema.json -methods schema/method-results.json -out .`.

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

### Handwritten complements (`unions_manual.go`, owned by `feat/gen`)

Unions without a discriminator are handwritten types implementing
`MarshalJSON`/`UnmarshalJSON`; the generator skips these definitions via its
configuration and references the handwritten names: `PopupSize` (integer or a
`"80%"` string), `AgentViewValue` (string | bool | uint64 | `{"context":...}`),
`AgentViewField` and `AgentViewSortField` (built-in enum | `{"token":...}`).

### Generator (`cmd/herdr-clientgen`, `internal/gen`)

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

## Package `plugin`

```go
type Env struct {
    PluginID, PluginRoot, ConfigDir, StateDir string
    SocketPath, BinPath                      string
    WorkspaceID, TabID, PaneID               string   // may be empty
    ActionID, EntrypointID                   string   // present depending on the entrypoint kind
    Event                                    string   // dotted event name; "startup" for startup hooks
    ContextJSON, EventJSON                   []byte   // raw JSON, may be empty
    ClickedURL, LinkHandlerID                string
}
func Load() (*Env, error)                 // fails when HERDR_ENV != "1" or HERDR_PLUGIN_ID is missing
func LoadFrom(lookup func(string) (string, bool)) (*Env, error)
func (e *Env) Kind() EntryKind            // Startup | Action | Event | Pane
func (e *Env) Client(opts ...herdr.Option) *herdr.Client
```

Phase 2, once the generated types are merged: `(e *Env) Context()
(*herdr.PluginInvocationContext, error)`, `(e *Env) EventEnvelope()
(*herdr.EventEnvelope, error)`, and `Run(ctx, Handlers) int` dispatching by
entrypoint kind.

## Package `plugin/manifest`

Models and validates `herdr-plugin.toml`: top-level `id`, `name`, `version`
and `min_herdr_version` are required; `id` uses ASCII letters, digits, `.`,
`:`, `_`, `-`; local ids (actions, panes, link handlers) contain no `.` and are
unique within their kind; `platforms` values are `linux`, `macos`, `windows`;
`command` is a non-empty argv; `link_handlers[].action` must reference an
action of the same plugin; `events[].on` is compared against the known dotted
event names and an unknown name produces a warning rather than an error,
matching herdr. TOML parsing may use `github.com/BurntSushi/toml`, the only
third-party dependency permitted for this package.

## Error codes

The schema does not enumerate error codes. The following, taken from the
herdr v0.9.0 sources, are provided as constants in `errors.go` while plain
string comparison remains possible:
`invalid_request`, `invalid_params`, `internal_error`, `timeout`, `not_found`,
`pane_not_found`, `workspace_not_found`, `tab_not_found`, `agent_not_found`,
`plugin_not_found`, `plugin_disabled`, `plugin_pane_not_found`,
`platform_unsupported`, `feature_disabled`, `ui_busy`, `popup_not_open`,
`stream_conflict`, `stream_closed`, `agent_blocked`, `agent_prompt_stalled`,
`workspace_group_close_required`, `unsupported_in_app_mode`.

## Versioning and compatibility

- Generated code decodes known fields only and ignores unknown ones. Unknown
  `type`/`event` values surface as `Unknown*Error` instead of failing.
- To upgrade herdr: replace `schema/herdr-client.schema.json`, complete
  `method-results.json`, regenerate, and run `just check`.
- At runtime, compare the `protocol` returned by `Ping` with `SchemaProtocol`
  to detect a server newer than the generated code.

## Phase 2

Phase 1 delivered the transport, the generated API surface and the plugin
manifest. Phase 2 makes the module usable for writing a real plugin and proves
the generated surface against a running server. The three tracks own disjoint
files and can be developed in parallel.

| Track | Files | Branch |
| --- | --- | --- |
| Plugin runtime | `plugin/` (except `plugin/manifest/`), `examples/` | `feat/plugin-runtime` |
| Session mirror | `session.go`, `subscribe.go` and their tests in the root package | `feat/session` |
| End-to-end verification | `internal/e2e/` | `feat/e2e` |

### Plugin runtime

`plugin.Env` gains typed accessors over the raw JSON it already carries, and a
dispatcher so a single binary can serve every entrypoint a manifest declares:

```go
func (e *Env) Context() (*herdr.PluginInvocationContext, error)
func (e *Env) EventEnvelope() (*herdr.EventEnvelope, error)   // event hooks only

type Handlers struct {
    Startup func(context.Context, *Env) error
    Action  func(context.Context, *Env, string) error          // action id
    Event   func(context.Context, *Env, *herdr.EventEnvelope) error
    Pane    func(context.Context, *Env, string) error          // entrypoint id
}
func Run(ctx context.Context, h Handlers) int
```

`Run` loads the environment, selects the handler by `Env.Kind()`, and returns a
process exit code. A missing handler for the invoked kind is an error, not a
silent success, because Herdr records the exit status in its command log. A
handler's `error` is written to stderr, which Herdr captures in that same log.

`examples/` holds one worked plugin exercising a startup hook, an action and an
event hook through `Run`, with a manifest that
`plugin/manifest` validates in a test.

### Session mirror

Two additions to the root package, both built on `OpenStream`:

```go
func (c *Client) Subscribe(ctx context.Context, subs ...Subscription) (*EventStream, error)
func (s *EventStream) Next(ctx context.Context) (Event, error)

type Session struct{ /* private */ }
func OpenSession(ctx context.Context, c *Client, subs ...Subscription) (*Session, error)
func (s *Session) Workspaces() []WorkspaceInfo
func (s *Session) Pane(paneID string) (PaneInfo, bool)
func (s *Session) Agents() []AgentInfo
func (s *Session) Layout(tabID string) (PaneLayoutSnapshot, bool)
func (s *Session) Next(ctx context.Context) (Event, error)
func (s *Session) Close() error
```

`OpenSession` implements the bootstrap the socket API documents: subscribe
first, buffer what arrives, call `session.snapshot`, install it, then apply the
buffered events in order and keep streaming. Each event updates the cache
before `Next` returns it, so a caller that reads the cache after `Next` sees
the state that event produced. `Session` is safe for concurrent readers.

A server restart, which happens on live handoff, ends the stream. `Session`
reconnects and re-bootstraps rather than reporting the stream as finished, and
surfaces the gap as one synthetic resync event so callers can discard derived
state. Reconnection backs off and stops when its context is done.

### End-to-end verification

`internal/e2e` runs behind the `e2e` build tag against a server the tests own:
`herdr --session <name> server` on a temporary `XDG_CONFIG_HOME`, torn down
with `server.stop`. It never touches the caller's session.

Its purpose is to prove `schema/method-results.json`. Every method the harness
can reach is called and its response decoded through the generated wrapper, so
a wrong result type fails as a decode or assertion error. The suite reports
which of the 102 methods it covered and why each remaining one is out of reach,
for example the graphics and popup methods that need an attached client.

## Known gaps

`pane.graphics.stream` is the one method the server accepts that the schema
does not declare, so no wrapper is generated for it. Comparing the method list
the server reports in an `invalid_request` error against the schema snapshot
of herdr 0.9.0 shows that single difference; every other method the server
accepts is generated. The method is absent from the schema because its framing
is not newline-delimited JSON: after the server acknowledges the request, the
client sends one JSON header followed by exactly `data_length` raw bytes per
frame. Supporting it means a hand-written streaming type next to the
transport, not a generated wrapper, and it is a phase 3 candidate along with
the `pane.graphics.*` helpers that would make it usable.

Rerun that comparison after a schema refresh: a method that appears in the
error list but not in the snapshot is a method this module cannot reach.

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

## Phase 3

Phase 2 left three things undone, each backed by something the work turned up
rather than by speculation.

| Track | Files | Branch |
| --- | --- | --- |
| Graphics streaming | `graphics.go` and its tests in the root package | `feat/graphics` |
| Plugin ergonomics | `plugin/`, `examples/` | `feat/plugin-ergonomics` |
| Agent lifecycle coverage | `internal/e2e/` | continues on `feat/e2e` |

### Graphics streaming

`pane.graphics.stream` is the only method the server accepts that no generated
wrapper reaches, because its framing is not newline-delimited JSON. After the
acknowledgement the client sends one JSON header and then exactly
`data_length` raw bytes per frame, on a connection that stays open. It needs a
handwritten type beside the transport:

```go
func (c *Client) PaneGraphicsStream(ctx context.Context, params PaneGraphicsStreamParams) (*GraphicsStream, error)
func (s *GraphicsStream) SendFrame(ctx context.Context, frame GraphicsFrame) (*PaneGraphicsFrameAckResponse, error)
func (s *GraphicsStream) Close() error
```

`pane_graphics_frame_ack` is the one result variant no method in
`method-results.json` returns, which is consistent with it belonging to this
method. Confirm that against the server rather than assuming it.

### Plugin ergonomics

Two gaps the worked example exposed. `Run` passes the caller's context
straight through, so a pane entrypoint that runs until the user closes it has
no way to shut down cleanly; an option that cancels on SIGINT and SIGTERM
belongs next to it. And a plugin that watches events wants `Session`, which no
example demonstrates.

### Agent lifecycle coverage

`agent.start`, `agent.prompt` and `agent.send_keys` are out of reach for the
current suite because they need a real agent process in the pane. A machine
with a supported agent CLI can cover them; a machine without one skips. That
is the last group of methods with no execution behind them.
