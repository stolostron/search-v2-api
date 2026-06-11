# search-v2-api

ACM Search API. Reads from the shared PostgreSQL database written by search-indexer RBAC and serves queries with a GraphQL endpoint.

For system architecture, data flows, and module layout, see [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).

## Commands

```bash
make setup          # Generate TLS cert + print DB env var instructions (run once before first local run)
make run            # Run locally (enables playground and dev feature flags automatically)
make test           # Unit tests
make test-race      # Unit tests with race detector
make coverage       # Unit tests with HTML coverage report (runs test first)
make docker-build   # Build Docker image
make lint           # Run golangci-lint + gosec (downloads golangci-lint if not present)
make gqlgen         # Regenerate GraphQL code from schema (run after editing graph/schema.graphqls)
make send QUERY=schema          # Send a test GraphQL request (values: schema, search, searchComplete, searchCount, searchRelated, vm)
```

## Required environment variables

The service exits on startup if these are missing:

| Variable | Description |
|---|---|
| `DB_NAME` | PostgreSQL database name |
| `DB_USER` | PostgreSQL user |
| `DB_PASS` | PostgreSQL password |

`make setup` prints the `oc` commands to extract these from a live cluster and the port-forward command to reach it locally.

Optional overrides (defaults): `DB_HOST=localhost`, `DB_PORT=5432`, `HTTP_PORT=4010`, `CONTEXT_PATH=/searchapi`.

## Non-obvious conventions

- **`-tags development` build tag** enables development mode: sets `DevelopmentMode=true` and enables `FEATURE_FEDERATED_SEARCH`, `FEATURE_FINE_GRAINED_RBAC`, and `FEATURE_SUBSCRIPTION` by default. `make run` uses this tag automatically.
- **`PLAYGROUND_MODE=true`** is set by `make run` — opens the GraphQL playground at `https://localhost:4010/playground`. Not enabled in production.
- **`graph/generated/` is generated code** — never edit manually. Run `make gqlgen` after any change to `graph/schema.graphqls`.
- **TLS cert required** even locally. `make setup` generates a self-signed cert at `sslcert/tls.crt` + `sslcert/tls.key`.
- **Port 4010** (not 3010, which is search-indexer).
- **Federated search** is off by default (`FEATURE_FEDERATED_SEARCH=false`). It requires a `ManagedHubConfig` in the cluster and routes requests to remote hub search APIs. Enabled automatically in dev mode.
- **`make coverage` depends on `make test`** — it reads `cover.out` produced by the test target rather than re-running tests.
- **`make lint` excludes `graph/generated/`** via `-exclude-dir=graph/generated` in the gosec invocation.

## Fleet Engineering Skills

All skills are available as slash commands. See the [Fleet Engineering skills catalog](https://github.com/OpenShift-Fleet/agentic-sdlc/blob/main/skills/README.md) for the full list with when-to-use guidance.

## Personal configuration

Read personal config at the start of any task that needs an assignee, email, or project key.
Use the tool-aware fallback chain: `~/.config/opencode/user.local.md` (OpenCode),
`.claude/user.local.md` (Claude Code), or `.cursor/rules/user.local.mdc` (Cursor, already in context).
If none exist, fall back to agent memory (`user-config`), then placeholders.
Run `make personalize` to generate all three files (if this repo uses Fleet Engineering tooling).

