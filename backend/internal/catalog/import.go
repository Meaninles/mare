package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"mam/backend/internal/connectors"
	"mam/backend/internal/store"
)

const (
	deviceRoleManagedStorage = "managed_storage"
	deviceRoleImportSource   = "import_source"

	importRuleTypeMediaType = "media_type"
	importRuleTypeExtension = "extension"

	taskTypeImportExecute = "import_execute"

	defaultImportBrowseLimit = 800
)

type deviceRoleSelection struct {
	Role       string
	EndpointID string
	SelectedAt time.Time
}

type ImportDeviceRecord struct {
	Device             connectors.DeviceInfo `json:"device"`
	IdentitySignature  string                `json:"identitySignature"`
	KnownEndpoint      *EndpointRecord       `json:"knownEndpoint,omitempty"`
	SuggestedRole      string                `json:"suggestedRole"`
	CurrentSessionRole string                `json:"currentSessionRole,omitempty"`
	SelectedAt         *time.Time            `json:"selectedAt,omitempty"`
}

type SelectImportDeviceRoleRequest struct {
	IdentitySignature string `json:"identitySignature"`
	Role              string `json:"role"`
	Name              string `json:"name"`
}

type ImportDeviceRoleSelectionRecord struct {
	Device     ImportDeviceRecord `json:"device"`
	Role       string             `json:"role"`
	Endpoint   *EndpointRecord    `json:"endpoint,omitempty"`
	SelectedAt time.Time          `json:"selectedAt"`
}

type ImportSourceBrowseRequest struct {
	IdentitySignature string `json:"identitySignature"`
	MediaType         string `json:"mediaType"`
	Limit             int    `json:"limit"`
}

type ImportSourceEntryRecord struct {
	Path         string     `json:"path"`
	RelativePath string     `json:"relativePath"`
	Name         string     `json:"name"`
	MediaType    string     `json:"mediaType"`
	Size         int64      `json:"size"`
	ModifiedAt   *time.Time `json:"modifiedAt,omitempty"`
}

type ImportSourceBrowseResult struct {
	Device    ImportDeviceRecord        `json:"device"`
	MediaType string                    `json:"mediaType"`
	Limit     int                       `json:"limit"`
	Entries   []ImportSourceEntryRecord `json:"entries"`
}

type ImportRuleRecord struct {
	ID                string    `json:"id"`
	RuleType          string    `json:"ruleType"`
	MatchValue        string    `json:"matchValue"`
	TargetEndpointIDs []string  `json:"targetEndpointIds"`
	CreatedAt         time.Time `json:"createdAt"`
	UpdatedAt         time.Time `json:"updatedAt"`
}

type SaveImportRulesRequest struct {
	Rules []ImportRuleInput `json:"rules"`
}

type ImportRuleInput struct {
	RuleType          string   `json:"ruleType"`
	MatchValue        string   `json:"matchValue"`
	TargetEndpointIDs []string `json:"targetEndpointIds"`
}

type ExecuteImportRequest struct {
	IdentitySignature string   `json:"identitySignature"`
	EntryPaths        []string `json:"entryPaths"`
}

type ImportTargetResult struct {
	EndpointID   string `json:"endpointId"`
	EndpointName string `json:"endpointName"`
	Status       string `json:"status"`
	Error        string `json:"error,omitempty"`
}

type ImportExecutionItem struct {
	RelativePath   string               `json:"relativePath"`
	DisplayName    string               `json:"displayName"`
	LogicalPathKey string               `json:"logicalPathKey"`
	MediaType      string               `json:"mediaType"`
	Status         string               `json:"status"`
	AssetID        string               `json:"assetId,omitempty"`
	TargetResults  []ImportTargetResult `json:"targetResults"`
	Error          string               `json:"error,omitempty"`
}

type ImportExecutionSummary struct {
	TaskID            string                `json:"taskId"`
	IdentitySignature string                `json:"identitySignature"`
	DeviceLabel       string                `json:"deviceLabel"`
	Status            string                `json:"status"`
	TotalFiles        int                   `json:"totalFiles"`
	SuccessCount      int                   `json:"successCount"`
	PartialCount      int                   `json:"partialCount"`
	FailedCount       int                   `json:"failedCount"`
	ProgressPercent   int                   `json:"progressPercent"`
	ProgressLabel     string                `json:"progressLabel,omitempty"`
	StartedAt         time.Time             `json:"startedAt"`
	FinishedAt        time.Time             `json:"finishedAt"`
	Items             []ImportExecutionItem `json:"items"`
	Error             string                `json:"error,omitempty"`
}

func (service *Service) ListImportDevices(ctx context.Context) ([]ImportDeviceRecord, error) {
	devices, err := service.removableEnumerator.ListDevices(ctx)
	if err != nil {
		return nil, err
	}

	knownEndpointsByIdentity, err := service.listKnownRemovableEndpointsByIdentity(ctx)
	if err != nil {
		return nil, err
	}

	records := make([]ImportDeviceRecord, 0, len(devices))
	for _, device := range devices {
		identitySignature := connectors.GenerateDeviceIdentity(device)
		record := buildImportDeviceRecord(
			device,
			identitySignature,
			knownEndpointsByIdentity[identitySignature],
			service.currentDeviceRoleSelection(identitySignature),
		)
		records = append(records, record)
	}

	sort.Slice(records, func(left, right int) bool {
		return strings.ToLower(records[left].Device.MountPoint) < strings.ToLower(records[right].Device.MountPoint)
	})

	return records, nil
}

func (service *Service) SelectImportDeviceRole(ctx context.Context, request SelectImportDeviceRoleRequest) (ImportDeviceRoleSelectionRecord, error) {
	role := normalizeDeviceRole(request.Role)
	if role == "" {
		return ImportDeviceRoleSelectionRecord{}, errors.New("role is required")
	}

	device, identitySignature, err := service.findRemovableDeviceByIdentity(ctx, request.IdentitySignature)
	if err != nil {
		return ImportDeviceRoleSelectionRecord{}, err
	}

	now := time.Now().UTC()
	selection := &deviceRoleSelection{
		Role:       role,
		SelectedAt: now,
	}

	var endpointRecord *EndpointRecord
	if role == deviceRoleManagedStorage {
		connectionConfig, err := json.Marshal(map[string]any{
			"device": device,
		})
		if err != nil {
			return ImportDeviceRoleSelectionRecord{}, err
		}

		endpoint, err := service.RegisterEndpoint(ctx, RegisterEndpointRequest{
			Name:               defaultString(strings.TrimSpace(request.Name), defaultString(strings.TrimSpace(device.VolumeLabel), "移动存储")),
			EndpointType:       string(connectors.EndpointTypeRemovable),
			RootPath:           strings.TrimSpace(device.MountPoint),
			RoleMode:           defaultRoleMode,
			AvailabilityStatus: defaultAvailabilityStatus,
			IdentitySignature:  identitySignature,
			ConnectionConfig:   connectionConfig,
		})
		if err != nil {
			return ImportDeviceRoleSelectionRecord{}, err
		}

		selection.EndpointID = endpoint.ID
		endpointCopy := endpoint
		endpointRecord = &endpointCopy
	}

	service.deviceRoleSelections.Store(identitySignature, *selection)

	knownEndpointsByIdentity, err := service.listKnownRemovableEndpointsByIdentity(ctx)
	if err != nil {
		return ImportDeviceRoleSelectionRecord{}, err
	}

	deviceRecord := buildImportDeviceRecord(
		device,
		identitySignature,
		knownEndpointsByIdentity[identitySignature],
		selection,
	)

	return ImportDeviceRoleSelectionRecord{
		Device:     deviceRecord,
		Role:       role,
		Endpoint:   endpointRecord,
		SelectedAt: now,
	}, nil
}

func (service *Service) BrowseImportSource(ctx context.Context, request ImportSourceBrowseRequest) (ImportSourceBrowseResult, error) {
	device, identitySignature, err := service.findRemovableDeviceByIdentity(ctx, request.IdentitySignature)
	if err != nil {
		return ImportSourceBrowseResult{}, err
	}

	knownEndpointsByIdentity, err := service.listKnownRemovableEndpointsByIdentity(ctx)
	if err != nil {
		return ImportSourceBrowseResult{}, err
	}

	connector, err := connectors.NewRemovableConnector(connectors.RemovableConfig{
		Name:   defaultString(strings.TrimSpace(device.VolumeLabel), "导入源设备"),
		Device: device,
	})
	if err != nil {
		return ImportSourceBrowseResult{}, err
	}

	mediaTypeFilter := normalizeImportMediaType(request.MediaType)
	limit := request.Limit
	if limit <= 0 {
		limit = defaultImportBrowseLimit
	}

	entries, err := service.collectImportSourceEntries(ctx, connector, mediaTypeFilter, limit)
	if err != nil {
		return ImportSourceBrowseResult{}, err
	}

	return ImportSourceBrowseResult{
		Device: buildImportDeviceRecord(
			device,
			identitySignature,
			knownEndpointsByIdentity[identitySignature],
			service.currentDeviceRoleSelection(identitySignature),
		),
		MediaType: mediaTypeFilter,
		Limit:     limit,
		Entries:   entries,
	}, nil
}

func (service *Service) ListImportRules(ctx context.Context) ([]ImportRuleRecord, error) {
	rules, err := service.store.ListImportRules(ctx)
	if err != nil {
		return nil, err
	}

	return toImportRuleRecords(rules)
}

func (service *Service) SaveImportRules(ctx context.Context, request SaveImportRulesRequest) ([]ImportRuleRecord, error) {
	endpoints, err := service.store.ListStorageEndpoints(ctx)
	if err != nil {
		return nil, err
	}

	managedEndpointIDs := make(map[string]struct{}, len(endpoints))
	for _, endpoint := range endpoints {
		if !isManagedEndpoint(endpoint) {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(endpoint.AvailabilityStatus), "DISABLED") {
			continue
		}
		managedEndpointIDs[endpoint.ID] = struct{}{}
	}

	type aggregatedRule struct {
		ruleType   string
		matchValue string
		targets    map[string]struct{}
	}

	aggregated := make(map[string]*aggregatedRule)
	for _, input := range request.Rules {
		ruleType := normalizeImportRuleType(input.RuleType)
		if ruleType == "" {
			return nil, errors.New("ruleType is required")
		}

		matchValue := normalizeImportRuleMatchValue(ruleType, input.MatchValue)
		if matchValue == "" {
			continue
		}

		key := ruleType + "|" + matchValue
		entry, exists := aggregated[key]
		if !exists {
			entry = &aggregatedRule{
				ruleType:   ruleType,
				matchValue: matchValue,
				targets:    make(map[string]struct{}),
			}
			aggregated[key] = entry
		}

		for _, endpointID := range uniqueStrings(input.TargetEndpointIDs) {
			if _, ok := managedEndpointIDs[endpointID]; !ok {
				return nil, fmt.Errorf("target endpoint %q is not available for import rules", endpointID)
			}
			entry.targets[endpointID] = struct{}{}
		}
	}

	now := time.Now().UTC()
	keys := make([]string, 0, len(aggregated))
	for key, value := range aggregated {
		if len(value.targets) == 0 {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)

	rules := make([]store.ImportRule, 0, len(keys))
	for _, key := range keys {
		entry := aggregated[key]
		targetEndpointIDs := mapKeysSorted(entry.targets)
		payload, err := json.Marshal(targetEndpointIDs)
		if err != nil {
			return nil, err
		}

		rules = append(rules, store.ImportRule{
			ID:                uuid.NewString(),
			RuleType:          entry.ruleType,
			MatchValue:        entry.matchValue,
			TargetEndpointIDs: string(payload),
			CreatedAt:         now,
			UpdatedAt:         now,
		})
	}

	if err := service.store.ReplaceImportRules(ctx, rules); err != nil {
		return nil, err
	}

	return toImportRuleRecords(rules)
}

func (service *Service) ExecuteImport(ctx context.Context, request ExecuteImportRequest) (ImportExecutionSummary, error) {
	task, err := service.createCatalogTask(ctx, taskTypeImportExecute, request)
	if err != nil {
		return ImportExecutionSummary{}, err
	}

	startedAt := time.Now().UTC()
	summary := ImportExecutionSummary{
		TaskID:     task.ID,
		Status:     taskStatusRunning,
		StartedAt:  startedAt,
		FinishedAt: startedAt,
	}

	slog.Info("import task started", "taskId", task.ID, "identitySignature", request.IdentitySignature, "fileCount", len(request.EntryPaths))

	if err := service.store.UpdateTaskStatus(ctx, task.ID, store.TaskStatusUpdate{
		Status:     taskStatusRunning,
		RetryCount: task.RetryCount,
		StartedAt:  &startedAt,
		UpdatedAt:  startedAt,
	}); err != nil {
		return summary, err
	}

	executeErr := service.executeImport(ctx, request, task.ID, task.RetryCount, startedAt, &summary)
	finishedAt := time.Now().UTC()
	summary.FinishedAt = finishedAt
	if executeErr != nil && summary.Error == "" {
		summary.Error = executeErr.Error()
	}

	taskStatus := taskStatusSuccess
	if summary.FailedCount > 0 || summary.PartialCount > 0 || executeErr != nil {
		taskStatus = taskStatusFailed
		summary.Status = taskStatusFailed
	} else {
		summary.Status = taskStatusSuccess
	}

	resultText := fmt.Sprintf(
		"已处理 %d/%d 个文件（100%%），成功 %d 个，部分完成 %d 个，失败 %d 个。",
		summary.TotalFiles,
		summary.TotalFiles,
		summary.SuccessCount,
		summary.PartialCount,
		summary.FailedCount,
	)
	var errorMessage *string
	if summary.Error != "" {
		errorMessage = &summary.Error
	}

	if err := service.store.UpdateTaskStatus(ctx, task.ID, store.TaskStatusUpdate{
		Status:        taskStatus,
		ResultSummary: &resultText,
		ErrorMessage:  errorMessage,
		RetryCount:    task.RetryCount,
		StartedAt:     &startedAt,
		FinishedAt:    &finishedAt,
		UpdatedAt:     finishedAt,
	}); err != nil {
		return summary, err
	}

	if executeErr != nil {
		slog.Error("import task failed", "taskId", task.ID, "identitySignature", request.IdentitySignature, "error", executeErr)
		return summary, executeErr
	}

	if summary.FailedCount > 0 || summary.PartialCount > 0 {
		slog.Warn(
			"import task completed with partial failures",
			"taskId", task.ID,
			"identitySignature", request.IdentitySignature,
			"successCount", summary.SuccessCount,
			"partialCount", summary.PartialCount,
			"failedCount", summary.FailedCount,
		)
		return summary, errors.New("import completed with failures")
	}

	slog.Info(
		"import task completed",
		"taskId", task.ID,
		"identitySignature", request.IdentitySignature,
		"successCount", summary.SuccessCount,
		"partialCount", summary.PartialCount,
		"failedCount", summary.FailedCount,
	)

	return summary, nil
}

func (service *Service) executeImport(
	ctx context.Context,
	request ExecuteImportRequest,
	scanRevision string,
	retryCount int,
	startedAt time.Time,
	summary *ImportExecutionSummary,
) error {
	identitySignature := strings.TrimSpace(request.IdentitySignature)
	if identitySignature == "" {
		return errors.New("identitySignature is required")
	}

	entryPaths := uniqueStrings(request.EntryPaths)
	if len(entryPaths) == 0 {
		return errors.New("entryPaths is required")
	}

	device, resolvedIdentity, err := service.findRemovableDeviceByIdentity(ctx, identitySignature)
	if err != nil {
		return err
	}

	sourceConnector, err := connectors.NewRemovableConnector(connectors.RemovableConfig{
		Name:   defaultString(strings.TrimSpace(device.VolumeLabel), "导入源设备"),
		Device: device,
	})
	if err != nil {
		return err
	}
	if !sourceConnector.Descriptor().Capabilities.CanReadStream {
		return errors.New("source device does not support read stream")
	}

	rules, err := service.ListImportRules(ctx)
	if err != nil {
		return err
	}
	if len(rules) == 0 {
		return errors.New("尚未配置导入规则")
	}

	targetEndpoints, err := service.listManagedImportEndpoints(ctx)
	if err != nil {
		return err
	}

	summary.IdentitySignature = resolvedIdentity
	summary.DeviceLabel = defaultString(strings.TrimSpace(device.VolumeLabel), strings.TrimSpace(device.MountPoint))
	summary.TotalFiles = len(entryPaths)
	summary.Items = make([]ImportExecutionItem, 0, len(entryPaths))
	if err := service.updateImportTaskProgress(ctx, scanRevision, retryCount, startedAt, *summary); err != nil {
		return err
	}

	for _, entryPath := range entryPaths {
		item := service.executeImportItem(ctx, sourceConnector, resolvedIdentity, entryPath, rules, targetEndpoints, scanRevision)
		summary.Items = append(summary.Items, item)
		switch item.Status {
		case taskStatusSuccess:
			summary.SuccessCount++
		case "partial":
			summary.PartialCount++
		default:
			summary.FailedCount++
		}
		if err := service.updateImportTaskProgress(ctx, scanRevision, retryCount, startedAt, *summary); err != nil {
			return err
		}
	}

	if summary.FailedCount > 0 || summary.PartialCount > 0 {
		return errors.New("导入已完成，但部分文件未成功写入目标端点")
	}

	return nil
}

func (service *Service) executeImportItem(
	ctx context.Context,
	sourceConnector *connectors.RemovableConnector,
	sourceIdentity string,
	entryPath string,
	rules []ImportRuleRecord,
	targetEndpoints map[string]store.StorageEndpoint,
	scanRevision string,
) ImportExecutionItem {
	relativePath := normalizeImportRelativePath(entryPath, "", "")
	item := ImportExecutionItem{
		RelativePath: relativePath,
		DisplayName:  relativePath,
		Status:       taskStatusFailed,
	}

	if relativePath == "" {
		item.Error = "文件路径不能为空。"
		return item
	}

	entry, err := sourceConnector.StatEntry(ctx, relativePath)
	if err != nil {
		item.Error = err.Error()
		return item
	}
	if entry.IsDir {
		item.Error = "暂不支持直接导入目录。"
		return item
	}
	if entry.MediaType == connectors.MediaTypeUnknown {
		item.Error = "当前仅支持导入图片、视频和音频文件。"
		return item
	}

	relativePath = normalizeImportRelativePath(entry.RelativePath, entry.Name, entry.Path)
	item.RelativePath = relativePath
	item.DisplayName = defaultString(strings.TrimSpace(entry.Name), relativePath)
	item.MediaType = string(entry.MediaType)

	logicalPathKey, err := NormalizeLogicalPathKey(sourceConnector.Descriptor().RootPath, relativePath)
	if err != nil {
		item.Error = err.Error()
		return item
	}
	item.LogicalPathKey = logicalPathKey

	targets := resolveImportTargets(entry, relativePath, sourceIdentity, rules, targetEndpoints)
	if len(targets) == 0 {
		item.Error = "没有匹配到可用的导入规则或目标端点。"
		return item
	}

	successCount := 0
	for _, endpoint := range targets {
		targetResult := ImportTargetResult{
			EndpointID:   endpoint.ID,
			EndpointName: endpoint.Name,
			Status:       taskStatusFailed,
		}

		targetConnector, err := service.buildConnector(endpoint)
		if err != nil {
			targetResult.Error = err.Error()
			item.TargetResults = append(item.TargetResults, targetResult)
			continue
		}
		if !targetConnector.Descriptor().Capabilities.CanWrite {
			targetResult.Error = "目标端点当前不可写。"
			item.TargetResults = append(item.TargetResults, targetResult)
			continue
		}

		reader, err := sourceConnector.ReadStream(ctx, relativePath)
		if err != nil {
			targetResult.Error = err.Error()
			item.TargetResults = append(item.TargetResults, targetResult)
			continue
		}

		copiedEntry, copyErr := targetConnector.CopyIn(ctx, relativePath, reader)
		_ = reader.Close()
		if copyErr != nil {
			targetResult.Error = copyErr.Error()
			item.TargetResults = append(item.TargetResults, targetResult)
			continue
		}

		scanResult := ScanResult{
			EndpointID:     endpoint.ID,
			PhysicalPath:   defaultString(strings.TrimSpace(copiedEntry.Path), relativePath),
			LogicalPathKey: logicalPathKey,
			Size:           copiedEntry.Size,
			MTime:          cloneTimePointer(copiedEntry.ModifiedAt),
			MediaType:      string(entry.MediaType),
			IsDir:          false,
		}
		if scanResult.Size == 0 {
			scanResult.Size = entry.Size
		}
		if scanResult.MTime == nil {
			scanResult.MTime = cloneTimePointer(entry.ModifiedAt)
		}

		if _, err := service.MergeScanResults(ctx, []ScanResult{scanResult}, scanRevision); err != nil {
			targetResult.Error = err.Error()
			item.TargetResults = append(item.TargetResults, targetResult)
			continue
		}

		targetResult.Status = taskStatusSuccess
		item.TargetResults = append(item.TargetResults, targetResult)
		successCount++
	}

	switch {
	case successCount == len(item.TargetResults):
		item.Status = taskStatusSuccess
	case successCount > 0:
		item.Status = "partial"
		item.Error = "部分目标端点导入成功。"
	default:
		item.Status = taskStatusFailed
		if item.Error == "" {
			item.Error = "所有目标端点导入都失败了。"
		}
	}

	asset, err := service.store.GetAssetByLogicalPathKey(ctx, logicalPathKey)
	if err == nil {
		item.AssetID = asset.ID
	}

	return item
}

func (service *Service) updateImportTaskProgress(
	ctx context.Context,
	taskID string,
	retryCount int,
	startedAt time.Time,
	summary ImportExecutionSummary,
) error {
	processedCount := summary.SuccessCount + summary.PartialCount + summary.FailedCount
	progressPercent := calcProgressPercent(processedCount, summary.TotalFiles)
	resultText := fmt.Sprintf(
		"已处理 %d/%d 个文件（%d%%），成功 %d 个，部分完成 %d 个，失败 %d 个。",
		processedCount,
		summary.TotalFiles,
		progressPercent,
		summary.SuccessCount,
		summary.PartialCount,
		summary.FailedCount,
	)
	now := time.Now().UTC()
	return service.store.UpdateTaskStatus(ctx, taskID, store.TaskStatusUpdate{
		Status:        taskStatusRunning,
		ResultSummary: &resultText,
		RetryCount:    retryCount,
		StartedAt:     &startedAt,
		UpdatedAt:     now,
	})
}

func (service *Service) collectImportSourceEntries(
	ctx context.Context,
	connector *connectors.RemovableConnector,
	mediaTypeFilter string,
	limit int,
) ([]ImportSourceEntryRecord, error) {
	queue := []string{""}
	entries := make([]ImportSourceEntryRecord, 0, limit)

	for len(queue) > 0 {
		currentPath := queue[0]
		queue = queue[1:]

		fileEntries, err := connector.ListEntries(ctx, connectors.ListEntriesRequest{
			Path:               currentPath,
			Recursive:          false,
			IncludeDirectories: true,
			MediaOnly:          false,
		})
		if err != nil {
			return nil, err
		}

		for _, entry := range fileEntries {
			if entry.IsDir {
				nextPath := normalizeImportRelativePath(entry.RelativePath, entry.Name, entry.Path)
				if nextPath != "" {
					queue = append(queue, nextPath)
				}
				continue
			}

			if entry.MediaType == connectors.MediaTypeUnknown {
				continue
			}

			if mediaTypeFilter != "all" && string(entry.MediaType) != mediaTypeFilter {
				continue
			}

			entries = append(entries, ImportSourceEntryRecord{
				Path:         entry.Path,
				RelativePath: normalizeImportRelativePath(entry.RelativePath, entry.Name, entry.Path),
				Name:         defaultString(strings.TrimSpace(entry.Name), normalizeImportRelativePath(entry.RelativePath, entry.Name, entry.Path)),
				MediaType:    string(entry.MediaType),
				Size:         entry.Size,
				ModifiedAt:   cloneTimePointer(entry.ModifiedAt),
			})
			if limit > 0 && len(entries) >= limit {
				sortImportSourceEntries(entries)
				return entries, nil
			}
		}
	}

	sortImportSourceEntries(entries)
	return entries, nil
}

func (service *Service) listKnownRemovableEndpointsByIdentity(ctx context.Context) (map[string]*store.StorageEndpoint, error) {
	endpoints, err := service.store.ListStorageEndpoints(ctx)
	if err != nil {
		return nil, err
	}

	lookup := make(map[string]*store.StorageEndpoint)
	for index := range endpoints {
		endpoint := endpoints[index]
		if normalizeEndpointType(endpoint.EndpointType) != string(connectors.EndpointTypeRemovable) {
			continue
		}
		endpointCopy := endpoint
		lookup[endpoint.IdentitySignature] = &endpointCopy
	}

	return lookup, nil
}

func (service *Service) findRemovableDeviceByIdentity(ctx context.Context, requestedIdentity string) (connectors.DeviceInfo, string, error) {
	identity := strings.TrimSpace(requestedIdentity)
	if identity == "" {
		return connectors.DeviceInfo{}, "", errors.New("identitySignature is required")
	}

	devices, err := service.removableEnumerator.ListDevices(ctx)
	if err != nil {
		return connectors.DeviceInfo{}, "", err
	}

	for _, device := range devices {
		candidateIdentity := connectors.GenerateDeviceIdentity(device)
		if candidateIdentity == identity {
			return device, candidateIdentity, nil
		}
	}

	return connectors.DeviceInfo{}, "", errors.New("未找到对应的可移动设备，请确认设备仍然已连接")
}

func (service *Service) currentDeviceRoleSelection(identitySignature string) *deviceRoleSelection {
	if value, ok := service.deviceRoleSelections.Load(strings.TrimSpace(identitySignature)); ok {
		selection, valid := value.(deviceRoleSelection)
		if valid {
			return &selection
		}
	}
	return nil
}

func (service *Service) listManagedImportEndpoints(ctx context.Context) (map[string]store.StorageEndpoint, error) {
	endpoints, err := service.store.ListStorageEndpoints(ctx)
	if err != nil {
		return nil, err
	}

	lookup := make(map[string]store.StorageEndpoint)
	for _, endpoint := range endpoints {
		if !isManagedEndpoint(endpoint) {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(endpoint.AvailabilityStatus), "DISABLED") {
			continue
		}
		lookup[endpoint.ID] = endpoint
	}

	return lookup, nil
}

func buildImportDeviceRecord(
	device connectors.DeviceInfo,
	identitySignature string,
	knownEndpoint *store.StorageEndpoint,
	selection *deviceRoleSelection,
) ImportDeviceRecord {
	record := ImportDeviceRecord{
		Device:            device,
		IdentitySignature: identitySignature,
		SuggestedRole:     deviceRoleImportSource,
	}

	if knownEndpoint != nil {
		endpointRecord := toEndpointRecord(*knownEndpoint)
		record.KnownEndpoint = &endpointRecord
		record.SuggestedRole = deviceRoleManagedStorage
	}

	if selection != nil {
		record.CurrentSessionRole = selection.Role
		record.SelectedAt = cloneTimePointer(&selection.SelectedAt)
	}

	return record
}

func normalizeDeviceRole(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case deviceRoleManagedStorage:
		return deviceRoleManagedStorage
	case deviceRoleImportSource:
		return deviceRoleImportSource
	default:
		return ""
	}
}

func normalizeImportMediaType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "all":
		return "all"
	case string(connectors.MediaTypeImage):
		return string(connectors.MediaTypeImage)
	case string(connectors.MediaTypeVideo):
		return string(connectors.MediaTypeVideo)
	case string(connectors.MediaTypeAudio):
		return string(connectors.MediaTypeAudio)
	default:
		return "all"
	}
}

func normalizeImportRuleType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case importRuleTypeMediaType:
		return importRuleTypeMediaType
	case importRuleTypeExtension:
		return importRuleTypeExtension
	default:
		return ""
	}
}

func normalizeImportRuleMatchValue(ruleType, value string) string {
	matchValue := strings.ToLower(strings.TrimSpace(value))
	switch ruleType {
	case importRuleTypeMediaType:
		switch matchValue {
		case string(connectors.MediaTypeImage), string(connectors.MediaTypeVideo), string(connectors.MediaTypeAudio):
			return matchValue
		default:
			return ""
		}
	case importRuleTypeExtension:
		matchValue = strings.TrimPrefix(matchValue, ".")
		if matchValue == "" {
			return ""
		}
		return "." + matchValue
	default:
		return ""
	}
}

func normalizeImportRelativePath(relativePath, name, pathValue string) string {
	for _, candidate := range []string{relativePath, name, pathValue} {
		normalized := strings.TrimPrefix(canonicalizePath(candidate), "/")
		if normalized != "" {
			return normalized
		}
	}
	return ""
}

func toImportRuleRecords(rules []store.ImportRule) ([]ImportRuleRecord, error) {
	records := make([]ImportRuleRecord, 0, len(rules))
	for _, rule := range rules {
		var targetEndpointIDs []string
		if err := json.Unmarshal([]byte(rule.TargetEndpointIDs), &targetEndpointIDs); err != nil {
			return nil, fmt.Errorf("decode import rule target endpoints: %w", err)
		}

		records = append(records, ImportRuleRecord{
			ID:                rule.ID,
			RuleType:          rule.RuleType,
			MatchValue:        rule.MatchValue,
			TargetEndpointIDs: uniqueStrings(targetEndpointIDs),
			CreatedAt:         rule.CreatedAt,
			UpdatedAt:         rule.UpdatedAt,
		})
	}
	return records, nil
}

func resolveImportTargets(
	entry connectors.FileEntry,
	relativePath string,
	sourceIdentity string,
	rules []ImportRuleRecord,
	targetEndpoints map[string]store.StorageEndpoint,
) []store.StorageEndpoint {
	extension := strings.ToLower(strings.TrimSpace(fileExtension(relativePath)))
	targetIDs := make(map[string]struct{})

	for _, rule := range rules {
		switch rule.RuleType {
		case importRuleTypeMediaType:
			if normalizeImportMediaType(rule.MatchValue) != string(entry.MediaType) {
				continue
			}
		case importRuleTypeExtension:
			if extension == "" || normalizeImportRuleMatchValue(importRuleTypeExtension, rule.MatchValue) != extension {
				continue
			}
		default:
			continue
		}

		for _, endpointID := range rule.TargetEndpointIDs {
			targetIDs[endpointID] = struct{}{}
		}
	}

	endpoints := make([]store.StorageEndpoint, 0, len(targetIDs))
	for _, endpointID := range mapKeysSorted(targetIDs) {
		endpoint, ok := targetEndpoints[endpointID]
		if !ok {
			continue
		}
		if normalizeEndpointType(endpoint.EndpointType) == string(connectors.EndpointTypeRemovable) && endpoint.IdentitySignature == sourceIdentity {
			continue
		}
		endpoints = append(endpoints, endpoint)
	}
	return endpoints
}

func sortImportSourceEntries(entries []ImportSourceEntryRecord) {
	sort.Slice(entries, func(left, right int) bool {
		leftTime := time.Time{}
		rightTime := time.Time{}
		if entries[left].ModifiedAt != nil {
			leftTime = entries[left].ModifiedAt.UTC()
		}
		if entries[right].ModifiedAt != nil {
			rightTime = entries[right].ModifiedAt.UTC()
		}
		if !leftTime.Equal(rightTime) {
			return leftTime.After(rightTime)
		}
		return strings.ToLower(entries[left].RelativePath) < strings.ToLower(entries[right].RelativePath)
	})
}

func mapKeysSorted(values map[string]struct{}) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func fileExtension(pathValue string) string {
	normalized := normalizeImportRelativePath(pathValue, "", pathValue)
	if normalized == "" {
		return ""
	}
	index := strings.LastIndex(normalized, ".")
	if index < 0 || index == len(normalized)-1 {
		return ""
	}
	return normalized[index:]
}
