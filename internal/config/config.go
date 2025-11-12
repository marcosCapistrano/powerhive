package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type AppConfig struct {
	Database  DatabaseConfig `json:"database"`
	Network   NetworkConfig  `json:"network"`
	Intervals IntervalConfig `json:"intervals"`
	HTTP      HTTPConfig     `json:"http"`
	Plant     PlantConfig    `json:"plant"`
}

type DatabaseConfig struct {
	Path string `json:"path"`
}

type NetworkConfig struct {
	Subnets             []string `json:"subnets"`
	LightScanTimeoutMs  int      `json:"light_scan_timeout_ms"`
	MinerProbeTimeoutMs int      `json:"miner_probe_timeout_ms"`
}

type IntervalConfig struct {
	DiscoverySeconds int `json:"discovery_seconds"`
	StatusSeconds    int `json:"status_seconds"`
	TelemetrySeconds int `json:"telemetry_seconds"`
	PlantSeconds     int `json:"plant_seconds"`
	BalancerSeconds  int `json:"balancer_seconds"`
}

type HTTPConfig struct {
	Addr string `json:"addr"`
}

type PlantConfig struct {
	APIEndpoint   string `json:"api_endpoint"`
	APIKey        string `json:"api_key"`
	PlantID       string `json:"plant_id"`
	TestMode      bool   `json:"test_mode"`
	TestServerURL string `json:"test_server_url"`
}

func Load(path string) (AppConfig, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return AppConfig{}, fmt.Errorf("resolve config path %s: %w", path, err)
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		return AppConfig{}, fmt.Errorf("read config %s: %w", absPath, err)
	}

	var cfg AppConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return AppConfig{}, fmt.Errorf("parse config %s: %w", filepath.Base(absPath), err)
	}

	if err := cfg.validate(filepath.Dir(absPath)); err != nil {
		return AppConfig{}, err
	}

	return cfg, nil
}

func (c *AppConfig) validate(baseDir string) error {
	if c.Database.Path == "" {
		return fmt.Errorf("database path is required")
	}

	if !filepath.IsAbs(c.Database.Path) {
		c.Database.Path = filepath.Clean(filepath.Join(baseDir, c.Database.Path))
	}

	if len(c.Network.Subnets) == 0 {
		return fmt.Errorf("at least one network subnet is required")
	}

	if c.Network.LightScanTimeoutMs <= 0 {
		c.Network.LightScanTimeoutMs = 300
	}

	if c.Network.MinerProbeTimeoutMs <= 0 {
		c.Network.MinerProbeTimeoutMs = 1500
	}

	if c.Intervals.DiscoverySeconds <= 0 {
		c.Intervals.DiscoverySeconds = 30
	}

	if c.Intervals.StatusSeconds <= 0 {
		c.Intervals.StatusSeconds = 15
	}

	if c.Intervals.TelemetrySeconds <= 0 {
		c.Intervals.TelemetrySeconds = 60
	}

	if c.Intervals.PlantSeconds <= 0 {
		c.Intervals.PlantSeconds = 15
	}

	if c.Intervals.BalancerSeconds <= 0 {
		c.Intervals.BalancerSeconds = 15
	}

	if c.HTTP.Addr == "" {
		c.HTTP.Addr = ":8080"
	}

	if c.Plant.APIEndpoint == "" {
		c.Plant.APIEndpoint = "https://energy-aggregator.fly.dev/data/latest"
	}

	if c.Plant.APIKey == "" {
		return fmt.Errorf("plant API key is required")
	}

	if c.Plant.PlantID == "" {
		return fmt.Errorf("plant ID is required")
	}

	return nil
}
