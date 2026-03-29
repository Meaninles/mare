package cd2client

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	cd2pb "mam/backend/internal/cd2/pb"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/emptypb"
)

const testBearerToken = "managed-token-123"

type fakeCloudDriveServer struct {
	cd2pb.UnimplementedCloudDriveFileSrvServer
	token string

	mu          sync.Mutex
	loginCounts map[string]int
}

func (server *fakeCloudDriveServer) GetSystemInfo(context.Context, *emptypb.Empty) (*cd2pb.CloudDriveSystemInfo, error) {
	return &cd2pb.CloudDriveSystemInfo{
		IsLogin:     true,
		UserName:    "tester",
		SystemReady: true,
	}, nil
}

func (server *fakeCloudDriveServer) GetApiTokenInfo(ctx context.Context, request *cd2pb.StringValue) (*cd2pb.TokenInfo, error) {
	values := metadata.ValueFromIncomingContext(ctx, "authorization")
	if len(values) > 0 {
		return nil, status.Error(codes.PermissionDenied, "token management permission required")
	}
	if request == nil || request.GetValue() != server.token {
		return nil, status.Error(codes.NotFound, "token not found")
	}

	return &cd2pb.TokenInfo{
		Token:        request.GetValue(),
		RootDir:      "/",
		FriendlyName: "mam-backend",
	}, nil
}

func (server *fakeCloudDriveServer) Login(_ context.Context, request *cd2pb.UserLoginRequest) (*cd2pb.FileOperationResult, error) {
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

func (server *fakeCloudDriveServer) GetToken(_ context.Context, request *cd2pb.GetTokenRequest) (*cd2pb.JWTToken, error) {
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

func (server *fakeCloudDriveServer) CreateToken(ctx context.Context, request *cd2pb.CreateTokenRequest) (*cd2pb.TokenInfo, error) {
	values := metadata.ValueFromIncomingContext(ctx, "authorization")
	if len(values) == 0 || values[0] != "Bearer bootstrap-jwt" {
		return nil, status.Error(codes.Unauthenticated, "missing bootstrap bearer")
	}
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}

	return &cd2pb.TokenInfo{
		Token:        server.token,
		RootDir:      request.GetRootDir(),
		FriendlyName: request.GetFriendlyName(),
	}, nil
}

func TestProbeWithConfiguredAPIToken(t *testing.T) {
	t.Parallel()

	manager, cleanup := newTestManager(t, Config{
		Enabled:             true,
		Target:              "bufnet",
		AuthAPIToken:        testBearerToken,
		DialTimeout:         time.Second,
		RequestTimeout:      time.Second,
		PersistManagedToken: false,
	})
	defer cleanup()

	state := manager.Probe(context.Background())
	if !state.PublicReady {
		t.Fatalf("expected public gRPC readiness, got %+v", state)
	}
	if !state.AuthReady || !state.Ready {
		t.Fatalf("expected authenticated readiness, got %+v", state)
	}
	if state.TokenSource != "config" {
		t.Fatalf("expected token source config, got %q", state.TokenSource)
	}
	if state.ActiveAuthMode != AuthModeAPIToken {
		t.Fatalf("expected auth mode %q, got %q", AuthModeAPIToken, state.ActiveAuthMode)
	}
}

func TestProbeBootstrapsManagedTokenFromPassword(t *testing.T) {
	t.Parallel()

	manager, cleanup := newTestManager(t, Config{
		Enabled:                  true,
		Target:                   "bufnet",
		AuthUserName:             "admin",
		AuthPassword:             "secret",
		ManagedTokenFriendlyName: "mam-backend",
		ManagedTokenRootDir:      "/",
		DialTimeout:              time.Second,
		RequestTimeout:           time.Second,
		PersistManagedToken:      false,
	})
	defer cleanup()

	state := manager.Probe(context.Background())
	if !state.AuthReady || !state.Ready {
		t.Fatalf("expected managed token bootstrap success, got %+v", state)
	}
	if state.TokenSource != "minted" {
		t.Fatalf("expected minted token source, got %q", state.TokenSource)
	}
	if state.ActiveAuthMode != AuthModePassword {
		t.Fatalf("expected auth mode %q, got %q", AuthModePassword, state.ActiveAuthMode)
	}

	state = manager.Probe(context.Background())
	if !state.AuthReady || !state.Ready {
		t.Fatalf("expected repeated password probe success, got %+v", state)
	}
}

func newTestManager(t *testing.T, cfg Config) (*Manager, func()) {
	t.Helper()

	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	cd2pb.RegisterCloudDriveFileSrvServer(server, &fakeCloudDriveServer{token: testBearerToken})

	go func() {
		_ = server.Serve(listener)
	}()

	cfg.ContextDialer = func(ctx context.Context, _ string) (net.Conn, error) {
		return listener.DialContext(ctx)
	}

	manager := NewManager(cfg)
	cleanup := func() {
		_ = manager.Close()
		server.Stop()
		_ = listener.Close()
	}
	return manager, cleanup
}
