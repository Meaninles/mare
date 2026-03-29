package cd2runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"mam/backend/internal/config"
)

const (
	ModeDisabled = "disabled"
	ModeExternal = "external"
)

var titlePattern = regexp.MustCompile(`(?is)<title>\s*([^<]+?)\s*</title>`)

type Config struct {
	Enabled         bool
	Mode            string
	BaseURL         string
	ExpectedName    string
	ExpectedVersion string
	ProbeTimeout    time.Duration
}

type State struct {
	Enabled                bool       `json:"enabled"`
	Mode                   string     `json:"mode"`
	BaseURL                string     `json:"baseUrl"`
	Reachable              bool       `json:"reachable"`
	Ready                  bool       `json:"ready"`
	ProductName            string     `json:"productName,omitempty"`
	ProductVersion         string     `json:"productVersion,omitempty"`
	ExpectedProductName    string     `json:"expectedProductName,omitempty"`
	ExpectedProductVersion string     `json:"expectedProductVersion,omitempty"`
	NameMatched            bool       `json:"nameMatched"`
	VersionMatched         bool       `json:"versionMatched"`
	VersionCheckStatus     string     `json:"versionCheckStatus"`
	HTTPStatus             int        `json:"httpStatus,omitempty"`
	LastCheckedAt          *time.Time `json:"lastCheckedAt,omitempty"`
	LastError              string     `json:"lastError,omitempty"`
}

type manifest struct {
	Name      string `json:"name"`
	ShortName string `json:"short_name"`
}

type Manager struct {
	config Config
	client *http.Client

	mu    sync.RWMutex
	state State
}

func ConfigFromApp(cfg config.Config) Config {
	mode := strings.ToLower(strings.TrimSpace(cfg.CD2Mode))
	if mode == "" {
		mode = ModeExternal
	}
	if !cfg.CD2Enabled {
		mode = ModeDisabled
	}

	timeout := cfg.CD2ProbeTimeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	return Config{
		Enabled:         cfg.CD2Enabled,
		Mode:            mode,
		BaseURL:         normalizeBaseURL(cfg.CD2BaseURL),
		ExpectedName:    strings.TrimSpace(cfg.CD2ExpectedName),
		ExpectedVersion: strings.TrimSpace(cfg.CD2ExpectedVersion),
		ProbeTimeout:    timeout,
	}
}

func NewManager(cfg Config) *Manager {
	initial := State{
		Enabled:                cfg.Enabled,
		Mode:                   cfg.Mode,
		BaseURL:                cfg.BaseURL,
		ExpectedProductName:    cfg.ExpectedName,
		ExpectedProductVersion: cfg.ExpectedVersion,
		VersionCheckStatus:     "unavailable",
		NameMatched:            strings.TrimSpace(cfg.ExpectedName) == "",
		VersionMatched:         strings.TrimSpace(cfg.ExpectedVersion) == "",
	}
	if !cfg.Enabled || cfg.Mode == ModeDisabled {
		initial.Mode = ModeDisabled
		initial.VersionCheckStatus = "disabled"
		initial.NameMatched = true
		initial.VersionMatched = true
	}

	return &Manager{
		config: cfg,
		client: &http.Client{
			Timeout: cfg.ProbeTimeout,
		},
		state: initial,
	}
}

func (manager *Manager) Snapshot() State {
	manager.mu.RLock()
	defer manager.mu.RUnlock()
	return manager.state
}

func (manager *Manager) Probe(ctx context.Context) State {
	now := time.Now().UTC()
	next := State{
		Enabled:                manager.config.Enabled,
		Mode:                   manager.config.Mode,
		BaseURL:                manager.config.BaseURL,
		ExpectedProductName:    manager.config.ExpectedName,
		ExpectedProductVersion: manager.config.ExpectedVersion,
		VersionCheckStatus:     "unavailable",
		LastCheckedAt:          &now,
		NameMatched:            strings.TrimSpace(manager.config.ExpectedName) == "",
		VersionMatched:         strings.TrimSpace(manager.config.ExpectedVersion) == "",
	}

	if !manager.config.Enabled || manager.config.Mode == ModeDisabled {
		next.Mode = ModeDisabled
		next.NameMatched = true
		next.VersionMatched = true
		next.VersionCheckStatus = "disabled"
		manager.store(next)
		return next
	}

	if manager.config.Mode != ModeExternal {
		next.LastError = fmt.Sprintf("unsupported cd2 runtime mode: %s", manager.config.Mode)
		manager.store(next)
		return next
	}

	probeCtx, cancel := context.WithTimeout(ctx, manager.config.ProbeTimeout)
	defer cancel()

	rootBody, statusCode, err := manager.fetchBody(probeCtx, manager.config.BaseURL)
	if err != nil {
		next.LastError = err.Error()
		manager.store(next)
		return next
	}

	next.Reachable = true
	next.HTTPStatus = statusCode
	if statusCode >= http.StatusBadRequest {
		next.LastError = fmt.Sprintf("cd2 probe returned http %d", statusCode)
		manager.store(next)
		return next
	}

	next.ProductName = detectHTMLTitle(rootBody)
	if manifestName, manifestErr := manager.fetchManifestName(probeCtx); manifestErr == nil && strings.TrimSpace(manifestName) != "" {
		next.ProductName = manifestName
	}

	if expectedName := strings.TrimSpace(manager.config.ExpectedName); expectedName != "" {
		next.NameMatched = strings.EqualFold(strings.TrimSpace(next.ProductName), expectedName)
		if strings.TrimSpace(next.ProductName) == "" {
			next.NameMatched = false
		}
	} else {
		next.NameMatched = true
	}

	switch {
	case strings.TrimSpace(manager.config.ExpectedVersion) == "":
		next.VersionMatched = true
		next.VersionCheckStatus = "skipped"
	case strings.TrimSpace(next.ProductVersion) == "":
		next.VersionMatched = false
		next.VersionCheckStatus = "unavailable"
	default:
		next.VersionMatched = strings.EqualFold(strings.TrimSpace(next.ProductVersion), strings.TrimSpace(manager.config.ExpectedVersion))
		if next.VersionMatched {
			next.VersionCheckStatus = "matched"
		} else {
			next.VersionCheckStatus = "mismatch"
		}
	}

	next.Ready = next.Reachable && next.NameMatched
	if !next.NameMatched && strings.TrimSpace(next.ProductName) != "" {
		next.LastError = fmt.Sprintf("cd2 product name mismatch: expected %q, got %q", manager.config.ExpectedName, next.ProductName)
	}

	manager.store(next)
	return next
}

func (manager *Manager) fetchManifestName(ctx context.Context) (string, error) {
	body, statusCode, err := manager.fetchBody(ctx, manager.config.BaseURL+"/public/manifest.json")
	if err != nil {
		return "", err
	}
	if statusCode >= http.StatusBadRequest {
		return "", fmt.Errorf("manifest probe returned http %d", statusCode)
	}

	var parsed manifest
	if err := json.Unmarshal([]byte(body), &parsed); err != nil {
		return "", err
	}

	if strings.TrimSpace(parsed.Name) != "" {
		return strings.TrimSpace(parsed.Name), nil
	}
	return strings.TrimSpace(parsed.ShortName), nil
}

func (manager *Manager) fetchBody(ctx context.Context, url string) (string, int, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", 0, err
	}

	response, err := manager.client.Do(request)
	if err != nil {
		return "", 0, fmt.Errorf("cd2 runtime request failed: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode == http.StatusNotFound {
		return "", response.StatusCode, nil
	}

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return "", response.StatusCode, err
	}
	return string(body), response.StatusCode, nil
}

func (manager *Manager) store(state State) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	manager.state = state
}

func normalizeBaseURL(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "http://127.0.0.1:29798"
	}
	return strings.TrimRight(trimmed, "/")
}

func detectHTMLTitle(body string) string {
	match := titlePattern.FindStringSubmatch(body)
	if len(match) < 2 {
		return ""
	}
	return strings.TrimSpace(match[1])
}
