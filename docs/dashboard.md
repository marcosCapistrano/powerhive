# PowerHive Dashboard Overview

The dashboard bundles miner discovery, health polling, and chip telemetry into a single interface. This document summarises the moving parts and the API surfaces exposed for the vanilla HTML/JS frontend (`internal/server/web`).

## Background Services

- **Discovery loop** (`internal/app/discovery.go`): rescans configured subnets every `intervals.discovery_seconds`, probes `/api/v1/info` on responsive hosts, tracks MAC addresses as miner IDs, provisions automation API keys, and refreshes model presets.
- **Status loop** (`internal/app/status_poller.go`): polls `/api/v1/summary` and `/api/v1/perf-summary` for managed miners whose models have `max_preset` assigned; snapshots are persisted via `RecordMinerStatus`, including the active preset at capture time.
- **Telemetry loop** (`internal/app/telemetry_poller.go`): pulls `/api/v1/chains` once per minute for managed miners and stores chip-level metrics through `RecordChainTelemetry`.

Intervals, network timeouts, and HTTP bind address are controlled in `config.json`.

## API Endpoints

All endpoints are served from the automation binary (default `:8080`):

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/api/miners` | List miners with latest status summary. |
| `GET` | `/api/miners/{id}` | Fetch a single miner entry. |
| `PATCH` | `/api/miners/{id}` | Update `managed` or `unlock_pass`. |
| `GET` | `/api/miners/{id}/statuses?limit=N` | Retrieve recent status snapshots (fans, chains, chips). |
| `GET` | `/api/miners/{id}/telemetry?limit=N` | Retrieve chip-level telemetry history (chain snapshots + chips). |
| `GET` | `/api/models` | Enumerate models, presets, and the selected `max_preset`. |
| `GET` | `/api/models/{alias}` | Fetch a single model. |
| `PATCH` | `/api/models/{alias}` | Set or clear the `max_preset`. |

Responses are JSON; errors return `{ "error": "<message>" }`.

## Dashboard Usage

The vanilla frontend (`index.html`, `styles.css`, `app.js`) is embedded into the binary and served at `/`.

- **Miners table**: shows MAC, IP, model, latest hashrate, and automation status.
  - Toggle the *Managed* switch to opt miners into automation.
  - Supply a new unlock password per miner via the inline form (default is `admin` until changed).
  - Selecting a row loads the last 5 status snapshots with fan/chip telemetry.
- **Models section**: set `max_preset` from discovered presets. Managed miners are controlled only when a preset is selected.
- **Status detail**: provides a quick feed of recent snapshots (preset, state, hashrate, fans, chain health).
- **Telemetry history**: renders the per-chain telemetry timeline (drawn from `/api/miners/{id}/telemetry`) so you can inspect chain/chip hashrate and temperature trends.

The dashboard refreshes miner inventory every 10 seconds; manual refresh is available through the header button. Notifications appear in the top-left to confirm actions or surface errors.
