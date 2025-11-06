# PowerHive Energy Management Implementation Summary

## Completed Backend Implementation

### 1. Database Layer ✅
- **Schema Updates** (`internal/database/schema.go`):
  - `plant_readings` table: Stores hydro plant generation & consumption data
  - `power_balance_events` table: Logs all preset changes made by balancer
  - `app_settings` table: Stores safety margin and other global settings
  - `model_presets.expected_power_w` column: Stores expected power for each preset

- **Types** (`internal/database/types.go`):
  - `PlantReading`, `PlantReadingInput`
  - `PowerBalanceEvent`, `PowerBalanceEventInput`
  - `ModelPreset` (with ExpectedPowerW)

- **Store Methods**:
  - `internal/database/plant.go`: Plant reading CRUD, app settings
  - `internal/database/power_balance.go`: Balance event logging and retrieval
  - `internal/database/models.go`: Preset power management

### 2. Configuration ✅
- **Updated** (`internal/config/config.go`):
  - `PlantConfig`: API endpoint + API key for energy aggregator
  - `IntervalConfig`: Added `PlantSeconds` (15s) and `BalancerSeconds` (15s)
  - Validation ensures plant API key is required

- **Config File** (`config.json`):
  - Plant API endpoint: `https://energy-aggregator.fly.dev/data/latest`
  - Plant API key: `kdM_bTzzDxLxlpOf9ki29S9LyVA-hFmx`
  - Poll intervals: 15s for both plant and balancer

### 3. Background Services ✅

#### Plant Poller (`internal/app/plant_poller.go`)
- Polls energy aggregator API every 15 seconds
- Calculates:
  - Total container consumption (sum of all containers)
  - Available power = generation - container consumption
- Stores readings in `plant_readings` table
- Handles API failures gracefully

#### Power Balancer (`internal/app/power_balancer.go`)
- **Core Algorithm**:
  1. Gets latest plant reading
  2. Applies safety margin (default 10%): target = available × 0.90
  3. Calculates current consumption from managed miners
  4. If consumption > target: reduces power
  5. If consumption < target: increases power (respecting max_preset)

- **Efficiency-Based Prioritization**:
  - Calculates W/TH (watts per terahash) for each miner
  - When reducing: adjusts least efficient miners first
  - When increasing: adjusts most efficient miners first

- **Safety Features**:
  - 30-second cooldown between changes per miner (prevents thrashing)
  - Never exceeds model's `max_preset`
  - Direct preset changes (immediate adjustment)
  - Logs all changes to `power_balance_events`
  - Falls back to real-time power consumption if preset power unavailable

- **Error Handling**:
  - Graceful degradation if plant API unavailable
  - Continues other miners if one fails
  - Logs all failures with context

#### Discovery Updates
- Attempts to extract power consumption from preset `tune_settings`
- Stores preset power if available from firmware API
- Falls back to measured power from status readings

### 4. API Endpoints ✅
- **Plant Data**:
  - `GET /api/plant/latest`: Latest plant reading
  - `GET /api/plant/history?limit=N`: Historical readings

- **Balance Management**:
  - `GET /api/balance/events?miner_id=X&limit=N`: Balance event log
  - `GET /api/balance/status`: Current balance status with computed metrics

- **Settings**:
  - `GET /api/settings`: Get all settings (safety margin)
  - `PATCH /api/settings/safety-margin`: Update safety margin (0-50%)

- **Models** (Enhanced):
  - Added `presets_power` array to model DTOs
  - Includes preset name + expected power for each preset

### 5. Firmware Client Updates ✅
- **New Method**: `SetPreset(ctx, apiKey, preset)` - Changes miner preset
- **Enhanced Type**: `AutotunePreset.TuneSettings` - Captures power data if available

### 6. Service Orchestration ✅
- `internal/app/app.go` updated to start:
  - PlantPoller
  - PowerBalancer
- Both services run concurrently with existing services
- Proper shutdown handling on context cancellation

---

## Frontend Implementation (Remaining)

### Required Changes

#### 1. Add Chart.js Library
**File**: `internal/server/web/index.html`
```html
<!-- Add before closing </body> -->
<script src="https://cdn.jsdelivr.net/npm/chart.js@4.4.0/dist/chart.umd.min.js"></script>
```

#### 2. Energy Management Dashboard Section
**File**: `internal/server/web/index.html`

Add after existing sections:
```html
<section id="energy-section" class="section">
  <h2>Energy Management</h2>

  <!-- Balance Status Card -->
  <div class="balance-status">
    <h3>Current Status: <span id="balance-status-text">...</span></h3>
    <div class="metrics-grid">
      <div class="metric">
        <label>Plant Generation:</label>
        <span id="plant-generation">...</span> kW
      </div>
      <div class="metric">
        <label>Container Consumption:</label>
        <span id="container-consumption">...</span> kW
      </div>
      <div class="metric">
        <label>Available Power:</label>
        <span id="available-power">...</span> kW
      </div>
      <div class="metric">
        <label>Target Power:</label>
        <span id="target-power">...</span> kW
      </div>
      <div class="metric">
        <label>Current Consumption:</label>
        <span id="current-consumption">...</span> W
      </div>
      <div class="metric">
        <label>Safety Margin:</label>
        <input type="number" id="safety-margin-input" min="0" max="50" step="1" value="10">%
        <button id="update-safety-margin">Update</button>
      </div>
    </div>
  </div>

  <!-- Generation vs Consumption Chart -->
  <div class="chart-container">
    <canvas id="energy-chart"></canvas>
  </div>

  <!-- Balance Events Log -->
  <div class="balance-events">
    <h3>Recent Balance Events</h3>
    <table id="balance-events-table">
      <thead>
        <tr>
          <th>Time</th>
          <th>Miner</th>
          <th>Change</th>
          <th>Reason</th>
          <th>Status</th>
        </tr>
      </thead>
      <tbody></tbody>
    </table>
  </div>
</section>
```

#### 3. JavaScript Implementation
**File**: `internal/server/web/app.js`

Add state management:
```javascript
const state = {
    // ... existing state
    plantData: [],
    balanceStatus: null,
    balanceEvents: [],
    energyChart: null,
};
```

Add Chart.js initialization:
```javascript
function initEnergyChart() {
    const ctx = document.getElementById('energy-chart').getContext('2d');
    state.energyChart = new Chart(ctx, {
        type: 'line',
        data: {
            labels: [],
            datasets: [
                {
                    label: 'Generation (kW)',
                    data: [],
                    borderColor: 'rgb(75, 192, 192)',
                    backgroundColor: 'rgba(75, 192, 192, 0.1)',
                    tension: 0.1
                },
                {
                    label: 'Consumption (kW)',
                    data: [],
                    borderColor: 'rgb(255, 99, 132)',
                    backgroundColor: 'rgba(255, 99, 132, 0.1)',
                    tension: 0.1
                },
                {
                    label: 'Target (kW)',
                    data: [],
                    borderColor: 'rgb(255, 205, 86)',
                    backgroundColor: 'rgba(255, 205, 86, 0.1)',
                    borderDash: [5, 5],
                    tension: 0.1
                }
            ]
        },
        options: {
            responsive: true,
            maintainAspectRatio: false,
            scales: {
                y: {
                    beginAtZero: true,
                    title: { display: true, text: 'Power (kW)' }
                },
                x: {
                    title: { display: true, text: 'Time' }
                }
            },
            plugins: {
                legend: { position: 'top' },
                title: { display: true, text: 'Generation vs Consumption' }
            }
        }
    });
}
```

Add API fetching functions:
```javascript
async function fetchBalanceStatus() {
    const data = await fetchJSON('/api/balance/status');
    state.balanceStatus = data;
    renderBalanceStatus();
}

async function fetchPlantHistory() {
    const data = await fetchJSON('/api/plant/history?limit=50');
    state.plantData = data;
    updateEnergyChart();
}

async function fetchBalanceEvents() {
    const data = await fetchJSON('/api/balance/events?limit=20');
    state.balanceEvents = data;
    renderBalanceEvents();
}

async function updateSafetyMargin() {
    const value = parseFloat(document.getElementById('safety-margin-input').value);
    await fetchJSON('/api/settings/safety-margin', {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ safety_margin_percent: value })
    });
    showToast('Safety margin updated', 'success');
    await fetchBalanceStatus();
}
```

Add rendering functions:
```javascript
function renderBalanceStatus() {
    if (!state.balanceStatus) return;

    const status = state.balanceStatus;
    document.getElementById('balance-status-text').textContent = status.status;
    document.getElementById('plant-generation').textContent = status.plant_generation_kw.toFixed(2);
    document.getElementById('container-consumption').textContent = status.plant_container_kw.toFixed(2);
    document.getElementById('available-power').textContent = status.available_power_kw.toFixed(2);
    document.getElementById('target-power').textContent = status.target_power_kw.toFixed(2);
    document.getElementById('current-consumption').textContent = (status.current_consumption_w).toFixed(0);
    document.getElementById('safety-margin-input').value = status.safety_margin_percent;

    // Color code status
    const statusEl = document.getElementById('balance-status-text');
    statusEl.className = 'status-' + status.status.toLowerCase();
}

function updateEnergyChart() {
    if (!state.energyChart || !state.plantData.length) return;

    const data = state.plantData.slice().reverse().slice(-30); // Last 30 readings
    const labels = data.map(d => new Date(d.recorded_at).toLocaleTimeString());
    const generation = data.map(d => d.total_generation);
    const consumption = data.map(d => d.total_container_consumption);

    // Calculate target for each point
    const safetyMargin = state.balanceStatus?.safety_margin_percent || 10;
    const target = data.map(d => d.available_power * (1 - safetyMargin / 100));

    state.energyChart.data.labels = labels;
    state.energyChart.data.datasets[0].data = generation;
    state.energyChart.data.datasets[1].data = consumption;
    state.energyChart.data.datasets[2].data = target;
    state.energyChart.update();
}
```

Update main refresh function:
```javascript
async function refresh() {
    await Promise.all([
        fetchMiners(),
        fetchModels(),
        fetchBalanceStatus(),
        fetchPlantHistory(),
        fetchBalanceEvents()
    ]);
}
```

#### 4. CSS Styling
**File**: `internal/server/web/styles.css`

```css
.balance-status {
    background: var(--card-bg);
    padding: 1.5rem;
    border-radius: 8px;
    margin-bottom: 2rem;
}

.metrics-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
    gap: 1rem;
    margin-top: 1rem;
}

.metric {
    padding: 0.5rem;
}

.metric label {
    display: block;
    font-weight: 600;
    margin-bottom: 0.25rem;
}

.chart-container {
    background: var(--card-bg);
    padding: 1.5rem;
    border-radius: 8px;
    margin-bottom: 2rem;
    height: 400px;
}

#balance-status-text.status-ok {
    color: var(--success);
}

#balance-status-text.status-warning {
    color: var(--warning);
}

#balance-status-text.status-over_target {
    color: var(--danger);
}

.balance-events table {
    width: 100%;
    margin-top: 1rem;
}
```

#### 5. Update Model Display
Show preset power in models section - already implemented in backend, just need to display in frontend.

---

## Safety Review & Recommendations

### Critical Safety Mechanisms ✅
1. **Max Preset Enforcement**: Balancer NEVER exceeds `model.max_preset`
2. **Cooldown Protection**: 30-second minimum between changes per miner
3. **Direct Preset Changes**: Immediate response, no gradual stepping
4. **Safety Margin**: Default 10%, user-configurable 0-50%
5. **Graceful Degradation**: System continues if plant API fails
6. **Error Logging**: All failures logged to `power_balance_events`

### Identified Risks & Mitigations

#### 1. Hardware Overheating Risk: **LOW**
- **Risk**: Setting presets too high could overheat miners
- **Mitigation**:
  - Max preset strictly enforced from model database
  - User must explicitly set max_preset per model
  - Default: no preset changes until max_preset configured

#### 2. Preset Thrashing Risk: **MITIGATED**
- **Risk**: Rapid preset changes could stress hardware
- **Mitigation**:
  - 30-second cooldown per miner
  - 500W tolerance before making changes
  - Direct changes (no oscillation)

#### 3. Plant Overload Risk: **MITIGATED**
- **Risk**: Consuming more than plant can generate
- **Mitigation**:
  - Safety margin (default 10%)
  - Real-time monitoring every 15s
  - Automatic reduction when approaching limit
  - Target calculation: available_power × (1 - safety_margin)

####4. Model Mismatch Risk: **MITIGATED**
- **Risk**: Applying wrong presets to wrong model
- **Mitigation**:
  - Presets loaded per model alias
  - Model validation before applying changes
  - Skip miners without model data

#### 5. Network/API Failure Risk: **MITIGATED**
- **Risk**: Loss of plant data or miner connectivity
- **Mitigation**:
  - Services continue independently
  - Balancer skips offline miners
  - Logs warn on failures, doesn't crash
  - Last-known-good data used if plant API fails

#### 6. Database Corruption Risk: **LOW**
- **Risk**: SQLite corruption under concurrent writes
- **Mitigation**:
  - SQLite handles concurrency
  - Each service has own transaction scope
  - Regular backups recommended (document in ops manual)

#### 7. Authentication/Security Risk: **NOTED**
- **Current**: No dashboard authentication
- **Acceptable**: SSH tunnel access only (farm deployment)
- **Recommendation**: Document that dashboard should only be exposed via SSH tunnel or VPN

### Operational Questions for Plant Owners

Create file: `PLANT_OWNER_QUESTIONNAIRE.md`:

1. **Frequency of Changes**: Are you comfortable with preset changes every 15-30 seconds when needed?
2. **Safety Margin**: Is 10% safety margin acceptable, or do you prefer higher/lower?
3. **Response Speed**: Can we make immediate preset changes, or do you prefer gradual transitions?
4. **Electrical Concerns**: Are there any phase balance or frequency stability requirements we should know about?
5. **Emergency Contacts**: Who should be notified if the system detects critical issues?
6. **Consumption Limits**: Are there any absolute power consumption limits beyond generation capacity?
7. **Monitoring Preferences**: How often do you want status reports/alerts?
8. **Backup Power**: What happens if generation drops suddenly? Should we shut down miners completely?

---

## Algorithm Documentation

### Power Balancing Logic

**Input**:
- Plant generation (kW)
- Container consumption (kW)
- Safety margin (%)
- Managed miners with current presets

**Process**:
1. Calculate available power: `generation - container_consumption`
2. Calculate target: `available × (1 - safety_margin/100)`
3. Calculate current miner consumption (sum of preset powers or measured powers)
4. Determine action:
   - If `current > target + 500W`: **REDUCE**
   - If `current < target - 500W`: **INCREASE**
   - Else: **NO ACTION**

5. **For REDUCE**:
   - Calculate efficiency (W/TH) for each miner
   - Sort by efficiency (worst first)
   - For each miner (worst to best):
     - Check cooldown (skip if <30s since last change)
     - Find next lower preset
     - Apply change via firmware API
     - Log event
     - Update delta
     - Stop if within 500W of target

6. **For INCREASE**:
   - Calculate efficiency (W/TH) for each miner
   - Sort by efficiency (best first)
   - For each miner (best to worst):
     - Check cooldown (skip if <30s since last change)
     - Find next higher preset (not exceeding max_preset)
     - Apply change via firmware API
     - Log event
     - Update delta
     - Stop if within 500W of target

**Output**:
- Preset changes logged to `power_balance_events`
- System reaches equilibrium: consumption ≈ target

**Why Efficiency-Based?**
- Maximizes farm hashrate for given power budget
- When reducing: drop least efficient miners first (lose less hashrate)
- When increasing: boost most efficient miners first (gain more hashrate per watt)

---

## Testing Checklist

Before deployment:

- [ ] Test with plant API offline (should continue operation)
- [ ] Test with no miners managed (should not crash)
- [ ] Test with miner offline during balance (should skip gracefully)
- [ ] Test max_preset enforcement (never exceed)
- [ ] Test safety margin update (should affect next balance cycle)
- [ ] Test rapid generation changes (should respond within 15-30s)
- [ ] Test database backup/restore
- [ ] Verify cooldown prevents thrashing
- [ ] Monitor logs for unexpected errors
- [ ] Test firmware API failures (should log and continue)

---

## Next Steps

1. **Complete Frontend** (2-3 hours):
   - Add Chart.js
   - Implement energy dashboard UI
   - Add safety margin controls
   - Display preset power in models section

2. **Testing** (1-2 hours):
   - Integration testing with mock plant API
   - Verify preset changes work correctly
   - Test edge cases (offline miners, API failures)

3. **Documentation** (1 hour):
   - Create operational manual
   - Document troubleshooting steps
   - Create plant owner questionnaire

4. **Deployment**:
   - Update config.json with correct plant API endpoint/key
   - Set max_preset for each model
   - Configure safety margin per plant owner preference
   - Enable managed flag on miners to include in balancing
