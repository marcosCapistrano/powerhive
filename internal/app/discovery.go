package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"powerhive/internal/config"
	"powerhive/internal/database"
	"powerhive/internal/firmware"
)

const (
	apiKeyDescription   = "PowerHive"
	defaultHTTPPort     = "80"
	lightScanWorkers    = 32
	probeWorkers        = 8
	apiKeyLengthBytes   = 16
	maxScanResultsQueue = 128
)

// Discoverer performs network discovery to inventory miners.
type Discoverer struct {
	store        *database.Store
	cfg          config.AppConfig
	log          *slog.Logger
	httpClient   *http.Client
	lightTimeout time.Duration
	probeTimeout time.Duration
	interval     time.Duration
}

// NewDiscoverer constructs a discovery service.
func NewDiscoverer(store *database.Store, cfg config.AppConfig, logger *slog.Logger) *Discoverer {
	if logger == nil {
		logger = slog.Default()
	}

	probeTimeout := time.Duration(cfg.Network.MinerProbeTimeoutMs) * time.Millisecond

	return &Discoverer{
		store:        store,
		cfg:          cfg,
		log:          logger.With("component", "discovery"),
		httpClient:   &http.Client{Timeout: probeTimeout},
		lightTimeout: time.Duration(cfg.Network.LightScanTimeoutMs) * time.Millisecond,
		probeTimeout: probeTimeout,
		interval:     time.Duration(cfg.Intervals.DiscoverySeconds) * time.Second,
	}
}

// Run executes the discovery loop until the context is cancelled.
func (d *Discoverer) Run(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}

	d.log.Info("starting discovery loop", "interval", d.interval)

	if err := d.scan(ctx); err != nil {
		d.log.Error("initial discovery failed", "err", err)
	}

	ticker := time.NewTicker(d.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			d.log.Info("stopping discovery loop", "reason", ctx.Err())
			return
		case <-ticker.C:
			if err := d.scan(ctx); err != nil {
				d.log.Error("discovery run failed", "err", err)
			}
		}
	}
}

func (d *Discoverer) scan(ctx context.Context) error {
	hosts, err := d.enumerateHosts()
	if err != nil {
		return fmt.Errorf("enumerate hosts: %w", err)
	}
	if len(hosts) == 0 {
		return nil
	}

	candidates := d.lightScan(ctx, hosts)
	if len(candidates) == 0 {
		return d.markOffline(ctx, map[string]struct{}{})
	}

	type discoveryResult struct {
		IP     string
		Client *firmware.Client
		Info   firmware.InfoResponse
		Model  firmware.ModelResponse
	}

	resultCh := make(chan discoveryResult, maxScanResultsQueue)
	ipCh := make(chan string)

	var wg sync.WaitGroup
	workerCount := probeWorkers
	if len(candidates) < workerCount {
		workerCount = len(candidates)
	}
	if workerCount < 1 {
		workerCount = 1
	}

	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ip := range ipCh {
				select {
				case <-ctx.Done():
					return
				default:
				}

				client, err := firmware.NewClient(ip, firmware.WithHTTPClient(d.httpClient))
				if err != nil {
					d.log.Warn("create firmware client", "ip", ip, "err", err)
					continue
				}

				infoCtx, cancelInfo := context.WithTimeout(ctx, d.probeTimeout)
				info, err := client.Info(infoCtx)
				cancelInfo()
				if err != nil {
					d.log.Debug("probe host skipped", "ip", ip, "err", err)
					continue
				}

				modelCtx, cancelModel := context.WithTimeout(ctx, d.probeTimeout)
				model, err := client.Model(modelCtx)
				cancelModel()
				if err != nil {
					d.log.Warn("fetch model data", "ip", ip, "err", err)
					continue
				}

				select {
				case <-ctx.Done():
					return
				case resultCh <- discoveryResult{
					IP:     ip,
					Client: client,
					Info:   info,
					Model:  model,
				}:
				}
			}
		}()
	}

	go func() {
		for _, ip := range candidates {
			select {
			case <-ctx.Done():
				close(ipCh)
				return
			case ipCh <- ip:
			}
		}
		close(ipCh)
	}()

	go func() {
		wg.Wait()
		close(resultCh)
	}()

	discovered := make(map[string]struct{})
	for res := range resultCh {
		if err := d.applyDiscovery(ctx, res, discovered); err != nil {
			d.log.Error("apply discovery", "ip", res.IP, "err", err)
		}
	}

	return d.markOffline(ctx, discovered)
}

func (d *Discoverer) enumerateHosts() ([]string, error) {
	seen := make(map[string]struct{})
	for _, subnet := range d.cfg.Network.Subnets {
		subnet = strings.TrimSpace(subnet)
		if subnet == "" {
			continue
		}
		ipNet, err := parseCIDR(subnet)
		if err != nil {
			d.log.Warn("parse subnet failed", "subnet", subnet, "err", err)
			continue
		}
		for _, ip := range expandCIDR(ipNet) {
			seen[ip] = struct{}{}
		}
	}

	hosts := make([]string, 0, len(seen))
	for ip := range seen {
		hosts = append(hosts, ip)
	}
	return hosts, nil
}

func (d *Discoverer) lightScan(ctx context.Context, hosts []string) []string {
	var (
		results []string
		wg      sync.WaitGroup
		inCh    = make(chan string)
		outCh   = make(chan string, len(hosts))
	)

	workers := lightScanWorkers
	if len(hosts) < workers {
		workers = len(hosts)
	}
	if workers < 1 {
		workers = 1
	}

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ip := range inCh {
				if ctx.Err() != nil {
					return
				}
				if d.pingHost(ctx, ip) {
					select {
					case outCh <- ip:
					case <-ctx.Done():
						return
					}
				}
			}
		}()
	}

	go func() {
		for _, ip := range hosts {
			select {
			case <-ctx.Done():
				close(inCh)
				return
			case inCh <- ip:
			}
		}
		close(inCh)
	}()

	go func() {
		wg.Wait()
		close(outCh)
	}()

	for ip := range outCh {
		results = append(results, ip)
	}

	return results
}

func (d *Discoverer) pingHost(ctx context.Context, ip string) bool {
	dialer := &net.Dialer{
		Timeout: d.lightTimeout,
	}

	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(ip, defaultHTTPPort))
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func (d *Discoverer) applyDiscovery(ctx context.Context, res struct {
	IP     string
	Client *firmware.Client
	Info   firmware.InfoResponse
	Model  firmware.ModelResponse
}, discovered map[string]struct{}) error {
	mac := strings.TrimSpace(strings.ToLower(res.Info.System.NetworkStatus.MAC))
	if mac == "" {
		return fmt.Errorf("missing mac address for ip %s", res.IP)
	}

	modelName := strings.TrimSpace(res.Model.FullName)
	if modelName == "" {
		modelName = strings.TrimSpace(res.Info.Miner)
	}
	modelAlias := strings.TrimSpace(res.Model.Model)
	if modelAlias == "" {
		modelAlias = strings.TrimSpace(res.Info.Model)
	}
	if modelAlias == "" {
		return fmt.Errorf("model alias unavailable for mac %s", mac)
	}

	var currentMaxPreset *string
	if existing, err := d.store.GetModelByAlias(ctx, modelAlias); err == nil {
		currentMaxPreset = existing.MaxPreset
	}

	if _, err := d.store.UpsertModel(ctx, database.ModelInput{
		Name:      modelName,
		Alias:     modelAlias,
		MaxPreset: currentMaxPreset,
	}); err != nil {
		return fmt.Errorf("upsert model %s: %w", modelAlias, err)
	}

	ipCopy := res.IP
	miner, err := d.store.UpsertMiner(ctx, database.UpsertMinerParams{
		ID:         strings.ToLower(mac),
		IP:         &ipCopy,
		ModelAlias: &modelAlias,
	})
	if err != nil {
		return fmt.Errorf("upsert miner %s: %w", mac, err)
	}

	discovered[strings.ToLower(miner.ID)] = struct{}{}

	apiKey, err := d.ensureAPIKey(ctx, miner, res.Client)
	if err != nil {
		d.log.Warn("ensure api key", "miner", miner.ID, "ip", res.IP, "err", err)
	} else if apiKey != "" {
		miner.APIKey = &apiKey
	}

	if miner.Model != nil && len(miner.Model.Presets) == 0 && miner.APIKey != nil && strings.TrimSpace(*miner.APIKey) != "" {
		presets, err := d.fetchPresets(ctx, res.Client, modelName, modelAlias, miner.Model.MaxPreset)
		if err != nil {
			d.log.Warn("fetch presets", "miner", miner.ID, "ip", res.IP, "err", err)
		}
		if len(presets) > 0 {
			d.log.Info("model presets captured", "model", modelAlias, "count", len(presets))
		}
	}

	return nil
}

func (d *Discoverer) ensureAPIKey(ctx context.Context, miner database.Miner, client *firmware.Client) (string, error) {
	if client == nil {
		return "", fmt.Errorf("firmware client is nil")
	}

	if miner.APIKey != nil && strings.TrimSpace(*miner.APIKey) != "" {
		client.SetAPIKey(strings.TrimSpace(*miner.APIKey))
		return strings.TrimSpace(*miner.APIKey), nil
	}

	ctxUnlock, cancelUnlock := context.WithTimeout(ctx, d.probeTimeout)
	defer cancelUnlock()

	token, err := client.Unlock(ctxUnlock, miner.UnlockPass)
	if err != nil {
		return "", fmt.Errorf("unlock miner: %w", err)
	}

	ctxKeys, cancelKeys := context.WithTimeout(ctx, d.probeTimeout)
	defer cancelKeys()

	keys, err := client.ListAPIKeys(ctxKeys, token)
	if err != nil {
		return "", fmt.Errorf("list api keys: %w", err)
	}

	for _, k := range keys {
		if strings.EqualFold(k.Description, apiKeyDescription) && strings.TrimSpace(k.Key) != "" {
			value := strings.TrimSpace(k.Key)
			if err := d.storeAPIKey(ctx, miner.ID, value); err != nil {
				return "", err
			}
			client.SetAPIKey(value)
			return value, nil
		}
	}

	apiKey, err := generateAPIKey()
	if err != nil {
		return "", fmt.Errorf("generate api key: %w", err)
	}

	ctxCreate, cancelCreate := context.WithTimeout(ctx, d.probeTimeout)
	defer cancelCreate()

	if err := client.CreateAPIKey(ctxCreate, token, apiKey, apiKeyDescription); err != nil {
		return "", fmt.Errorf("create api key: %w", err)
	}

	if err := d.storeAPIKey(ctx, miner.ID, apiKey); err != nil {
		return "", err
	}

	client.SetAPIKey(apiKey)
	d.log.Info("api key provisioned", "miner", miner.ID)
	return apiKey, nil
}

func (d *Discoverer) storeAPIKey(ctx context.Context, minerID, key string) error {
	keyCopy := key
	_, err := d.store.UpsertMiner(ctx, database.UpsertMinerParams{
		ID:     strings.ToLower(minerID),
		APIKey: &keyCopy,
	})
	if err != nil {
		return fmt.Errorf("store api key for miner %s: %w", minerID, err)
	}
	return nil
}

func (d *Discoverer) fetchPresets(ctx context.Context, client *firmware.Client, modelName, modelAlias string, maxPreset *string) ([]string, error) {
	if client == nil {
		return nil, fmt.Errorf("firmware client is nil")
	}

	ctxPreset, cancelPreset := context.WithTimeout(ctx, d.probeTimeout)
	defer cancelPreset()

	presets, err := client.AutotunePresets(ctxPreset, "")
	if err != nil {
		return nil, fmt.Errorf("autotune presets: %w", err)
	}

	if len(presets) == 0 {
		return nil, nil
	}

	values := make([]string, 0, len(presets))
	for _, preset := range presets {
		name := strings.TrimSpace(preset.Name)
		if name != "" {
			values = append(values, name)
		}
	}
	if len(values) == 0 {
		return nil, nil
	}

	if _, err := d.store.UpsertModel(ctx, database.ModelInput{
		Name:      modelName,
		Alias:     modelAlias,
		Presets:   values,
		MaxPreset: maxPreset,
	}); err != nil {
		return nil, fmt.Errorf("update model presets %s: %w", modelAlias, err)
	}

	// Try to extract and store power consumption if available in tune_settings
	for _, preset := range presets {
		if preset.TuneSettings != nil {
			// Try to extract power from tune_settings
			// Common fields might be "power", "target_power", "power_limit", etc.
			var power float64
			found := false

			if powerVal, ok := preset.TuneSettings["power"]; ok {
				if p, ok := powerVal.(float64); ok {
					power = p
					found = true
				}
			}
			if !found {
				if powerVal, ok := preset.TuneSettings["target_power"]; ok {
					if p, ok := powerVal.(float64); ok {
						power = p
						found = true
					}
				}
			}

			if found && power > 0 {
				if err := d.store.UpdatePresetPower(ctx, modelAlias, preset.Name, power); err != nil {
					d.log.Warn("failed to update preset power", "model", modelAlias, "preset", preset.Name, "err", err)
				} else {
					d.log.Debug("stored preset power", "model", modelAlias, "preset", preset.Name, "power_w", power)
				}
			}
		}
	}

	return values, nil
}

func (d *Discoverer) markOffline(ctx context.Context, discovered map[string]struct{}) error {
	miners, err := d.store.ListMiners(ctx)
	if err != nil {
		return fmt.Errorf("list miners: %w", err)
	}

	for _, miner := range miners {
		if _, ok := discovered[strings.ToLower(miner.ID)]; ok {
			continue
		}
		if miner.IP == nil || strings.TrimSpace(*miner.IP) == "" {
			continue
		}
		empty := ""
		if _, err := d.store.UpsertMiner(ctx, database.UpsertMinerParams{
			ID: strings.ToLower(miner.ID),
			IP: &empty,
		}); err != nil {
			if errors.Is(err, context.Canceled) {
				return err
			}
			d.log.Warn("mark miner offline failed", "miner", miner.ID, "err", err)
			continue
		}
		d.log.Info("miner offline", "miner", miner.ID)
	}
	return nil
}

func parseCIDR(cidr string) (*net.IPNet, error) {
	_, network, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, err
	}
	return network, nil
}

func expandCIDR(network *net.IPNet) []string {
	var (
		ips []string
		ip  = network.IP.Mask(network.Mask)
	)

	for current := ip; network.Contains(current); current = incIP(append([]byte{}, current...)) {
		ips = append(ips, current.String())
	}

	// Trim network and broadcast addresses.
	if len(ips) <= 2 {
		return []string{}
	}
	return ips[1 : len(ips)-1]
}

func incIP(ip net.IP) net.IP {
	res := append(net.IP{}, ip...)
	for j := len(res) - 1; j >= 0; j-- {
		res[j]++
		if res[j] != 0 {
			break
		}
	}
	return res
}

func generateAPIKey() (string, error) {
	buf := make([]byte, apiKeyLengthBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
