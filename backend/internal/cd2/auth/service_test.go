package cd2auth

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	cd2client "mam/backend/internal/cd2/client"
	cd2pb "mam/backend/internal/cd2/pb"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/emptypb"
)

type fakeAuthCloudDriveServer struct {
	cd2pb.UnimplementedCloudDriveFileSrvServer
	token string

	mu          sync.Mutex
	loginCounts map[string]int
}

func (server *fakeAuthCloudDriveServer) GetSystemInfo(context.Context, *emptypb.Empty) (*cd2pb.CloudDriveSystemInfo, error) {
	return &cd2pb.CloudDriveSystemInfo{
		IsLogin:     true,
		UserName:    "tester",
		SystemReady: true,
	}, nil
}

func (server *fakeAuthCloudDriveServer) GetApiTokenInfo(ctx context.Context, request *cd2pb.StringValue) (*cd2pb.TokenInfo, error) {
	values := metadata.ValueFromIncomingContext(ctx, "authorization")
	if len(values) > 0 {
		return nil, status.Error(codes.PermissionDenied, "token management permission required")
	}
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	switch request.GetValue() {
	case "provided-api-token", server.token:
		return &cd2pb.TokenInfo{
			Token:        request.GetValue(),
			RootDir:      "/",
			FriendlyName: "mam-backend",
		}, nil
	default:
		return nil, status.Error(codes.NotFound, "token not found")
	}
}

func (server *fakeAuthCloudDriveServer) Login(_ context.Context, request *cd2pb.UserLoginRequest) (*cd2pb.FileOperationResult, error) {
	if request == nil || request.GetUserName() != "admin" || request.GetPassword() != "secret" {
		return &cd2pb.FileOperationResult{
			Success:      false,
			ErrorMessage: "bad credentials",
		}, nil
	}

	server.mu.Lock()
	defer server.mu.Unlock()
	if server.loginCounts == nil {
		server.loginCounts = map[string]int{}
	}
	server.loginCounts[request.GetUserName()]++
	if server.loginCounts[request.GetUserName()] > 1 {
		return &cd2pb.FileOperationResult{
			Success:      false,
			ErrorMessage: "already login",
		}, nil
	}

	return &cd2pb.FileOperationResult{
		Success: true,
	}, nil
}

func (server *fakeAuthCloudDriveServer) GetToken(_ context.Context, request *cd2pb.GetTokenRequest) (*cd2pb.JWTToken, error) {
	if request == nil || request.GetUserName() != "admin" || request.GetPassword() != "secret" {
		return &cd2pb.JWTToken{
			Success:      false,
			ErrorMessage: "bad credentials",
		}, nil
	}
	server.mu.Lock()
	defer server.mu.Unlock()
	if server.loginCounts == nil || server.loginCounts[request.GetUserName()] == 0 {
		return nil, status.Error(codes.Unauthenticated, "CloudFS not login")
	}
	return &cd2pb.JWTToken{
		Success: true,
		Token:   "bootstrap-jwt",
	}, nil
}

func (server *fakeAuthCloudDriveServer) CreateToken(ctx context.Context, request *cd2pb.CreateTokenRequest) (*cd2pb.TokenInfo, error) {
	values := metadata.ValueFromIncomingContext(ctx, "authorization")
	if len(values) == 0 || values[0] != "Bearer bootstrap-jwt" {
		return nil, status.Error(codes.Unauthenticated, "missing bootstrap bearer")
	}
	return &cd2pb.TokenInfo{
		Token:        server.token,
		RootDir:      request.GetRootDir(),
		FriendlyName: request.GetFriendlyName(),
	}, nil
}

func (server *fakeAuthCloudDriveServer) Register(_ context.Context, request *cd2pb.UserRegisterRequest) (*cd2pb.FileOperationResult, error) {
	if request == nil || request.GetUserName() == "" || request.GetPassword() == "" {
		return &cd2pb.FileOperationResult{
			Success:      false,
			ErrorMessage: "missing credentials",
		}, nil
	}
	if request.GetUserName() == "duplicate" {
		return &cd2pb.FileOperationResult{
			Success:      false,
			ErrorMessage: "duplicate user",
		}, nil
	}
	return &cd2pb.FileOperationResult{
		Success:         true,
		ResultFilePaths: []string{"/accounts/" + request.GetUserName()},
	}, nil
}

func TestConfigurePasswordModePersistsManagedToken(t *testing.T) {
	t.Parallel()

	service, cleanup := newTestAuthService(t)
	defer cleanup()

	status, err := service.Configure(context.Background(), UpdateRequest{
		Mode:                     ModePassword,
		ServerAddress:            "bufnet",
		UserName:                 "admin",
		Password:                 "secret",
		ManagedTokenFriendlyName: "mam-backend",
		ManagedTokenRootDir:      "/",
	})
	if err != nil {
		t.Fatalf("configure password mode: %v", err)
	}
	if !status.Configured || !status.Client.AuthReady || !status.Client.Ready {
		t.Fatalf("expected configured auth-ready status, got %+v", status)
	}
	if status.Profile == nil || status.Profile.Mode != ModePassword {
		t.Fatalf("expected password profile, got %+v", status.Profile)
	}

	stored, err := service.vault.Resolve(service.config.BaseClient.ManagedTokenRef)
	if err != nil {
		t.Fatalf("resolve managed token: %v", err)
	}
	if stored.Secret != "managed-token-456" {
		t.Fatalf("expected managed token to be persisted, got %q", stored.Secret)
	}

	status, err = service.Configure(context.Background(), UpdateRequest{
		Mode:                     ModePassword,
		ServerAddress:            "bufnet",
		UserName:                 "admin",
		Password:                 "secret",
		ManagedTokenFriendlyName: "mam-backend",
		ManagedTokenRootDir:      "/",
	})
	if err != nil {
		t.Fatalf("configure password mode second time: %v", err)
	}
	if !status.Client.AuthReady || !status.Client.Ready {
		t.Fatalf("expected second configure to remain auth-ready, got %+v", status)
	}
}

func TestConfigureAPITokenModePersistsProfile(t *testing.T) {
	t.Parallel()

	service, cleanup := newTestAuthService(t)
	defer cleanup()

	status, err := service.Configure(context.Background(), UpdateRequest{
		Mode:                     ModeAPIToken,
		ServerAddress:            "bufnet",
		APIToken:                 "provided-api-token",
		ManagedTokenFriendlyName: "mam-backend",
		ManagedTokenRootDir:      "/",
	})
	if err != nil {
		t.Fatalf("configure api token mode: %v", err)
	}
	if !status.Client.AuthReady || !status.Client.Ready {
		t.Fatalf("expected auth-ready status, got %+v", status)
	}
	if status.Profile == nil || status.Profile.Mode != ModeAPIToken {
		t.Fatalf("expected api_token profile, got %+v", status.Profile)
	}

	profileStatus, err := service.GetStatus(context.Background(), false)
	if err != nil {
		t.Fatalf("get status: %v", err)
	}
	if !profileStatus.Configured || profileStatus.Profile == nil {
		t.Fatalf("expected persisted profile, got %+v", profileStatus)
	}

	if _, err := os.Stat(filepath.Clean(service.config.ProfilePath)); err != nil {
		t.Fatalf("expected auth profile file to exist: %v", err)
	}
}

func TestClearRemovesProfileAndToken(t *testing.T) {
	t.Parallel()

	service, cleanup := newTestAuthService(t)
	defer cleanup()

	if _, err := service.Configure(context.Background(), UpdateRequest{
		Mode:                     ModeAPIToken,
		ServerAddress:            "bufnet",
		APIToken:                 "provided-api-token",
		ManagedTokenFriendlyName: "mam-backend",
		ManagedTokenRootDir:      "/",
	}); err != nil {
		t.Fatalf("configure api token mode: %v", err)
	}

	status, err := service.Clear(context.Background())
	if err != nil {
		t.Fatalf("clear auth profile: %v", err)
	}
	if status.Configured {
		t.Fatalf("expected cleared status, got %+v", status)
	}
	if _, err := service.vault.Resolve(service.config.BaseClient.ManagedTokenRef); err == nil {
		t.Fatalf("expected managed token to be removed from vault")
	}
}

func TestRegisterUsesPublicEndpoint(t *testing.T) {
	t.Parallel()

	service, cleanup := newTestAuthService(t)
	defer cleanup()

	result, err := service.Register(context.Background(), RegisterRequest{
		ServerAddress: "bufnet",
		UserName:      "new-user",
		Password:      "secret-pass",
	})
	if err != nil {
		t.Fatalf("register account: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected register success, got %+v", result)
	}
	if result.ServerAddress != "bufnet" || result.UserName != "new-user" {
		t.Fatalf("unexpected register result: %+v", result)
	}
}

func newTestAuthService(t *testing.T) (*Service, func()) {
	t.Helper()

	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	cd2pb.RegisterCloudDriveFileSrvServer(server, &fakeAuthCloudDriveServer{token: "managed-token-456"})
	go func() {
		_ = server.Serve(listener)
	}()

	dialer := func(ctx context.Context, _ string) (net.Conn, error) {
		return listener.DialContext(ctx)
	}

	tempDir := t.TempDir()
	baseClient := cd2client.Config{
		Enabled:                  true,
		Target:                   "bufnet",
		DialTimeout:              time.Second,
		RequestTimeout:           time.Second,
		ManagedTokenRef:          "cred_cd2_managed_token",
		ManagedTokenFriendlyName: "mam-backend",
		ManagedTokenRootDir:      "/",
		PersistManagedToken:      true,
		ContextDialer:            dialer,
	}
	clientManager := cd2client.NewManager(baseClient)

	service, err := NewService(Config{
		ProfilePath: filepath.Join(tempDir, "cd2-auth-profile.json"),
		VaultRoot:   filepath.Join(tempDir, "vault"),
		BaseClient:  baseClient,
	}, clientManager)
	if err != nil {
		t.Fatalf("new auth service: %v", err)
	}

	cleanup := func() {
		_ = clientManager.Close()
		server.Stop()
		_ = listener.Close()
	}
	return service, cleanup
}
