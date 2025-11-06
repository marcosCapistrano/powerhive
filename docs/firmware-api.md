# XMiner Firmware API Guide

This guide reformats `firmware-api.json` into an operator-friendly reference with richer explanations, tables, and example payloads. Use it when integrating with or automating the XMiner firmware API. Syntax-highlighted examples help when exporting this document to PDF or HTML.

## Base Server
- `/api/v1` — Current miner API

## Authentication Overview
- **apikeyAuth**: API key in header, header `x-api-key`.
- **bearerAuth**: Bearer `JWT` token supplied in the `Authorization` header.

All endpoints that list both schemes will accept either credential. When testing locally, you can omit unused headers.

## Using This Guide
- Each functional area starts with a quick-view table followed by detailed call breakdowns.
- Tables highlight required parameters and payload fields. Optional fields are included for context but can be omitted if not needed.
- Example `curl` commands provide syntax-highlighted request templates you can paste directly into a shell.
- JSON snippets include representative placeholder values to show the expected shape; replace them with data from your environment.

## API Key Administration
Provision API credentials for remote tooling and retire them when no longer needed.

| Method | Path | Summary | Auth | Request | Success (200) |
| --- | --- | --- | --- | --- | --- |
| `GET` | `/apikeys` | Get apikeys | `bearerAuth`, `apikeyAuth` | — | application/json → array<`ApiKeysJsonItem`> |
| `POST` | `/apikeys` | Add api key | `bearerAuth`, `apikeyAuth` | `AddApikeyQuery` | application/json → `AddApiKeyRes` |
| `POST` | `/apikeys/delete` | Delete api key | `bearerAuth`, `apikeyAuth` | `DeleteApikeyQuery` | Key deleted successfully |

### `GET /apikeys`
**Summary**: Get apikeys.
**When to use**: List every API key that has been provisioned on the miner for automation or third-party monitoring.
**Authentication**: JWT bearer token (`Authorization: Bearer <token>`); API key header (`x-api-key: <key>`).
**Responses**:
| Code | Description | Payload |
| --- | --- | --- |
| `200` | Api key list read successfully | application/json → array<`ApiKeysJsonItem`> |
| `401` | Unauthorized | — |
| `500` | Internal Server Error | application/json → `ErrDescr` |
**Sample request**:
```bash
curl -X GET "http://<miner-ip>/api/v1/apikeys" \
  -H "Authorization: Bearer <token>" \
  -H "x-api-key: <key>"
```

### `POST /apikeys`
**Summary**: Add api key.
**When to use**: Create a fresh API key for a new integration or operator tool that needs authenticated access.
**Authentication**: JWT bearer token (`Authorization: Bearer <token>`); API key header (`x-api-key: <key>`).
**Request body**:
- Required
- `application/json` payload uses `AddApikeyQuery`:
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `description` | string | Yes | — | — |
| `key` | string | Yes | — | — |
  Example:
```json
{
  "description": "description_value",
  "key": "key_value"
}
```
**Responses**:
| Code | Description | Payload |
| --- | --- | --- |
| `200` | Api key was added successfully | application/json → `AddApiKeyRes` |
| `401` | Unauthorized | — |
| `500` | Internal Server Error | application/json → `ErrDescr` |
**Success payload fields (`AddApiKeyRes`)**:
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `status` | `AddApiKeyStatus` | Yes | — | — |
```json
{
  "status": "inserted"
}
```
**Sample request**:
```bash
curl -X POST "http://<miner-ip>/api/v1/apikeys" \
  -H "Authorization: Bearer <token>" \
  -H "x-api-key: <key>" \
  -H "Content-Type: application/json" \
  -d '{
  "description": "description_value",
  "key": "key_value"
}'
```

### `POST /apikeys/delete`
**Summary**: Delete api key.
**When to use**: Remove an API key that is no longer required or may be compromised.
**Authentication**: JWT bearer token (`Authorization: Bearer <token>`); API key header (`x-api-key: <key>`).
**Request body**:
- Required
- `application/json` payload uses `DeleteApikeyQuery`:
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `key` | string | Yes | — | — |
  Example:
```json
{
  "key": "key_value"
}
```
**Responses**:
| Code | Description | Payload |
| --- | --- | --- |
| `200` | Key deleted successfully | — |
| `401` | Unauthorized | — |
| `500` | Internal Server Error | application/json → `ErrDescr` |
**Sample request**:
```bash
curl -X POST "http://<miner-ip>/api/v1/apikeys/delete" \
  -H "Authorization: Bearer <token>" \
  -H "x-api-key: <key>" \
  -H "Content-Type: application/json" \
  -d '{
  "key": "key_value"
}'
```

## Authentication & Session Control
Manage screen locks, unlock windows, and validation of session credentials.

| Method | Path | Summary | Auth | Request | Success (200) |
| --- | --- | --- | --- | --- | --- |
| `GET` | `/auth-check` | Auth Check | `bearerAuth`, `apikeyAuth` | — | application/json → `AuthCheck` |
| `POST` | `/lock` | Lock miner | `bearerAuth` | — | Session dropped |
| `POST` | `/lock/others` | Lock other miner sessions | `bearerAuth` | `UnlockScreenBody` | Other sessions dropped |
| `POST` | `/unlock` | Auth Check | — | `UnlockScreenBody` | application/json → `UnlockSuccess` |

### `GET /auth-check`
**Summary**: Auth Check.
**When to use**: Confirm that the current credentials are valid and that the session remains unlocked.
**Authentication**: JWT bearer token (`Authorization: Bearer <token>`); API key header (`x-api-key: <key>`).
**Responses**:
| Code | Description | Payload |
| --- | --- | --- |
| `200` | Authorized | application/json → `AuthCheck` |
| `401` | Unauthorized | application/json → `AuthCheck` |
**Success payload fields (`AuthCheck`)**:
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `unlock_timeout` | integer (int64) | No | — | — |
```json
{
  "unlock_timeout": 1
}
```
**Sample request**:
```bash
curl -X GET "http://<miner-ip>/api/v1/auth-check" \
  -H "Authorization: Bearer <token>" \
  -H "x-api-key: <key>"
```

### `POST /lock`
**Summary**: Lock miner.
**When to use**: Lock the local UI to prevent changes until an operator unlocks it again.
**Authentication**: JWT bearer token (`Authorization: Bearer <token>`).
**Responses**:
| Code | Description | Payload |
| --- | --- | --- |
| `200` | Session dropped | — |
| `401` | Unauthorized | — |
**Sample request**:
```bash
curl -X POST "http://<miner-ip>/api/v1/lock" \
  -H "Authorization: Bearer <token>"
```

### `POST /lock/others`
**Summary**: Lock other miner sessions.
**When to use**: Force-log out any other active sessions, keeping only the caller authenticated.
**Authentication**: JWT bearer token (`Authorization: Bearer <token>`).
**Request body**:
- Required
- `application/json` payload uses `UnlockScreenBody`:
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `pw` | string | Yes | Target device(s) password | — |
  Example:
```json
{
  "pw": "pw_value"
}
```
**Responses**:
| Code | Description | Payload |
| --- | --- | --- |
| `200` | Other sessions dropped | — |
| `400` | Wrong password | — |
| `401` | Unauthorized | — |
| `500` | Internal Server Error | application/json → `ErrDescr` |
**Sample request**:
```bash
curl -X POST "http://<miner-ip>/api/v1/lock/others" \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
  "pw": "pw_value"
}'
```

### `POST /unlock`
**Summary**: Auth Check.
**When to use**: Provide the device password to unlock the UI for a limited window.
**Request body**:
- Required
- `application/json` payload uses `UnlockScreenBody`:
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `pw` | string | Yes | Target device(s) password | — |
  Example:
```json
{
  "pw": "pw_value"
}
```
**Responses**:
| Code | Description | Payload |
| --- | --- | --- |
| `200` | Authorized | application/json → `UnlockSuccess` |
| `403` | Wrong password | application/json → `AuthCheck` |
| `429` | Too many requests | application/json → `AuthCheck` |
| `500` | Internal Server Error | application/json → `ErrDescr` |
**Success payload fields (`UnlockSuccess`)**:
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `token` | string | Yes | — | — |
```json
{
  "token": "token-value"
}
```
**Sample request**:
```bash
curl -X POST "http://<miner-ip>/api/v1/unlock" \
  -H "Content-Type: application/json" \
  -d '{
  "pw": "pw_value"
}'
```

## Autotune & Presets
Control autotune calibration data and enumerate the available performance presets.

| Method | Path | Summary | Auth | Request | Success (200) |
| --- | --- | --- | --- | --- | --- |
| `GET` | `/autotune/presets` | Get autotune preset list | `bearerAuth`, `apikeyAuth` | — | application/json → array<`AutotunePresetDto`> |
| `POST` | `/autotune/reset` | Autotune reset list of profiles | `bearerAuth`, `apikeyAuth` | `AutotuneReset` | Reset list of autotune profiles done successfully |
| `POST` | `/autotune/reset-all` | Autotune reset all profiles | `bearerAuth`, `apikeyAuth` | `AutotuneResetAll` | Reset all autotune profiles done successfully |

### `GET /autotune/presets`
**Summary**: Get autotune preset list.
**When to use**: Inspect the autotune preset catalog that ships with the firmware and any custom presets.
**Authentication**: JWT bearer token (`Authorization: Bearer <token>`); API key header (`x-api-key: <key>`).
**Responses**:
| Code | Description | Payload |
| --- | --- | --- |
| `200` | Autotune preset list read successfully | application/json → array<`AutotunePresetDto`> |
| `401` | Unauthorized | — |
| `500` | Internal Server Error | application/json → `ErrDescr` |
**Sample request**:
```bash
curl -X GET "http://<miner-ip>/api/v1/autotune/presets" \
  -H "Authorization: Bearer <token>" \
  -H "x-api-key: <key>"
```

### `POST /autotune/reset`
**Summary**: Autotune reset list of profiles.
**When to use**: Clear autotune statistics for a targeted set of presets so the miner can retune them from scratch.
**Authentication**: JWT bearer token (`Authorization: Bearer <token>`); API key header (`x-api-key: <key>`).
**Request body**:
- Required
- `application/json` payload uses `AutotuneReset`:
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `presets` | array<string> | Yes | List of presets to reset | — |
| `restart` | boolean | Yes | Restart after presets remove | — |
  Example:
```json
{
  "presets": [
    "preset_value"
  ],
  "restart": true
}
```
**Responses**:
| Code | Description | Payload |
| --- | --- | --- |
| `200` | Reset list of autotune profiles done successfully | — |
| `401` | Unauthorized | — |
| `500` | Internal Server Error | application/json → `ErrDescr` |
**Sample request**:
```bash
curl -X POST "http://<miner-ip>/api/v1/autotune/reset" \
  -H "Authorization: Bearer <token>" \
  -H "x-api-key: <key>" \
  -H "Content-Type: application/json" \
  -d '{
  "presets": [
    "preset_value"
  ],
  "restart": true
}'
```

### `POST /autotune/reset-all`
**Summary**: Autotune reset all profiles.
**When to use**: Erase autotune data for every preset and force the firmware to rebuild tuning tables.
**Authentication**: JWT bearer token (`Authorization: Bearer <token>`); API key header (`x-api-key: <key>`).
**Request body**:
- Required
- `application/json` payload uses `AutotuneResetAll`:
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `restart` | boolean | Yes | — | — |
  Example:
```json
{
  "restart": true
}
```
**Responses**:
| Code | Description | Payload |
| --- | --- | --- |
| `200` | Reset all autotune profiles done successfully | — |
| `401` | Unauthorized | — |
| `500` | Internal Server Error | application/json → `ErrDescr` |
**Sample request**:
```bash
curl -X POST "http://<miner-ip>/api/v1/autotune/reset-all" \
  -H "Authorization: Bearer <token>" \
  -H "x-api-key: <key>" \
  -H "Content-Type: application/json" \
  -d '{
  "restart": true
}'
```

## Configuration Management
Inspect, back up, restore, or persist the miner configuration.

| Method | Path | Summary | Auth | Request | Success (200) |
| --- | --- | --- | --- | --- | --- |
| `GET` | `/settings` | Get all miner settings | `apikeyAuth` | — | application/json → `ViewConfig` |
| `POST` | `/settings` | Save miner settings | `bearerAuth`, `apikeyAuth` | `InputConfig` | application/json → `SaveConfigResult` |
| `POST` | `/settings/backup` | Settings backup | `bearerAuth`, `apikeyAuth` | — | application/octet-stream → array<integer (int32)> |
| `POST` | `/settings/factory-reset` | Settings factory reset | `bearerAuth`, `apikeyAuth` | — | application/json → `RebootAfter` |
| `POST` | `/settings/restore` | Settings restore | `bearerAuth`, `apikeyAuth` | `SchemaSettingsRestore` (multipart) | application/json → `RebootAfter` |

### `GET /settings`
**Summary**: Get all miner settings.
**When to use**: Pull the full current configuration snapshot.
**Authentication**: API key header (`x-api-key: <key>`).
**Responses**:
| Code | Description | Payload |
| --- | --- | --- |
| `200` | Config read successfully | application/json → `ViewConfig` |
| `401` | Unauthorized | — |
| `500` | Internal Server Error | application/json → `ErrDescr` |
**Success payload fields (`ViewConfig`)**:
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `layout` | oneOf | No | — | Accepts one of multiple shapes |
| `miner` | `MinerConfigRaw` | Yes | — | — |
| `network` | `NetworkConfFile` | Yes | — | — |
| `password` | oneOf | No | — | Accepts one of multiple shapes |
| `regional` | `RegionalSettings` | Yes | — | — |
| `ui` | `UiSettings` | Yes | — | — |
```json
{
  "miner": {
    "cooling": "...",
    "misc": "...",
    "overclock": "..."
  },
  "network": {
    "dhcp": true,
    "dnsservers": [
      "dnsserver_value"
    ],
    "gateway": "gateway_value",
    "hostname": "hostname_value",
    "ipaddress": "ipaddress_value",
    "netmask": "netmask_value"
  },
  "regional": {
    "timezone": {
      "current": "..."
    }
  },
  "ui": {
    "consts": "...",
    "dark_side_pane": true,
    "disable_animation": true
  }
}
```
**Sample request**:
```bash
curl -X GET "http://<miner-ip>/api/v1/settings" \
  -H "x-api-key: <key>"
```

### `POST /settings`
**Summary**: Save miner settings.
**When to use**: Persist configuration changes provided in the request body.
**Authentication**: JWT bearer token (`Authorization: Bearer <token>`); API key header (`x-api-key: <key>`).
**Request body**:
- Required
- `application/json` payload uses `InputConfig`:
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `layout` | oneOf | No | — | Accepts one of multiple shapes |
| `miner` | oneOf | No | — | Accepts one of multiple shapes |
| `network` | oneOf | No | — | Accepts one of multiple shapes |
| `password` | oneOf | No | — | Accepts one of multiple shapes |
| `regional` | oneOf | No | — | Accepts one of multiple shapes |
| `ui` | oneOf | No | — | Accepts one of multiple shapes |
  Example:
```json
{
  "layout": "...",
  "miner": "...",
  "network": "..."
}
```
**Responses**:
| Code | Description | Payload |
| --- | --- | --- |
| `200` | Config saved successfully | application/json → `SaveConfigResult` |
| `401` | Unauthorized | — |
| `500` | Internal Server Error | application/json → `ErrDescr` |
**Success payload fields (`SaveConfigResult`)**:
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `reboot_required` | boolean | Yes | Miner restart required to apply | — |
| `restart_required` | boolean | Yes | Miner restart required to apply config | — |
```json
{
  "reboot_required": true,
  "restart_required": true
}
```
**Sample request**:
```bash
curl -X POST "http://<miner-ip>/api/v1/settings" \
  -H "Authorization: Bearer <token>" \
  -H "x-api-key: <key>" \
  -H "Content-Type: application/json" \
  -d '{
  "layout": "...",
  "miner": "...",
  "network": "..."
}'
```

### `POST /settings/backup`
**Summary**: Settings backup.
**When to use**: Capture a downloadable backup of the current configuration.
**Authentication**: JWT bearer token (`Authorization: Bearer <token>`); API key header (`x-api-key: <key>`).
**Responses**:
| Code | Description | Payload |
| --- | --- | --- |
| `200` | Backup binary | application/octet-stream → array<integer (int32)> |
| `401` | Unauthorized | — |
**Sample request**:
```bash
curl -X POST "http://<miner-ip>/api/v1/settings/backup" \
  -H "Authorization: Bearer <token>" \
  -H "x-api-key: <key>"
```

### `POST /settings/factory-reset`
**Summary**: Settings factory reset.
**When to use**: Factory reset configuration to default values without reflashing firmware.
**Authentication**: JWT bearer token (`Authorization: Bearer <token>`); API key header (`x-api-key: <key>`).
**Responses**:
| Code | Description | Payload |
| --- | --- | --- |
| `200` | Settings factory reset done successfully. System will reboot after | application/json → `RebootAfter` |
| `401` | Unauthorized | — |
| `500` | Internal Server Error | application/json → `ErrDescr` |
**Success payload fields (`RebootAfter`)**:
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `after` | integer (int64) | Yes | Number of seconds after the system will reboot By default, is 3 seconds | — |
```json
{
  "after": 1
}
```
**Sample request**:
```bash
curl -X POST "http://<miner-ip>/api/v1/settings/factory-reset" \
  -H "Authorization: Bearer <token>" \
  -H "x-api-key: <key>"
```
> **Caution:** Factory reset clears all runtime settings. Capture a backup first if you may need to restore.

### `POST /settings/restore`
**Summary**: Settings restore.
**When to use**: Restore configuration from a previously captured backup archive.
**Authentication**: JWT bearer token (`Authorization: Bearer <token>`); API key header (`x-api-key: <key>`).
**Request body**:
- Required
- `multipart/form-data` payload uses `SchemaSettingsRestore`:
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `file` | string (binary) | Yes | — | File upload (binary) |
  Example:
```json
{
  "file": "/path/to/file.bin"
}
```
**Responses**:
| Code | Description | Payload |
| --- | --- | --- |
| `200` | Firmware restored successfully. System will reboot after | application/json → `RebootAfter` |
| `401` | Unauthorized | — |
| `403` | Miner have warranty. Cancel warranty first | — |
| `500` | Internal Server Error | application/json → `ErrDescr` |
**Success payload fields (`RebootAfter`)**:
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `after` | integer (int64) | Yes | Number of seconds after the system will reboot By default, is 3 seconds | — |
```json
{
  "after": 1
}
```
**Sample request**:
```bash
curl -X POST "http://<miner-ip>/api/v1/settings/restore" \
  -H "Authorization: Bearer <token>" \
  -H "x-api-key: <key>" \
  -F "file=@/path/to/firmware.tar.gz"
```
> **Caution:** Restoring configuration overwrites current settings and typically triggers a service restart.

## Firmware Maintenance
Upload new firmware images or roll back to stock builds when required.

| Method | Path | Summary | Auth | Request | Success (200) |
| --- | --- | --- | --- | --- | --- |
| `POST` | `/firmware/remove` | Remove firmware and boot from stock | `bearerAuth`, `apikeyAuth` | — | application/json → `RebootAfter` |
| `POST` | `/firmware/update` | Update firmware | `bearerAuth`, `apikeyAuth` | `SchemaFirmwareUpdate` (multipart) | application/json → `RebootAfter` |

### `POST /firmware/remove`
**Summary**: Remove firmware and boot from stock.
**When to use**: Rollback to the stock firmware image and reboot from the factory partition.
**Authentication**: JWT bearer token (`Authorization: Bearer <token>`); API key header (`x-api-key: <key>`).
**Responses**:
| Code | Description | Payload |
| --- | --- | --- |
| `200` | Firmware was successfully removed. System will reboot after | application/json → `RebootAfter` |
| `400` | This model has no 'remove firmware' | application/json → `ErrDescr` |
| `401` | Unauthorized | — |
| `500` | Internal Server Error | application/json → `ErrDescr` |
**Success payload fields (`RebootAfter`)**:
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `after` | integer (int64) | Yes | Number of seconds after the system will reboot By default, is 3 seconds | — |
```json
{
  "after": 1
}
```
**Sample request**:
```bash
curl -X POST "http://<miner-ip>/api/v1/firmware/remove" \
  -H "Authorization: Bearer <token>" \
  -H "x-api-key: <key>"
```
> **Caution:** Rolling back to stock firmware will reboot the miner and discards custom features until you reinstall the custom build.

### `POST /firmware/update`
**Summary**: Update firmware.
**When to use**: Upload a new firmware image and schedule a reboot once flashing finishes.
**Authentication**: JWT bearer token (`Authorization: Bearer <token>`); API key header (`x-api-key: <key>`).
**Request body**:
- Required
- `multipart/form-data` payload uses `SchemaFirmwareUpdate`:
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `file` | string (binary) | Yes | — | File upload (binary) |
| `keep_settings` | oneOf | No | — | Accepts one of multiple shapes |
  Example:
```json
{
  "file": "/path/to/file.bin",
  "keep_settings": "..."
}
```
**Responses**:
| Code | Description | Payload |
| --- | --- | --- |
| `200` | Firmware update successfully. System will reboot after | application/json → `RebootAfter` |
| `401` | Unauthorized | — |
| `500` | Internal Server Error | application/json → `ErrDescr` |
**Success payload fields (`RebootAfter`)**:
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `after` | integer (int64) | Yes | Number of seconds after the system will reboot By default, is 3 seconds | — |
```json
{
  "after": 1
}
```
**Sample request**:
```bash
curl -X POST "http://<miner-ip>/api/v1/firmware/update" \
  -H "Authorization: Bearer <token>" \
  -H "x-api-key: <key>" \
  -F "file=@/path/to/firmware.tar.gz" \
  -F "keep_settings=..."
```
> **Caution:** Uploading firmware is disruptive—queue the request during a maintenance window and validate the checksum before flashing.

## General Information
Supporting endpoints used by the UI to render live data and topology.

| Method | Path | Summary | Auth | Request | Success (200) |
| --- | --- | --- | --- | --- | --- |
| `GET` | `/chains` | Get miner chains | — | — | application/json → array<`AntmChainChips`> |
| `GET` | `/chains/factory-info` | Get miner chains factory info | — | — | application/json → `FactoryInfoReply` |
| `GET` | `/chips` | Get miner chips. Deprecated. Use /chains route instead | — | — | application/json → `AntmChainsChipsStats` |
| `POST` | `/find-miner` | Find miner | — | — | application/json → oneOf |
| `GET` | `/info` | Get miner info | — | — | application/json → `InfoJson` |
| `GET` | `/layout` | Layout | — | — | application/json → oneOf |
| `GET` | `/model` | Get miner model info | — | — | application/json → `MinerModelInfo` |
| `GET` | `/perf-summary` | Summary | — | — | application/json → `PerfSummary` |
| `GET` | `/status` | Get status | — | — | application/json → `StatusPane` |
| `GET` | `/summary` | Summary | `bearerAuth`, `apikeyAuth` | — | application/json → `Summary` |
| `GET` | `/ui` | UI | — | — | application/json → `UiSettings` |

### `GET /chains`
**Summary**: Get miner chains.
**When to use**: Retrieve chain-level telemetry including temperatures, hashrate, and ASIC status.
**Responses**:
| Code | Description | Payload |
| --- | --- | --- |
| `200` | Chains read successfully | application/json → array<`AntmChainChips`> |
**Sample request**:
```bash
curl -X GET "http://<miner-ip>/api/v1/chains"
```

### `GET /chains/factory-info`
**Summary**: Get miner chains factory info.
**When to use**: Pull manufacturing metadata for each chain such as serials and batch identifiers.
**Responses**:
| Code | Description | Payload |
| --- | --- | --- |
| `200` | Chains factory info read successfully | application/json → `FactoryInfoReply` |
**Success payload fields (`FactoryInfoReply`)**:
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `chains` | array<`FactoryInfoChain`> | No | — | — |
| `has_pics` | boolean | No | — | — |
| `hr_stock` | number (double) | No | — | — |
| `psu_model` | string | No | — | — |
| `psu_serial` | string | No | — | — |
```json
{
  "chains": [
    {
      "board_model": "board_model_value",
      "chip_bin": 1,
      "freq": 1,
      "hashrate": 1.0,
      "id": 1,
      "serial": "serial_value",
      "volt": 1
    }
  ],
  "has_pics": true,
  "hr_stock": 1.0
}
```
**Sample request**:
```bash
curl -X GET "http://<miner-ip>/api/v1/chains/factory-info"
```

### `GET /chips`
**Summary**: Get miner chips. Deprecated. Use /chains route instead.
**When to use**: Legacy endpoint for chip-level data; prefer `/chains` for current builds.
**Responses**:
| Code | Description | Payload |
| --- | --- | --- |
| `200` | Chips read successfully | application/json → `AntmChainsChipsStats` |
**Success payload fields (`AntmChainsChipsStats`)**:
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `chains` | array<`AntmChainChipsStats`> | Yes | — | — |
| `chips_per_chain` | integer | Yes | — | — |
```json
{
  "chains": [
    {
      "chips": [
        "..."
      ],
      "id": 1
    }
  ],
  "chips_per_chain": 1
}
```
**Sample request**:
```bash
curl -X GET "http://<miner-ip>/api/v1/chips"
```

### `POST /find-miner`
**Summary**: Find miner.
**When to use**: Trigger the miner's physical locator (beeper or LEDs) to identify the unit on the floor.
**Responses**:
| Code | Description | Payload |
| --- | --- | --- |
| `200` | Request handled successfully | application/json → oneOf |
| `401` | Unauthorized | — |
**Sample request**:
```bash
curl -X POST "http://<miner-ip>/api/v1/find-miner"
```

### `GET /info`
**Summary**: Get miner info.
**When to use**: Fetch firmware version, algorithm, and platform metadata for the miner.
**Responses**:
| Code | Description | Payload |
| --- | --- | --- |
| `200` | Miner Info | application/json → `InfoJson` |
**Success payload fields (`InfoJson`)**:
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `build_name` | string | No | — | — |
| `build_time` | string | Yes | Build time | — |
| `build_uuid` | string | No | — | — |
| `fw_name` | string | Yes | Firmware name | — |
| `fw_version` | string | Yes | Firmware version | — |
| `install_type` | `InstallType` | Yes | — | — |
| `miner` | string | Yes | Pretty miner name | — |
| `model` | string | Yes | Miner model code | — |
| `platform` | `Platform` | Yes | — | — |
| `algorithm` | `MiningAlgorithm` | Yes | — | — |
| `hr_measure` | `HrMeasure` | Yes | — | — |
| `serial` | string | Yes | — | — |
| `system` | `SystemInfo` | Yes | — | — |
```json
{
  "build_time": "build_time_value",
  "fw_name": "fw_name_value",
  "fw_version": "fw_version_value",
  "install_type": "sd",
  "miner": "miner_value",
  "model": "model_value",
  "platform": "aml",
  "algorithm": "sha256d",
  "hr_measure": "GH/s",
  "serial": "serial_value",
  "system": {
    "mem_buf": 1,
    "mem_buf_percent": 1,
    "mem_free": 1,
    "mem_free_percent": 1,
    "mem_total": 1,
    "file_system_version": "file_system_version_value",
    "miner_name": "miner_name_value",
    "network_status": {
      "dns": "...",
      "gateway": "...",
      "hostname": "...",
      "ip": "...",
      "mac": "...",
      "netmask": "..."
    },
    "os": "os_value",
    "uptime": "uptime_value"
  }
}
```
**Sample request**:
```bash
curl -X GET "http://<miner-ip>/api/v1/info"
```

### `GET /layout`
**Summary**: Layout.
**When to use**: Retrieve the UI layout configuration used by the web interface.
**Responses**:
| Code | Description | Payload |
| --- | --- | --- |
| `200` | Dashboard elements layout | application/json → oneOf |
**Sample request**:
```bash
curl -X GET "http://<miner-ip>/api/v1/layout"
```

### `GET /model`
**Summary**: Get miner model info.
**When to use**: Inspect static model capabilities including supported cooling and presets.
**Responses**:
| Code | Description | Payload |
| --- | --- | --- |
| `200` | Miner model Info | application/json → `MinerModelInfo` |
**Success payload fields (`MinerModelInfo`)**:
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `algorithm` | `MiningAlgorithm` | Yes | — | — |
| `chain` | `ModelInfoChain` | Yes | — | — |
| `cooling` | `CoolingConsts` | Yes | — | — |
| `full_name` | string | Yes | Pretty miner name | — |
| `hr_measure` | `HrMeasure` | Yes | — | — |
| `install_type` | `InstallType` | Yes | — | — |
| `model` | string | Yes | Miner model code | — |
| `overclock` | `Overclock` | Yes | — | — |
| `platform` | `Platform` | Yes | — | — |
| `serial` | string | Yes | — | — |
| `series` | `Series` | Yes | — | — |
```json
{
  "algorithm": "sha256d",
  "chain": {
    "chips_per_chain": 1,
    "chips_per_domain": 1,
    "num_chains": 1,
    "topology": {
      "chips": "...",
      "num_cols": "...",
      "num_rows": "..."
    }
  },
  "cooling": {
    "max_target_temp": 1,
    "min_fan_pwm": 1,
    "min_target_temp": 1
  },
  "full_name": "full_name_value",
  "hr_measure": "GH/s",
  "install_type": "sd",
  "model": "model_value",
  "overclock": {
    "default_freq": 1,
    "default_voltage": 1,
    "max_freq": 1,
    "max_voltage": 1,
    "max_voltage_stock_psu": 1,
    "min_freq": 1,
    "min_voltage": 1,
    "warn_freq": 1
  },
  "platform": "aml",
  "serial": "serial_value",
  "series": "l7"
}
```
**Sample request**:
```bash
curl -X GET "http://<miner-ip>/api/v1/model"
```

### `GET /perf-summary`
**Summary**: Summary.
**When to use**: Summarize recent performance metrics across chains and pools.
**Responses**:
| Code | Description | Payload |
| --- | --- | --- |
| `200` | Chips read successfully | application/json → `PerfSummary` |
| `500` | Internal Server Error | application/json → `ErrDescr` |
**Success payload fields (`PerfSummary`)**:
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `current_preset` | oneOf | No | — | Accepts one of multiple shapes |
| `preset_switcher` | `PresetSwitcherRaw` | Yes | — | — |
```json
{
  "preset_switcher": {
    "autochange_top_preset": true,
    "check_time": 1,
    "decrease_temp": 1
  },
  "current_preset": "..."
}
```
**Sample request**:
```bash
curl -X GET "http://<miner-ip>/api/v1/perf-summary"
```

### `GET /status`
**Summary**: Get status.
**When to use**: Check real-time miner status including state flags and unlock data.
**Responses**:
| Code | Description | Payload |
| --- | --- | --- |
| `200` | success | application/json → `StatusPane` |
**Success payload fields (`StatusPane`)**:
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `description` | string | No | Optional. Description if status is failure | — |
| `failure_code` | integer (int32) | No | — | — |
| `miner_state` | `MinerState` | Yes | — | — |
| `miner_state_time` | integer (int64) | Yes | Time spent in the current state. For now implemented for `mining` state only. | — |
| `reboot_required` | boolean | Yes | Miner restart required to apply | — |
| `restart_required` | boolean | Yes | Miner restart required to apply config | — |
| `find_miner` | boolean | Yes | Flag to switch find_miner function on target devices. Optional, default `false` | — |
| `unlock_timeout` | integer (int64) | No | — | — |
| `unlocked` | boolean | Yes | Show screen-lock status (checks that  any of auth methods satisfies) | — |
| `warranty` | `Warranty` | No | — | — |
```json
{
  "miner_state": "mining",
  "miner_state_time": 1,
  "reboot_required": true,
  "restart_required": true,
  "find_miner": true,
  "unlocked": true
}
```
**Sample request**:
```bash
curl -X GET "http://<miner-ip>/api/v1/status"
```

### `GET /summary`
**Summary**: Summary.
**When to use**: Collect the concise dashboard summary used by the UI.
**Authentication**: JWT bearer token (`Authorization: Bearer <token>`); API key header (`x-api-key: <key>`).
**Responses**:
| Code | Description | Payload |
| --- | --- | --- |
| `200` | Summary read successfully | application/json → `Summary` |
**Success payload fields (`Summary`)**:
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `miner` | oneOf | No | — | Accepts one of multiple shapes |
```json
{
  "miner": "..."
}
```
**Sample request**:
```bash
curl -X GET "http://<miner-ip>/api/v1/summary" \
  -H "Authorization: Bearer <token>" \
  -H "x-api-key: <key>"
```

### `GET /ui`
**Summary**: UI.
**When to use**: Retrieve UI preferences and constants used by the frontend.
**Responses**:
| Code | Description | Payload |
| --- | --- | --- |
| `200` | Ui read successfully | application/json → `UiSettings` |
| `500` | Internal Server Error | application/json → `ErrDescr` |
**Success payload fields (`UiSettings`)**:
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `consts` | oneOf | No | — | Accepts one of multiple shapes |
| `dark_side_pane` | boolean | No | — | — |
| `disable_animation` | boolean | No | — | — |
| `locale` | oneOf | No | — | Accepts one of multiple shapes |
| `theme` | oneOf | No | — | Accepts one of multiple shapes |
| `timezone` | oneOf | No | — | Accepts one of multiple shapes |
```json
{
  "consts": "...",
  "dark_side_pane": true,
  "disable_animation": true
}
```
**Sample request**:
```bash
curl -X GET "http://<miner-ip>/api/v1/ui"
```

## Logs & Diagnostics
Fetch or clear on-device log files to assist troubleshooting.

| Method | Path | Summary | Auth | Request | Success (200) |
| --- | --- | --- | --- | --- | --- |
| `GET` | `/logs/{log_type}` | Read log file | — | — | Log file was read successfully |
| `POST` | `/logs/{log_type}/clear` | Clear logs | `bearerAuth`, `apikeyAuth` | — | Logs was cleared successfully |

### `GET /logs/{log_type}`
**Summary**: Read log file.
**When to use**: Download the contents of a selected log file for troubleshooting.
**Parameters**:
| Name | Location | Type | Required | Description |
| --- | --- | --- | --- | --- |
| `log_type` | `path` | `LogType` | Yes | Log type name. All logs `*` are not implemented for this route |
**Responses**:
| Code | Description | Payload |
| --- | --- | --- |
| `200` | Log file was read successfully | — |
| `500` | Internal Server Error | application/json → `ErrDescr` |
**Sample request**:
```bash
curl -X GET "http://<miner-ip>/api/v1/logs/{log_type}"
```

### `POST /logs/{log_type}/clear`
**Summary**: Clear logs.
**When to use**: Purge a specific log file once you have archived it elsewhere.
**Authentication**: JWT bearer token (`Authorization: Bearer <token>`); API key header (`x-api-key: <key>`).
**Parameters**:
| Name | Location | Type | Required | Description |
| --- | --- | --- | --- | --- |
| `log_type` | `path` | `LogType` | Yes | Log type |
**Responses**:
| Code | Description | Payload |
| --- | --- | --- |
| `200` | Logs was cleared successfully | — |
| `401` | Unauthorized | — |
| `500` | Internal Server Error | application/json → `ErrDescr` |
**Sample request**:
```bash
curl -X POST "http://<miner-ip>/api/v1/logs/{log_type}/clear" \
  -H "Authorization: Bearer <token>" \
  -H "x-api-key: <key>"
```

## Mining Control Loop
Start, stop, or adjust mining operations without rebooting the rig.

| Method | Path | Summary | Auth | Request | Success (200) |
| --- | --- | --- | --- | --- | --- |
| `POST` | `/mining/pause` | Mining pause | `bearerAuth`, `apikeyAuth` | — | Mining paused |
| `POST` | `/mining/restart` | Mining restart | `bearerAuth`, `apikeyAuth` | — | Mining restart |
| `POST` | `/mining/resume` | Mining resume | `bearerAuth`, `apikeyAuth` | — | Mining resumed |
| `POST` | `/mining/start` | Mining start | `bearerAuth`, `apikeyAuth` | — | Mining started |
| `POST` | `/mining/stop` | Mining stop | `bearerAuth`, `apikeyAuth` | — | Mining stopped |
| `POST` | `/mining/switch-pool` | Mining switch pool | `bearerAuth`, `apikeyAuth` | `SwitchPoolQuery` | Pool was switched successfully |

### `POST /mining/pause`
**Summary**: Mining pause.
**When to use**: Pause the hashing loop while leaving the miner otherwise online.
**Authentication**: JWT bearer token (`Authorization: Bearer <token>`); API key header (`x-api-key: <key>`).
**Responses**:
| Code | Description | Payload |
| --- | --- | --- |
| `200` | Mining paused | — |
| `401` | Unauthorized | — |
| `500` | Internal Server Error | application/json → `ErrDescr` |
**Sample request**:
```bash
curl -X POST "http://<miner-ip>/api/v1/mining/pause" \
  -H "Authorization: Bearer <token>" \
  -H "x-api-key: <key>"
```

### `POST /mining/restart`
**Summary**: Mining restart.
**When to use**: Restart the mining process without rebooting the device.
**Authentication**: JWT bearer token (`Authorization: Bearer <token>`); API key header (`x-api-key: <key>`).
**Responses**:
| Code | Description | Payload |
| --- | --- | --- |
| `200` | Mining restart | — |
| `401` | Unauthorized | — |
| `500` | Internal Server Error | application/json → `ErrDescr` |
**Sample request**:
```bash
curl -X POST "http://<miner-ip>/api/v1/mining/restart" \
  -H "Authorization: Bearer <token>" \
  -H "x-api-key: <key>"
```

### `POST /mining/resume`
**Summary**: Mining resume.
**When to use**: Resume hashing after a pause event.
**Authentication**: JWT bearer token (`Authorization: Bearer <token>`); API key header (`x-api-key: <key>`).
**Responses**:
| Code | Description | Payload |
| --- | --- | --- |
| `200` | Mining resumed | — |
| `401` | Unauthorized | — |
| `500` | Internal Server Error | application/json → `ErrDescr` |
**Sample request**:
```bash
curl -X POST "http://<miner-ip>/api/v1/mining/resume" \
  -H "Authorization: Bearer <token>" \
  -H "x-api-key: <key>"
```

### `POST /mining/start`
**Summary**: Mining start.
**When to use**: Kick off hashing after configuration or maintenance.
**Authentication**: JWT bearer token (`Authorization: Bearer <token>`); API key header (`x-api-key: <key>`).
**Responses**:
| Code | Description | Payload |
| --- | --- | --- |
| `200` | Mining started | — |
| `401` | Unauthorized | — |
| `500` | Internal Server Error | application/json → `ErrDescr` |
**Sample request**:
```bash
curl -X POST "http://<miner-ip>/api/v1/mining/start" \
  -H "Authorization: Bearer <token>" \
  -H "x-api-key: <key>"
```

### `POST /mining/stop`
**Summary**: Mining stop.
**When to use**: Stop hashing gracefully, typically before service work.
**Authentication**: JWT bearer token (`Authorization: Bearer <token>`); API key header (`x-api-key: <key>`).
**Responses**:
| Code | Description | Payload |
| --- | --- | --- |
| `200` | Mining stopped | — |
| `401` | Unauthorized | — |
| `500` | Internal Server Error | application/json → `ErrDescr` |
**Sample request**:
```bash
curl -X POST "http://<miner-ip>/api/v1/mining/stop" \
  -H "Authorization: Bearer <token>" \
  -H "x-api-key: <key>"
```

### `POST /mining/switch-pool`
**Summary**: Mining switch pool.
**When to use**: Point the miner at a different configured pool slot.
**Authentication**: JWT bearer token (`Authorization: Bearer <token>`); API key header (`x-api-key: <key>`).
**Request body**:
- Required
- `application/json` payload uses `SwitchPoolQuery`:
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `pool_id` | integer (int32) | Yes | — | — |
  Example:
```json
{
  "pool_id": 1
}
```
**Responses**:
| Code | Description | Payload |
| --- | --- | --- |
| `200` | Pool was switched successfully | — |
| `401` | Unauthorized | — |
| `500` | Internal Server Error | application/json → `ErrDescr` |
**Sample request**:
```bash
curl -X POST "http://<miner-ip>/api/v1/mining/switch-pool" \
  -H "Authorization: Bearer <token>" \
  -H "x-api-key: <key>" \
  -H "Content-Type: application/json" \
  -d '{
  "pool_id": 1
}'
```

## Operational Metrics
Pull streaming telemetry used for dashboards and automation triggers.

| Method | Path | Summary | Auth | Request | Success (200) |
| --- | --- | --- | --- | --- | --- |
| `GET` | `/metrics` | Get metrics | — | — | application/json → `MetricsReply` |

### `GET /metrics`
**Summary**: Get metrics.
**When to use**: Stream time-series metrics suitable for dashboards or automation triggers.
**Parameters**:
| Name | Location | Type | Required | Description |
| --- | --- | --- | --- | --- |
| `time_slice` | `query` | integer (int32) | No | Amount of seconds until now. Max is 3 days (3 * 24 * 60 * 60) Default is 1 day (24 * 60 * 60) |
| `step` | `query` | integer (int32) | No | Resample step in seconds to count average, default is 15 min (15 * 60) |
**Responses**:
| Code | Description | Payload |
| --- | --- | --- |
| `200` | Config saved successfully | application/json → `MetricsReply` |
| `500` | Internal Server Error | application/json → `ErrDescr` |
**Success payload fields (`MetricsReply`)**:
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `annotations` | array<`TimeRecord_MetricAnnotation`> | Yes | — | — |
| `metrics` | array<`TimeRecord_MetricsData`> | Yes | — | — |
| `timezone` | `Timezone` | Yes | — | — |
```json
{
  "annotations": [
    {
      "data": {
        "chain_id": "...",
        "type": "..."
      },
      "time": 1
    }
  ],
  "metrics": [
    {
      "data": {
        "chip_max_temp": "...",
        "fan_duty": "...",
        "hashrate": "..."
      },
      "time": 1
    }
  ],
  "timezone": "GMT+1"
}
```
**Sample request**:
```bash
curl -X GET "http://<miner-ip>/api/v1/metrics"
```

## Operator Notes
CRUD endpoints for the lightweight key/value note store on the miner.

| Method | Path | Summary | Auth | Request | Success (200) |
| --- | --- | --- | --- | --- | --- |
| `GET` | `/notes` | Read all notes | — | — | application/json → map<string, string> |
| `POST` | `/notes` | Add note | `bearerAuth`, `apikeyAuth` | `NoteKeyValue` | Note add successfully |
| `DELETE` | `/notes/{note}` | Delete note | `bearerAuth`, `apikeyAuth` | — | Note delete successfully |
| `GET` | `/notes/{note}` | Get one note json | — | — | application/json → `NoteValue` |
| `PUT` | `/notes/{note}` | Update notes | `bearerAuth`, `apikeyAuth` | `NoteValue` | Notes add successfully |

### `GET /notes`
**Summary**: Read all notes.
**When to use**: Read all operator notes stored on the miner.
**Responses**:
| Code | Description | Payload |
| --- | --- | --- |
| `200` | Notes read successfully | application/json → map<string, string> |
| `500` | Internal Server Error | application/json → `ErrDescr` |
**Sample request**:
```bash
curl -X GET "http://<miner-ip>/api/v1/notes"
```

### `POST /notes`
**Summary**: Add note.
**When to use**: Create a new note for operators or automation scripts to reference.
**Authentication**: JWT bearer token (`Authorization: Bearer <token>`); API key header (`x-api-key: <key>`).
**Request body**:
- Required
- `application/json` payload uses `NoteKeyValue`:
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `value` | string | Yes | — | — |
| `key` | string | Yes | — | — |
  Example:
```json
{
  "value": "value_value",
  "key": "key_value"
}
```
**Responses**:
| Code | Description | Payload |
| --- | --- | --- |
| `200` | Note add successfully | — |
| `401` | Unauthorized | — |
| `409` | Conflict | application/json → `ErrDescr` |
| `500` | Internal Server Error | application/json → `ErrDescr` |
| `507` | Insufficient Storage | application/json → `ErrDescr` |
**Sample request**:
```bash
curl -X POST "http://<miner-ip>/api/v1/notes" \
  -H "Authorization: Bearer <token>" \
  -H "x-api-key: <key>" \
  -H "Content-Type: application/json" \
  -d '{
  "value": "value_value",
  "key": "key_value"
}'
```

### `DELETE /notes/{note}`
**Summary**: Delete note.
**When to use**: Delete a note once it is obsolete.
**Authentication**: JWT bearer token (`Authorization: Bearer <token>`); API key header (`x-api-key: <key>`).
**Parameters**:
| Name | Location | Type | Required | Description |
| --- | --- | --- | --- | --- |
| `note` | `path` | string | Yes | Note key |
**Responses**:
| Code | Description | Payload |
| --- | --- | --- |
| `200` | Note delete successfully | — |
| `401` | Unauthorized | — |
| `404` | Not found | application/json → `ErrDescr` |
| `500` | Internal Server Error | application/json → `ErrDescr` |
**Sample request**:
```bash
curl -X DELETE "http://<miner-ip>/api/v1/notes/{note}" \
  -H "Authorization: Bearer <token>" \
  -H "x-api-key: <key>"
```

### `GET /notes/{note}`
**Summary**: Get one note json.
**When to use**: Retrieve the value for a specific operator note.
**Parameters**:
| Name | Location | Type | Required | Description |
| --- | --- | --- | --- | --- |
| `note` | `path` | string | Yes | Note key |
**Responses**:
| Code | Description | Payload |
| --- | --- | --- |
| `200` | Notes read successfully | application/json → `NoteValue` |
| `404` | Not found | application/json → `ErrDescr` |
| `500` | Internal Server Error | application/json → `ErrDescr` |
**Success payload fields (`NoteValue`)**:
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `value` | string | Yes | — | — |
```json
{
  "value": "value_value"
}
```
**Sample request**:
```bash
curl -X GET "http://<miner-ip>/api/v1/notes/{note}"
```

### `PUT /notes/{note}`
**Summary**: Update notes.
**When to use**: Update the contents of an existing operator note.
**Authentication**: JWT bearer token (`Authorization: Bearer <token>`); API key header (`x-api-key: <key>`).
**Parameters**:
| Name | Location | Type | Required | Description |
| --- | --- | --- | --- | --- |
| `note` | `path` | string | Yes | Note key |
**Request body**:
- Required
- `application/json` payload uses `NoteValue`:
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `value` | string | Yes | — | — |
  Example:
```json
{
  "value": "value_value"
}
```
**Responses**:
| Code | Description | Payload |
| --- | --- | --- |
| `200` | Notes add successfully | — |
| `401` | Unauthorized | — |
| `500` | Internal Server Error | application/json → `ErrDescr` |
**Sample request**:
```bash
curl -X PUT "http://<miner-ip>/api/v1/notes/{note}" \
  -H "Authorization: Bearer <token>" \
  -H "x-api-key: <key>" \
  -H "Content-Type: application/json" \
  -d '{
  "value": "value_value"
}'
```

## System Control
Device-level lifecycle operations prone to reboot or power changes.

| Method | Path | Summary | Auth | Request | Success (200) |
| --- | --- | --- | --- | --- | --- |
| `POST` | `/system/reboot` | System reboot | `bearerAuth`, `apikeyAuth` | — | application/json → `RebootAfter` |

### `POST /system/reboot`
**Summary**: System reboot.
**When to use**: Initiate an immediate system reboot.
**Authentication**: JWT bearer token (`Authorization: Bearer <token>`); API key header (`x-api-key: <key>`).
**Responses**:
| Code | Description | Payload |
| --- | --- | --- |
| `200` | System reboot after | application/json → `RebootAfter` |
| `401` | Unauthorized | — |
**Success payload fields (`RebootAfter`)**:
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `after` | integer (int64) | Yes | Number of seconds after the system will reboot By default, is 3 seconds | — |
```json
{
  "after": 1
}
```
**Sample request**:
```bash
curl -X POST "http://<miner-ip>/api/v1/system/reboot" \
  -H "Authorization: Bearer <token>" \
  -H "x-api-key: <key>"
```
> **Note:** The HTTP response returns immediately; the device begins rebooting a few seconds later.

## Warranty Lifecycle
Endpoints that activate or cancel the miner's hardware warranty record.

| Method | Path | Summary | Auth | Request | Success (200) |
| --- | --- | --- | --- | --- | --- |
| `POST` | `/activate-warranty` | Warranty activate | `bearerAuth`, `apikeyAuth` | — | application/json → `WarrantyStatus` |
| `POST` | `/cancel-warranty` | Warranty cancel | `bearerAuth`, `apikeyAuth` | — | application/json → `WarrantyStatus` |

### `POST /activate-warranty`
**Summary**: Warranty activate.
**When to use**: Activate the miner's warranty record after onboarding new hardware or completing a major repair.
**Authentication**: JWT bearer token (`Authorization: Bearer <token>`); API key header (`x-api-key: <key>`).
**Responses**:
| Code | Description | Payload |
| --- | --- | --- |
| `200` | Warranty was successfully activated | application/json → `WarrantyStatus` |
| `401` | Unauthorized | — |
| `500` | Internal Server Error | application/json → `ErrDescr` |
**Success payload fields (`WarrantyStatus`)**:
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `success` | boolean | Yes | — | — |
| `warranty` | `Warranty` | No | — | — |
```json
{
  "success": true,
  "warranty": "active"
}
```
**Sample request**:
```bash
curl -X POST "http://<miner-ip>/api/v1/activate-warranty" \
  -H "Authorization: Bearer <token>" \
  -H "x-api-key: <key>"
```

### `POST /cancel-warranty`
**Summary**: Warranty cancel.
**When to use**: Cancel the active warranty when hardware is decommissioned or moved out of contract.
**Authentication**: JWT bearer token (`Authorization: Bearer <token>`); API key header (`x-api-key: <key>`).
**Responses**:
| Code | Description | Payload |
| --- | --- | --- |
| `200` | Warranty canceled successfully, or was not provided | application/json → `WarrantyStatus` |
| `401` | Unauthorized | — |
| `500` | Internal Server Error | application/json → `ErrDescr` |
**Success payload fields (`WarrantyStatus`)**:
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `success` | boolean | Yes | — | — |
| `warranty` | `Warranty` | No | — | — |
```json
{
  "success": true,
  "warranty": "active"
}
```
**Sample request**:
```bash
curl -X POST "http://<miner-ip>/api/v1/cancel-warranty" \
  -H "Authorization: Bearer <token>" \
  -H "x-api-key: <key>"
```

## Appendix A – Shared Data Models
The tables below describe the schemas referenced across multiple endpoints. Optional fields appear with `No` in the Required column.

### `AddApiKeyRes`
Type: `object`
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `status` | `AddApiKeyStatus` | Yes | — | — |

### `AddApiKeyStatus`
Enum values: inserted, updated, nochanges
Type: `string`

### `AddApikeyQuery`
Type: `object`
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `description` | string | Yes | — | — |
| `key` | string | Yes | — | — |

### `AdvancedSettingsRaw`
Type: `object`
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `asic_boost` | boolean | No | — | — |
| `auto_chip_throttling` | boolean | No | Automatically adjusts chip frequencies based on temperatures | — |
| `bitmain_disable_volt_comp` | boolean | No | Disable voltage compensation feature | — |
| `disable_chain_break_protection` | boolean | No | — | — |
| `disable_restart_unbalanced` | boolean | No | — | — |
| `disable_volt_checks` | boolean | No | — | — |
| `downscale_preset_on_failure` | boolean | No | Automatic preset reduction in case of miner overheating or chain break error | — |
| `higher_volt_offset` | integer (int32) | No | Higher voltage offset during initialization stage | — |
| `ignore_broken_sensors` | boolean | No | — | — |
| `ignore_chip_sensors` | boolean | No | — | — |
| `max_restart_attempts` | integer (int32) | No | — | — |
| `max_startup_delay_time` | integer (int32) | No | Maximum delay time before mining startup | — |
| `min_operational_chains` | integer (int32) | No | — | — |
| `quick_start` | boolean | No | — | — |
| `quiet_mode` | boolean | No | — | — |
| `remain_stopped_on_reboot` | boolean | No | — | — |
| `restart_hashrate` | integer (int32) | No | Percent, `0` to disable | — |
| `restart_temp` | integer (int32) | No | — | — |
| `tuner_bad_chip_hr_threshold` | integer (int32) | No | Autotuning: hashrate threshold below which the chips are marked as bad ones | — |

### `AntmChain`
Type: `object`
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `chip_statuses` | `ChainChipStatuses` | Yes | — | — |
| `chip_temp` | `TempRange` | Yes | — | — |
| `frequency` | number (float) | Yes | — | — |
| `hashrate_ideal` | number (float) | Yes | — | — |
| `hashrate_percentage` | number (float) | Yes | — | — |
| `hashrate_rt` | number (float) | Yes | — | — |
| `hr_error` | number (float) | Yes | — | — |
| `hw_errors` | integer (int32) | Yes | — | — |
| `id` | integer | Yes | — | — |
| `inlet_water_temp` | integer (int32) | No | — | — |
| `outlet_water_temp` | integer (int32) | No | — | — |
| `pcb_temp` | `TempRange` | Yes | — | — |
| `power_consumption` | integer (int32) | Yes | — | — |
| `status` | `ChainStatus` | Yes | — | — |
| `voltage` | integer (int32) | Yes | — | — |

### `AntmChainChips`
Type: `object`
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `chips` | array<`AntmChip`> | Yes | — | — |
| `freq` | number (float) | Yes | — | — |
| `hr_nominal` | number (float) | Yes | — | — |
| `hr_realtime` | number (float) | Yes | — | — |
| `id` | integer | Yes | — | — |
| `sensors` | array<`AntmChipSensor`> | Yes | — | — |
| `status` | `CgChainStatus` | Yes | — | — |

### `AntmChainChipsStats`
Type: `object`
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `chips` | array<`AntmiChipStats`> | Yes | — | — |
| `id` | integer | Yes | — | — |

### `AntmChainsChipsStats`
Type: `object`
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `chains` | array<`AntmChainChipsStats`> | Yes | — | — |
| `chips_per_chain` | integer | Yes | — | — |

### `AntmChip`
Type: `object`
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `errs` | integer (int32) | Yes | — | — |
| `freq` | integer (int32) | Yes | — | — |
| `grade` | `ChipGrade` | Yes | — | — |
| `hr` | number (float) | Yes | — | — |
| `id` | integer | Yes | — | — |
| `temp` | number (float) | Yes | — | — |
| `throttled` | boolean | Yes | — | — |
| `volt` | integer (int32) | Yes | — | — |

### `AntmChipSensor`
Type: `object`
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `board` | integer (int32) | Yes | — | — |
| `chip` | integer (int32) | Yes | — | — |
| `loc` | integer (int32) | Yes | Location that refers to `chip.id` | — |
| `state` | `TempSensorStatus` | Yes | — | — |

### `AntmMinerStats`
Type: `object`
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `average_hashrate` | number (float) | Yes | Deprecated. Same as hr_average but measure is MG/s. | — |
| `best_share` | integer (int64) | Yes | — | — |
| `chains` | array<`AntmChain`> | Yes | — | — |
| `chip_temp` | `TempRange` | Yes | — | — |
| `cooling` | `Cooling` | Yes | — | — |
| `devfee` | number (float) | Yes | — | — |
| `devfee_percent` | number (float) | Yes | — | — |
| `found_blocks` | integer (int32) | Yes | — | — |
| `hr_average` | number (float) | Yes | — | — |
| `hr_error` | number (float) | Yes | Errors rate | — |
| `hr_nominal` | number (float) | Yes | — | — |
| `hr_realtime` | number (float) | Yes | — | — |
| `hr_stock` | number (float) | Yes | — | — |
| `hw_errors` | integer (int32) | Yes | — | — |
| `hw_errors_percent` | number (float) | Yes | — | — |
| `instant_hashrate` | number (float) | Yes | Deprecated. Same as hr_realtime but measure is MG/s. | — |
| `miner_status` | `MinerStatus` | Yes | — | — |
| `miner_type` | string | Yes | — | — |
| `pcb_temp` | `TempRange` | Yes | — | — |
| `pools` | array<`PoolStats`> | Yes | — | — |
| `power_consumption` | integer (int32) | Yes | — | — |
| `power_efficiency` | number (float) | Yes | — | — |
| `power_usage` | integer (int32) | Yes | Deprecated. Same as power_efficiency | — |
| `psu` | oneOf | No | — | Accepts one of multiple shapes |

### `AntmPsuInfo`
Type: `object`
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `psu_power_metering` | boolean | No | — | — |
| `temps` | oneOf | No | — | Accepts one of multiple shapes |

### `AntmPsuTemps`
Type: `object`
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `llc1_temp` | integer (int32) | No | — | — |
| `llc2_temp` | integer (int32) | No | — | — |
| `pfc_temp` | integer (int32) | No | — | — |

### `AntmiChipStats`
Type: `object`
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `errors` | integer (int32) | Yes | — | — |
| `freq` | integer (int32) | Yes | — | — |
| `hashrate` | number (float) | Yes | — | — |
| `id` | integer (int32) | Yes | — | — |
| `sensor` | oneOf | No | — | Accepts one of multiple shapes |
| `status` | `ChipGrade` | Yes | — | — |
| `temp` | number (float) | Yes | — | — |

### `ApiKeysJson`
Type: `array`

### `ApiKeysJsonItem`
Type: `object`
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `description` | string | Yes | — | — |
| `key` | string | Yes | — | — |

### `Apikey`
Type: `string`

### `AuthCheck`
Type: `object`
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `unlock_timeout` | integer (int64) | No | — | — |

### `AutotuneChain`
Type: `object`
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `chips` | array<integer (int32)> | Yes | — | — |
| `freq` | integer (int32) | Yes | — | — |
| `serial` | string | No | — | — |

### `AutotunePresetDto`
Type: `object`
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `modded_psu_required` | boolean | Yes | — | — |
| `name` | string | Yes | Preset id name | — |
| `pretty` | string | Yes | Preset human-readable name | — |
| `status` | `AutotunePresetStatus` | Yes | — | — |
| `tune_settings` | oneOf | No | — | Accepts one of multiple shapes |

### `AutotunePresetStatus`
Preset status. `tuned` means that preset tuned successfully
Enum values: untuned, tuned
Type: `string`

### `AutotunePresets`
Type: `array`

### `AutotunePresetsItem`
Type: `object`
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `modded_psu_required` | boolean | Yes | — | — |
| `name` | string | Yes | Preset id name | — |
| `pretty` | string | Yes | Preset human-readable name | — |
| `status` | `AutotunePresetStatus` | Yes | — | — |

### `AutotuneReset`
Type: `object`
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `presets` | array<string> | Yes | List of presets to reset | — |
| `restart` | boolean | Yes | Restart after presets remove | — |

### `AutotuneResetAll`
Type: `object`
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `restart` | boolean | Yes | — | — |

### `AutotuneResultsItem`
Type: `object`
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `chains` | array<`AutotuneChain`> | Yes | — | — |
| `freq` | integer (int32) | Yes | — | — |
| `hashrate` | integer (int32) | Yes | — | — |
| `modified` | boolean | Yes | — | — |
| `volt` | integer (int32) | Yes | — | — |

### `CgChainStatus`
Type: `object`
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `failure_message` | string | No | — | — |
| `state` | `ChainState` | Yes | — | — |

### `ChainChipStatuses`
Type: `object`
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `grey` | integer (int32) | Yes | — | — |
| `orange` | integer (int32) | Yes | — | — |
| `red` | integer (int32) | Yes | — | — |

### `ChainRaw`
Type: `object`
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `chips` | array<integer (int32)> | No | An array of per chip `freq` settings values. `0` (zero) value means that value used from `chain` settings | — |
| `disabled` | boolean | No | Chain `disabled` settings value | — |
| `freq` | integer (int32) | No | Chain `freq` settings values. `0` (zero) value means that value used from `globals` settings | — |

### `ChainState`
Enum values: initializing, mining, stopped, failure, disconnected, disabled, unknown
Type: `string`

### `ChainStatus`
Type: `object`
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `description` | string | No | — | — |
| `state` | `ChainState` | Yes | — | — |

### `ChipGrade`
Enum values: grey, orange, red
Type: `string`

### `Consts`
Type: `object`
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `cooling` | `CoolingConsts` | Yes | — | — |
| `overclock` | `Overclock` | Yes | — | — |
| `timezones` | array<array<False>> | Yes | Available timezones list. A purpose for this field is to display timezones list in UI. Makes sense for GET queries only and shall ignore for UPDATE queries. | — |

### `Cooling`
Type: `object`
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `fan_duty` | integer (int32) | Yes | — | — |
| `fan_num` | integer | Yes | — | — |
| `fans` | array<`Fan`> | Yes | — | — |
| `settings` | `FanSettings` | Yes | — | — |

### `CoolingConsts`
Type: `object`
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `fan_min_count` | oneOf | No | — | Accepts one of multiple shapes |
| `max_target_temp` | integer (int32) | Yes | — | — |
| `min_fan_pwm` | integer (int32) | Yes | — | — |
| `min_target_temp` | integer (int32) | Yes | — | — |

### `CoolingSettingsRaw`
Type: `object`
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `fan_max_duty` | integer (int32) | No | — | — |
| `fan_min_count` | integer (int32) | No | — | — |
| `fan_min_duty` | integer (int32) | No | — | — |
| `mode` | oneOf | No | — | Accepts one of multiple shapes |

### `CurrentPreset`
Type: `object`
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `modded_psu_required` | boolean | Yes | — | — |
| `name` | string | Yes | Preset id name | — |
| `pretty` | string | Yes | Preset human-readable name | — |
| `status` | `AutotunePresetStatus` | Yes | — | — |
| `globals` | oneOf | No | — | Accepts one of multiple shapes |

### `DeleteApikeyQuery`
Type: `object`
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `key` | string | Yes | — | — |

### `DiagReportQueryInput`
Type: `object`
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `issue` | string | Yes | Issue text. Max 16KB | — |

### `ErrDescr`
Type: `object`
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `err` | string | Yes | — | — |

### `FactoryInfoChain`
Type: `object`
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `board_model` | string | Yes | — | — |
| `chip_bin` | integer (int32) | Yes | — | — |
| `freq` | integer (int32) | Yes | — | — |
| `hashrate` | number (double) | Yes | — | — |
| `id` | integer (int32) | Yes | — | — |
| `serial` | string | Yes | — | — |
| `volt` | integer (int32) | Yes | — | — |

### `FactoryInfoReply`
Type: `object`
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `chains` | array<`FactoryInfoChain`> | No | — | — |
| `has_pics` | boolean | No | — | — |
| `hr_stock` | number (double) | No | — | — |
| `psu_model` | string | No | — | — |
| `psu_serial` | string | No | — | — |

### `Fan`
Type: `object`
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `id` | integer | Yes | — | — |
| `max_rpm` | integer (int32) | Yes | — | — |
| `rpm` | integer (int32) | Yes | — | — |
| `status` | `FanStatus` | Yes | — | — |

### `FanMode`
Enum values: manual, immersion, auto
Type: `string`

### `FanSettings`

### `FanStatus`
Enum values: ok, lost
Type: `string`

### `FindMinerStatus`
Type: `object`
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `on` | boolean | Yes | Find miner on/off | — |

### `FwInfo`
Type: `object`
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `build_name` | string | No | — | — |
| `build_time` | string | Yes | Build time | — |
| `build_uuid` | string | No | — | — |
| `fw_name` | string | Yes | Firmware name | — |
| `fw_version` | string | Yes | Firmware version | — |
| `install_type` | `InstallType` | Yes | — | — |
| `miner` | string | Yes | Pretty miner name | — |
| `model` | string | Yes | Miner model code | — |
| `platform` | `Platform` | Yes | — | — |

### `GlobalsRaw`
Type: `object`
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `freq` | integer (int32) | No | — | — |
| `volt` | integer (int32) | No | — | — |

### `HrMeasure`
Enum values: GH/s, MH/s
Type: `string`

### `InfoJson`
Type: `object`
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `build_name` | string | No | — | — |
| `build_time` | string | Yes | Build time | — |
| `build_uuid` | string | No | — | — |
| `fw_name` | string | Yes | Firmware name | — |
| `fw_version` | string | Yes | Firmware version | — |
| `install_type` | `InstallType` | Yes | — | — |
| `miner` | string | Yes | Pretty miner name | — |
| `model` | string | Yes | Miner model code | — |
| `platform` | `Platform` | Yes | — | — |
| `algorithm` | `MiningAlgorithm` | Yes | — | — |
| `hr_measure` | `HrMeasure` | Yes | — | — |
| `serial` | string | Yes | — | — |
| `system` | `SystemInfo` | Yes | — | — |

### `InputConfig`
Type: `object`
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `layout` | oneOf | No | — | Accepts one of multiple shapes |
| `miner` | oneOf | No | — | Accepts one of multiple shapes |
| `network` | oneOf | No | — | Accepts one of multiple shapes |
| `password` | oneOf | No | — | Accepts one of multiple shapes |
| `regional` | oneOf | No | — | Accepts one of multiple shapes |
| `ui` | oneOf | No | — | Accepts one of multiple shapes |

### `InputNetworkConfFile`
Type: `object`
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `dhcp` | boolean | Yes | — | — |
| `dnsservers` | array<string> | Yes | — | — |
| `gateway` | string | Yes | — | — |
| `hostname` | string | Yes | — | — |
| `ipaddress` | string | Yes | — | — |
| `netmask` | string | Yes | — | — |
| `enable_network_check` | boolean | No | — | — |

### `InstallType`
Install type code sd|nand
Enum values: sd, nand
Type: `string`

### `Layout`
Type: `object`
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `lg` | map<string, string> | No | — | — |
| `md` | map<string, string> | No | — | — |
| `sm` | map<string, string> | No | — | — |
| `xs` | map<string, string> | No | — | — |
| `xxs` | map<string, string> | No | — | — |

### `Locale`
Enum values: ru, en, fa, ua, zh
Type: `string`

### `LogType`
Log type name, `*` for all log types
Enum values: status, miner, autotune, system, messages, api, *
Type: `string`

### `MetricAnnotation`
Type: `object`
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `chain_id` | integer (int32) | No | — | — |
| `type` | `MinerEvent` | Yes | — | — |

### `MetricsData`
Type: `object`
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `chip_max_temp` | integer (int32) | Yes | — | — |
| `fan_duty` | integer (int32) | Yes | — | — |
| `hashrate` | number (double) | Yes | — | — |
| `pcb_max_temp` | integer (int32) | Yes | — | — |
| `power_consumption` | integer (int32) | Yes | — | — |

### `MetricsReply`
Type: `object`
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `annotations` | array<`TimeRecord_MetricAnnotation`> | Yes | — | — |
| `metrics` | array<`TimeRecord_MetricsData`> | Yes | — | — |
| `timezone` | `Timezone` | Yes | — | — |

### `MinerConfigRaw`
Type: `object`
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `cooling` | oneOf | No | — | Accepts one of multiple shapes |
| `misc` | oneOf | No | — | Accepts one of multiple shapes |
| `overclock` | oneOf | No | — | Accepts one of multiple shapes |
| `pools` | array<False> | No | — | — |

### `MinerEvent`
Enum values: start, stop, restart, reboot, overheat, disable_chain, enable_chain
Type: `string`

### `MinerFanMinCount`
Type: `object`
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `default` | integer (int32) | Yes | — | — |
| `max` | integer (int32) | Yes | — | — |
| `min` | integer (int32) | Yes | — | — |

### `MinerModelInfo`
Type: `object`
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `algorithm` | `MiningAlgorithm` | Yes | — | — |
| `chain` | `ModelInfoChain` | Yes | — | — |
| `cooling` | `CoolingConsts` | Yes | — | — |
| `full_name` | string | Yes | Pretty miner name | — |
| `hr_measure` | `HrMeasure` | Yes | — | — |
| `install_type` | `InstallType` | Yes | — | — |
| `model` | string | Yes | Miner model code | — |
| `overclock` | `Overclock` | Yes | — | — |
| `platform` | `Platform` | Yes | — | — |
| `serial` | string | Yes | — | — |
| `series` | `Series` | Yes | — | — |

### `MinerState`
Enum values: mining, initializing, starting, auto-tuning, restarting, shutting-down, stopped, failure
Type: `string`

### `MinerStatus`
Type: `object`
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `description` | string | No | Optional. Description if status is failure | — |
| `failure_code` | integer (int32) | No | — | — |
| `miner_state` | `MinerState` | Yes | — | — |
| `miner_state_time` | integer (int64) | Yes | Time spent in the current state. For now implemented for `mining` state only. | — |

### `MiningAlgorithm`
Enum values: sha256d, scrypt
Type: `string`

### `ModeRaw`

### `ModelInfoChain`
Type: `object`
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `chips_per_chain` | integer | Yes | — | — |
| `chips_per_domain` | integer (int32) | Yes | — | — |
| `num_chains` | integer | Yes | — | — |
| `topology` | `Topology` | Yes | — | — |

### `NetworkConfFile`
Type: `object`
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `dhcp` | boolean | Yes | — | — |
| `dnsservers` | array<string> | Yes | — | — |
| `gateway` | string | Yes | — | — |
| `hostname` | string | Yes | — | — |
| `ipaddress` | string | Yes | — | — |
| `netmask` | string | Yes | — | — |

### `NetworkStatus`
Type: `object`
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `dhcp` | boolean | No | — | — |
| `dns` | array<string> | Yes | — | — |
| `gateway` | string | Yes | — | — |
| `hostname` | string | Yes | — | — |
| `ip` | string | Yes | — | — |
| `mac` | string | Yes | — | — |
| `netmask` | string | Yes | — | — |

### `NoteKeyValue`
Type: `object`
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `value` | string | Yes | — | — |
| `key` | string | Yes | — | — |

### `NoteValue`
Type: `object`
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `value` | string | Yes | — | — |

### `Overclock`
Type: `object`
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `default_freq` | integer (int32) | Yes | — | — |
| `default_voltage` | integer (int32) | Yes | — | — |
| `max_freq` | integer (int32) | Yes | — | — |
| `max_voltage` | integer (int32) | Yes | — | — |
| `max_voltage_stock_psu` | integer (int32) | Yes | — | — |
| `min_freq` | integer (int32) | Yes | — | — |
| `min_voltage` | integer (int32) | Yes | — | — |
| `warn_freq` | integer (int32) | Yes | — | — |

### `OverclockSettingsRaw`
Type: `object`
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `chains` | array<`ChainRaw`> | No | — | — |
| `globals` | oneOf | No | — | Accepts one of multiple shapes |
| `modded_psu` | boolean | No | — | — |
| `preset` | string | No | Profile name | — |
| `preset_switcher` | oneOf | No | — | Accepts one of multiple shapes |

### `PasswordChange`
Type: `object`
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `current` | string | Yes | — | — |
| `pw` | string | Yes | — | — |

### `PerfSummary`
Type: `object`
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `current_preset` | oneOf | No | — | Accepts one of multiple shapes |
| `preset_switcher` | `PresetSwitcherRaw` | Yes | — | — |

### `Platform`
Platform type code aml|bb|cv|stm|xil (Amlogic/BeagleBone/Cvitek/STM/Xilix)
Enum values: aml, bb, cv, stm, xil
Type: `string`

### `Pool`
Type: `object`
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `pass` | string | Yes | — | — |
| `url` | string | Yes | — | — |
| `user` | string | Yes | — | — |

### `PoolStats`
Type: `object`
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `accepted` | integer (int32) | Yes | — | — |
| `asic_boost` | boolean | Yes | — | — |
| `diff` | string | Yes | — | — |
| `diffa` | number (double) | Yes | — | — |
| `id` | integer (int32) | Yes | — | — |
| `ls_diff` | number (float) | Yes | — | — |
| `ls_time` | string | Yes | — | — |
| `ping` | integer (int32) | Yes | — | — |
| `pool_type` | `PoolType` | Yes | — | — |
| `rejected` | integer (int32) | Yes | — | — |
| `stale` | integer (int32) | Yes | — | — |
| `status` | `PoolStatus` | Yes | — | — |
| `url` | string | Yes | — | — |
| `user` | string | Yes | — | — |

### `PoolStatus`
Enum values: offline, working, disabled, active, rejecting, unknown
Type: `string`

### `PoolType`
Enum values: UserPool, DevFee, Refund
Type: `string`

### `PresetSwitcherRaw`
PresetSwitcher settings
Type: `object`
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `autochange_top_preset` | boolean | No | — | — |
| `check_time` | integer (int32) | No | — | — |
| `decrease_temp` | integer (int32) | No | — | — |
| `enabled` | boolean | No | — | — |
| `ignore_fan_speed` | boolean | No | — | — |
| `min_preset` | string | No | — | — |
| `power_delta` | integer (int32) | No | — | — |
| `rise_temp` | integer (int32) | No | — | — |
| `top_preset` | string | No | Profile name. Max profile that preset_switcher can switch | — |

### `RebootAfter`
Type: `object`
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `after` | integer (int64) | Yes | Number of seconds after the system will reboot By default, is 3 seconds | — |

### `RegionalSettings`
Type: `object`
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `timezone` | `TimezoneSettings` | Yes | — | — |

### `SaveConfigResult`
Apply config result
Type: `object`
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `reboot_required` | boolean | Yes | Miner restart required to apply | — |
| `restart_required` | boolean | Yes | Miner restart required to apply config | — |

### `SchemaBoolEnum`
Enum values: true, false
Type: `string`

### `SchemaFirmwareUpdate`
Type: `object`
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `file` | string (binary) | Yes | — | File upload (binary) |
| `keep_settings` | oneOf | No | — | Accepts one of multiple shapes |

### `SchemaSettingsRestore`
Type: `object`
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `file` | string (binary) | Yes | — | File upload (binary) |

### `Series`
Enum values: l7, l9, x19, x21
Type: `string`

### `StatusPane`
Apply config result
Type: `object`
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `description` | string | No | Optional. Description if status is failure | — |
| `failure_code` | integer (int32) | No | — | — |
| `miner_state` | `MinerState` | Yes | — | — |
| `miner_state_time` | integer (int64) | Yes | Time spent in the current state. For now implemented for `mining` state only. | — |
| `reboot_required` | boolean | Yes | Miner restart required to apply | — |
| `restart_required` | boolean | Yes | Miner restart required to apply config | — |
| `find_miner` | boolean | Yes | Flag to switch find_miner function on target devices. Optional, default `false` | — |
| `unlock_timeout` | integer (int64) | No | — | — |
| `unlocked` | boolean | Yes | Show screen-lock status (checks that  any of auth methods satisfies) | — |
| `warranty` | `Warranty` | No | — | — |

### `Summary`
Type: `object`
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `miner` | oneOf | No | — | Accepts one of multiple shapes |

### `SwitchPoolQuery`
Type: `object`
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `pool_id` | integer (int32) | Yes | — | — |

### `SystemInfo`
Type: `object`
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `mem_buf` | integer (int32) | Yes | — | — |
| `mem_buf_percent` | integer (int32) | Yes | — | — |
| `mem_free` | integer (int32) | Yes | — | — |
| `mem_free_percent` | integer (int32) | Yes | — | — |
| `mem_total` | integer (int32) | Yes | — | — |
| `file_system_version` | string | Yes | — | — |
| `miner_name` | string | Yes | — | — |
| `network_status` | `NetworkStatus` | Yes | — | — |
| `os` | string | Yes | — | — |
| `uptime` | string | Yes | — | — |

### `SystemMem`
Type: `object`
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `mem_buf` | integer (int32) | Yes | — | — |
| `mem_buf_percent` | integer (int32) | Yes | — | — |
| `mem_free` | integer (int32) | Yes | — | — |
| `mem_free_percent` | integer (int32) | Yes | — | — |
| `mem_total` | integer (int32) | Yes | — | — |

### `TempRange`
Type: `object`
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `max` | integer (int32) | Yes | — | — |
| `min` | integer (int32) | Yes | — | — |

### `TempSensor`
Type: `object`
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `chip_temp` | integer (int32) | Yes | — | — |
| `pcb_temp` | integer (int32) | Yes | — | — |
| `status` | `TempSensorStatus` | Yes | — | — |

### `TempSensorStatus`
Enum values: init, ready, measure, error, unknown
Type: `string`

### `TimeRecord_MetricAnnotation`
Type: `object`
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `data` | object | Yes | — | — |
| `time` | integer (int64) | Yes | UNIX time | — |

### `TimeRecord_MetricsData`
Type: `object`
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `data` | object | Yes | — | — |
| `time` | integer (int64) | Yes | UNIX time | — |

### `Timezone`
Current timezone name (code)
Enum values: GMT+1, GMT+2, GMT+3, GMT+4, GMT+5, GMT+6, GMT+7, GMT+8, GMT+9, GMT+10, GMT+11, GMT+12, GMT, GMT-1, GMT-2, GMT-3, GMT-4, GMT-5, GMT-6, GMT-7, GMT-8, GMT-9, GMT-10, GMT-11
Type: `string`

### `TimezoneSettings`
Type: `object`
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `current` | `Timezone` | Yes | — | — |

### `Topology`
Type: `object`
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `chips` | array<array<integer (int32)>> | Yes | — | — |
| `num_cols` | integer (int32) | Yes | — | — |
| `num_rows` | integer (int32) | Yes | — | — |

### `UiSettings`
Type: `object`
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `consts` | oneOf | No | — | Accepts one of multiple shapes |
| `dark_side_pane` | boolean | No | — | — |
| `disable_animation` | boolean | No | — | — |
| `locale` | oneOf | No | — | Accepts one of multiple shapes |
| `theme` | oneOf | No | — | Accepts one of multiple shapes |
| `timezone` | oneOf | No | — | Accepts one of multiple shapes |

### `UiTheme`
Enum values: light, dark, auto
Type: `string`

### `UnlockScreenBody`
Type: `object`
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `pw` | string | Yes | Target device(s) password | — |

### `UnlockSuccess`
Type: `object`
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `token` | string | Yes | — | — |

### `ViewConfig`
Type: `object`
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `layout` | oneOf | No | — | Accepts one of multiple shapes |
| `miner` | `MinerConfigRaw` | Yes | — | — |
| `network` | `NetworkConfFile` | Yes | — | — |
| `password` | oneOf | No | — | Accepts one of multiple shapes |
| `regional` | `RegionalSettings` | Yes | — | — |
| `ui` | `UiSettings` | Yes | — | — |

### `Warranty`
Enum values: active, inactive, expired, cancelled, not_provided
Type: `string`

### `WarrantyStatus`
Type: `object`
| Field | Type | Required | Description | Notes |
| --- | --- | --- | --- | --- |
| `success` | boolean | Yes | — | — |
| `warranty` | `Warranty` | No | — | — |
