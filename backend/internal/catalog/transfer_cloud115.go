package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"mam/backend/internal/connectors"
	"mam/backend/internal/store"
)

const (
	transferEngineKindCloud115      = "cloud115"
	cloud115TransferEngineLabel     = "115 缃戠洏涓婁紶"
	cloud115TransferSessionFileName = "cloud115-upload-session.json"
	cloud115DefaultUploadPartSize   = 10 * 1024 * 1024
	cloud115UploadMaxPartsPerCall   = 1
)

type transferItemCloud115Metadata struct {
	UploadID        string          `json:"uploadId,omitempty"`
	ResumeStatePath string          `json:"resumeStatePath,omitempty"`
	RootID          string          `json:"rootId,omitempty"`
	AppType         string          `json:"appType,omitempty"`
	ParentID        int             `json:"parentId,omitempty"`
	FileName        string          `json:"fileName,omitempty"`
	PartSize        int64           `json:"partSize,omitempty"`
	SessionURL      string          `json:"sessionUrl,omitempty"`
	SessionCallback json.RawMessage `json:"sessionCallback,omitempty"`
	Status          string          `json:"status,omitempty"`
	UploadedBytes   int64           `json:"uploadedBytes,omitempty"`
	UploadedParts   int64           `json:"uploadedParts,omitempty"`
	TotalParts      int64           `json:"totalParts,omitempty"`
}

type cloud115UploadTarget struct {
	RootID  string
	AppType string
}

type cloud115UploadClient interface {
	OpenUploadSession(context.Context, connectors.Cloud115UploadSessionRequest) (*connectors.Cloud115UploadSession, error)
	ListUploadSessionParts(context.Context, connectors.Cloud115UploadSessionRequest) (*connectors.Cloud115UploadSession, error)
	UploadSessionParts(context.Context, connectors.Cloud115UploadSessionRequest) (*connectors.Cloud115UploadSession, error)
	CompleteUploadSession(context.Context, connectors.Cloud115UploadSessionRequest) (*connectors.Cloud115UploadSession, error)
	AbortUploadSession(context.Context, connectors.Cloud115UploadSessionRequest) (*connectors.Cloud115UploadSession, error)
	StatEntry(context.Context, string, string) (connectors.FileEntry, error)
}

type cloud115UploadClientFactory func(context.Context, store.StorageEndpoint) (cloud115UploadClient, cloud115UploadTarget, error)

func shouldUseCloud115UploadSession(endpoint store.StorageEndpoint) bool {
	if normalizeEndpointType(endpoint.EndpointType) != string(connectors.EndpointTypeNetwork) {
		return false
	}

	config, err := parseNetworkStorageEndpointConfig(json.RawMessage(endpoint.ConnectionConfig))
	if err != nil {
		return false
	}
	return normalizeNetworkStorageProvider(config.Provider) == networkStorageProvider115
}

func hasCloud115TransferMetadata(metadata transferItemMetadata) bool {
	if metadata.Cloud115 == nil {
		return false
	}
	return strings.TrimSpace(metadata.Cloud115.ResumeStatePath) != "" ||
		strings.TrimSpace(metadata.Cloud115.UploadID) != "" ||
		strings.TrimSpace(metadata.Cloud115.RootID) != "" ||
		strings.TrimSpace(metadata.Cloud115.Status) != ""
}

func isCloud115ResumableTransfer(item store.TransferTaskItem) bool {
	metadata := readTransferItemMetadata(item)
	return strings.TrimSpace(metadata.EngineKind) == transferEngineKindCloud115 || hasCloud115TransferMetadata(metadata)
}

func (service *Service) buildCloud115UploadClient(
	ctx context.Context,
	endpoint store.StorageEndpoint,
) (cloud115UploadClient, cloud115UploadTarget, error) {
	if service.cloud115UploadFactory != nil {
		return service.cloud115UploadFactory(ctx, endpoint)
	}

	hydrated, err := service.hydrateEndpointForConnector(endpoint)
	if err != nil {
		return nil, cloud115UploadTarget{}, err
	}

	config, err := parseNetworkStorageEndpointConfig(json.RawMessage(hydrated.ConnectionConfig))
	if err != nil {
		return nil, cloud115UploadTarget{}, err
	}
	if normalizeNetworkStorageProvider(config.Provider) != networkStorageProvider115 {
		return nil, cloud115UploadTarget{}, fmt.Errorf("endpoint %q is not a 115 network storage target", endpoint.Name)
	}
	if strings.TrimSpace(config.Credential) == "" {
		return nil, cloud115UploadTarget{}, fmt.Errorf("115 缃戠洏鍑瘉涓虹┖锛屾棤娉曞垱寤轰笂浼犱細璇?")
	}

	return connectors.NewCloud115PythonClient(config.Credential, config.AppType), cloud115UploadTarget{
		RootID:  defaultString(strings.TrimSpace(config.RootFolderID), "0"),
		AppType: normalizeNetworkStorageAppType(config.AppType),
	}, nil
}

func (service *Service) commitTransferItemToCloud115(
	ctx context.Context,
	taskID string,
	item *store.TransferTaskItem,
	endpoint store.StorageEndpoint,
) (connectors.FileEntry, error) {
	client, target, err := service.buildCloud115UploadClient(ctx, endpoint)
	if err != nil {
		return connectors.FileEntry{}, err
	}

	request, err := service.buildCloud115UploadSessionRequest(*item, target, true)
	if err != nil {
		return connectors.FileEntry{}, err
	}

	if recoveredEntry, ok, err := service.tryResolveCompletedCloud115Upload(ctx, client, *item, request, false); err != nil {
		return connectors.FileEntry{}, err
	} else if ok {
		service.applyCloud115CompletedEntry(item, target, request, recoveredEntry, readTransferItemMetadata(*item))
		if err := service.persistTransferItemState(context.Background(), taskID, *item); err != nil {
			return connectors.FileEntry{}, err
		}
		return recoveredEntry, nil
	}

	session, err := client.OpenUploadSession(ctx, request)
	if err != nil {
		recoveredSession, recoveredEntry, progressed, recoverErr := service.reconcileCloud115UploadProgress(
			ctx,
			taskID,
			item,
			client,
			target,
			request,
			cloud115ShouldIgnoreStateFileDuringRecovery(*item, err),
		)
		if recoverErr != nil || (!progressed && recoveredEntry == nil) {
			return connectors.FileEntry{}, err
		}
		if recoveredEntry != nil {
			return *recoveredEntry, nil
		}
		session = recoveredSession
	}

	service.applyCloud115UploadSession(item, target, request, session, 0)
	if err := service.persistTransferItemState(context.Background(), taskID, *item); err != nil {
		return connectors.FileEntry{}, err
	}
	if request, err = service.buildCloud115UploadSessionRequest(*item, target, false); err != nil {
		return connectors.FileEntry{}, err
	}

	lastBytes := item.CommittedBytes
	lastObservedAt := time.Now().UTC()
	for !cloud115UploadCompleted(session) {
		if err := ctx.Err(); err != nil {
			return connectors.FileEntry{}, err
		}

		nextSession, uploadErr := client.UploadSessionParts(ctx, connectors.Cloud115UploadSessionRequest{
			RootID:          request.RootID,
			LocalPath:       request.LocalPath,
			RemotePath:      request.RemotePath,
			ResumeStatePath: request.ResumeStatePath,
			PartSize:        request.PartSize,
			MaxParts:        cloud115UploadMaxPartsPerCall,
			ParentID:        request.ParentID,
			UploadID:        request.UploadID,
			UploadURL:       request.UploadURL,
			Callback:        cloneCloud115RawJSON(request.Callback),
		})
		if uploadErr != nil {
			recoveredSession, recoveredEntry, progressed, recoverErr := service.reconcileCloud115UploadProgress(
				ctx,
				taskID,
				item,
				client,
				target,
				request,
				cloud115ShouldIgnoreStateFileDuringRecovery(*item, uploadErr),
			)
			if recoverErr == nil {
				if recoveredEntry != nil {
					return *recoveredEntry, nil
				}
				if recoveredSession != nil && progressed {
					session = recoveredSession
					if request, err = service.buildCloud115UploadSessionRequest(*item, target, false); err != nil {
						return connectors.FileEntry{}, err
					}
					lastBytes = item.CommittedBytes
					lastObservedAt = time.Now().UTC()
					continue
				}
			}
			return connectors.FileEntry{}, uploadErr
		}

		now := time.Now().UTC()
		currentBytes := cloud115UploadedBytes(nextSession)
		speed := estimateTransferSpeed(currentBytes-lastBytes, now.Sub(lastObservedAt))
		if currentBytes <= lastBytes && !cloud115UploadCompleted(nextSession) && len(nextSession.UploadedInCall) == 0 {
			return connectors.FileEntry{}, fmt.Errorf("115 涓婁紶浼氳瘽鏈繑鍥炴柊鐨勫垎鐗囪繘搴?")
		}

		service.applyCloud115UploadSession(item, target, request, nextSession, speed)
		if err := service.persistTransferItemState(context.Background(), taskID, *item); err != nil {
			return connectors.FileEntry{}, err
		}
		if request, err = service.buildCloud115UploadSessionRequest(*item, target, false); err != nil {
			return connectors.FileEntry{}, err
		}

		lastBytes = item.CommittedBytes
		lastObservedAt = now
		session = nextSession
	}

	completedSession, err := client.CompleteUploadSession(ctx, request)
	if err != nil {
		recoveredSession, recoveredEntry, _, recoverErr := service.reconcileCloud115UploadProgress(
			ctx,
			taskID,
			item,
			client,
			target,
			request,
			true,
		)
		if recoverErr == nil {
			if recoveredEntry != nil {
				return *recoveredEntry, nil
			}
			if recoveredSession != nil && cloud115UploadCompleted(recoveredSession) {
				completedSession = recoveredSession
				err = nil
			}
		}
	}
	if err != nil {
		return connectors.FileEntry{}, err
	}

	entry, err := service.resolveCloud115CompletedEntry(ctx, client, request, completedSession)
	if err != nil {
		return connectors.FileEntry{}, err
	}
	service.applyCloud115CompletedEntry(item, target, request, entry, completedSession)
	if err := service.persistTransferItemState(context.Background(), taskID, *item); err != nil {
		return connectors.FileEntry{}, err
	}
	return entry, nil
}

func (service *Service) reconcileInterruptedCloud115Transfer(ctx context.Context, item *store.TransferTaskItem) error {
	metadata := readTransferItemMetadata(*item)
	if !hasCloud115TransferMetadata(metadata) || strings.TrimSpace(item.TargetEndpointID) == "" {
		return nil
	}

	endpoint, err := service.store.GetStorageEndpointByID(ctx, item.TargetEndpointID)
	if err != nil || !shouldUseCloud115UploadSession(endpoint) {
		return nil
	}

	client, target, err := service.buildCloud115UploadClient(ctx, endpoint)
	if err != nil {
		return err
	}

	request, err := service.buildCloud115UploadSessionRequest(*item, target, false)
	if err != nil {
		return err
	}

	recoveredSession, _, _, err := service.reconcileCloud115UploadProgress(
		ctx,
		"",
		item,
		client,
		target,
		request,
		cloud115ShouldIgnoreStateFileDuringRecovery(*item, nil),
	)
	if err != nil {
		return nil
	}
	if recoveredSession != nil {
		service.applyCloud115UploadSession(item, target, request, recoveredSession, 0)
	}
	return nil
}

func (service *Service) tryResolveCompletedCloud115Upload(
	ctx context.Context,
	client cloud115UploadClient,
	item store.TransferTaskItem,
	request connectors.Cloud115UploadSessionRequest,
	ignoreStateFile bool,
) (connectors.FileEntry, bool, error) {
	metadata := readTransferItemMetadata(item)
	if !hasCloud115TransferMetadata(metadata) && item.CommittedBytes <= 0 {
		return connectors.FileEntry{}, false, nil
	}

	if !ignoreStateFile {
		if statePath := strings.TrimSpace(request.ResumeStatePath); statePath != "" {
			if _, err := os.Stat(statePath); err == nil {
				return connectors.FileEntry{}, false, nil
			} else if !os.IsNotExist(err) {
				return connectors.FileEntry{}, false, err
			}
		}
	}

	entry, err := client.StatEntry(ctx, request.RootID, request.RemotePath)
	if err != nil || entry.IsDir {
		return connectors.FileEntry{}, false, nil
	}

	expectedSize := max(item.TotalBytes, item.StagedBytes)
	if expectedSize > 0 && entry.Size > 0 && entry.Size < expectedSize {
		return connectors.FileEntry{}, false, nil
	}
	return entry, true, nil
}

func (service *Service) reconcileCloud115UploadProgress(
	ctx context.Context,
	taskID string,
	item *store.TransferTaskItem,
	client cloud115UploadClient,
	target cloud115UploadTarget,
	request connectors.Cloud115UploadSessionRequest,
	ignoreStateFile bool,
) (*connectors.Cloud115UploadSession, *connectors.FileEntry, bool, error) {
	if item == nil {
		return nil, nil, false, nil
	}

	if recoveredEntry, ok, err := service.tryResolveCompletedCloud115Upload(ctx, client, *item, request, ignoreStateFile); err != nil {
		return nil, nil, false, err
	} else if ok {
		service.applyCloud115CompletedEntry(item, target, request, recoveredEntry, readTransferItemMetadata(*item))
		if strings.TrimSpace(taskID) != "" {
			if err := service.persistTransferItemState(context.Background(), taskID, *item); err != nil {
				return nil, nil, false, err
			}
		}
		return nil, &recoveredEntry, true, nil
	}

	beforeBytes := item.CommittedBytes
	beforeParts := cloud115MetadataUploadedParts(readTransferItemMetadata(*item))
	session, err := client.ListUploadSessionParts(ctx, request)
	if err != nil {
		return nil, nil, false, err
	}
	service.applyCloud115UploadSession(item, target, request, session, 0)
	if strings.TrimSpace(taskID) != "" {
		if err := service.persistTransferItemState(context.Background(), taskID, *item); err != nil {
			return nil, nil, false, err
		}
	}
	afterParts := cloud115MetadataUploadedParts(readTransferItemMetadata(*item))
	progressed := item.CommittedBytes != beforeBytes || afterParts != beforeParts || cloud115UploadCompleted(session)
	return session, nil, progressed, nil
}

func cloud115MetadataUploadedParts(metadata transferItemMetadata) int64 {
	if metadata.Cloud115 == nil {
		return 0
	}
	return max(metadata.Cloud115.UploadedParts, 0)
}

func cloud115ShouldIgnoreStateFileDuringRecovery(item store.TransferTaskItem, err error) bool {
	metadata := readTransferItemMetadata(item)
	if metadata.Cloud115 != nil && strings.EqualFold(strings.TrimSpace(metadata.Cloud115.Status), "complete") {
		return true
	}
	if item.TotalBytes > 0 && item.CommittedBytes >= item.TotalBytes {
		return true
	}
	var connectorErr *connectors.ConnectorError
	if errors.As(err, &connectorErr) && connectorErr.Code == connectors.ErrorCodeNotFound && item.CommittedBytes > 0 {
		return true
	}
	return false
}

func cloneCloud115RawJSON(value json.RawMessage) json.RawMessage {
	if len(value) == 0 {
		return nil
	}
	cloned := make(json.RawMessage, len(value))
	copy(cloned, value)
	return cloned
}

func (service *Service) resolveCloud115CompletedEntry(
	ctx context.Context,
	client cloud115UploadClient,
	request connectors.Cloud115UploadSessionRequest,
	session *connectors.Cloud115UploadSession,
) (connectors.FileEntry, error) {
	if session != nil && session.Entry != nil && !session.Entry.IsDir {
		return *session.Entry, nil
	}
	return client.StatEntry(ctx, request.RootID, request.RemotePath)
}

func cloud115UploadedBytes(session *connectors.Cloud115UploadSession) int64 {
	if session == nil {
		return 0
	}
	if session.Progress != nil {
		return max(session.Progress.UploadedBytes, 0)
	}
	var uploadedBytes int64
	for _, part := range session.Parts {
		uploadedBytes += max(part.Size, 0)
	}
	return max(uploadedBytes, 0)
}

func cloud115UploadedParts(session *connectors.Cloud115UploadSession) int64 {
	if session == nil {
		return 0
	}
	if session.Progress != nil {
		return max(session.Progress.UploadedParts, 0)
	}
	return int64(len(session.Parts))
}

func cloud115TotalParts(session *connectors.Cloud115UploadSession) int64 {
	if session == nil {
		return 0
	}
	if session.Progress != nil {
		return max(session.Progress.TotalParts, 0)
	}
	return int64(len(session.Parts))
}

func cloud115SessionPartSize(session *connectors.Cloud115UploadSession, fallback int) int64 {
	if session != nil {
		if session.PartSize > 0 {
			return session.PartSize
		}
		if session.Progress != nil && session.Progress.PartSize > 0 {
			return session.Progress.PartSize
		}
	}
	return int64(max(fallback, 0))
}

func cloud115SessionParentID(session *connectors.Cloud115UploadSession, fallback int) int {
	if session != nil && session.ParentID > 0 {
		return session.ParentID
	}
	return max(fallback, 0)
}

func cloud115SessionFileName(session *connectors.Cloud115UploadSession, request connectors.Cloud115UploadSessionRequest) string {
	if session != nil && strings.TrimSpace(session.FileName) != "" {
		return strings.TrimSpace(session.FileName)
	}
	if baseName := filepath.Base(strings.TrimSpace(request.RemotePath)); baseName != "" && baseName != "." {
		return baseName
	}
	return ""
}

func cloud115SessionUploadURL(session *connectors.Cloud115UploadSession, existing string) string {
	if session != nil && strings.TrimSpace(session.UploadURL) != "" {
		return strings.TrimSpace(session.UploadURL)
	}
	return strings.TrimSpace(existing)
}

func cloud115SessionCallback(session *connectors.Cloud115UploadSession, existing json.RawMessage) json.RawMessage {
	if session != nil && len(session.Callback) > 0 {
		return cloneCloud115RawJSON(session.Callback)
	}
	return cloneCloud115RawJSON(existing)
}

func (service *Service) applyCloud115UploadSession(
	item *store.TransferTaskItem,
	target cloud115UploadTarget,
	request connectors.Cloud115UploadSessionRequest,
	session *connectors.Cloud115UploadSession,
	speed int64,
) {
	if item == nil {
		return
	}

	existingMetadata := readTransferItemMetadata(*item)
	fileSize := max(item.TotalBytes, item.StagedBytes)
	uploadedBytes := item.CommittedBytes
	uploadedParts := int64(0)
	totalParts := int64(0)
	status := "uploading"
	uploadID := ""
	sessionURL := strings.TrimSpace(request.UploadURL)
	sessionCallback := cloneCloud115RawJSON(request.Callback)
	parentID := max(request.ParentID, 0)
	fileName := cloud115SessionFileName(nil, request)
	partSize := int64(max(request.PartSize, 0))

	if existingMetadata.Cloud115 != nil {
		uploadID = strings.TrimSpace(existingMetadata.Cloud115.UploadID)
		if existingMetadata.Cloud115.UploadedBytes > 0 {
			uploadedBytes = existingMetadata.Cloud115.UploadedBytes
		}
		uploadedParts = existingMetadata.Cloud115.UploadedParts
		totalParts = existingMetadata.Cloud115.TotalParts
		sessionURL = defaultString(strings.TrimSpace(existingMetadata.Cloud115.SessionURL), sessionURL)
		sessionCallback = cloud115SessionCallback(nil, existingMetadata.Cloud115.SessionCallback)
		parentID = max(parentID, existingMetadata.Cloud115.ParentID)
		fileName = defaultString(strings.TrimSpace(existingMetadata.Cloud115.FileName), fileName)
		partSize = max(partSize, existingMetadata.Cloud115.PartSize)
		if strings.TrimSpace(existingMetadata.Cloud115.Status) != "" {
			status = strings.TrimSpace(existingMetadata.Cloud115.Status)
		}
	}

	if session != nil {
		uploadID = defaultString(strings.TrimSpace(session.UploadID), uploadID)
		if session.Progress != nil {
			fileSize = max(fileSize, max(session.Progress.FileSize, 0))
			uploadedBytes = cloud115UploadedBytes(session)
			uploadedParts = cloud115UploadedParts(session)
			totalParts = max(cloud115TotalParts(session), uploadedParts)
		} else if len(session.Parts) > 0 {
			uploadedBytes = cloud115UploadedBytes(session)
			uploadedParts = cloud115UploadedParts(session)
			totalParts = max(cloud115TotalParts(session), uploadedParts)
		}
		parentID = cloud115SessionParentID(session, parentID)
		fileName = cloud115SessionFileName(session, request)
		partSize = cloud115SessionPartSize(session, request.PartSize)
		sessionURL = cloud115SessionUploadURL(session, sessionURL)
		sessionCallback = cloud115SessionCallback(session, sessionCallback)
		if cloud115UploadCompleted(session) {
			status = "complete"
		} else {
			status = "uploading"
		}
	}

	if fileSize <= 0 {
		fileSize = max(fileSize, uploadedBytes)
	}

	item.TotalBytes = max(item.TotalBytes, fileSize)
	item.CommittedBytes = max(uploadedBytes, 0)
	if status == "complete" && item.TotalBytes > 0 {
		item.CommittedBytes = item.TotalBytes
	}
	progressPhase := transferPhaseCommitting
	if status == "complete" && item.TotalBytes > 0 {
		progressPhase = transferPhaseFinalizing
	}
	item.ProgressPercent = calcTransferItemProgress(item.TotalBytes, item.StagedBytes, item.CommittedBytes, progressPhase)
	item.UpdatedAt = time.Now().UTC()

	updateTransferItemMetadata(item, func(metadata *transferItemMetadata) {
		metadata.EngineKind = transferEngineKindCloud115
		metadata.EngineLabel = cloud115TransferEngineLabel
		metadata.CurrentSpeed = max(speed, 0)
		metadata.RefreshInterval = 1
		if metadata.Cloud115 == nil {
			metadata.Cloud115 = &transferItemCloud115Metadata{}
		}
		if uploadID != "" {
			metadata.Cloud115.UploadID = uploadID
		}
		metadata.Cloud115.ResumeStatePath = strings.TrimSpace(request.ResumeStatePath)
		metadata.Cloud115.RootID = strings.TrimSpace(request.RootID)
		metadata.Cloud115.AppType = strings.TrimSpace(target.AppType)
		metadata.Cloud115.ParentID = parentID
		metadata.Cloud115.FileName = fileName
		metadata.Cloud115.PartSize = partSize
		metadata.Cloud115.SessionURL = sessionURL
		metadata.Cloud115.SessionCallback = sessionCallback
		metadata.Cloud115.Status = status
		metadata.Cloud115.UploadedBytes = item.CommittedBytes
		metadata.Cloud115.UploadedParts = uploadedParts
		metadata.Cloud115.TotalParts = max(totalParts, uploadedParts)
	})
}

func (service *Service) applyCloud115CompletedEntry(
	item *store.TransferTaskItem,
	target cloud115UploadTarget,
	request connectors.Cloud115UploadSessionRequest,
	entry connectors.FileEntry,
	session any,
) {
	if item == nil {
		return
	}

	sessionMetadata := readTransferItemMetadata(*item)
	if uploadSession, ok := session.(*connectors.Cloud115UploadSession); ok {
		service.applyCloud115UploadSession(item, target, request, uploadSession, 0)
		sessionMetadata = readTransferItemMetadata(*item)
	}

	if entry.Size > 0 {
		item.TotalBytes = max(item.TotalBytes, entry.Size)
	}
	item.CommittedBytes = max(item.CommittedBytes, item.TotalBytes)
	item.ProgressPercent = calcTransferItemProgress(item.TotalBytes, item.StagedBytes, item.CommittedBytes, transferPhaseFinalizing)
	item.UpdatedAt = time.Now().UTC()

	updateTransferItemMetadata(item, func(metadata *transferItemMetadata) {
		metadata.EngineKind = transferEngineKindCloud115
		metadata.EngineLabel = cloud115TransferEngineLabel
		metadata.CurrentSpeed = 0
		metadata.RefreshInterval = 1
		if metadata.Cloud115 == nil {
			metadata.Cloud115 = &transferItemCloud115Metadata{}
		}
		if sessionMetadata.Cloud115 != nil {
			if metadata.Cloud115.UploadID == "" {
				metadata.Cloud115.UploadID = sessionMetadata.Cloud115.UploadID
			}
			metadata.Cloud115.UploadedParts = max(metadata.Cloud115.UploadedParts, sessionMetadata.Cloud115.UploadedParts)
			metadata.Cloud115.TotalParts = max(metadata.Cloud115.TotalParts, sessionMetadata.Cloud115.TotalParts)
			metadata.Cloud115.ParentID = max(metadata.Cloud115.ParentID, sessionMetadata.Cloud115.ParentID)
			metadata.Cloud115.FileName = defaultString(strings.TrimSpace(metadata.Cloud115.FileName), strings.TrimSpace(sessionMetadata.Cloud115.FileName))
			metadata.Cloud115.PartSize = max(metadata.Cloud115.PartSize, sessionMetadata.Cloud115.PartSize)
			metadata.Cloud115.SessionURL = defaultString(strings.TrimSpace(metadata.Cloud115.SessionURL), strings.TrimSpace(sessionMetadata.Cloud115.SessionURL))
			if len(metadata.Cloud115.SessionCallback) == 0 {
				metadata.Cloud115.SessionCallback = cloneCloud115RawJSON(sessionMetadata.Cloud115.SessionCallback)
			}
		}
		metadata.Cloud115.ResumeStatePath = strings.TrimSpace(request.ResumeStatePath)
		metadata.Cloud115.RootID = strings.TrimSpace(request.RootID)
		metadata.Cloud115.AppType = strings.TrimSpace(target.AppType)
		metadata.Cloud115.Status = "complete"
		metadata.Cloud115.UploadedBytes = item.CommittedBytes
	})
}

func (service *Service) buildCloud115UploadSessionRequest(
	item store.TransferTaskItem,
	target cloud115UploadTarget,
	ensureStateDir bool,
) (connectors.Cloud115UploadSessionRequest, error) {
	metadata := readTransferItemMetadata(item)
	statePath := strings.TrimSpace(metadata.Cloud115ResumeStatePath())
	if statePath == "" {
		statePath = service.cloud115TransferResumeStatePath(item)
	}
	if ensureStateDir {
		if err := ensureDirectory(filepath.Dir(statePath)); err != nil {
			return connectors.Cloud115UploadSessionRequest{}, err
		}
	}

	partSize := resolveCloud115UploadPartSize(max(item.TotalBytes, item.StagedBytes))
	parentID := 0
	uploadID := ""
	uploadURL := ""
	var callback json.RawMessage
	if metadata.Cloud115 != nil {
		if metadata.Cloud115.PartSize > 0 {
			partSize = int(metadata.Cloud115.PartSize)
		}
		parentID = max(metadata.Cloud115.ParentID, 0)
		uploadID = strings.TrimSpace(metadata.Cloud115.UploadID)
		uploadURL = strings.TrimSpace(metadata.Cloud115.SessionURL)
		callback = cloneCloud115RawJSON(metadata.Cloud115.SessionCallback)
	}

	return connectors.Cloud115UploadSessionRequest{
		RootID:          defaultString(strings.TrimSpace(target.RootID), "0"),
		LocalPath:       strings.TrimSpace(item.StagingPath),
		RemotePath:      normalizeCloud115UploadPath(item.TargetPath),
		ResumeStatePath: statePath,
		PartSize:        partSize,
		ParentID:        parentID,
		UploadID:        uploadID,
		UploadURL:       uploadURL,
		Callback:        callback,
	}, nil
}

func (service *Service) cloud115TransferResumeStatePath(item store.TransferTaskItem) string {
	return filepath.Join(service.transferStateRoot(), "tasks", item.TaskID, item.ID, cloud115TransferSessionFileName)
}

func (service *Service) cloud115AbortUploadSession(ctx context.Context, item store.TransferTaskItem) error {
	if strings.TrimSpace(item.TargetEndpointID) == "" {
		return nil
	}

	endpoint, err := service.store.GetStorageEndpointByID(ctx, item.TargetEndpointID)
	if err != nil || !shouldUseCloud115UploadSession(endpoint) {
		return nil
	}

	client, target, err := service.buildCloud115UploadClient(ctx, endpoint)
	if err != nil {
		return err
	}

	request, err := service.buildCloud115UploadSessionRequest(item, target, false)
	if err != nil {
		return err
	}
	_, err = client.AbortUploadSession(ctx, request)
	return err
}

func cloud115UploadCompleted(session *connectors.Cloud115UploadSession) bool {
	if session == nil {
		return false
	}
	if session.Completed {
		return true
	}
	return session.Progress != nil && session.Progress.Completed
}

func normalizeCloud115UploadPath(value string) string {
	normalized := filepath.ToSlash(strings.TrimSpace(value))
	return strings.Trim(normalized, "/")
}

func resolveCloud115UploadPartSize(totalBytes int64) int {
	if totalBytes > 0 && totalBytes < cloud115DefaultUploadPartSize {
		return int(totalBytes)
	}
	return cloud115DefaultUploadPartSize
}

func (metadata transferItemMetadata) Cloud115ResumeStatePath() string {
	if metadata.Cloud115 == nil {
		return ""
	}
	return strings.TrimSpace(metadata.Cloud115.ResumeStatePath)
}
