# Schema snapshot

`herdr-api.schema.json` is the unmodified output of `herdr api schema --json`
from the herdr binary recorded below. It is the single source for every
generated Go type and method wrapper in this module.

| Field | Value |
| --- | --- |
| herdr version | 0.9.0 |
| `protocol` | 22 |
| `schema_version` | 1 |

`method-results.json` maps each request method to the `ResponseResult`
variant(s) it returns. The schema does not carry this relation, so the table is
maintained by hand and verified against the herdr sources for the version above.
The generator refuses to run when a method in the schema has no entry, or when
an entry names an unknown method or result type.

## Refreshing

```bash
just schema-update          # rewrites herdr-api.schema.json from the installed herdr
just gen                    # regenerates *_gen.go
```

After a refresh, update the version table above, add entries for new methods to
`method-results.json`, and run `just check`.
