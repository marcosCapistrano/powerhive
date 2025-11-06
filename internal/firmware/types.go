package firmware

import "encoding/json"

// UnlockResponse represents the payload returned by /unlock.
type UnlockResponse struct {
	Token string `json:"token"`
}

// StatusResponse mirrors the lightweight /status payload.
type StatusResponse struct {
	Description     *string `json:"description"`
	FailureCode     *int    `json:"failure_code"`
	MinerState      string  `json:"miner_state"`
	MinerStateTime  *int64  `json:"miner_state_time"`
	RebootRequired  bool    `json:"reboot_required"`
	RestartRequired bool    `json:"restart_required"`
	FindMiner       bool    `json:"find_miner"`
	Unlocked        bool    `json:"unlocked"`
	UnlockTimeout   *int64  `json:"unlock_timeout"`
}

// InfoResponse maps the fields required from /info.
type InfoResponse struct {
	Miner     string     `json:"miner"`
	Model     string     `json:"model"`
	FWName    string     `json:"fw_name"`
	FWVersion string     `json:"fw_version"`
	System    SystemInfo `json:"system"`
	Serial    string     `json:"serial"`
}

// SystemInfo describes the nested system block.
type SystemInfo struct {
	OS            string        `json:"os"`
	MinerName     string        `json:"miner_name"`
	Uptime        string        `json:"uptime"`
	NetworkStatus NetworkStatus `json:"network_status"`
}

// NetworkStatus captures network identifiers.
type NetworkStatus struct {
	MAC      string   `json:"mac"`
	DHCP     bool     `json:"dhcp"`
	IP       string   `json:"ip"`
	Netmask  string   `json:"netmask"`
	Gateway  string   `json:"gateway"`
	DNS      []string `json:"dns"`
	Hostname string   `json:"hostname"`
}

// ModelResponse extracts key model metadata.
type ModelResponse struct {
	FullName string `json:"full_name"`
	Model    string `json:"model"`
}

// APIKey represents an entry returned by /apikeys.
type APIKey struct {
	Key         string `json:"key"`
	Description string `json:"description"`
}

// SummaryResponse mirrors the /summary payload relevant fields.
type SummaryResponse struct {
	Miner SummaryMiner `json:"miner"`
}

// SummaryMiner holds the main summary block.
type SummaryMiner struct {
	MinerStatus      SummaryMinerStatus `json:"miner_status"`
	MinerType        string             `json:"miner_type"`
	InstantHashrate  *float64           `json:"instant_hashrate"`
	AverageHashrate  *float64           `json:"average_hashrate"`
	HashrateRealtime *float64           `json:"hr_realtime"`
	HashrateNominal  *float64           `json:"hr_nominal"`
	HashrateAverage  *float64           `json:"hr_average"`
	HashrateStock    *float64           `json:"hr_stock"`
	PowerConsumption *float64           `json:"power_consumption"`
	PowerUsage       *float64           `json:"power_usage"`
	PowerEfficiency  *float64           `json:"power_efficiency"`
	Cooling          SummaryCooling     `json:"cooling"`
	Chains           []SummaryChain     `json:"chains"`
	Pools            []SummaryPool      `json:"pools"`
}

// SummaryMinerStatus tracks the miner state and duration.
type SummaryMinerStatus struct {
	MinerState     string `json:"miner_state"`
	MinerStateTime *int64 `json:"miner_state_time"`
}

// SummaryCooling captures cooling data.
type SummaryCooling struct {
	FanNum   int          `json:"fan_num"`
	Fans     []SummaryFan `json:"fans"`
	FanDuty  *int         `json:"fan_duty"`
	Settings *CoolingMode `json:"settings"`
}

// CoolingMode exposes the configured cooling profile.
type CoolingMode struct {
	Mode ModeName `json:"mode"`
}

// ModeName is a helper for JSON.
type ModeName struct {
	Name string `json:"name"`
}

// SummaryFan represents an individual fan state.
type SummaryFan struct {
	ID     int    `json:"id"`
	RPM    *int   `json:"rpm"`
	Status string `json:"status"`
	MaxRPM *int   `json:"max_rpm"`
}

// SummaryPool reflects mining pool stats.
type SummaryPool struct {
	ID       int    `json:"id"`
	URL      string `json:"url"`
	PoolType string `json:"pool_type"`
	User     string `json:"user"`
	Status   string `json:"status"`
	Accepted *int   `json:"accepted"`
	Rejected *int   `json:"rejected"`
	Stale    *int   `json:"stale"`
}

// SummaryChain summarises chain-level metrics.
type SummaryChain struct {
	ID                 int               `json:"id"`
	Frequency          *float64          `json:"frequency"`
	Voltage            *float64          `json:"voltage"`
	PowerConsumption   *float64          `json:"power_consumption"`
	HashrateIdeal      *float64          `json:"hashrate_ideal"`
	HashrateRealtime   *float64          `json:"hashrate_rt"`
	HashratePercentage *float64          `json:"hashrate_percentage"`
	HashrateError      *float64          `json:"hr_error"`
	HWErrors           *int              `json:"hw_errors"`
	PCBTemp            TemperatureRange  `json:"pcb_temp"`
	ChipTemp           TemperatureRange  `json:"chip_temp"`
	ChipStatuses       SummaryChipStatus `json:"chip_statuses"`
	Status             ChainStatus       `json:"status"`
}

// TemperatureRange captures min/max temperatures.
type TemperatureRange struct {
	Min *float64 `json:"min"`
	Max *float64 `json:"max"`
}

// SummaryChipStatus groups chip state counts.
type SummaryChipStatus struct {
	Red    *int `json:"red"`
	Orange *int `json:"orange"`
	Grey   *int `json:"grey"`
}

// ChainStatus wraps the `state` string.
type ChainStatus struct {
	State string `json:"state"`
}

// ChainTelemetry details chip-level stats from /chains.
type ChainTelemetry struct {
	ID               int             `json:"id"`
	Status           ChainStatus     `json:"status"`
	HashrateRealtime *float64        `json:"hr_realtime"`
	HashrateNominal  *float64        `json:"hr_nominal"`
	Frequency        *float64        `json:"freq"`
	Chips            []ChipTelemetry `json:"chips"`
}

// ChipTelemetry captures per chip metrics.
type ChipTelemetry struct {
	ID        int      `json:"id"`
	Hashrate  *float64 `json:"hr"`
	Frequency *float64 `json:"freq"`
	Voltage   *float64 `json:"volt"`
	Temp      *float64 `json:"temp"`
	Errors    *int     `json:"errs"`
	Grade     string   `json:"grade"`
	Throttled *bool    `json:"throttled"`
}

// AutotunePreset enumerates firmware presets.
type AutotunePreset struct {
	Name              string                 `json:"name"`
	Pretty            string                 `json:"pretty"`
	Status            string                 `json:"status"`
	ModdedPSURequired bool                   `json:"modded_psu_required"`
	TuneSettings      map[string]interface{} `json:"tune_settings,omitempty"`
}

// PerfSummaryResponse mirrors the /perf-summary payload.
type PerfSummaryResponse struct {
	CurrentPreset json.RawMessage `json:"current_preset"`
}

// SetPresetRequest is the minimal payload structure for POST /settings
// to change only the preset. All fields in the firmware settings API are
// optional, so we only send what we want to change.
type SetPresetRequest struct {
	Miner MinerConfig `json:"miner"`
}

// MinerConfig wraps the overclock settings in the settings payload.
type MinerConfig struct {
	Overclock OverclockSettings `json:"overclock"`
}

// OverclockSettings contains the preset field and other optional overclock settings.
type OverclockSettings struct {
	Preset string `json:"preset"`
}

// SaveConfigResult is returned by POST /settings indicating if restart/reboot is needed.
type SaveConfigResult struct {
	RebootRequired  bool `json:"reboot_required"`
	RestartRequired bool `json:"restart_required"`
}
