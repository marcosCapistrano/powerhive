package database

import "time"

// Model represents an ASIC miner model and its available presets.
type Model struct {
	ID        int64
	Name      string
	Alias     string
	Presets   []string
	MaxPreset *string
	CreatedAt time.Time
}

// ModelInput collects the data required to create or update a model.
type ModelInput struct {
	Name      string
	Alias     string
	Presets   []string
	MaxPreset *string
}

// Miner models the persisted state for a physical miner.
type Miner struct {
	ID             string
	IP             *string
	APIKey         *string
	Managed        bool
	UnlockPass     string
	Model          *Model
	Settings       *Settings
	LatestStatus   *Status
	LatestStatusID *int64
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// UpsertMinerParams exposes the mutable fields on the miners table.
type UpsertMinerParams struct {
	ID         string
	IP         *string
	APIKey     *string
	Managed    *bool
	UnlockPass *string
	ModelAlias *string
}

// Settings represents the persisted miner configuration payload.
type Settings struct {
	ID        int64
	Cooling   CoolingSettings
	Misc      MiscSettings
	Preset    *string
	Pools     []Pool
	CreatedAt time.Time
}

// CoolingSettings matches the firmware cooling configuration block.
type CoolingSettings struct {
	FanMinCount *int
	FanMinDuty  *int
	FanMaxDuty  *int
	Mode        string
}

// MiscSettings tracks optional settings unrelated to cooling.
type MiscSettings struct {
	IgnoreBrokenSensors  bool
	MinOperationalChains *int
}

// Pool represents a single mining pool entry.
type Pool struct {
	ID         int64
	SettingsID int64
	Position   int
	URL        string
	Username   *string
	Password   *string
}

// SettingsInput collects inputs for creating a new settings snapshot.
type SettingsInput struct {
	Cooling CoolingSettingsInput
	Misc    MiscSettingsInput
	Preset  *string
	Pools   []PoolInput
}

// CoolingSettingsInput mirrors CoolingSettings but allows partial updates.
type CoolingSettingsInput struct {
	FanMinCount *int
	FanMinDuty  *int
	FanMaxDuty  *int
	Mode        string
}

// MiscSettingsInput mirrors MiscSettings but allows partial updates.
type MiscSettingsInput struct {
	IgnoreBrokenSensors  bool
	MinOperationalChains *int
}

// PoolInput is the input representation for a mining pool entry.
type PoolInput struct {
	URL      string
	Username *string
	Password *string
}

// Status captures a persisted miner status snapshot.
type Status struct {
	ID               int64
	MinerID          string
	Uptime           *int64
	State            *string
	Preset           *string
	Hashrate         *float64
	PowerUsage       *float64
	PowerConsumption *float64
	RecordedAt       time.Time
	Fans             []FanStatus
	Chains           []ChainSnapshot
}

// MinerStatusInput is used when recording a fresh status snapshot.
type MinerStatusInput struct {
	Uptime           *int64
	State            *string
	Preset           *string
	Hashrate         *float64
	PowerUsage       *float64
	PowerConsumption *float64
	RecordedAt       time.Time
	Fans             []FanStatusInput
	Chains           []ChainSnapshotInput
}

// FanStatus represents the persisted state of a single fan.
type FanStatus struct {
	ID            int64
	StatusID      int64
	FanIdentifier *string
	RPM           *int
	Status        *string
}

// FanStatusInput is the input representation for a fan entry.
type FanStatusInput struct {
	FanIdentifier *string
	RPM           *int
	Status        *string
}

// ChainSnapshot stores the state of a hashboard at a particular time.
type ChainSnapshot struct {
	ID              int64
	MinerID         string
	StatusID        *int64
	ChainIdentifier *string
	State           *string
	Hashrate        *float64
	PCBTempMin      *float64
	PCBTempMax      *float64
	ChipTempMin     *float64
	ChipTempMax     *float64
	RecordedAt      time.Time
	Chips           []ChipSnapshot
}

// ChainSnapshotInput is used when persisting hashboard state.
type ChainSnapshotInput struct {
	ChainIdentifier *string
	State           *string
	Hashrate        *float64
	PCBTempMin      *float64
	PCBTempMax      *float64
	ChipTempMin     *float64
	ChipTempMax     *float64
	Chips           []ChipSnapshotInput
}

// ChipSnapshot stores chip-level metrics for historical analysis.
type ChipSnapshot struct {
	ID              int64
	ChainSnapshotID int64
	ChipIdentifier  *string
	Hashrate        *float64
	Temperature     *float64
}

// ChipSnapshotInput represents chip-level status inputs.
type ChipSnapshotInput struct {
	ChipIdentifier *string
	Hashrate       *float64
	Temperature    *float64
}

// ModelPreset represents a single preset for a model with its expected power consumption.
type ModelPreset struct {
	ID                 int64
	ModelID            int64
	Value              string
	Position           int
	ExpectedPowerW     *float64
	ExpectedHashrateTH *float64
	CreatedAt          time.Time
}

// PlantReading stores a snapshot of hydro plant generation and consumption.
type PlantReading struct {
	ID                         int64
	PlantID                    string
	TotalGeneration            float64
	TotalContainerConsumption  float64
	AvailablePower             float64
	GenerationSources          map[string]float64 // Individual generator sources (e.g., "generoso", "nogueira") in MW
	ConsumptionSources         map[string]float64 // Individual container sources (e.g., "container_eles", "container_mazp") in MW
	RawData                    *string
	RecordedAt                 time.Time
}

// PlantReadingInput is used when recording plant energy data.
type PlantReadingInput struct {
	PlantID                   string
	TotalGeneration           float64
	TotalContainerConsumption float64
	AvailablePower            float64
	GenerationSources         map[string]float64 // Individual generator sources in MW
	ConsumptionSources        map[string]float64 // Individual container sources in MW
	RawData                   *string
	RecordedAt                time.Time
}

// PowerBalanceEvent logs preset changes made by the power balancing system.
type PowerBalanceEvent struct {
	ID                      int64
	MinerID                 string
	OldPreset               *string
	NewPreset               *string
	OldPower                *float64
	NewPower                *float64
	Reason                  string
	TotalConsumptionBefore  *float64
	TotalConsumptionAfter   *float64
	AvailablePower          *float64
	TargetPower             *float64
	Success                 bool
	ErrorMessage            *string
	RecordedAt              time.Time
}

// PowerBalanceEventInput is used when logging a power balance event.
type PowerBalanceEventInput struct {
	MinerID                string
	OldPreset              *string
	NewPreset              *string
	OldPower               *float64
	NewPower               *float64
	Reason                 string
	TotalConsumptionBefore *float64
	TotalConsumptionAfter  *float64
	AvailablePower         *float64
	TargetPower            *float64
	Success                bool
	ErrorMessage           *string
	RecordedAt             time.Time
}
