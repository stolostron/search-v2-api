# search-v2-api Architecture

## Overview

search-v2-api is the read path for ACM Search. It is an HTTPS GraphQL service running in the hub cluster. It reads from the same PostgreSQL database that `search-indexer` writes to and enforces RBAC on every response. Clients (the ACM console and the federated hub) talk exclusively to this service.

```text
ACM Console / CLI
        │  HTTPS POST /searchapi/graphql
        ▼
  search-v2-api ──────────────────────────────► PostgreSQL (read)
        │                                        search.resources
        │  (optional, FEATURE_FEDERATED_SEARCH)  search.edges
        └──────────────► Remote Hub search-v2-api instances
```

## Packages

| Package | Responsibility |
|---|---|
| `main` | Bootstrap: init config, connect DB, start RBAC background validation, start server, wait for SIGINT/SIGTERM |
| `pkg/config` | All configuration from environment variables. `Cfg` is a package-level singleton. Development mode is a build tag (`-tags development`), not an env var. |
| `pkg/server` | HTTPS server on `:4010`. Routes: `/liveness`, `/readiness`, `/metrics`, `/searchapi/graphql` (authenticated), `/federated` (optional), `/playground` (dev only). Applies middleware: timeout, Prometheus, DB availability check, authn, authz. Configures gqlgen handler with GET/POST/WebSocket transports. |
| `pkg/rbac` | RBAC enforcement. TokenReview cache (`AuthCacheTTL`), shared resource cache (`SharedCacheTTL`), per-user namespace permission cache (`UserCacheTTL`). Background goroutine invalidates stale cache entries. |
| `pkg/resolver` | GraphQL resolver implementations: `search`, `searchComplete`, `searchSchema`, `messages`, `watch` (subscription). Translates GraphQL input to SQL via goqu and applies RBAC filtering to results. |
| `pkg/federated` | Federated search: reads `ManagedHubConfig` from the cluster, maintains an HTTP client pool, fans out queries to remote hub APIs, and merges responses. |
| `pkg/database` | PostgreSQL connection pool (`pgxpool`). Also manages the `LISTEN/NOTIFY` listener used by GraphQL subscriptions. |
| `pkg/metrics` | Prometheus registry and `PrometheusMiddleware`. |
| `graph/` | gqlgen schema (`schema.graphqls`), generated code (`generated/`), and resolver wiring (`resolver.go`, `schema.resolvers.go`). Never edit `generated/` by hand — run `make gqlgen` after schema changes. |

## GraphQL API surface

| Operation | Type | Description |
|---|---|---|
| `search(input)` | Query | Search for resources and their relationships. Returns `items`, `count`, `related`. |
| `searchComplete(property, query, limit)` | Query | All distinct values for a property, optionally filtered. |
| `searchSchema(query)` | Query | All indexed property names, optionally filtered. |
| `messages` | Query | Service-level status messages (e.g. DB unavailable). |
| `watch(input)` | Subscription | Real-time stream of INSERT/UPDATE/DELETE events matching the filter. Delivered over WebSocket. |

Filters support operators (`=`, `!`, `!=`, `>`, `>=`, `<`, `<=`), wildcard (`*`), and datetime shortcuts (`hour`, `day`, `week`, `month`, `year`). Multiple values within a filter are OR'd; multiple filters are AND'd.

## Key data flows

### Standard query (`search`, `searchComplete`, `searchSchema`)

1. Request hits `/searchapi/graphql` and passes through middleware in order: timeout → Prometheus → DB availability → authn (TokenReview) → authz (RBAC namespace lookup).
2. `pkg/rbac.AuthorizeUser` populates the request context with the user's allowed namespaces and cluster-scoped resources.
3. Resolver (`pkg/resolver/search.go`) builds a SQL query against `search.resources` / `search.edges` using goqu, inlining the RBAC namespace allowlist as a `WHERE` clause.
4. Results are returned directly — no further post-filtering.

### GraphQL subscription (`watch`)

1. Client opens a WebSocket to `/searchapi/graphql`.
2. On subscribe, the resolver registers a listener channel with `pkg/database`'s PostgreSQL `LISTEN/NOTIFY` listener.
3. The DB listener receives `NOTIFY` events from `listenerTrigger.sql` (a trigger on `search.resources`) and broadcasts change payloads.
4. Each event is RBAC-filtered before being sent to the client.
5. Subscriptions are bounded by `SUBSCRIPTION_MAX_ACTIVE`, `SUBSCRIPTION_MAX_LIFETIME`, and `SUBSCRIPTION_IDLE_TIMEOUT`.

### Federated search (`/federated`)

1. Only active when `FEATURE_FEDERATED_SEARCH=true`.
2. `federated.HandleFederatedRequest` reads the `ManagedHubConfig` ConfigMap (cached, `FEDERATION_CONFIG_CACHE_TTL`) to discover remote hub API endpoints.
3. The incoming GraphQL request is proxied concurrently to all registered remote hubs via an HTTP client pool.
4. Responses are merged and returned as a unified result (`pkg/federated/mergeResponses.go`).

## RBAC cache layers

Three independently-TTL'd caches protect the Kubernetes API server:

| Cache | Default TTL | What it stores |
|---|---|---|
| Auth (TokenReview) | 1 min (`AUTH_CACHE_TTL`) | Whether a bearer token is valid and which user it belongs to |
| Shared | 5 min (`SHARED_CACHE_TTL`) | Cluster-scoped resources visible to all users |
| User | 5 min (`USER_CACHE_TTL`) | Per-user namespace list and resource permissions |

A background goroutine (`StartBackgroundValidation`) periodically re-validates entries and evicts stale ones.

## Feature flags

| Env variable | Default | Effect |
|---|---|---|
| `FEATURE_FEDERATED_SEARCH` | `false` (dev: `true`) | Enables `/federated` endpoint and federated query fan-out |
| `FEATURE_FINE_GRAINED_RBAC` | `false` (dev: `true`) | Enables per-resource-type RBAC filtering beyond namespace-level |
| `FEATURE_SUBSCRIPTION` | `true` | Enables the `watch` GraphQL subscription over WebSocket |
| `PLAYGROUND_MODE` | `false` (set by `make run`) | Exposes `/playground` GraphQL UI |
| `API_DOCUMENTATION` | `false` | Enables GraphQL introspection (schema documentation endpoint) |

## Design decisions

- **gqlgen code generation**: The GraphQL schema (`graph/schema.graphqls`) is the source of truth. `make gqlgen` regenerates `graph/generated/` and stubs in `graph/schema.resolvers.go`. Resolver logic lives in `pkg/resolver/`, separate from the generated wiring.
- **RBAC inlined into SQL**: Rather than fetching all results and filtering in Go, the allowed namespace list is passed directly into the SQL `WHERE` clause. This avoids loading data the user cannot see.
- **PostgreSQL LISTEN/NOTIFY for subscriptions**: Avoids polling. A SQL trigger fires `NOTIFY` on every insert/update/delete in `search.resources`; the Go listener receives these and fans out to WebSocket subscribers.
- **Federated response merging**: Remote hub responses are merged in memory. Deduplication is by resource UID.
- **No ORM**: Raw SQL via `pgx`/`goqu`, consistent with search-indexer.
