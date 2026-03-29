package cd2auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	cd2client "mam/backend/internal/cd2/client"
	cd2pb "mam/backend/internal/cd2/pb"
	"mam/backend/internal/config"
	"mam/backend/internal/credentials"
)

const (
	ModePassword = "password"
	ModeAPIToken = "api_token"
)

type Profile struct {
	Mode                     string     `json:"mode"`
	ServerAddress            string     `json:"serverAddress"`
	UserName                 string     `json:"userName,omitempty"`
	TokenRef                 string     `json:"tokenRef,omitempty"`
	TokenExpiresAt           *time.Time `json:"tokenExpiresAt,omitempty"`
	LastVerifiedAt           *time.Time `json:"lastVerifiedAt,omitempty"`
	ManagedTokenFriendlyName string     `json:"managedTokenFriendlyName,omitempty"`
	ManagedTokenRootDir      string     `json:"managedTokenRootDir,omitempty"`
	UpdatedAt                time.Time  `json:"updatedAt"`
}

type Status struct {
	Configured bool            `json:"configured"`
	Profile    *Profile        `json:"profile,omitempty"`
	Client     cd2client.State `json:"client"`
}

type UpdateRequest struct {
	Mode                     string `json:"mode"`
	ServerAddress            string `json:"serverAddress,omitempty"`
	UserName                 string `json:"userName,omitempty"`
	Password                 string `json:"password,omitempty"`
	APIToken                 string `json:"apiToken,omitempty"`
	ManagedTokenFriendlyName string `json:"managedTokenFriendlyName,omitempty"`
	ManagedTokenRootDir      string `json:"managedTokenRootDir,omitempty"`
}

type RegisterRequest struct {
	ServerAddress string `json:"serverAddress,omitempty"`
	UserName      string `json:"userName,omitempty"`
	Password      string `json:"password,omitempty"`
}

type RegisterResult struct {
	ServerAddress   string   `json:"serverAddress"`
	UserName        string   `json:"userName"`
	Success         bool     `json:"success"`
	ErrorMessage    string   `json:"errorMessage,omitempty"`
	ResultFilePaths []string `json:"resultFilePaths,omitempty"`
}

type Config struct {
	ProfilePath string
	VaultRoot   string
	BaseClient  cd2client.Config
}

type Service struct {
	config     Config
	store      *profileStore
	vault      *credentials.Vault
	client     *cd2client.Manager
	stagingRef string

	mu      sync.RWMutex
	profile *Profile
}

func ConfigFromApp(cfg config.Config) Config {
	return Config{
		ProfilePath: strings.TrimSpace(cfg.CD2AuthProfilePath),
		BaseClient:  cd2client.ConfigFromApp(cfg),
	}
}

func NewService(cfg Config, client *cd2client.Manager) (*Service, error) {
	cfg = normalizeConfig(cfg)

	vault := cfg.BaseClient.Vault
	if vault == nil {
		resolvedVault, err := credentials.NewVault(cfg.VaultRoot)
		if err != nil {
			return nil, err
		}
		vault = resolvedVault
	}
	cfg.BaseClient.Vault = vault

	store, err := newProfileStore(cfg.ProfilePath)
	if err != nil {
		return nil, err
	}

	return &Service{
		config:     cfg,
		store:      store,
		vault:      vault,
		client:     client,
		stagingRef: cfg.BaseClient.ManagedTokenRef + "_staging",
	}, nil
}

func (service *Service) Bootstrap() error {
	profile, err := service.store.Load()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}

	service.setProfile(profile)
	if service.client != nil {
		return service.client.Reconfigure(service.clientConfigFromProfile(profile))
	}
	return nil
}

func (service *Service) GetStatus(ctx context.Context, refresh bool) (Status, error) {
	profile := service.profileSnapshot()
	state := cd2client.State{}
	if service.client != nil {
		if refresh {
			state = service.client.Probe(ctx)
			if profile != nil && state.AuthReady {
				now := time.Now().UTC()
				profile.LastVerifiedAt = &now
				profile.UpdatedAt = now
				if err := service.store.Save(*profile); err == nil {
					service.setProfile(profile)
				}
			}
		} else {
			state = service.client.Snapshot()
		}
	}

	return Status{
		Configured: profile != nil,
		Profile:    profile,
		Client:     state,
	}, nil
}

func (service *Service) Configure(ctx context.Context, request UpdateRequest) (Status, error) {
	mode := normalizeMode(request.Mode)
	if mode == "" {
		return Status{}, errors.New("认证模式无效，必须是 password 或 api_token")
	}

	target := defaultString(strings.TrimSpace(request.ServerAddress), service.config.BaseClient.Target)
	friendlyName := defaultString(strings.TrimSpace(request.ManagedTokenFriendlyName), service.config.BaseClient.ManagedTokenFriendlyName)
	rootDir := defaultString(strings.TrimSpace(request.ManagedTokenRootDir), service.config.BaseClient.ManagedTokenRootDir)
	tokenRef := service.config.BaseClient.ManagedTokenRef
	now := time.Now().UTC()

	var (
		tokenInfo *cd2pb.TokenInfo
		profile   Profile
	)

	switch mode {
	case ModePassword:
		if strings.TrimSpace(request.UserName) == "" || strings.TrimSpace(request.Password) == "" {
			return Status{}, errors.New("密码模式必须提供 userName 和 password")
		}

		_ = service.vault.Delete(service.stagingRef)
		candidate := service.config.BaseClient
		candidate.Target = target
		candidate.Vault = service.vault
		candidate.AuthUserName = strings.TrimSpace(request.UserName)
		candidate.AuthPassword = request.Password
		candidate.AuthAPIToken = ""
		candidate.ManagedTokenRef = service.stagingRef
		candidate.ManagedTokenFriendlyName = friendlyName
		candidate.ManagedTokenRootDir = rootDir
		candidate.PersistManagedToken = true

		tokenInfoCandidate, err := service.validateCandidatePassword(ctx, candidate)
		if err != nil {
			_ = service.vault.Delete(service.stagingRef)
			return Status{}, err
		}
		tokenInfo = tokenInfoCandidate

		stagedToken, err := service.vault.Resolve(service.stagingRef)
		if err != nil {
			return Status{}, err
		}
		if _, err := service.vault.Put("cd2-api-token", tokenRef, stagedToken.Secret, friendlyName); err != nil {
			return Status{}, err
		}
		_ = service.vault.Delete(service.stagingRef)

		profile = Profile{
			Mode:                     ModePassword,
			ServerAddress:            target,
			UserName:                 strings.TrimSpace(request.UserName),
			TokenRef:                 tokenRef,
			TokenExpiresAt:           tokenExpiresAt(tokenInfo, now),
			LastVerifiedAt:           &now,
			ManagedTokenFriendlyName: friendlyName,
			ManagedTokenRootDir:      rootDir,
			UpdatedAt:                now,
		}

	case ModeAPIToken:
		if strings.TrimSpace(request.APIToken) == "" {
			return Status{}, errors.New("API Token 模式必须提供 apiToken")
		}

		tokenInfoCandidate, err := service.validateCandidateToken(ctx, target, request.APIToken)
		if err != nil {
			return Status{}, err
		}
		tokenInfo = tokenInfoCandidate

		if _, err := service.vault.Put("cd2-api-token", tokenRef, strings.TrimSpace(request.APIToken), friendlyName); err != nil {
			return Status{}, err
		}

		profile = Profile{
			Mode:                     ModeAPIToken,
			ServerAddress:            target,
			UserName:                 strings.TrimSpace(request.UserName),
			TokenRef:                 tokenRef,
			TokenExpiresAt:           tokenExpiresAt(tokenInfo, now),
			LastVerifiedAt:           &now,
			ManagedTokenFriendlyName: friendlyName,
			ManagedTokenRootDir:      rootDir,
			UpdatedAt:                now,
		}
	}

	if err := service.store.Save(profile); err != nil {
		return Status{}, err
	}
	service.setProfile(&profile)

	if service.client != nil {
		if err := service.client.Reconfigure(service.clientConfigFromProfile(&profile)); err != nil {
			return Status{}, err
		}
		state := service.client.Probe(ctx)
		if !state.AuthReady {
			return Status{
				Configured: true,
				Profile:    service.profileSnapshot(),
				Client:     state,
			}, errors.New(defaultString(strings.TrimSpace(state.LastError), "CD2 认证验证失败"))
		}
		return Status{
			Configured: true,
			Profile:    service.profileSnapshot(),
			Client:     state,
		}, nil
	}

	return Status{
		Configured: true,
		Profile:    service.profileSnapshot(),
	}, nil
}

func (service *Service) Refresh(ctx context.Context) (Status, error) {
	profile := service.profileSnapshot()
	if profile == nil {
		return service.GetStatus(ctx, false)
	}
	if service.client == nil {
		return Status{
			Configured: true,
			Profile:    profile,
		}, nil
	}

	state := service.client.Probe(ctx)
	if state.AuthReady {
		now := time.Now().UTC()
		profile.LastVerifiedAt = &now
		profile.UpdatedAt = now
		if err := service.store.Save(*profile); err != nil {
			return Status{
				Configured: true,
				Profile:    profile,
				Client:     state,
			}, err
		}
		service.setProfile(profile)
	}

	return Status{
		Configured: true,
		Profile:    service.profileSnapshot(),
		Client:     state,
	}, nil
}

func (service *Service) Clear(ctx context.Context) (Status, error) {
	if err := service.store.Delete(); err != nil {
		return Status{}, err
	}
	_ = service.vault.Delete(service.config.BaseClient.ManagedTokenRef)
	_ = service.vault.Delete(service.stagingRef)
	service.setProfile(nil)

	state := cd2client.State{}
	if service.client != nil {
		if err := service.client.Reconfigure(service.baseClientConfig()); err != nil {
			return Status{}, err
		}
		state = service.client.Probe(ctx)
	}

	return Status{
		Configured: false,
		Client:     state,
	}, nil
}

func (service *Service) Register(ctx context.Context, request RegisterRequest) (RegisterResult, error) {
	target := defaultString(strings.TrimSpace(request.ServerAddress), service.config.BaseClient.Target)
	userName := strings.TrimSpace(request.UserName)
	password := request.Password
	if userName == "" {
		return RegisterResult{}, errors.New("注册 CD2 账号时必须提供 userName")
	}
	if strings.TrimSpace(password) == "" {
		return RegisterResult{}, errors.New("注册 CD2 账号时必须提供 password")
	}

	candidate := service.baseClientConfig()
	candidate.Target = target
	candidate.AuthUserName = ""
	candidate.AuthPassword = ""
	candidate.AuthAPIToken = ""
	candidate.PersistManagedToken = false
	candidate.ManagedTokenRef = ""
	candidate.ManagedTokenFriendlyName = ""
	candidate.ManagedTokenRootDir = ""
	candidate.Vault = nil

	manager := cd2client.NewManager(candidate)
	defer manager.Close()

	state := manager.Probe(ctx)
	if !state.PublicReady {
		return RegisterResult{
			ServerAddress: target,
			UserName:      userName,
			Success:       false,
			ErrorMessage:  defaultString(strings.TrimSpace(state.LastError), "CD2 gRPC 连接失败"),
		}, errors.New(defaultString(strings.TrimSpace(state.LastError), "CD2 gRPC 连接失败"))
	}

	client, err := manager.Client(ctx)
	if err != nil {
		return RegisterResult{
			ServerAddress: target,
			UserName:      userName,
			Success:       false,
			ErrorMessage:  err.Error(),
		}, err
	}

	requestCtx, cancel := manager.WithRequestTimeout(ctx)
	defer cancel()

	response, err := client.Register(requestCtx, &cd2pb.UserRegisterRequest{
		UserName: userName,
		Password: password,
	})
	if err != nil {
		return RegisterResult{
			ServerAddress: target,
			UserName:      userName,
			Success:       false,
			ErrorMessage:  err.Error(),
		}, err
	}
	if response == nil {
		return RegisterResult{
			ServerAddress: target,
			UserName:      userName,
			Success:       false,
			ErrorMessage:  "CD2 未返回注册结果",
		}, errors.New("CD2 未返回注册结果")
	}

	result := RegisterResult{
		ServerAddress:   target,
		UserName:        userName,
		Success:         response.GetSuccess(),
		ErrorMessage:    strings.TrimSpace(response.GetErrorMessage()),
		ResultFilePaths: append([]string(nil), response.GetResultFilePaths()...),
	}
	if !result.Success {
		return result, errors.New(defaultString(result.ErrorMessage, "CD2 注册失败"))
	}
	return result, nil
}

func (service *Service) validateCandidatePassword(ctx context.Context, candidate cd2client.Config) (*cd2pb.TokenInfo, error) {
	manager := cd2client.NewManager(candidate)
	defer manager.Close()

	state := manager.Probe(ctx)
	if !state.PublicReady {
		return nil, errors.New(defaultString(strings.TrimSpace(state.LastError), "CD2 gRPC 连接失败"))
	}
	if !state.AuthReady {
		return nil, errors.New(defaultString(strings.TrimSpace(state.LastError), "CD2 认证失败"))
	}

	stagedToken, err := service.vault.Resolve(candidate.ManagedTokenRef)
	if err != nil {
		return nil, err
	}
	return service.lookupTokenInfo(ctx, manager, stagedToken.Secret)
}

func (service *Service) validateCandidateToken(ctx context.Context, target, apiToken string) (*cd2pb.TokenInfo, error) {
	candidate := service.baseClientConfig()
	candidate.Target = target
	candidate.AuthAPIToken = strings.TrimSpace(apiToken)
	candidate.AuthUserName = ""
	candidate.AuthPassword = ""
	candidate.PersistManagedToken = false
	candidate.ManagedTokenRef = ""
	candidate.Vault = nil

	manager := cd2client.NewManager(candidate)
	defer manager.Close()

	state := manager.Probe(ctx)
	if !state.PublicReady {
		return nil, errors.New(defaultString(strings.TrimSpace(state.LastError), "CD2 gRPC 连接失败"))
	}
	if !state.AuthReady {
		return nil, errors.New(defaultString(strings.TrimSpace(state.LastError), "CD2 API Token 校验失败"))
	}
	return service.lookupTokenInfo(ctx, manager, apiToken)
}

func (service *Service) lookupTokenInfo(ctx context.Context, manager *cd2client.Manager, token string) (*cd2pb.TokenInfo, error) {
	if manager == nil {
		return nil, errors.New("CD2 client manager 未初始化")
	}
	client, err := manager.Client(ctx)
	if err != nil {
		return nil, err
	}
	requestCtx, cancel := manager.WithRequestTimeout(ctx)
	defer cancel()

	info, err := client.GetApiTokenInfo(requestCtx, &cd2pb.StringValue{Value: strings.TrimSpace(token)})
	if err != nil {
		return nil, err
	}
	if info == nil || strings.TrimSpace(info.GetToken()) == "" {
		return nil, errors.New("CD2 未返回有效的 Token 信息")
	}
	return info, nil
}

func (service *Service) clientConfigFromProfile(profile *Profile) cd2client.Config {
	cfg := service.baseClientConfig()
	if profile == nil {
		return cfg
	}

	cfg.Target = defaultString(strings.TrimSpace(profile.ServerAddress), cfg.Target)
	cfg.AuthUserName = ""
	cfg.AuthPassword = ""
	cfg.AuthAPIToken = ""
	cfg.ManagedTokenRef = defaultString(strings.TrimSpace(profile.TokenRef), cfg.ManagedTokenRef)
	cfg.ManagedTokenFriendlyName = defaultString(strings.TrimSpace(profile.ManagedTokenFriendlyName), cfg.ManagedTokenFriendlyName)
	cfg.ManagedTokenRootDir = defaultString(strings.TrimSpace(profile.ManagedTokenRootDir), cfg.ManagedTokenRootDir)
	cfg.PersistManagedToken = true
	cfg.Vault = service.vault
	return cfg
}

func (service *Service) baseClientConfig() cd2client.Config {
	cfg := service.config.BaseClient
	cfg.Vault = service.vault
	return cfg
}

func (service *Service) profileSnapshot() *Profile {
	service.mu.RLock()
	defer service.mu.RUnlock()
	if service.profile == nil {
		return nil
	}

	cloned := *service.profile
	if cloned.TokenExpiresAt != nil {
		expiresAt := cloned.TokenExpiresAt.UTC()
		cloned.TokenExpiresAt = &expiresAt
	}
	if cloned.LastVerifiedAt != nil {
		lastVerifiedAt := cloned.LastVerifiedAt.UTC()
		cloned.LastVerifiedAt = &lastVerifiedAt
	}
	cloned.UpdatedAt = cloned.UpdatedAt.UTC()
	return &cloned
}

func (service *Service) setProfile(profile *Profile) {
	service.mu.Lock()
	defer service.mu.Unlock()
	if profile == nil {
		service.profile = nil
		return
	}
	cloned := *profile
	service.profile = &cloned
}

func normalizeConfig(cfg Config) Config {
	if strings.TrimSpace(cfg.ProfilePath) == "" {
		cfg.ProfilePath = defaultProfilePath()
	}
	cfg.BaseClient = normalizeBaseClientConfig(cfg.BaseClient)
	return cfg
}

func normalizeBaseClientConfig(cfg cd2client.Config) cd2client.Config {
	if strings.TrimSpace(cfg.Target) == "" {
		cfg.Target = "127.0.0.1:29798"
	}
	if strings.TrimSpace(cfg.ManagedTokenRef) == "" {
		cfg.ManagedTokenRef = "cred_cd2_managed_token"
	}
	if strings.TrimSpace(cfg.ManagedTokenFriendlyName) == "" {
		cfg.ManagedTokenFriendlyName = "mam-backend"
	}
	if strings.TrimSpace(cfg.ManagedTokenRootDir) == "" {
		cfg.ManagedTokenRootDir = "/"
	}
	if cfg.DialTimeout <= 0 {
		cfg.DialTimeout = 5 * time.Second
	}
	if cfg.RequestTimeout <= 0 {
		cfg.RequestTimeout = 15 * time.Second
	}
	return cfg
}

func defaultProfilePath() string {
	configRoot, err := os.UserConfigDir()
	if err != nil || strings.TrimSpace(configRoot) == "" {
		return filepath.Join(".", "data", "local-state", "cd2", "auth-profile.json")
	}
	return filepath.Join(configRoot, "mam", "local-state", "cd2", "auth-profile.json")
}

func normalizeMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case ModePassword:
		return ModePassword
	case ModeAPIToken:
		return ModeAPIToken
	default:
		return ""
	}
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return strings.TrimSpace(fallback)
}

func tokenExpiresAt(info *cd2pb.TokenInfo, now time.Time) *time.Time {
	if info == nil || info.ExpiresIn == nil || info.GetExpiresIn() == 0 {
		return nil
	}
	expiresAt := now.UTC().Add(time.Duration(info.GetExpiresIn()) * time.Second)
	return &expiresAt
}

type profileStore struct {
	path string
}

func newProfileStore(path string) (*profileStore, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return nil, errors.New("CD2 auth profile path is required")
	}
	return &profileStore{path: filepath.Clean(trimmed)}, nil
}

func (store *profileStore) Load() (*Profile, error) {
	payload, err := os.ReadFile(store.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, os.ErrNotExist
		}
		return nil, fmt.Errorf("read CD2 auth profile: %w", err)
	}

	var profile Profile
	if err := json.Unmarshal(payload, &profile); err != nil {
		return nil, fmt.Errorf("decode CD2 auth profile: %w", err)
	}
	return &profile, nil
}

func (store *profileStore) Save(profile Profile) error {
	if err := os.MkdirAll(filepath.Dir(store.path), 0o700); err != nil {
		return fmt.Errorf("create CD2 auth profile directory: %w", err)
	}

	payload, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal CD2 auth profile: %w", err)
	}

	if err := os.WriteFile(store.path, payload, 0o600); err != nil {
		return fmt.Errorf("write CD2 auth profile: %w", err)
	}
	return nil
}

func (store *profileStore) Delete() error {
	if err := os.Remove(store.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("delete CD2 auth profile: %w", err)
	}
	return nil
}
