package cloudapi

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	cd2client "mam/backend/internal/cd2/client"
	cd2pb "mam/backend/internal/cd2/pb"

	"google.golang.org/protobuf/types/known/emptypb"
)

const (
	Provider115              = "115"
	Provider115Open          = "115open"
	defaultQRCodeSessionTTL  = 10 * time.Minute
	defaultQRCodeStreamLimit = 5 * time.Minute
)

type Account struct {
	CloudName                   string `json:"cloudName"`
	UserName                    string `json:"userName"`
	NickName                    string `json:"nickName,omitempty"`
	DisplayName                 string `json:"displayName"`
	Path                        string `json:"path,omitempty"`
	IsLocked                    bool   `json:"isLocked"`
	SupportMultiThreadUploading bool   `json:"supportMultiThreadUploading"`
	SupportQPSLimit             bool   `json:"supportQpsLimit"`
	IsCloudEventListenerRunning bool   `json:"isCloudEventListenerRunning"`
	HasPromotions               bool   `json:"hasPromotions"`
	PromotionTitle              string `json:"promotionTitle,omitempty"`
	SupportHTTPDownload         bool   `json:"supportHttpDownload"`
}

type Proxy struct {
	ProxyType string `json:"proxyType"`
	Host      string `json:"host,omitempty"`
	Port      uint32 `json:"port,omitempty"`
	UserName  string `json:"userName,omitempty"`
	Password  string `json:"password,omitempty"`
}

type AccountConfig struct {
	CloudName                string  `json:"cloudName"`
	UserName                 string  `json:"userName"`
	MaxDownloadThreads       uint32  `json:"maxDownloadThreads"`
	MinReadLengthKB          uint64  `json:"minReadLengthKB"`
	MaxReadLengthKB          uint64  `json:"maxReadLengthKB"`
	DefaultReadLengthKB      uint64  `json:"defaultReadLengthKB"`
	MaxBufferPoolSizeMB      uint64  `json:"maxBufferPoolSizeMB"`
	MaxQueriesPerSecond      float64 `json:"maxQueriesPerSecond"`
	ForceIPv4                bool    `json:"forceIpv4"`
	APIProxy                 *Proxy  `json:"apiProxy,omitempty"`
	DataProxy                *Proxy  `json:"dataProxy,omitempty"`
	CustomUserAgent          string  `json:"customUserAgent,omitempty"`
	MaxUploadThreads         *uint32 `json:"maxUploadThreads,omitempty"`
	InsecureTLS              *bool   `json:"insecureTls,omitempty"`
	UseHTTPDownload          *bool   `json:"useHttpDownload,omitempty"`
	SupportDirectLink        *bool   `json:"supportDirectLink,omitempty"`
	SupportDirectDownloadURL *bool   `json:"supportDirectDownloadUrl,omitempty"`
}

type UpdateAccountConfigRequest struct {
	CloudName           string   `json:"cloudName"`
	UserName            string   `json:"userName"`
	MaxDownloadThreads  *uint32  `json:"maxDownloadThreads,omitempty"`
	MinReadLengthKB     *uint64  `json:"minReadLengthKB,omitempty"`
	MaxReadLengthKB     *uint64  `json:"maxReadLengthKB,omitempty"`
	DefaultReadLengthKB *uint64  `json:"defaultReadLengthKB,omitempty"`
	MaxBufferPoolSizeMB *uint64  `json:"maxBufferPoolSizeMB,omitempty"`
	MaxQueriesPerSecond *float64 `json:"maxQueriesPerSecond,omitempty"`
	ForceIPv4           *bool    `json:"forceIpv4,omitempty"`

	APIProxy       *Proxy `json:"apiProxy,omitempty"`
	ClearAPIProxy  bool   `json:"clearApiProxy,omitempty"`
	DataProxy      *Proxy `json:"dataProxy,omitempty"`
	ClearDataProxy bool   `json:"clearDataProxy,omitempty"`

	CustomUserAgent        *string `json:"customUserAgent,omitempty"`
	ClearCustomUserAgent   bool    `json:"clearCustomUserAgent,omitempty"`
	MaxUploadThreads       *uint32 `json:"maxUploadThreads,omitempty"`
	ClearMaxUploadThreads  bool    `json:"clearMaxUploadThreads,omitempty"`
	InsecureTLS            *bool   `json:"insecureTls,omitempty"`
	ClearInsecureTLS       bool    `json:"clearInsecureTls,omitempty"`
	UseHTTPDownload        *bool   `json:"useHttpDownload,omitempty"`
	ClearUseHTTPDownload   bool    `json:"clearUseHttpDownload,omitempty"`
	SupportDirectLink      *bool   `json:"supportDirectLink,omitempty"`
	ClearSupportDirectLink bool    `json:"clearSupportDirectLink,omitempty"`
}

type RemoveAccountRequest struct {
	CloudName       string `json:"cloudName"`
	UserName        string `json:"userName"`
	PermanentRemove bool   `json:"permanentRemove"`
}

type Import115CookieRequest struct {
	EditThisCookie string `json:"editThisCookie"`
}

type Import115CookieResult struct {
	Success  bool      `json:"success"`
	Message  string    `json:"message,omitempty"`
	Accounts []Account `json:"accounts,omitempty"`
}

type Start115QRCodeRequest struct {
	Platform string `json:"platform,omitempty"`
}

type QRCodeSession struct {
	ID            string     `json:"id"`
	Provider      string     `json:"provider"`
	Platform      string     `json:"platform,omitempty"`
	Status        string     `json:"status"`
	QRCodeImage   string     `json:"qrCodeImage,omitempty"`
	QRCodeContent string     `json:"qrCodeContent,omitempty"`
	LastMessage   string     `json:"lastMessage,omitempty"`
	Error         string     `json:"error,omitempty"`
	AddedAccounts []Account  `json:"addedAccounts,omitempty"`
	StartedAt     time.Time  `json:"startedAt"`
	UpdatedAt     time.Time  `json:"updatedAt"`
	FinishedAt    *time.Time `json:"finishedAt,omitempty"`
}

type Service struct {
	client *cd2client.Manager

	qrMu       sync.RWMutex
	qrSessions map[string]*QRCodeSession
}

func NewService(client *cd2client.Manager) *Service {
	return &Service{
		client:     client,
		qrSessions: make(map[string]*QRCodeSession),
	}
}

func (service *Service) ListAccounts(ctx context.Context) ([]Account, error) {
	client, _, err := service.authorizedClient(ctx)
	if err != nil {
		return nil, err
	}

	requestCtx, cancel := service.client.WithRequestTimeout(ctx)
	defer cancel()

	result, err := client.GetAllCloudApis(requestCtx, &emptypb.Empty{})
	if err != nil {
		return nil, err
	}

	accounts := make([]Account, 0, len(result.GetApis()))
	for _, item := range result.GetApis() {
		accounts = append(accounts, accountFromProto(item))
	}
	return accounts, nil
}

func (service *Service) GetAccountConfig(ctx context.Context, cloudName, userName string) (AccountConfig, error) {
	client, _, err := service.authorizedClient(ctx)
	if err != nil {
		return AccountConfig{}, err
	}

	requestCtx, cancel := service.client.WithRequestTimeout(ctx)
	defer cancel()

	result, err := client.GetCloudAPIConfig(requestCtx, &cd2pb.GetCloudAPIConfigRequest{
		CloudName: strings.TrimSpace(cloudName),
		UserName:  strings.TrimSpace(userName),
	})
	if err != nil {
		return AccountConfig{}, err
	}
	return accountConfigFromProto(strings.TrimSpace(cloudName), strings.TrimSpace(userName), result), nil
}

func (service *Service) UpdateAccountConfig(ctx context.Context, request UpdateAccountConfigRequest) (AccountConfig, error) {
	cloudName := strings.TrimSpace(request.CloudName)
	userName := strings.TrimSpace(request.UserName)
	if cloudName == "" || userName == "" {
		return AccountConfig{}, errors.New("cloudName 和 userName 不能为空")
	}

	client, _, err := service.authorizedClient(ctx)
	if err != nil {
		return AccountConfig{}, err
	}

	current, err := service.GetAccountConfig(ctx, cloudName, userName)
	if err != nil {
		return AccountConfig{}, err
	}

	next := protoConfigFromAccountConfig(current)
	applyAccountConfigPatch(&next, request)

	requestCtx, cancel := service.client.WithRequestTimeout(ctx)
	defer cancel()

	if _, err := client.SetCloudAPIConfig(requestCtx, &cd2pb.SetCloudAPIConfigRequest{
		CloudName: cloudName,
		UserName:  userName,
		Config:    &next,
	}); err != nil {
		return AccountConfig{}, err
	}

	return service.GetAccountConfig(ctx, cloudName, userName)
}

func (service *Service) RemoveAccount(ctx context.Context, request RemoveAccountRequest) error {
	cloudName := strings.TrimSpace(request.CloudName)
	userName := strings.TrimSpace(request.UserName)
	if cloudName == "" || userName == "" {
		return errors.New("cloudName 和 userName 不能为空")
	}

	client, _, err := service.authorizedClient(ctx)
	if err != nil {
		return err
	}

	requestCtx, cancel := service.client.WithRequestTimeout(ctx)
	defer cancel()

	result, err := client.RemoveCloudAPI(requestCtx, &cd2pb.RemoveCloudAPIRequest{
		CloudName:       cloudName,
		UserName:        userName,
		PermanentRemove: request.PermanentRemove,
	})
	if err != nil {
		return err
	}
	if result != nil && !result.GetSuccess() {
		return errors.New(defaultString(strings.TrimSpace(result.GetErrorMessage()), "CD2 删除云账号失败"))
	}
	return nil
}

func (service *Service) Import115Cookie(ctx context.Context, request Import115CookieRequest) (Import115CookieResult, error) {
	cookie := strings.TrimSpace(request.EditThisCookie)
	if cookie == "" {
		return Import115CookieResult{}, errors.New("editThisCookie 不能为空")
	}

	client, _, err := service.authorizedClient(ctx)
	if err != nil {
		return Import115CookieResult{}, err
	}

	before, _ := service.ListAccounts(ctx)

	requestCtx, cancel := service.client.WithRequestTimeout(ctx)
	defer cancel()

	result, err := client.APILogin115Editthiscookie(requestCtx, &cd2pb.Login115EditthiscookieRequest{
		EditThiscookieString: cookie,
	})
	if err != nil {
		return Import115CookieResult{}, err
	}
	if result != nil && !result.GetSuccess() {
		return Import115CookieResult{}, errors.New(defaultString(strings.TrimSpace(result.GetErrorMessage()), "115 cookie 导入失败"))
	}

	after, _ := service.ListAccounts(ctx)
	added := diffAccounts(before, after, Provider115)
	return Import115CookieResult{
		Success:  true,
		Message:  "115 cookie 已导入到 CD2",
		Accounts: added,
	}, nil
}

func (service *Service) Start115QRCode(ctx context.Context, request Start115QRCodeRequest) (*QRCodeSession, error) {
	client, state, err := service.authorizedClient(ctx)
	if err != nil {
		return nil, err
	}
	if !state.AuthReady {
		return nil, errors.New("CD2 认证未就绪，无法发起 115 二维码登录")
	}

	before, _ := service.ListAccounts(ctx)

	now := time.Now().UTC()
	session := &QRCodeSession{
		ID:        uuid.NewString(),
		Provider:  Provider115,
		Platform:  strings.TrimSpace(request.Platform),
		Status:    "starting",
		StartedAt: now,
		UpdatedAt: now,
	}
	service.storeQRCodeSession(session)

	streamCtx, cancel := context.WithTimeout(context.Background(), defaultQRCodeStreamLimit)
	stream, err := client.APILogin115QRCode(streamCtx, &cd2pb.Login115QrCodeRequest{
		PlatformString: optionalString(strings.TrimSpace(request.Platform)),
	})
	if err != nil {
		cancel()
		service.finishQRCodeSession(session.ID, "error", "", fmt.Errorf("启动 115 二维码登录失败: %w", err), nil)
		return service.GetQRCodeSession(session.ID)
	}

	go service.consume115QRCodeStream(streamCtx, cancel, session.ID, stream, before)
	return service.GetQRCodeSession(session.ID)
}

func (service *Service) Start115OpenQRCode(ctx context.Context, request Start115QRCodeRequest) (*QRCodeSession, error) {
	client, state, err := service.authorizedClient(ctx)
	if err != nil {
		return nil, err
	}
	if !state.AuthReady {
		return nil, errors.New("CD2 认证未就绪，无法发起 115open 二维码登录")
	}

	before, _ := service.ListAccounts(ctx)

	now := time.Now().UTC()
	session := &QRCodeSession{
		ID:        uuid.NewString(),
		Provider:  Provider115Open,
		Platform:  strings.TrimSpace(request.Platform),
		Status:    "starting",
		StartedAt: now,
		UpdatedAt: now,
	}
	service.storeQRCodeSession(session)

	streamCtx, cancel := context.WithTimeout(context.Background(), defaultQRCodeStreamLimit)
	stream, err := client.APILogin115OpenQRCode(streamCtx, &cd2pb.Login115OpenQRCodeRequest{})
	if err != nil {
		cancel()
		service.finishQRCodeSession(session.ID, "error", "", fmt.Errorf("启动 115open 二维码登录失败: %w", err), nil)
		return service.GetQRCodeSession(session.ID)
	}

	go service.consume115OpenQRCodeStream(streamCtx, cancel, session.ID, stream, before)
	return service.GetQRCodeSession(session.ID)
}

func (service *Service) GetQRCodeSession(id string) (*QRCodeSession, error) {
	service.qrMu.RLock()
	defer service.qrMu.RUnlock()

	stored, ok := service.qrSessions[strings.TrimSpace(id)]
	if !ok {
		return nil, errors.New("二维码登录会话不存在")
	}
	cloned := *stored
	if stored.FinishedAt != nil {
		finishedAt := stored.FinishedAt.UTC()
		cloned.FinishedAt = &finishedAt
	}
	if len(stored.AddedAccounts) > 0 {
		cloned.AddedAccounts = append([]Account(nil), stored.AddedAccounts...)
	}
	return &cloned, nil
}

func (service *Service) consume115QRCodeStream(ctx context.Context, cancel context.CancelFunc, sessionID string, stream cd2pb.CloudDriveFileSrv_APILogin115QRCodeClient, before []Account) {
	defer cancel()

	var finalError error
	finalStatus := "closed"

	for {
		message, err := stream.Recv()
		if err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "eof") {
				break
			}
			finalStatus = "error"
			finalError = err
			break
		}
		service.applyQRCodeMessage(sessionID, message)
		switch message.GetMessageType() {
		case cd2pb.QRCodeScanMessageType_ERROR:
			finalStatus = "error"
			finalError = errors.New(defaultString(strings.TrimSpace(message.GetMessage()), "115 二维码登录失败"))
			service.finishQRCodeSession(sessionID, finalStatus, strings.TrimSpace(message.GetMessage()), finalError, nil)
			return
		case cd2pb.QRCodeScanMessageType_CLOSE:
			finalStatus = "closed"
		}
	}

	after, _ := service.ListAccounts(context.Background())
	added := diffAccounts(before, after, Provider115)
	if len(added) > 0 && finalError == nil {
		finalStatus = "success"
	}
	service.finishQRCodeSession(sessionID, finalStatus, "", finalError, added)
}

func (service *Service) consume115OpenQRCodeStream(ctx context.Context, cancel context.CancelFunc, sessionID string, stream cd2pb.CloudDriveFileSrv_APILogin115OpenQRCodeClient, before []Account) {
	defer cancel()

	var finalError error
	finalStatus := "closed"

	for {
		message, err := stream.Recv()
		if err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "eof") {
				break
			}
			finalStatus = "error"
			finalError = err
			break
		}
		service.applyQRCodeMessage(sessionID, message)
		switch message.GetMessageType() {
		case cd2pb.QRCodeScanMessageType_ERROR:
			finalStatus = "error"
			finalError = errors.New(defaultString(strings.TrimSpace(message.GetMessage()), "115open 二维码登录失败"))
			service.finishQRCodeSession(sessionID, finalStatus, strings.TrimSpace(message.GetMessage()), finalError, nil)
			return
		case cd2pb.QRCodeScanMessageType_CLOSE:
			finalStatus = "closed"
		}
	}

	after, _ := service.ListAccounts(context.Background())
	added := diffAccounts(before, after, Provider115Open)
	if len(added) == 0 {
		added = diffAccounts(before, after, "")
	}
	if len(added) > 0 && finalError == nil {
		finalStatus = "success"
	}
	service.finishQRCodeSession(sessionID, finalStatus, "", finalError, added)
}

func (service *Service) applyQRCodeMessage(sessionID string, message *cd2pb.QRCodeScanMessage) {
	service.qrMu.Lock()
	defer service.qrMu.Unlock()

	session, ok := service.qrSessions[sessionID]
	if !ok || message == nil {
		return
	}

	session.UpdatedAt = time.Now().UTC()
	session.LastMessage = strings.TrimSpace(message.GetMessage())

	switch message.GetMessageType() {
	case cd2pb.QRCodeScanMessageType_SHOW_IMAGE:
		session.Status = "show_image"
		session.QRCodeImage = strings.TrimSpace(message.GetMessage())
	case cd2pb.QRCodeScanMessageType_SHOW_IMAGE_CONTENT:
		session.Status = "show_image_content"
		session.QRCodeContent = strings.TrimSpace(message.GetMessage())
	case cd2pb.QRCodeScanMessageType_CHANGE_STATUS:
		session.Status = normalizeQRCodeStatus(message.GetMessage())
	case cd2pb.QRCodeScanMessageType_CLOSE:
		session.Status = "closed"
	case cd2pb.QRCodeScanMessageType_ERROR:
		session.Status = "error"
		session.Error = strings.TrimSpace(message.GetMessage())
	default:
		session.Status = "unknown"
	}
}

func (service *Service) finishQRCodeSession(sessionID string, status string, message string, err error, added []Account) {
	service.qrMu.Lock()
	defer service.qrMu.Unlock()

	session, ok := service.qrSessions[sessionID]
	if !ok {
		return
	}

	now := time.Now().UTC()
	session.Status = defaultString(strings.TrimSpace(status), session.Status)
	if strings.TrimSpace(message) != "" {
		session.LastMessage = strings.TrimSpace(message)
	}
	if err != nil {
		session.Error = err.Error()
	}
	if len(added) > 0 {
		session.AddedAccounts = append([]Account(nil), added...)
	}
	session.UpdatedAt = now
	session.FinishedAt = &now
}

func (service *Service) storeQRCodeSession(session *QRCodeSession) {
	if session == nil {
		return
	}

	service.qrMu.Lock()
	service.qrSessions[session.ID] = session
	for id, existing := range service.qrSessions {
		if existing == nil || existing.FinishedAt == nil {
			continue
		}
		if time.Since(existing.FinishedAt.UTC()) > defaultQRCodeSessionTTL {
			delete(service.qrSessions, id)
		}
	}
	service.qrMu.Unlock()
}

func (service *Service) authorizedClient(ctx context.Context) (cd2pb.CloudDriveFileSrvClient, cd2client.State, error) {
	if service == nil || service.client == nil {
		return nil, cd2client.State{}, errors.New("CD2 Cloud API 服务未初始化")
	}

	state := service.client.Probe(ctx)
	if !state.PublicReady {
		return nil, state, errors.New(defaultString(strings.TrimSpace(state.LastError), "CD2 gRPC 未就绪"))
	}
	if !state.AuthReady {
		return nil, state, errors.New(defaultString(strings.TrimSpace(state.LastError), "CD2 认证未就绪"))
	}

	client, err := service.client.Client(ctx)
	if err != nil {
		return nil, state, err
	}
	return client, state, nil
}

func accountFromProto(value *cd2pb.CloudAPI) Account {
	if value == nil {
		return Account{}
	}
	displayName := strings.TrimSpace(value.GetNickName())
	if displayName == "" {
		displayName = strings.TrimSpace(value.GetUserName())
	}
	if displayName == "" {
		displayName = strings.TrimSpace(value.GetName())
	}

	return Account{
		CloudName:                   strings.TrimSpace(value.GetName()),
		UserName:                    strings.TrimSpace(value.GetUserName()),
		NickName:                    strings.TrimSpace(value.GetNickName()),
		DisplayName:                 displayName,
		Path:                        strings.TrimSpace(value.GetPath()),
		IsLocked:                    value.GetIsLocked(),
		SupportMultiThreadUploading: value.GetSupportMultiThreadUploading(),
		SupportQPSLimit:             value.GetSupportQpsLimit(),
		IsCloudEventListenerRunning: value.GetIsCloudEventListenerRunning(),
		HasPromotions:               value.GetHasPromotions(),
		PromotionTitle:              strings.TrimSpace(value.GetPromotionTitle()),
		SupportHTTPDownload:         value.GetSupportHttpDownload(),
	}
}

func accountConfigFromProto(cloudName string, userName string, value *cd2pb.CloudAPIConfig) AccountConfig {
	if value == nil {
		return AccountConfig{
			CloudName: cloudName,
			UserName:  userName,
		}
	}

	return AccountConfig{
		CloudName:                cloudName,
		UserName:                 userName,
		MaxDownloadThreads:       value.GetMaxDownloadThreads(),
		MinReadLengthKB:          value.GetMinReadLengthKB(),
		MaxReadLengthKB:          value.GetMaxReadLengthKB(),
		DefaultReadLengthKB:      value.GetDefaultReadLengthKB(),
		MaxBufferPoolSizeMB:      value.GetMaxBufferPoolSizeMB(),
		MaxQueriesPerSecond:      value.GetMaxQueriesPerSecond(),
		ForceIPv4:                value.GetForceIpv4(),
		APIProxy:                 proxyFromProto(value.ApiProxy),
		DataProxy:                proxyFromProto(value.DataProxy),
		CustomUserAgent:          strings.TrimSpace(value.GetCustomUserAgent()),
		MaxUploadThreads:         value.MaxUploadThreads,
		InsecureTLS:              value.InsecureTls,
		UseHTTPDownload:          value.UseHttpDownload,
		SupportDirectLink:        value.SupportDirectLink,
		SupportDirectDownloadURL: value.SupportDirectDownloadUrl,
	}
}

func protoConfigFromAccountConfig(value AccountConfig) cd2pb.CloudAPIConfig {
	return cd2pb.CloudAPIConfig{
		MaxDownloadThreads:       value.MaxDownloadThreads,
		MinReadLengthKB:          value.MinReadLengthKB,
		MaxReadLengthKB:          value.MaxReadLengthKB,
		DefaultReadLengthKB:      value.DefaultReadLengthKB,
		MaxBufferPoolSizeMB:      value.MaxBufferPoolSizeMB,
		MaxQueriesPerSecond:      value.MaxQueriesPerSecond,
		ForceIpv4:                value.ForceIPv4,
		ApiProxy:                 proxyToProto(value.APIProxy),
		DataProxy:                proxyToProto(value.DataProxy),
		CustomUserAgent:          optionalString(strings.TrimSpace(value.CustomUserAgent)),
		MaxUploadThreads:         value.MaxUploadThreads,
		InsecureTls:              value.InsecureTLS,
		UseHttpDownload:          value.UseHTTPDownload,
		SupportDirectLink:        value.SupportDirectLink,
		SupportDirectDownloadUrl: value.SupportDirectDownloadURL,
	}
}

func applyAccountConfigPatch(target *cd2pb.CloudAPIConfig, patch UpdateAccountConfigRequest) {
	if target == nil {
		return
	}

	if patch.MaxDownloadThreads != nil {
		target.MaxDownloadThreads = *patch.MaxDownloadThreads
	}
	if patch.MinReadLengthKB != nil {
		target.MinReadLengthKB = *patch.MinReadLengthKB
	}
	if patch.MaxReadLengthKB != nil {
		target.MaxReadLengthKB = *patch.MaxReadLengthKB
	}
	if patch.DefaultReadLengthKB != nil {
		target.DefaultReadLengthKB = *patch.DefaultReadLengthKB
	}
	if patch.MaxBufferPoolSizeMB != nil {
		target.MaxBufferPoolSizeMB = *patch.MaxBufferPoolSizeMB
	}
	if patch.MaxQueriesPerSecond != nil {
		target.MaxQueriesPerSecond = *patch.MaxQueriesPerSecond
	}
	if patch.ForceIPv4 != nil {
		target.ForceIpv4 = *patch.ForceIPv4
	}

	if patch.ClearAPIProxy {
		target.ApiProxy = nil
	} else if patch.APIProxy != nil {
		target.ApiProxy = proxyToProto(patch.APIProxy)
	}
	if patch.ClearDataProxy {
		target.DataProxy = nil
	} else if patch.DataProxy != nil {
		target.DataProxy = proxyToProto(patch.DataProxy)
	}

	if patch.ClearCustomUserAgent {
		target.CustomUserAgent = nil
	} else if patch.CustomUserAgent != nil {
		target.CustomUserAgent = optionalString(strings.TrimSpace(*patch.CustomUserAgent))
	}
	if patch.ClearMaxUploadThreads {
		target.MaxUploadThreads = nil
	} else if patch.MaxUploadThreads != nil {
		target.MaxUploadThreads = patch.MaxUploadThreads
	}
	if patch.ClearInsecureTLS {
		target.InsecureTls = nil
	} else if patch.InsecureTLS != nil {
		target.InsecureTls = patch.InsecureTLS
	}
	if patch.ClearUseHTTPDownload {
		target.UseHttpDownload = nil
	} else if patch.UseHTTPDownload != nil {
		target.UseHttpDownload = patch.UseHTTPDownload
	}
	if patch.ClearSupportDirectLink {
		target.SupportDirectLink = nil
	} else if patch.SupportDirectLink != nil {
		target.SupportDirectLink = patch.SupportDirectLink
	}
}

func proxyFromProto(value *cd2pb.ProxyInfo) *Proxy {
	if value == nil {
		return nil
	}
	return &Proxy{
		ProxyType: value.GetProxyType().String(),
		Host:      strings.TrimSpace(value.GetHost()),
		Port:      value.GetPort(),
		UserName:  strings.TrimSpace(value.GetUsername()),
		Password:  strings.TrimSpace(value.GetPassword()),
	}
}

func proxyToProto(value *Proxy) *cd2pb.ProxyInfo {
	if value == nil {
		return nil
	}
	info := &cd2pb.ProxyInfo{
		ProxyType: parseProxyType(value.ProxyType),
		Host:      strings.TrimSpace(value.Host),
		Port:      value.Port,
		Username:  optionalString(strings.TrimSpace(value.UserName)),
		Password:  optionalString(strings.TrimSpace(value.Password)),
	}
	return info
}

func parseProxyType(value string) cd2pb.ProxyType {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "NOPROXY":
		return cd2pb.ProxyType_NOPROXY
	case "HTTP":
		return cd2pb.ProxyType_HTTP
	case "SOCKS5":
		return cd2pb.ProxyType_SOCKS5
	default:
		return cd2pb.ProxyType_SYSTEM
	}
}

func diffAccounts(before []Account, after []Account, cloudName string) []Account {
	seen := make(map[string]struct{}, len(before))
	for _, item := range before {
		seen[accountKey(item)] = struct{}{}
	}

	added := make([]Account, 0)
	for _, item := range after {
		if cloudName != "" && !strings.EqualFold(strings.TrimSpace(item.CloudName), strings.TrimSpace(cloudName)) {
			continue
		}
		key := accountKey(item)
		if _, ok := seen[key]; ok {
			continue
		}
		added = append(added, item)
	}
	return added
}

func accountKey(value Account) string {
	return strings.ToLower(strings.TrimSpace(value.CloudName)) + "|" + strings.ToLower(strings.TrimSpace(value.UserName))
}

func normalizeQRCodeStatus(message string) string {
	lower := strings.ToLower(strings.TrimSpace(message))
	switch {
	case lower == "":
		return "waiting"
	case strings.Contains(lower, "scan"), strings.Contains(lower, "扫描"):
		return "waiting_scan"
	case strings.Contains(lower, "confirm"), strings.Contains(lower, "确认"):
		return "waiting_confirm"
	case strings.Contains(lower, "success"), strings.Contains(lower, "成功"), strings.Contains(lower, "logged"):
		return "success"
	case strings.Contains(lower, "expired"), strings.Contains(lower, "过期"):
		return "expired"
	default:
		return "status_changed"
	}
}

func optionalString(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	copied := strings.TrimSpace(value)
	return &copied
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return strings.TrimSpace(fallback)
}
