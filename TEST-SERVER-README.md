# Test Power Plant API Server

A Python-based test server that simulates the power plant API for testing PowerHive's power balancing automation with realistic but controllable power fluctuations.

## Features

- **Random Walk Fluctuation**: Power values drift gradually up and down within configured bounds
- **Realistic Data Splitting**: Generation split across 2 sources, consumption across 2 containers
- **Configurable Ranges**: Set min/max for both generation and consumption
- **Bearer Token Auth**: Matches PowerHive's authentication expectations
- **Real-time Updates**: Background thread continuously updates values

## Installation

```bash
pip install -r requirements-test-server.txt
```

Or install dependencies directly:
```bash
pip install Flask==3.1.0 Werkzeug==3.1.3
```

## Quick Start

### Basic Usage

```bash
python test-plant-server.py \
  --gen-min 0.05 --gen-max 0.15 \
  --cons-min 0.03 --cons-max 0.10
```

This simulates:
- **Generation**: 50-150 kW (0.05-0.15 MW)
- **Consumption**: 30-100 kW (0.03-0.10 MW)
- **Available power**: Fluctuates between ~-50 kW to ~120 kW

### Testing with Real Miners

For testing with 10-20 miners at ~1000W each:

```bash
python test-plant-server.py \
  --gen-min 0.02 --gen-max 0.05 \
  --cons-min 0.01 --cons-max 0.03 \
  --step 0.002 \
  --interval 15
```

This gives you:
- **Generation**: 20-50 kW
- **Consumption**: 10-30 kW
- **Available**: ~10-40 kW (enough for 10-40 miners)
- Slower changes (±2kW per 15 seconds)

### Aggressive Testing

Fast-changing conditions to stress-test the balancer:

```bash
python test-plant-server.py \
  --gen-min 0.1 --gen-max 0.3 \
  --cons-min 0.05 --cons-max 0.15 \
  --step 0.01 \
  --interval 5
```

## Command-Line Options

| Option | Required | Default | Description |
|--------|----------|---------|-------------|
| `--gen-min` | Yes | - | Minimum generation in MW |
| `--gen-max` | Yes | - | Maximum generation in MW |
| `--cons-min` | Yes | - | Minimum consumption in MW |
| `--cons-max` | Yes | - | Maximum consumption in MW |
| `--step` | No | 0.01 | Maximum change per update (MW) |
| `--interval` | No | 10 | Update interval in seconds |
| `--port` | No | 8090 | HTTP server port |
| `--token` | No | (matches config.json) | Bearer token for auth |
| `--debug` | No | false | Enable debug logging |

## Power Unit Conversion

The API uses **megawatts (MW)**:
- 1 MW = 1,000 kW
- 0.001 MW = 1 kW
- 0.0001 MW = 100 W

**Example conversions:**
- 100 kW → `--gen-min 0.1`
- 50 kW → `--cons-min 0.05`
- 1.5 MW → `--gen-max 1.5`

## API Endpoints

### GET /data/latest?plant_id=complexo-paranhos

Returns power plant reading matching the real API structure.

**Authentication**: Requires `Authorization: Bearer <token>` header

**Response**:
```json
{
  "reading": {
    "collection_timestamp": "2025-11-12T15:34:25.829179Z",
    "consumption": {
      "container_eles": {
        "source_timestamp": "2025-11-12T15:34:26.103732Z",
        "status": "success",
        "value_mw": 45.2
      },
      "container_mazp": {
        "source_timestamp": "2025-11-12T15:34:26.103778Z",
        "status": "success",
        "value_mw": 54.8
      }
    },
    "generation": {
      "generoso": {
        "source_timestamp": "2025-11-12T15:34:26.103678Z",
        "status": "success",
        "value_mw": 120.5
      },
      "nogueira": {
        "source_timestamp": "2025-11-12T15:34:26.103772Z",
        "status": "success",
        "value_mw": 115.3
      }
    },
    "totals": {
      "consumption_mw": 100.0,
      "exported_mw": 135.8,
      "generation_mw": 235.8
    },
    "trust": {
      "confidence_score": 1.0,
      "status": "trusted",
      "summary": "Test data - all checks passed"
    }
  }
}
```

### GET /health

Health check endpoint (no authentication required).

**Response**:
```json
{
  "status": "ok",
  "generation_range": [0.05, 0.15],
  "consumption_range": [0.03, 0.10],
  "current_generation": 0.12,
  "current_consumption": 0.07
}
```

## Integration with PowerHive

### 1. Update PowerHive Config

Edit `config.json`:

```json
{
  "plant": {
    "api_endpoint": "http://localhost:8090/data/latest",
    "api_key": "7ab39eed3b05b7e4efc17285bb416304256979967ec8d6ad2b2ef3bc10c0f5ed",
    "plant_id": "complexo-paranhos"
  }
}
```

### 2. Start Test Server

```bash
python test-plant-server.py --gen-min 0.05 --gen-max 0.15 --cons-min 0.03 --cons-max 0.10
```

### 3. Start PowerHive

```bash
go run cmd/automation/main.go
```

### 4. Monitor

Watch the test server logs to see power fluctuations:
```
2025-11-12 15:34:25 - INFO - Updated: Generation=0.12 MW, Consumption=0.07 MW, Available=0.05 MW
```

Watch PowerHive logs to see balancing decisions:
```
2025-11-12 15:35:10 - INFO - Power balancer: target=108.0kW, current=85.0kW, delta=+23.0kW
2025-11-12 15:35:10 - INFO - Adjusting miner 1 from preset 18 to 20 (+200W)
```

## How Random Walk Works

The algorithm maintains separate random walks for generation and consumption:

1. **Initialize**: Start at random value within range
2. **Each update**:
   - Add random change: `value += random(-step, +step)`
   - Clamp to bounds: `value = clamp(value, min, max)`
3. **Independent**: Generation and consumption walk independently

This creates realistic drift patterns where available power naturally fluctuates as the two values move relative to each other.

## Testing Scenarios

### Scenario 1: Excess Power (Scale Up)

Configure consumption well below generation to test scale-up behavior:

```bash
python test-plant-server.py \
  --gen-min 0.15 --gen-max 0.20 \
  --cons-min 0.02 --cons-max 0.05
```

Expected: PowerHive increases miner presets to consume available power.

### Scenario 2: Power Deficit (Scale Down)

Configure consumption near or above generation to test scale-down:

```bash
python test-plant-server.py \
  --gen-min 0.05 --gen-max 0.10 \
  --cons-min 0.08 --cons-max 0.12
```

Expected: PowerHive reduces miner presets when consumption exceeds generation.

### Scenario 3: Balanced Operation

Configure overlapping ranges for stable operation:

```bash
python test-plant-server.py \
  --gen-min 0.10 --gen-max 0.20 \
  --cons-min 0.08 --cons-max 0.18
```

Expected: PowerHive makes frequent adjustments to track available power.

### Scenario 4: Rapid Changes

Fast fluctuations to test cooldown and tolerance mechanisms:

```bash
python test-plant-server.py \
  --gen-min 0.05 --gen-max 0.15 \
  --cons-min 0.03 --cons-max 0.10 \
  --step 0.02 \
  --interval 5
```

Expected: PowerHive throttles adjustments due to 30s cooldown per miner and 2kW tolerance band.

## Troubleshooting

### Test server won't start

- **Port in use**: Change port with `--port 8091`
- **Missing Flask**: Run `pip install -r requirements-test-server.txt`

### PowerHive not connecting

1. Check `config.json` has correct endpoint: `http://localhost:8090/data/latest`
2. Verify token matches (default is already correct)
3. Check test server logs for incoming requests

### No power balancing happening

1. Ensure miners are marked as "managed" in PowerHive UI
2. Check miners have API keys configured
3. Verify `balancer_seconds` interval in config (default 60s)
4. Check available power is outside 2kW tolerance band
5. Verify miners have max_preset configured in their models

### Values not changing

- Check test server logs show updates
- Verify `--interval` is reasonable (not too large)
- Try `--debug` flag to see detailed updates

## Architecture

```
┌─────────────────────┐
│  PowerState         │
│  ┌───────────────┐  │
│  │ generation_mw │  │ ← Random walk
│  │ consumption_mw│  │ ← Independent walk
│  └───────────────┘  │
│         │           │
│    update() every   │
│    interval seconds │
└─────────┬───────────┘
          │
          │ get_reading()
          ▼
┌─────────────────────┐
│  Flask HTTP Server  │
│  /data/latest       │
│  /health            │
└─────────┬───────────┘
          │
          │ HTTP GET + Bearer auth
          ▼
┌─────────────────────┐
│  PowerHive          │
│  PlantPoller        │
│  PowerBalancer      │
└─────────────────────┘
```

## License

Same as PowerHive project.
