# herdr-api

Go client for the [Herdr](https://herdr.dev) socket API and a toolkit for
writing Herdr plugins in Go.

Status: under development. The public API is not stable yet.

## Layout

- Package `herdr` (module root): socket transport plus the generated wire
  types, result and event decoders, and typed method wrappers.
- `plugin`, `plugin/manifest`: plugin process environment and
  `herdr-plugin.toml` handling.
- `cmd/herdr-apigen`, `internal/gen`: the generator that produces `*_gen.go`
  from `schema/herdr-api.schema.json`.
- `schema/`: schema snapshot (herdr 0.9.0, protocol 22) and the method to
  result-type table.

The design and the generation rules are in [docs/design.md](docs/design.md).

## Development

```bash
just check          # build, test, lint, verify generated code is current
just gen            # regenerate from schema/
just schema-update  # refresh the schema snapshot from the installed herdr
```
