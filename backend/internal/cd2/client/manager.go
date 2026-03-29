package cd2client

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	cd2pb "mam/backend/internal/cd2/pb"
	cd2proto "mam/backend/internal/cd2/proto"
	"mam/backend/internal/config"
	appcredentials "mam/backend/internal/credentials"

	"google.golang.org/grpc"
	grpccredentials "google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/emptypb"
)

const (
	AuthModeNone     = "none"
	AuthModeAPIToken = "api_token"
	AuthModePassword = "password"
)

var publicMethodSet = map[string]struct{}{
	"/clouddrive.CloudDriveFileSrv/GetSystemInfo":              {},
	"/clouddrive.CloudDriveFileSrv/GetToken":                   {},
	"/clouddrive.CloudDriveFileSrv/Login":                      {},
	"/clouddrive.CloudDriveFileSrv/LoginWithThirdPartyAccount": {},
	"/clouddrive.CloudDriveFileSrv/Register":                   {},
	"/clouddrive.CloudDriveFileSrv/SendResetAccountEmail":      {},
	"/clouddrive.CloudDriveFileSrv/ResetAccount":               {},
	"/clouddrive.CloudDriveFileSrv/GetApiTokenInfo":            {},
	"/clouddrive.CloudDriveFileSrv/LoginWith2FA":               {},
}

type Config struct {
	Enabled                  bool
	Target                   string
	UseTLS                   bool
	DialTimeout              time.Duration
	RequestTimeout           time.Duration
	AuthUserName             string
	AuthPassword             string
	AuthAPIToken             string
	ManagedTokenRef          string
	ManagedTokenFriendlyName string
	ManagedTokenRootDir      string
	PersistManagedToken      bool
	Vault                    *appcredentials.Vault
	ContextDialer            func(context.Context, string) (net.Conn, error)
}

type SystemInfo struct {
	IsLogin       bool   `json:"isLogin"`
	UserName      string `json:"userName,omitempty"`
	SystemReady   bool   `json:"systemReady"`
	SystemMessage string `json:"systemMessage,omitempty"`
	HasError      bool   `json:"hasError"`
}

type State struct {
	Enabled                 bool       `json:"enabled"`
	Target                  string     `json:"target"`
	UseTLS                  bool       `json:"useTls"`
	Connected               bool       `json:"connected"`
	PublicReady             bool       `json:"publicReady"`
	AuthReady               bool       `json:"authReady"`
	Ready                   bool       `json:"ready"`
	AuthBootstrapConfigured bool       `json:"authBootstrapConfigured"`
	ActiveAuthMode          string     `json:"activeAuthMode"`
	TokenSource             string     `json:"tokenSource,omitempty"`
	TokenRef                string     `json:"tokenRef,omitempty"`
	TokenFriendlyName       string     `json:"tokenFriendlyName,omitempty"`
	ProtoVersion            string     `json:"protoVersion,omitempty"`
	ProtoDescriptorVersion  string     `json:"protoDescriptorVersion,omitempty"`
	ProtoSourceSHA256       string     `json:"protoSourceSha256,omitempty"`
	ProtoSourceURL          string     `json:"protoSourceUrl,omitempty"`
	ServiceName             string     `json:"serviceName,omitempty"`
	SystemInfo              SystemInfo `json:"systemInfo"`
	LastCheckedAt           *time.Time `json:"lastCheckedAt,omitempty"`
	LastError               string     `json:"lastError,omitempty"`
}

type Manager struct {
	config Config
	vault  *appcredentials.Vault

	mu             sync.RWMutex
	state          State
	conn           *grpc.ClientConn
	client         cd2pb.CloudDriveFileSrvClient
	activeToken    string
	activeAuthMode string
	tokenSource    string
}

func ConfigFromApp(cfg config.Config) Config {
	return Config{
		Enabled:                  cfg.CD2Enabled,
		Target:                   strings.TrimSpace(cfg.CD2GRPCTarget),
		UseTLS:                   cfg.CD2GRPCUseTLS,
		DialTimeout:              cfg.CD2GRPCDialTimeout,
		RequestTimeout:           cfg.CD2GRPCRequestTimeout,
		AuthUserName:             strings.TrimSpace(cfg.CD2AuthUserName),
		AuthPassword:             cfg.CD2AuthPassword,
		AuthAPIToken:             strings.TrimSpace(cfg.CD2AuthAPIToken),
		ManagedTokenRef:          strings.TrimSpace(cfg.CD2ManagedTokenRef),
		ManagedTokenFriendlyName: strings.TrimSpace(cfg.CD2ManagedTokenFriendlyName),
		ManagedTokenRootDir:      strings.TrimSpace(cfg.CD2ManagedTokenRootDir),
		PersistManagedToken:      cfg.CD2PersistManagedToken,
	}
}

func NewManager(cfg Config) *Manager {
	cfg = normalizeConfig(cfg)

	manager := &Manager{
		config: cfg,
		state: State{
			Enabled:                 cfg.Enabled,
			Target:                  cfg.Target,
			UseTLS:                  cfg.UseTLS,
			AuthBootstrapConfigured: hasBootstrapConfig(cfg),
			ActiveAuthMode:          AuthModeNone,
			ProtoVersion:            defaultString(cd2proto.DescriptorVersion(), cd2proto.DeclaredVersion()),
			ProtoDescriptorVersion:  cd2proto.DescriptorVersion(),
			ProtoSourceSHA256:       cd2proto.SourceSHA256(),
			ProtoSourceURL:          cd2proto.SourceURL,
			ServiceName:             cd2proto.ServiceName,
		},
	}

	if cfg.Vault != nil {
		manager.vault = cfg.Vault
	} else if cfg.PersistManagedToken {
		vault, err := appcredentials.NewVault("")
		if err == nil {
			manager.vault = vault
		} else {
			manager.state.LastError = err.Error()
		}
	}

	return manager
}

func (manager *Manager) Snapshot() State {
	manager.mu.RLock()
	defer manager.mu.RUnlock()
	return manager.state
}

func (manager *Manager) Reconfigure(cfg Config) error {
	cfg = normalizeConfig(cfg)

	var nextVault *appcredentials.Vault
	switch {
	case cfg.Vault != nil:
		nextVault = cfg.Vault
	case cfg.PersistManagedToken:
		if manager.vault != nil {
			nextVault = manager.vault
		} else {
			vault, err := appcredentials.NewVault("")
			if err != nil {
				return err
			}
			nextVault = vault
		}
	default:
		nextVault = nil
	}
	cfg.Vault = nextVault

	initialState := State{
		Enabled:                 cfg.Enabled,
		Target:                  cfg.Target,
		UseTLS:                  cfg.UseTLS,
		AuthBootstrapConfigured: hasBootstrapConfig(cfg),
		ActiveAuthMode:          AuthModeNone,
		ProtoVersion:            defaultString(cd2proto.DescriptorVersion(), cd2proto.DeclaredVersion()),
		ProtoDescriptorVersion:  cd2proto.DescriptorVersion(),
		ProtoSourceSHA256:       cd2proto.SourceSHA256(),
		ProtoSourceURL:          cd2proto.SourceURL,
		ServiceName:             cd2proto.ServiceName,
	}

	manager.mu.Lock()
	previousConn := manager.conn
	manager.config = cfg
	manager.vault = nextVault
	manager.state = initialState
	manager.conn = nil
	manager.client = nil
	manager.activeToken = ""
	manager.activeAuthMode = ""
	manager.tokenSource = ""
	manager.mu.Unlock()

	if previousConn != nil {
		return previousConn.Close()
	}
	return nil
}

func (manager *Manager) Probe(ctx context.Context) State {
	now := time.Now().UTC()
	next := State{
		Enabled:                 manager.config.Enabled,
		Target:                  manager.config.Target,
		UseTLS:                  manager.config.UseTLS,
		AuthBootstrapConfigured: hasBootstrapConfig(manager.config),
		ActiveAuthMode:          AuthModeNone,
		ProtoVersion:            defaultString(cd2proto.DescriptorVersion(), cd2proto.DeclaredVersion()),
		ProtoDescriptorVersion:  cd2proto.DescriptorVersion(),
		ProtoSourceSHA256:       cd2proto.SourceSHA256(),
		ProtoSourceURL:          cd2proto.SourceURL,
		ServiceName:             cd2proto.ServiceName,
		LastCheckedAt:           &now,
	}

	if !manager.config.Enabled {
		next.LastError = "cd2 gRPC client is disabled"
		manager.store(next)
		return next
	}
	if strings.TrimSpace(manager.config.Target) == "" {
		next.LastError = "cd2 gRPC target is not configured"
		manager.store(next)
		return next
	}

	client, err := manager.Client(ctx)
	if err != nil {
		next.LastError = err.Error()
		manager.store(next)
		return next
	}

	publicCtx, cancel := manager.requestContext(ctx)
	defer cancel()

	systemInfo, err := client.GetSystemInfo(publicCtx, &emptypb.Empty{})
	if err != nil {
		next.LastError = normalizeRPCError("GetSystemInfo", err).Error()
		manager.store(next)
		return next
	}

	next.Connected = true
	next.PublicReady = true
	next.SystemInfo = normalizeSystemInfo(systemInfo)

	if err := manager.bootstrapAuth(ctx, client, &next); err != nil {
		next.LastError = err.Error()
		manager.store(next)
		return next
	}

	next.Ready = next.PublicReady && next.AuthReady
	manager.store(next)
	return next
}

func (manager *Manager) Client(ctx context.Context) (cd2pb.CloudDriveFileSrvClient, error) {
	manager.mu.RLock()
	if manager.client != nil {
		client := manager.client
		manager.mu.RUnlock()
		return client, nil
	}
	manager.mu.RUnlock()

	manager.mu.Lock()
	defer manager.mu.Unlock()

	if manager.client != nil {
		return manager.client, nil
	}

	dialCtx, cancel := context.WithTimeout(ctx, manager.config.DialTimeout)
	defer cancel()

	conn, err := grpc.DialContext(dialCtx, manager.config.Target, manager.dialOptions()...)
	if err != nil {
		return nil, normalizeRPCError("Dial", err)
	}

	manager.conn = conn
	manager.client = cd2pb.NewCloudDriveFileSrvClient(conn)
	return manager.client, nil
}

func (manager *Manager) Close() error {
	manager.mu.Lock()
	defer manager.mu.Unlock()

	if manager.conn == nil {
		return nil
	}

	err := manager.conn.Close()
	manager.conn = nil
	manager.client = nil
	return err
}

func (manager *Manager) WithRequestTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return manager.requestContext(ctx)
}

func (manager *Manager) requestContext(ctx context.Context) (context.Context, context.CancelFunc) {
	timeout := manager.config.RequestTimeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return context.WithTimeout(ctx, timeout)
}

func (manager *Manager) bootstrapAuth(ctx context.Context, client cd2pb.CloudDriveFileSrvClient, next *State) error {
	if client == nil {
		return resultError("bootstrapAuth", "cd2 gRPC client is not initialized")
	}
	if next == nil {
		return resultError("bootstrapAuth", "state receiver is required")
	}

	token, authMode, tokenSource := manager.currentAuthSnapshot()
	if token != "" {
		info, err := manager.validateAPIToken(ctx, client, token)
		if err == nil {
			manager.applyToken(strings.TrimSpace(info.GetToken()), authMode, tokenSource)
			manager.decorateStateWithToken(next, info, authMode, tokenSource)
			return nil
		}
		manager.clearToken()
	}

	if token := strings.TrimSpace(manager.config.AuthAPIToken); token != "" {
		info, err := manager.validateAPIToken(ctx, client, token)
		if err != nil {
			return err
		}
		manager.applyToken(strings.TrimSpace(info.GetToken()), AuthModeAPIToken, "config")
		manager.decorateStateWithToken(next, info, AuthModeAPIToken, "config")
		return nil
	}

	if manager.config.PersistManagedToken && strings.TrimSpace(manager.config.ManagedTokenRef) != "" && manager.vault != nil {
		stored, err := manager.vault.Resolve(manager.config.ManagedTokenRef)
		if err == nil && strings.TrimSpace(stored.Secret) != "" {
			info, validateErr := manager.validateAPIToken(ctx, client, stored.Secret)
			if validateErr == nil {
				manager.applyToken(strings.TrimSpace(info.GetToken()), AuthModeAPIToken, "vault")
				manager.decorateStateWithToken(next, info, AuthModeAPIToken, "vault")
				return nil
			}
		}
	}

	if strings.TrimSpace(manager.config.AuthUserName) == "" || strings.TrimSpace(manager.config.AuthPassword) == "" {
		next.ActiveAuthMode = AuthModeNone
		return resultError("auth", "CD2 API token or user/password is not configured; only public gRPC methods are available")
	}

	info, err := manager.bootstrapManagedToken(ctx, client)
	if err != nil {
		return err
	}

	manager.applyToken(strings.TrimSpace(info.GetToken()), AuthModePassword, "minted")
	manager.decorateStateWithToken(next, info, AuthModePassword, "minted")
	return nil
}

func (manager *Manager) bootstrapManagedToken(ctx context.Context, client cd2pb.CloudDriveFileSrvClient) (*cd2pb.TokenInfo, error) {
	loginRequestCtx, cancel := manager.requestContext(ctx)
	defer cancel()

	loginResult, err := client.Login(loginRequestCtx, &cd2pb.UserLoginRequest{
		UserName:       manager.config.AuthUserName,
		Password:       manager.config.AuthPassword,
		SynDataToCloud: false,
	})
	if err != nil {
		return nil, normalizeRPCError("Login", err)
	}
	if loginResult == nil {
		return nil, resultError("Login", "cd2 returned an empty login result")
	}
	if !loginResult.GetSuccess() {
		loginMessage := defaultString(strings.TrimSpace(loginResult.GetErrorMessage()), "cd2 login failed")
		if !isAlreadyLoginMessage(loginMessage) {
			return nil, resultError("Login", loginMessage)
		}
	}

	tokenRequestCtx, cancel := manager.requestContext(ctx)
	defer cancel()

	jwtToken, err := client.GetToken(tokenRequestCtx, &cd2pb.GetTokenRequest{
		UserName: manager.config.AuthUserName,
		Password: manager.config.AuthPassword,
	})
	if err != nil {
		return nil, normalizeRPCError("GetToken", err)
	}
	if jwtToken == nil {
		return nil, resultError("GetToken", "cd2 returned an empty login result")
	}
	if !jwtToken.GetSuccess() {
		return nil, resultError("GetToken", defaultString(strings.TrimSpace(jwtToken.GetErrorMessage()), "cd2 login failed"))
	}
	if strings.TrimSpace(jwtToken.GetToken()) == "" {
		return nil, resultError("GetToken", "cd2 did not return a usable JWT token")
	}

	createCtx, cancel := manager.requestContext(ctx)
	defer cancel()
	createCtx = withBearer(createCtx, jwtToken.GetToken())

	tokenInfo, err := client.CreateToken(createCtx, &cd2pb.CreateTokenRequest{
		RootDir:      manager.config.ManagedTokenRootDir,
		Permissions:  leastPrivilegePermissions(),
		FriendlyName: manager.config.ManagedTokenFriendlyName,
	})
	if err != nil {
		return nil, normalizeRPCError("CreateToken", err)
	}
	if tokenInfo == nil || strings.TrimSpace(tokenInfo.GetToken()) == "" {
		return nil, resultError("CreateToken", "cd2 did not return a usable API token")
	}

	if manager.config.PersistManagedToken && manager.vault != nil && strings.TrimSpace(manager.config.ManagedTokenRef) != "" {
		if _, err := manager.vault.Put("cd2-api-token", manager.config.ManagedTokenRef, tokenInfo.GetToken(), manager.config.ManagedTokenFriendlyName); err != nil {
			return nil, err
		}
	}

	return tokenInfo, nil
}

func (manager *Manager) validateAPIToken(ctx context.Context, client cd2pb.CloudDriveFileSrvClient, token string) (*cd2pb.TokenInfo, error) {
	validateCtx, cancel := manager.requestContext(ctx)
	defer cancel()

	info, err := client.GetApiTokenInfo(validateCtx, &cd2pb.StringValue{Value: strings.TrimSpace(token)})
	if err != nil {
		return nil, normalizeRPCError("GetApiTokenInfo", err)
	}
	if info == nil || strings.TrimSpace(info.GetToken()) == "" {
		return nil, resultError("GetApiTokenInfo", "cd2 did not return valid API token info")
	}
	return info, nil
}

func (manager *Manager) dialOptions() []grpc.DialOption {
	transportCredentials := grpccredentials.TransportCredentials(insecure.NewCredentials())
	if manager.config.UseTLS {
		transportCredentials = grpccredentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS12})
	}

	options := []grpc.DialOption{
		grpc.WithBlock(),
		grpc.WithTransportCredentials(transportCredentials),
		grpc.WithChainUnaryInterceptor(manager.unaryAuthInterceptor),
		grpc.WithChainStreamInterceptor(manager.streamAuthInterceptor),
	}
	if manager.config.ContextDialer != nil {
		options = append(options, grpc.WithContextDialer(manager.config.ContextDialer))
	}
	return options
}

func (manager *Manager) unaryAuthInterceptor(ctx context.Context, method string, request any, reply any, connection *grpc.ClientConn, invoker grpc.UnaryInvoker, options ...grpc.CallOption) error {
	if token := manager.currentToken(); token != "" && !isPublicMethod(method) {
		ctx = withBearer(ctx, token)
	}
	return normalizeRPCError(method, invoker(ctx, method, request, reply, connection, options...))
}

func (manager *Manager) streamAuthInterceptor(ctx context.Context, description *grpc.StreamDesc, connection *grpc.ClientConn, method string, streamer grpc.Streamer, options ...grpc.CallOption) (grpc.ClientStream, error) {
	if token := manager.currentToken(); token != "" && !isPublicMethod(method) {
		ctx = withBearer(ctx, token)
	}
	stream, err := streamer(ctx, description, connection, method, options...)
	if err != nil {
		return nil, normalizeRPCError(method, err)
	}
	return stream, nil
}

func (manager *Manager) decorateStateWithToken(state *State, info *cd2pb.TokenInfo, authMode, tokenSource string) {
	if state == nil || info == nil {
		return
	}

	state.AuthReady = true
	state.Ready = state.PublicReady && state.AuthReady
	state.ActiveAuthMode = authMode
	state.TokenSource = tokenSource
	state.TokenFriendlyName = strings.TrimSpace(info.GetFriendlyName())
	if tokenSource == "vault" || tokenSource == "minted" {
		state.TokenRef = strings.TrimSpace(manager.config.ManagedTokenRef)
	}
}

func (manager *Manager) applyToken(token, authMode, tokenSource string) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	manager.activeToken = strings.TrimSpace(token)
	manager.activeAuthMode = strings.TrimSpace(authMode)
	manager.tokenSource = strings.TrimSpace(tokenSource)
}

func (manager *Manager) clearToken() {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	manager.activeToken = ""
	manager.activeAuthMode = ""
	manager.tokenSource = ""
}

func (manager *Manager) currentToken() string {
	manager.mu.RLock()
	defer manager.mu.RUnlock()
	return manager.activeToken
}

func (manager *Manager) currentAuthSnapshot() (token string, authMode string, tokenSource string) {
	manager.mu.RLock()
	defer manager.mu.RUnlock()
	return manager.activeToken, manager.activeAuthMode, manager.tokenSource
}

func (manager *Manager) store(state State) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	manager.state = state
}

func normalizeConfig(cfg Config) Config {
	if strings.TrimSpace(cfg.Target) == "" {
		cfg.Target = "127.0.0.1:29798"
	}
	if cfg.DialTimeout <= 0 {
		cfg.DialTimeout = 5 * time.Second
	}
	if cfg.RequestTimeout <= 0 {
		cfg.RequestTimeout = 15 * time.Second
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
	return cfg
}

func hasBootstrapConfig(cfg Config) bool {
	if strings.TrimSpace(cfg.AuthAPIToken) != "" {
		return true
	}
	if strings.TrimSpace(cfg.AuthUserName) != "" && strings.TrimSpace(cfg.AuthPassword) != "" {
		return true
	}
	return false
}

func normalizeSystemInfo(info *cd2pb.CloudDriveSystemInfo) SystemInfo {
	if info == nil {
		return SystemInfo{}
	}

	systemMessage := ""
	if info.SystemMessage != nil {
		systemMessage = strings.TrimSpace(info.GetSystemMessage())
	}

	hasError := false
	if info.HasError != nil {
		hasError = info.GetHasError()
	}

	return SystemInfo{
		IsLogin:       info.GetIsLogin(),
		UserName:      strings.TrimSpace(info.GetUserName()),
		SystemReady:   info.GetSystemReady(),
		SystemMessage: systemMessage,
		HasError:      hasError,
	}
}

func withBearer(ctx context.Context, token string) context.Context {
	return metadata.AppendToOutgoingContext(ctx, "Authorization", "Bearer "+strings.TrimSpace(token))
}

func leastPrivilegePermissions() *cd2pb.TokenPermissions {
	return &cd2pb.TokenPermissions{
		AllowList:                true,
		AllowSearch:              true,
		AllowCreateFolder:        true,
		AllowCreateFile:          true,
		AllowWrite:               true,
		AllowRead:                true,
		AllowRename:              true,
		AllowMove:                true,
		AllowCopy:                true,
		AllowDelete:              true,
		AllowViewProperties:      true,
		AllowGetTransferTasks:    true,
		AllowModifyTransferTasks: true,
		AllowGetCloudApis:        true,
		AllowModifyCloudApis:     true,
		AllowPushMessage:         true,
	}
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return strings.TrimSpace(fallback)
}

func isPublicMethod(method string) bool {
	_, ok := publicMethodSet[strings.TrimSpace(method)]
	return ok
}

func isAlreadyLoginMessage(message string) bool {
	normalized := strings.ToLower(strings.TrimSpace(message))
	return strings.Contains(normalized, "already login")
}

func (manager *Manager) String() string {
	state := manager.Snapshot()
	return fmt.Sprintf("cd2client(target=%s ready=%t authReady=%t)", state.Target, state.Ready, state.AuthReady)
}
