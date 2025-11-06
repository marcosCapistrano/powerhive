# PowerHive Automation & Dashboard Plan

This document captures the architecture and sequencing for the upcoming automation features. It consolidates discoveries from the firmware API probe (`192.168.1.13`) and maps the database schema onto the required services.

## 1. Network Discovery
- **Subnet targets**: configurable CIDRs (e.g. `192.168.1.0/24`). Discovery will iterate every address each cycle.
- **Light scan**: attempt a short TCP dial (port `80`) with a 300–500 ms timeout. Successful connections feed the candidate pool without hitting the firmware API yet.
- **Miner probe**: for each responsive host call `/api/v1/info` (no auth). Expected payload contains:
  - `model` (`"s19jpro"`), `miner` (`"Antminer S19j Pro"`), firmware metadata.
  - `system.network_status.mac` (unique ID) and `system.network_status.ip`.
- **MAC as primary key**: map `mac → ip`; persist using `miners.id` and `miners.ip`. Offline miners (`mac` present in DB but not in the latest scan) will have `ip = NULL`.
- **Model catalogue**: once a host is confirmed as a miner, call `/api/v1/model` to collect human-friendly name (`full_name`) and alias (`model`). Insert/update the `models` table (alias → name) and refresh presets via `/api/v1/autotune/presets` using the automation API key (see below).

## 2. Firmware API Integration
- **Unlock flow**:
  1. `POST /api/v1/unlock` with JSON `{"pw":"<unlock_pass>"}`.
  2. Response contains a JWT (`token`). Tokens expire quickly (<5 min) – noted during testing.
- **API key policy**:
  - `GET /api/v1/apikeys` requires a fresh bearer token.
  - New keys must be 32-char hex strings (other formats return `422 Wrong apikey format`).
  - `POST /api/v1/apikeys` accepts `{"description":"PowerHive","key":"<32-hex>"}`; response `{"status":"inserted"}`.
  - `POST /api/v1/apikeys/delete` requires a valid bearer token and must be issued before the token expires.
  - Once an API key exists, endpoints that advertise `apikeyAuth` accept `x-api-key` without a bearer token. We will store our generated key in `miners.api_key`.
- **Status data**:
  - `/api/v1/status`: lightweight miner state.
  - `/api/v1/summary`: richer payload with hashrate, fans, chains, pools — requires API key.
  - `/api/v1/chains`: chip-level telemetry for the 1-minute loop.

## 3. Background Services
- **Discovery loop** (default every 30 s, configurable):
  1. Enumerate subnet hosts → light scan.
  2. Probe candidates via `/info`; collect MAC/IP/model.
  3. Ensure `miners` row exists (`UpsertMiner`), update `ip`, `unlock_pass` (if customised), and attach the `model`.
  4. Ensure `models` entry exists and capture presets. Presets will populate the dashboard selector; `max_preset` remains user-driven.
  5. Generate/store the automation API key if `miners.api_key` is null; otherwise reuse existing key.
  6. Nullify `ip` for miners missing from this cycle to mark them offline.
- **Status loop** (e.g. every 15 s):
  - Filter miners where `managed = true`, `ip != NULL`, and `miner.model.max_preset` is set.
  - Fetch `/summary` using stored API key.
  - Persist via `RecordMinerStatus` (fills `statuses`, `status_fans`, `chain_snapshots`, `chain_chips` with `status_id`).
  - Update `miners.latest_status_id` automatically via existing helper.
- **Telemetry loop** (every 60 s):
  - Same miner filter as status loop.
  - Fetch `/chains` for chip metrics.
  - Persist using a new helper (to be added) that inserts rows into `chain_snapshots` / `chain_chips` with `status_id = NULL` for historical analysis.
- **Resilience**:
  - Per-request `context.WithTimeout`.
  - Backoff on repeated failures per miner to avoid hammering offline units.
  - Central logger for errors; metrics aggregated for dashboard display (e.g. last successful poll timestamps).

## 4. HTTP API & Dashboard
- **Backend**:
  - Serve JSON under `/api/...`:
    - `GET /api/miners`: list miners (+ latest status summary, managed flag, IP).
    - `PATCH /api/miners/{id}`: toggle `managed`, update `unlock_pass`.
    - `POST /api/miners/{id}/rescan`: optional manual refresh hook.
    - `GET /api/models`: list models with presets and `max_preset`.
    - `PATCH /api/models/{alias}`: update `max_preset`.
    - `GET /api/statuses/{id}`: recent status history (limited window) for detail panes.
  - Static assets from `/dashboard` (vanilla HTML/CSS/JS). Single-page app using `fetch`.
- **Frontend views**:
  - **Miner Overview** table: MAC, nickname (if added later), IP/current state/hashrate, managed toggle, last seen timestamp.
  - **Model Config**: cards or table with alias, name, dropdown for presets, save button.
  - **Status Detail**: collapsible panel per miner showing fans and recent chain metrics.
  - **Telemetry Snapshot**: summary of chip anomalies (e.g. flagged by grade/state) refreshed every minute.
  - Use lightweight charting (pure CSS/JS) — avoid dependencies beyond optional inline SVG.
- **Interaction flow**:
  - Dashboard polls `/api/miners` every ~10 s for live updates.
  - Updates to `managed` or `max_preset` trigger optimistic UI with rollback on failure.
  - Empty IP indicates offline; highlight row accordingly.

## 5. Configuration
- Extend `config.json`:
  ```json
  {
    "database": { "path": "./data/powerhive.db" },
    "network": { "subnets": ["192.168.1.0/24"] },
    "intervals": {
      "discovery_seconds": 30,
      "status_seconds": 15,
      "telemetry_seconds": 60
    },
    "http": { "addr": ":8080" }
  }
  ```
- The unlock password default remains `admin`; dashboard will expose a per-miner field for custom values.
- Future: optionally store router credentials to ingest DHCP leases instead of scanning.

## 6. Next Steps
1. Update configuration structs & validation.
2. Implement firmware API client (`internal/firmware`).
3. Build discovery/status/telemetry services leveraging `database.Store`.
4. Add REST handlers + static dashboard build.
5. Wire everything into `cmd/automation/main.go` with graceful shutdown.
6. Add integration tests / mocks where feasible; provide CLI hooks for manual rescan.

This plan will guide the implementation; deviations will be documented alongside the corresponding code changes.
