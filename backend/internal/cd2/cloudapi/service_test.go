package cloudapi

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	cd2client "mam/backend/internal/cd2/client"
	cd2pb "mam/backend/internal/cd2/pb"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/emptypb"
)

type fakeCloudAPIServiceServer struct {
	cd2pb.UnimplementedCloudDriveFileSrvServer
	accounts []*cd2pb.CloudAPI
	configs  map[string]*cd2pb.CloudAPIConfig
}

func (server *fakeCloudAPIServiceServer) GetSystemInfo(context.Context, *emptypb.Empty) (*cd2pb.CloudDriveSystemInfo, error) {
	return &cd2pb.CloudDriveSystemInfo{SystemReady: true}, nil
}

func (server *fakeCloudAPIServiceServer) GetApiTokenInfo(_ context.Context, request *cd2pb.StringValue) (*cd2pb.TokenInfo, error) {
	if request == nil || strings.TrimSpace(request.GetValue()) != "good-token" {
		return nil, status.Error(codes.PermissionDenied, "bad token")
	}
	return &cd2pb.TokenInfo{Token: "good-token", FriendlyName: "mam-backend"}, nil
}

func (server *fakeCloudAPIServiceServer) GetAllCloudApis(context.Context, *emptypb.Empty) (*cd2pb.CloudAPIList, error) {
	return &cd2pb.CloudAPIList{Apis: server.accounts}, nil
}

func (server *fakeCloudAPIServiceServer) GetCloudAPIConfig(_ context.Context, request *cd2pb.GetCloudAPIConfigRequest) (*cd2pb.CloudAPIConfig, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	value, ok := server.configs[request.GetCloudName()+"|"+request.GetUserName()]
	if !ok {
		return nil, status.Error(codes.NotFound, "config not found")
	}
	return value, nil
}

func (server *fakeCloudAPIServiceServer) SetCloudAPIConfig(_ context.Context, request *cd2pb.SetCloudAPIConfigRequest) (*emptypb.Empty, error) {
	if request == nil || request.GetConfig() == nil {
		return nil, status.Error(codes.InvalidArgument, "config is required")
	}
	server.configs[request.GetCloudName()+"|"+request.GetUserName()] = request.GetConfig()
	return &emptypb.Empty{}, nil
}

func (server *fakeCloudAPIServiceServer) RemoveCloudAPI(_ context.Context, request *cd2pb.RemoveCloudAPIRequest) (*cd2pb.FileOperationResult, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	filtered := make([]*cd2pb.CloudAPI, 0, len(server.accounts))
	for _, item := range server.accounts {
		if item.GetName() == request.GetCloudName() && item.GetUserName() == request.GetUserName() {
			continue
		}
		filtered = append(filtered, item)
	}
	server.accounts = filtered
	delete(server.configs, request.GetCloudName()+"|"+request.GetUserName())
	return &cd2pb.FileOperationResult{Success: true}, nil
}

func (server *fakeCloudAPIServiceServer) APILogin115Editthiscookie(_ context.Context, request *cd2pb.Login115EditthiscookieRequest) (*cd2pb.APILoginResult, error) {
	if request == nil || strings.TrimSpace(request.GetEditThiscookieString()) != "cookie-text" {
		return &cd2pb.APILoginResult{Success: false, ErrorMessage: "bad cookie"}, nil
	}
	server.accounts = append(server.accounts, &cd2pb.CloudAPI{
		Name:     "115",
		UserName: "cookie-user",
		NickName: "Cookie User",
	})
	return &cd2pb.APILoginResult{Success: true}, nil
}

func (server *fakeCloudAPIServiceServer) APILogin115QRCode(_ *cd2pb.Login115QrCodeRequest, stream grpc.ServerStreamingServer[cd2pb.QRCodeScanMessage]) error {
	if err := stream.Send(&cd2pb.QRCodeScanMessage{
		MessageType: cd2pb.QRCodeScanMessageType_SHOW_IMAGE,
		Message:     "data:image/png;base64,abc",
	}); err != nil {
		return err
	}
	if err := stream.Send(&cd2pb.QRCodeScanMessage{
		MessageType: cd2pb.QRCodeScanMessageType_CHANGE_STATUS,
		Message:     "waiting for confirm",
	}); err != nil {
		return err
	}
	server.accounts = append(server.accounts, &cd2pb.CloudAPI{
		Name:     "115",
		UserName: "qr-user",
		NickName: "QR User",
	})
	return nil
}

func (server *fakeCloudAPIServiceServer) APILogin115OpenQRCode(_ *cd2pb.Login115OpenQRCodeRequest, stream grpc.ServerStreamingServer[cd2pb.QRCodeScanMessage]) error {
	if err := stream.Send(&cd2pb.QRCodeScanMessage{
		MessageType: cd2pb.QRCodeScanMessageType_SHOW_IMAGE,
		Message:     "data:image/png;base64,open-abc",
	}); err != nil {
		return err
	}
	if err := stream.Send(&cd2pb.QRCodeScanMessage{
		MessageType: cd2pb.QRCodeScanMessageType_CHANGE_STATUS,
		Message:     "waiting for confirm",
	}); err != nil {
		return err
	}
	server.accounts = append(server.accounts, &cd2pb.CloudAPI{
		Name:     "115open",
		UserName: "open-qr-user",
		NickName: "Open QR User",
	})
	return nil
}

func TestListAccountsAndConfig(t *testing.T) {
	t.Parallel()

	service, cleanup := newCloudAPIService(t)
	defer cleanup()

	accounts, err := service.ListAccounts(context.Background())
	if err != nil {
		t.Fatalf("list accounts: %v", err)
	}
	if len(accounts) != 1 || accounts[0].CloudName != "115" {
		t.Fatalf("unexpected accounts: %+v", accounts)
	}

	config, err := service.GetAccountConfig(context.Background(), "115", "demo")
	if err != nil {
		t.Fatalf("get account config: %v", err)
	}
	if config.MaxDownloadThreads != 4 {
		t.Fatalf("unexpected config: %+v", config)
	}
}

func TestUpdateRemoveAndCookieImport(t *testing.T) {
	t.Parallel()

	service, cleanup := newCloudAPIService(t)
	defer cleanup()

	maxThreads := uint32(8)
	config, err := service.UpdateAccountConfig(context.Background(), UpdateAccountConfigRequest{
		CloudName:          "115",
		UserName:           "demo",
		MaxDownloadThreads: &maxThreads,
	})
	if err != nil {
		t.Fatalf("update account config: %v", err)
	}
	if config.MaxDownloadThreads != 8 {
		t.Fatalf("expected updated max threads, got %+v", config)
	}

	result, err := service.Import115Cookie(context.Background(), Import115CookieRequest{EditThisCookie: "cookie-text"})
	if err != nil {
		t.Fatalf("import 115 cookie: %v", err)
	}
	if !result.Success || len(result.Accounts) != 1 || result.Accounts[0].UserName != "cookie-user" {
		t.Fatalf("unexpected import result: %+v", result)
	}

	if err := service.RemoveAccount(context.Background(), RemoveAccountRequest{
		CloudName: "115",
		UserName:  "demo",
	}); err != nil {
		t.Fatalf("remove account: %v", err)
	}
	accounts, err := service.ListAccounts(context.Background())
	if err != nil {
		t.Fatalf("list accounts after remove: %v", err)
	}
	if len(accounts) != 1 || accounts[0].UserName != "cookie-user" {
		t.Fatalf("unexpected accounts after remove: %+v", accounts)
	}
}

func TestStart115QRCodeSession(t *testing.T) {
	t.Parallel()

	service, cleanup := newCloudAPIService(t)
	defer cleanup()

	session, err := service.Start115QRCode(context.Background(), Start115QRCodeRequest{Platform: "wechatmini"})
	if err != nil {
		t.Fatalf("start 115 qrcode: %v", err)
	}
	if session == nil || session.ID == "" {
		t.Fatalf("expected qrcode session, got %+v", session)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		current, getErr := service.GetQRCodeSession(session.ID)
		if getErr != nil {
			t.Fatalf("get qrcode session: %v", getErr)
		}
		if current.FinishedAt != nil {
			if current.Status != "success" {
				t.Fatalf("expected success status, got %+v", current)
			}
			if len(current.AddedAccounts) != 1 || current.AddedAccounts[0].UserName != "qr-user" {
				t.Fatalf("unexpected added accounts: %+v", current)
			}
			return
		}
		time.Sleep(30 * time.Millisecond)
	}
	t.Fatal("qrcode session did not finish in time")
}

func TestStart115OpenQRCodeSession(t *testing.T) {
	t.Parallel()

	service, cleanup := newCloudAPIService(t)
	defer cleanup()

	session, err := service.Start115OpenQRCode(context.Background(), Start115QRCodeRequest{Platform: "alipaymini"})
	if err != nil {
		t.Fatalf("start 115open qrcode: %v", err)
	}
	if session == nil || session.ID == "" {
		t.Fatalf("expected 115open qrcode session, got %+v", session)
	}
	if session.Provider != Provider115Open {
		t.Fatalf("expected provider %q, got %+v", Provider115Open, session)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		current, getErr := service.GetQRCodeSession(session.ID)
		if getErr != nil {
			t.Fatalf("get 115open qrcode session: %v", getErr)
		}
		if current.FinishedAt != nil {
			if current.Status != "success" {
				t.Fatalf("expected success status, got %+v", current)
			}
			if len(current.AddedAccounts) != 1 || current.AddedAccounts[0].UserName != "open-qr-user" {
				t.Fatalf("unexpected added accounts: %+v", current)
			}
			return
		}
		time.Sleep(30 * time.Millisecond)
	}
	t.Fatal("115open qrcode session did not finish in time")
}

func newCloudAPIService(t *testing.T) (*Service, func()) {
	t.Helper()

	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	fake := &fakeCloudAPIServiceServer{
		accounts: []*cd2pb.CloudAPI{
			{Name: "115", UserName: "demo", NickName: "Demo User"},
		},
		configs: map[string]*cd2pb.CloudAPIConfig{
			"115|demo": {MaxDownloadThreads: 4},
		},
	}
	cd2pb.RegisterCloudDriveFileSrvServer(server, fake)
	go func() {
		_ = server.Serve(listener)
	}()

	manager := cd2client.NewManager(cd2client.Config{
		Enabled:             true,
		Target:              "bufnet",
		AuthAPIToken:        "good-token",
		DialTimeout:         time.Second,
		RequestTimeout:      time.Second,
		PersistManagedToken: false,
		ContextDialer: func(ctx context.Context, _ string) (net.Conn, error) {
			return listener.DialContext(ctx)
		},
	})

	cleanup := func() {
		_ = manager.Close()
		server.Stop()
		_ = listener.Close()
	}
	return NewService(manager), cleanup
}
