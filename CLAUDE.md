# PowerHive Codebase Review

This document captures a detailed walkthrough of the current PowerHive codebase (as of 2025‑10‑21) to help new contributors or automation agents ramp quickly. It maps the major components, highlights notable implementation details, and records open questions or potential risks uncovered while reading the source.

## Top-Level Layout

- `cmd/automation/` — entry point that wires configuration, database, background services, and the HTTP server.
- `internal/app/` — long-running services: network discovery, status polling, telemetry polling, and application orchestration.
- `internal/config/` — configuration structures and JSON loader (not exhaustively reviewed here, but referenced widely).
- `internal/database/` — SQLite-backed persistence layer (schema creation, CRUD helpers, higher-level store API).
- `internal/firmware/` — typed client for the miner firmware REST API.
- `internal/server/` — HTTP API + static frontend (HTML/CSS/JS bundle) for the dashboard.
- `docs/` — reference material (notably `firmware-api.md`) that describes the firmware endpoints.
- `config.json` — default runtime config (database path, network subnets, poll intervals, HTTP bind address).
- `internal/server/web/` — dashboard assets (`index.html`, `styles.css`, `app.js`).

### External Dependencies

- Go toolchain set to `go 1.25.2` (future version; ensure local toolchain matches).
- Uses `modernc.org/sqlite` for pure-Go SQLite, which pulls a large dependency tree (`modernc.org/cc`, `modernc.org/libc`, etc.).
- Standard library otherwise; no third-party web frameworks.

## Application Bootstrap (`cmd/automation/main.go`)

1. Builds a slog text logger (Level INFO by default).
2. Loads configuration from `config.json` via `internal/config`.
3. Opens the SQLite database (relative path `./data/powerhive.db`) and pings it.
4. Instantiates `database.Store` and migrates schema (`store.Init`).
5. Constructs the application via `app.New`, which bundles:
   - `Discoverer`
   - `StatusPoller`
   - `TelemetryPoller`
   - HTTP server (`internal/server`)
6. Runs all services under a context canceled on SIGINT/SIGTERM, joining goroutines and gracefully shutting down HTTP.

**Observations**
- No command-line flags; config location hard-coded. Consider flag/env overrides if multiple environments are needed.
- Logger defaults to INFO; debugging requires config edits or code change.

## Background Services (`internal/app`)

### Shared Patterns

- Each service holds a reference to `database.Store`, configuration, a slog logger, and an `http.Client`.
- Timeouts derive from config (`network.miner_probe_timeout_ms`, `intervals.*`).
- Worker pools fan out API calls against discovered miners.
- All loops honor cancellation via context.

### Discoverer (`internal/app/discovery.go`)

Responsibilities:
- Enumerate candidate IPs using configured subnets (`cfg.Network.Subnets`).
- Perform lightweight scanning (implementation spans `enumerateHosts`, `lightScan`, etc.) to identify reachable HTTP ports.
- For responsive hosts, instantiate a firmware client and fetch `/api/v1/info` and `/api/v1/model`.
- Persist miner metadata (`store.UpsertMiner` + model presets) and ensure an API key exists (generates a random 16-byte key using firmware `/unlock` + `/apikeys` endpoints).
- Marks miners offline if they disappear from scans by nulling their IPs in DB (`markOffline`).

Notable details:
- Worker counts: 32 for light scan, 8 for deeper probes.
- API key description constant `PowerHive`.
- When generating API keys, uses cryptographic randomness (`crypto/rand`) and hex encoding.
- Discovery updates the `miners` table, populates associated model info, and ensures `managed` retains previous state.

Potential risks / follow-ups:
- Light scan implementation not shown above; verify it handles large networks gracefully and cleans up goroutines on shutdown.
- API key generation implicitly trusts firmware responses; consider rate limiting or retry backoff.

### Status Poller (`internal/app/status_poller.go`)

Responsibilities:
- Poll `/api/v1/summary` and `/api/v1/perf-summary` for each managed miner that has both an IP and API key.
- Parses summary payload (`firmware.SummaryResponse`) into `database.MinerStatusInput` (state, preset, realtime hashrate, fans, chains).
- Persists the snapshot via `store.RecordMinerStatus`, updating `miners.latest_status_id`.

Key behavior:
- Uses worker pool (default 4) with context timeouts.
- Captures preset from `perf-summary`'s `current_preset` JSON blob (`parseCurrentPreset` helper).
- On API errors, logs warnings but continues other miners.

Recent change (this session):
- Removed the requirement that a miner’s model have `max_preset` before polling status. Addresses missing metrics for miners awaiting configuration.

Potential improvements:
- Error handling for partial data (e.g., absent fan entries) appears robust (strings trimmed, nil checks).
- Consider recording firmware version/uptime if available for richer debugging.

### Telemetry Poller (`internal/app/telemetry_poller.go`)

Responsibilities:
- Poll `/api/v1/chains` for managed miners to capture per-chain and per-chip telemetry.
- Persists snapshots via `store.RecordChainTelemetry`.

Current constraints:
- Still requires `model.max_preset` to be set before polling (unlike status poller). Verify this is intentional; otherwise telemetry will lag until configuration completes.
- Similar worker-pool pattern with request timeouts.

Suggestions:
- Align eligibility with the status poller if telemetry is useful pre-configuration.
- Volume of telemetry data can grow quickly; ensure retention/cleanup strategy exists (`chain_snapshots` table size management not yet apparent).

### Utility Helpers (`internal/app/helpers.go`, etc.)

- Contains helpers like `safeString`, `stringPtr`, `parseCurrentPreset`. Judicious trimming of whitespace before storing strings.
- Review helper coverage to avoid duplication between services.

## Firmware Client (`internal/firmware`)

### `client.go`

- Constructs base URLs (`http://<addr>/api/v1`) and handles authenticated requests with either API key header (`x-api-key`) or bearer tokens.
- Methods for key firmware endpoints: `/info`, `/model`, `/status`, `/summary`, `/perf-summary`, `/chains`, `/autotune/presets`, plus unlock and key management.
- `do` method centralizes request execution, JSON encoding/decoding, header population, and error handling.

Observations:
- Timeouts are supplied by callers’ contexts; default client timeout is 5s but most operations pass a tighter context deadline.
- Error wrapping is descriptive (e.g., `"fetch summary: %w"` upstream).
- `PerfSummaryResponse.CurrentPreset` stored as `json.RawMessage`; parsing delegated to `parseCurrentPreset`.
- No explicit retry logic; errors propagate to pollers/discovery for logging.

### `types.go`

- Mirrors firmware JSON structures with tagged Go structs.
- Covers status, summary, cooling, pools, chains, chips, presets, etc.
- Maintained in sync with `docs/firmware-api.md`. Keep doc updated when firmware schema evolves.

## Database Layer (`internal/database`)

### Schema (`schema.go`)

- Creates tables: `miners`, `models`, `model_presets`, `settings`, `statuses`, `status_fans`, `chain_snapshots`, `chip_snapshots`, etc.
- Uses `CREATE TABLE IF NOT EXISTS` and `ALTER TABLE` statements; initialization runs on startup.

### Store (`store.go`, `*.go`)

- `Store` wraps `*sql.DB` with helper methods.
- `Init` applies schema in order.
- CRUD coverage:
  - `miners.go` for upserting miners, retrieving single miners, listing all, etc.
  - `models.go` for model metadata and preset maintenance.
  - `statuses.go` for inserting status snapshots and querying history.
  - `telemetry.go` for recording/listing chain telemetry.
  - `settings.go` for miner configuration persistence.
  - `types.go` defines corresponding Go structs / input types.
- Helpers convert between nullable SQL fields and Go pointers (`helpers.go`).

Recent change (this session):
- `ListMiners` now left-joins `statuses` to hydrate `LatestStatus`, ensuring the API can surface state/hashrate without an extra query.
- Care was taken to copy SQL nullable values into new stack variables before taking addresses to avoid pointer reuse.

Potential considerations:
- `ListMiners` still loads full `Settings` objects for each miner (depending on schema size, could be heavy). Evaluate caching/pagination if the fleet grows.
- No explicit transaction around discovery writes observed here; confirm `applyDiscovery` handles consistency when updating multiple tables.
- No vacuum/retention logic for historical tables; long-running deployments may need maintenance jobs.

## Server & API (`internal/server`)

### HTTP Layer

- `Server` sets up routes:
  - `GET /api/miners` — list.
  - `GET /api/miners/{id}` — detailed view (includes latest status, settings, telemetry?).
  - `PATCH /api/miners/{id}` — toggle `managed` or update unlock password.
  - `GET /api/miners/{id}/statuses?limit=` — recent status snapshots.
  - `GET /api/miners/{id}/telemetry?limit=` — chain telemetry history.
  - `GET /api/models` — list models.
  - `GET/PATCH /api/models/{alias}` — manage model max preset.
- Remaining paths serve static frontend assets via an embedded filesystem (see `static.go`).

### DTO Translation

- `toMinerDTO`, `toModelDTO`, `toStatusDTO`, `toChainTelemetryDTO` convert database entities into API responses, formatting timestamps via `formatTime`.
- `updateMinerRequest` / `updateModelRequest` validate inputs before writing to the database.
- Errors are serialized as `{"error": "<message>"}` with appropriate HTTP codes.

Observations:
- `listMiners` now leverages the hydrated status data, so the UI receives `latest_status` without extra API round-trips.
- When a miner IP is missing (null), `online` field is `false`.
- `updateMiner` forbids empty unlock passwords; `updateModel` enforces preset membership.

### Static Assets (`internal/server/static.go`)

- Likely uses Go’s `embed` (not displayed here) to serve assets from `internal/server/web`.
- Ensure asset pipeline (if any) rebuilds when frontend changes; file watchers not present.

## Frontend (`internal/server/web`)

### `index.html`

- Basic dashboard layout: miners table, models section, status detail, telemetry detail, notifications.

### `styles.css`

- Modern, responsive styles with CSS variables; includes `.badge.status-*` classes for status pills and `.toast` notifications.

### `app.js`

- On DOM ready:
  - Maintains local state for miners/models/telemetry.
  - Polls back-end every 10 seconds (`AUTO_REFRESH_MS`).
  - `renderMiners` constructs table rows; status column now shows a single pill via `deriveMinerStatus`.
  - Click handlers load detailed statuses/telemetry for a miner.
  - Forms allow toggling `managed`, updating unlock password, and setting model max preset.
- Helpers format hashrate (GH/s, TH/s, PH/s) and relative timestamps.
- Notifications via toast queue.

Recent change (this session):
- `deriveMinerStatus` collapsed redundant fields and maps firmware states to Offline / Mining / Auto-tuning / Error, fixing duplicated preset display.

Potential frontend enhancements:
- Introduce column sorting/filtering for large fleets.
- Surface miner IP directly even when offline (currently displays `IP: offline` in table).
- Provide manual refresh indicator or failure banner if polling fails repeatedly.

## Documentation (`docs`)

- `firmware-api.md` — comprehensive spec for firmware endpoints (status, summary, chains, presets, etc.). Useful cross-reference while adjusting clients.
- `implementation-plan.md` — high-level roadmap covering model catalog, automation logic, API endpoints, UI expectations.
- Keep these up to date when firmware or business logic changes to prevent drift.

## Data & Configuration

- `config.json` defines:
  - SQLite file path.
  - Network subnets (`192.168.1.0/24` default).
  - Poll intervals: discovery 30s, status 15s, telemetry 60s.
  - HTTP bind `:8080`.
- No explicit logging or security settings; consider adding auth for dashboard access in production environments.

## Observed Gaps & Recommendations

1. **Telemetry Eligibility** — Unlike the status poller, the telemetry poller still skips miners lacking `model.max_preset`. Decide if telemetry should also run immediately after discovery.
2. **Database Growth** — Status and telemetry snapshots accumulate indefinitely. Implement retention policies or aggregation if storage is a concern.
3. **Testing Coverage** — There are no automated tests. High-priority candidates include database store methods, firmware client request handling, and server handler integration.
4. **Configuration Overrides** — Introduce environment variable or flag overrides for config path, log level, and HTTP bind to support multi-environment deployments.
5. **API Authentication** — Dashboard API is currently unauthenticated. For production, add auth (at minimum, basic authentication or OAuth proxy).
6. **Error Surfacing** — Background pollers log errors but don’t expose them to the UI. Consider recording last-error fields per miner for operator visibility.
7. **Frontend Build Process** — Assets are plain static files. If complexity increases, adopt a build pipeline (e.g., npm + bundler) and update static handler accordingly.
8. **Go Version** — Module declares Go 1.25.2 (future). Verify toolchain availability; if not intentional, update to current stable (1.23.x at time of writing).

## Summary

PowerHive consists of three cooperating subsystems:

1. **Discovery** populates the database with miners and seeds API keys.
2. **Polling services** (status + telemetry) maintain up-to-date operational data.
3. **Dashboard server** exposes both APIs and a lightweight frontend for operators.

The architecture is clean and idiomatic Go, leaning on worker pools and context-aware HTTP clients. Key areas for future work include data lifecycle management, telemetry polling eligibility, stronger observability, and test coverage. This document should serve as a starting point for any agent tasked with extending or debugging the system.
