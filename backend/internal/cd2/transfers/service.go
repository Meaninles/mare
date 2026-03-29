package transfers

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"

	cd2client "mam/backend/internal/cd2/client"
	cd2pb "mam/backend/internal/cd2/pb"

	"google.golang.org/protobuf/types/known/emptypb"
)

const (
	KindUpload   = "upload"
	KindDownload = "download"
	KindCopy     = "copy"

	ActionPause  = "pause"
	ActionResume = "resume"
	ActionCancel = "cancel"

	maxRecentEvents = 20
)

type Stats struct {
	GeneratedAt    time.Time `json:"generatedAt"`
	TotalTasks     int       `json:"totalTasks"`
	UploadTasks    int       `json:"uploadTasks"`
	DownloadTasks  int       `json:"downloadTasks"`
	CopyTasks      int       `json:"copyTasks"`
	QueuedTasks    int       `json:"queuedTasks"`
	RunningTasks   int       `json:"runningTasks"`
	PausedTasks    int       `json:"pausedTasks"`
	FailedTasks    int       `json:"failedTasks"`
	SuccessTasks   int       `json:"successTasks"`
	CanceledTasks  int       `json:"canceledTasks"`
	SkippedTasks   int       `json:"skippedTasks"`
	TotalBytes     int64     `json:"totalBytes"`
	FinishedBytes  int64     `json:"finishedBytes"`
	BytesPerSecond float64   `json:"bytesPerSecond"`
}

type TaskRecord struct {
	Key              string     `json:"key"`
	Kind             string     `json:"kind"`
	Title            string     `json:"title"`
	Status           string     `json:"status"`
	ProgressPercent  int        `json:"progressPercent"`
	SourcePath       string     `json:"sourcePath,omitempty"`
	TargetPath       string     `json:"targetPath,omitempty"`
	FilePath         string     `json:"filePath,omitempty"`
	OperatorType     string     `json:"operatorType,omitempty"`
	RawStatus        string     `json:"rawStatus,omitempty"`
	Paused           bool       `json:"paused"`
	CanPause         bool       `json:"canPause"`
	CanResume        bool       `json:"canResume"`
	CanCancel        bool       `json:"canCancel"`
	TotalBytes       int64      `json:"totalBytes"`
	FinishedBytes    int64      `json:"finishedBytes"`
	BytesPerSecond   float64    `json:"bytesPerSecond"`
	BufferUsedBytes  int64      `json:"bufferUsedBytes,omitempty"`
	ThreadCount      int        `json:"threadCount,omitempty"`
	TotalFiles       int64      `json:"totalFiles,omitempty"`
	FinishedFiles    int64      `json:"finishedFiles,omitempty"`
	FailedFiles      int64      `json:"failedFiles,omitempty"`
	CanceledFiles    int64      `json:"canceledFiles,omitempty"`
	SkippedFiles     int64      `json:"skippedFiles,omitempty"`
	ErrorMessage     string     `json:"errorMessage,omitempty"`
	Detail           string     `json:"detail,omitempty"`
	StartedAt        *time.Time `json:"startedAt,omitempty"`
	FinishedAt       *time.Time `json:"finishedAt,omitempty"`
	LastObservedAt   time.Time  `json:"lastObservedAt"`
	ControlReference string     `json:"controlReference,omitempty"`
}

type PushEvent struct {
	ID         string     `json:"id"`
	Stream     string     `json:"stream"`
	EventType  string     `json:"eventType"`
	Summary    string     `json:"summary,omitempty"`
	Message    string     `json:"message,omitempty"`
	OccurredAt time.Time  `json:"occurredAt"`
	Level      string     `json:"level,omitempty"`
	Target     string     `json:"target,omitempty"`
	RelatedKey string     `json:"relatedKey,omitempty"`
	Counts     *TaskCount `json:"counts,omitempty"`
}

type TaskCount struct {
	UploadCount   uint32 `json:"uploadCount"`
	DownloadCount uint32 `json:"downloadCount"`
	CopyCount     uint32 `json:"copyCount"`
}

type WatcherState struct {
	Started               bool       `json:"started"`
	PushTaskChangeActive  bool       `json:"pushTaskChangeActive"`
	PushMessageActive     bool       `json:"pushMessageActive"`
	LastConnectedAt       *time.Time `json:"lastConnectedAt,omitempty"`
	LastEventAt           *time.Time `json:"lastEventAt,omitempty"`
	LastError             string     `json:"lastError,omitempty"`
	EventCount            uint64     `json:"eventCount"`
	ObservedTransferCount TaskCount  `json:"observedTransferCount"`
}

type ListResult struct {
	GeneratedAt  time.Time    `json:"generatedAt"`
	Stats        Stats        `json:"stats"`
	Tasks        []TaskRecord `json:"tasks"`
	RecentEvents []PushEvent  `json:"recentEvents"`
	Watcher      WatcherState `json:"watcher"`
}

type ActionRequest struct {
	Kind       string   `json:"kind"`
	Action     string   `json:"action"`
	Keys       []string `json:"keys,omitempty"`
	SourcePath string   `json:"sourcePath,omitempty"`
	DestPath   string   `json:"destPath,omitempty"`
	All        bool     `json:"all,omitempty"`
}

type ActionSummary struct {
	Kind      string   `json:"kind"`
	Action    string   `json:"action"`
	Requested int      `json:"requested"`
	Updated   int      `json:"updated"`
	Keys      []string `json:"keys,omitempty"`
	Message   string   `json:"message"`
}

type Service struct {
	client *cd2client.Manager

	backgroundCtx    context.Context
	backgroundCancel context.CancelFunc
	watchersOnce     sync.Once

	mu           sync.RWMutex
	recentEvents []PushEvent
	watcher      WatcherState
}

func NewService(client *cd2client.Manager) *Service {
	ctx, cancel := context.WithCancel(context.Background())
	return &Service{
		client:           client,
		backgroundCtx:    ctx,
		backgroundCancel: cancel,
	}
}

func (service *Service) Close() {
	if service == nil || service.backgroundCancel == nil {
		return
	}
	service.backgroundCancel()
}

func (service *Service) List(ctx context.Context) (ListResult, error) {
	service.ensureWatchers()

	client, _, err := service.authorizedClient(ctx)
	if err != nil {
		return ListResult{}, err
	}

	requestCtx, cancel := service.client.WithRequestTimeout(ctx)
	defer cancel()

	downloads, err := client.GetDownloadFileList(requestCtx, &emptypb.Empty{})
	if err != nil {
		return ListResult{}, err
	}
	uploads, err := client.GetUploadFileList(requestCtx, &cd2pb.GetUploadFileListRequest{GetAll: true})
	if err != nil {
		return ListResult{}, err
	}
	copyTasks, err := client.GetCopyTasks(requestCtx, &emptypb.Empty{})
	if err != nil {
		return ListResult{}, err
	}

	generatedAt := time.Now().UTC()
	tasks := make([]TaskRecord, 0, len(downloads.GetDownloadFiles())+len(uploads.GetUploadFiles())+len(copyTasks.GetCopyTasks()))
	for _, item := range uploads.GetUploadFiles() {
		tasks = append(tasks, uploadTaskFromProto(item, generatedAt))
	}
	for _, item := range downloads.GetDownloadFiles() {
		tasks = append(tasks, downloadTaskFromProto(item, generatedAt))
	}
	for _, item := range copyTasks.GetCopyTasks() {
		tasks = append(tasks, copyTaskFromProto(item, generatedAt))
	}
	sortTaskRecords(tasks)

	stats := buildStats(tasks, uploads, downloads)

	service.mu.RLock()
	recentEvents := append([]PushEvent(nil), service.recentEvents...)
	watcher := service.watcher
	service.mu.RUnlock()

	return ListResult{
		GeneratedAt:  generatedAt,
		Stats:        stats,
		Tasks:        tasks,
		RecentEvents: recentEvents,
		Watcher:      watcher,
	}, nil
}

func (service *Service) ApplyAction(ctx context.Context, request ActionRequest) (ActionSummary, error) {
	client, _, err := service.authorizedClient(ctx)
	if err != nil {
		return ActionSummary{}, err
	}

	action := strings.ToLower(strings.TrimSpace(request.Action))
	kind := strings.ToLower(strings.TrimSpace(request.Kind))
	if action == "" || kind == "" {
		return ActionSummary{}, errors.New("kind 和 action 不能为空")
	}

	summary := ActionSummary{
		Kind:   kind,
		Action: action,
	}

	requestCtx, cancel := service.client.WithRequestTimeout(ctx)
	defer cancel()

	switch kind {
	case KindUpload:
		keys := normalizeKeys(request.Keys)
		summary.Keys = keys
		if request.All {
			summary.Requested = 1
			switch action {
			case ActionPause:
				_, err = client.PauseAllUploadFiles(requestCtx, &emptypb.Empty{})
			case ActionResume:
				_, err = client.ResumeAllUploadFiles(requestCtx, &emptypb.Empty{})
			default:
				return ActionSummary{}, errors.New("上传任务仅支持 pause / resume / cancel")
			}
			if err != nil {
				return ActionSummary{}, err
			}
			summary.Updated = 1
			summary.Message = fmt.Sprintf("已对全部上传任务执行 %s", action)
			return summary, nil
		}
		if len(keys) == 0 {
			return ActionSummary{}, errors.New("请至少提供一个上传任务 key")
		}
		summary.Requested = len(keys)
		payload := &cd2pb.MultpleUploadFileKeyRequest{Keys: keys}
		switch action {
		case ActionPause:
			_, err = client.PauseUploadFiles(requestCtx, payload)
		case ActionResume:
			_, err = client.ResumeUploadFiles(requestCtx, payload)
		case ActionCancel:
			_, err = client.CancelUploadFiles(requestCtx, payload)
		default:
			return ActionSummary{}, errors.New("上传任务仅支持 pause / resume / cancel")
		}
		if err != nil {
			return ActionSummary{}, err
		}
		summary.Updated = len(keys)
		summary.Message = fmt.Sprintf("已对 %d 个上传任务执行 %s", len(keys), action)
		return summary, nil
	case KindCopy:
		sourcePath := strings.TrimSpace(request.SourcePath)
		destPath := strings.TrimSpace(request.DestPath)
		if request.All {
			summary.Requested = 1
			switch action {
			case ActionPause:
				_, err = client.PauseAllCopyTasks(requestCtx, &cd2pb.PauseAllCopyTasksRequest{Pause: true})
			case ActionResume:
				_, err = client.ResumeAllCopyTasks(requestCtx, &emptypb.Empty{})
			default:
				return ActionSummary{}, errors.New("全部复制任务仅支持 pause / resume")
			}
			if err != nil {
				return ActionSummary{}, err
			}
			summary.Updated = 1
			summary.Message = fmt.Sprintf("已对全部复制任务执行 %s", action)
			return summary, nil
		}
		if sourcePath == "" || destPath == "" {
			return ActionSummary{}, errors.New("复制任务控制需要 sourcePath 和 destPath")
		}
		summary.Keys = []string{buildCopyTaskKey(sourcePath, destPath)}
		summary.Requested = 1
		switch action {
		case ActionPause:
			_, err = client.PauseCopyTask(requestCtx, &cd2pb.PauseCopyTaskRequest{
				SourcePath: sourcePath,
				DestPath:   destPath,
				Pause:      true,
			})
		case ActionResume:
			_, err = client.PauseCopyTask(requestCtx, &cd2pb.PauseCopyTaskRequest{
				SourcePath: sourcePath,
				DestPath:   destPath,
				Pause:      false,
			})
		case ActionCancel:
			_, err = client.CancelCopyTask(requestCtx, &cd2pb.CopyTaskRequest{
				SourcePath: sourcePath,
				DestPath:   destPath,
			})
		default:
			return ActionSummary{}, errors.New("复制任务仅支持 pause / resume / cancel")
		}
		if err != nil {
			return ActionSummary{}, err
		}
		summary.Updated = 1
		summary.Message = fmt.Sprintf("已对复制任务执行 %s", action)
		return summary, nil
	case KindDownload:
		return ActionSummary{}, errors.New("CD2 当前官方 API 未提供下载任务的暂停/恢复/取消接口")
	default:
		return ActionSummary{}, errors.New("不支持的任务类型，仅支持 upload / download / copy")
	}
}

func (service *Service) ensureWatchers() {
	service.watchersOnce.Do(func() {
		service.setWatcherStarted()
		go service.runPushTaskChangeLoop()
		go service.runPushMessageLoop()
	})
}

func (service *Service) runPushTaskChangeLoop() {
	for {
		select {
		case <-service.backgroundCtx.Done():
			service.setPushTaskChangeActive(false, context.Canceled)
			return
		default:
		}

		client, _, err := service.authorizedClient(service.backgroundCtx)
		if err != nil {
			service.setPushTaskChangeActive(false, err)
			if !sleepWithContext(service.backgroundCtx, 2*time.Second) {
				return
			}
			continue
		}

		stream, err := client.PushTaskChange(service.backgroundCtx, &emptypb.Empty{})
		if err != nil {
			service.setPushTaskChangeActive(false, err)
			if !sleepWithContext(service.backgroundCtx, 2*time.Second) {
				return
			}
			continue
		}
		service.setPushTaskChangeActive(true, nil)

		for {
			message, recvErr := stream.Recv()
			if recvErr != nil {
				if errors.Is(recvErr, io.EOF) || errors.Is(recvErr, context.Canceled) {
					service.setPushTaskChangeActive(false, recvErr)
					break
				}
				service.setPushTaskChangeActive(false, recvErr)
				break
			}
			service.recordPushTaskChange(message)
		}

		if !sleepWithContext(service.backgroundCtx, 1200*time.Millisecond) {
			return
		}
	}
}

func (service *Service) runPushMessageLoop() {
	for {
		select {
		case <-service.backgroundCtx.Done():
			service.setPushMessageActive(false, context.Canceled)
			return
		default:
		}

		client, _, err := service.authorizedClient(service.backgroundCtx)
		if err != nil {
			service.setPushMessageActive(false, err)
			if !sleepWithContext(service.backgroundCtx, 2*time.Second) {
				return
			}
			continue
		}

		stream, err := client.PushMessage(service.backgroundCtx, &emptypb.Empty{})
		if err != nil {
			service.setPushMessageActive(false, err)
			if !sleepWithContext(service.backgroundCtx, 2*time.Second) {
				return
			}
			continue
		}
		service.setPushMessageActive(true, nil)

		for {
			message, recvErr := stream.Recv()
			if recvErr != nil {
				if errors.Is(recvErr, io.EOF) || errors.Is(recvErr, context.Canceled) {
					service.setPushMessageActive(false, recvErr)
					break
				}
				service.setPushMessageActive(false, recvErr)
				break
			}
			service.recordPushMessage(message)
		}

		if !sleepWithContext(service.backgroundCtx, 1200*time.Millisecond) {
			return
		}
	}
}

func (service *Service) recordPushTaskChange(message *cd2pb.GetAllTasksCountResult) {
	if message == nil {
		return
	}

	counts := TaskCount{
		UploadCount:   message.GetUploadCount(),
		DownloadCount: message.GetDownloadCount(),
		CopyCount:     message.GetCopyTaskCount(),
	}
	summary := fmt.Sprintf("上传 %d，下载 %d，复制 %d，上传状态变更 %d 条", counts.UploadCount, counts.DownloadCount, counts.CopyCount, len(message.GetUploadFileStatusChanges()))
	event := PushEvent{
		ID:         fmt.Sprintf("taskchange-%d", time.Now().UnixNano()),
		Stream:     "PushTaskChange",
		EventType:  "transfer_counts",
		Summary:    summary,
		OccurredAt: time.Now().UTC(),
		Counts:     &counts,
	}
	service.appendEvent(event, &counts, nil)

	if len(message.GetUploadFileStatusChanges()) > 0 {
		for _, item := range message.GetUploadFileStatusChanges() {
			if item == nil {
				continue
			}
			changeEvent := PushEvent{
				ID:         fmt.Sprintf("taskchange-upload-%d", time.Now().UnixNano()),
				Stream:     "PushTaskChange",
				EventType:  "upload_status_change",
				Summary:    strings.TrimSpace(item.GetStatus()),
				Message:    strings.TrimSpace(item.GetErrorMessage()),
				OccurredAt: time.Now().UTC(),
				RelatedKey: strings.TrimSpace(item.GetKey()),
				Counts:     &counts,
			}
			service.appendEvent(changeEvent, &counts, nil)
		}
	}
}

func (service *Service) recordPushMessage(message *cd2pb.CloudDrivePushMessage) {
	if message == nil {
		return
	}

	now := time.Now().UTC()
	event := PushEvent{
		ID:         fmt.Sprintf("pushmsg-%d", now.UnixNano()),
		Stream:     "PushMessage",
		EventType:  strings.TrimSpace(message.GetMessageType().String()),
		OccurredAt: now,
	}

	var counts *TaskCount
	switch {
	case message.GetTransferTaskStatus() != nil:
		status := message.GetTransferTaskStatus()
		taskCounts := TaskCount{
			UploadCount:   status.GetUploadCount(),
			DownloadCount: status.GetDownloadCount(),
			CopyCount:     status.GetCopyTaskCount(),
		}
		counts = &taskCounts
		event.Summary = fmt.Sprintf("上传 %d，下载 %d，复制 %d", taskCounts.UploadCount, taskCounts.DownloadCount, taskCounts.CopyCount)
	case message.GetLogMessage() != nil:
		logMessage := message.GetLogMessage()
		event.Summary = strings.TrimSpace(logMessage.GetMessage())
		event.Message = strings.TrimSpace(logMessage.GetMessage())
		event.Level = strings.ToLower(strings.TrimSpace(logMessage.GetLevel().String()))
		event.Target = strings.TrimSpace(logMessage.GetTarget())
	case message.GetFileSystemChange() != nil:
		change := message.GetFileSystemChange()
		event.Summary = strings.TrimSpace(change.GetPath())
		event.Message = strings.TrimSpace(change.GetChangeType().String())
	case message.GetMountPointChange() != nil:
		change := message.GetMountPointChange()
		event.Summary = strings.TrimSpace(change.GetMountPoint())
		event.Message = strings.TrimSpace(change.GetActionType().String())
	case message.GetMergeTaskUpdate() != nil:
		update := message.GetMergeTaskUpdate()
		event.Summary = fmt.Sprintf("merge tasks: %d", len(update.GetMergeTasks()))
		event.Message = strings.TrimSpace(update.GetLastMergedPath())
	case message.GetExitedMessage() != nil:
		exitMessage := message.GetExitedMessage()
		event.Summary = strings.TrimSpace(exitMessage.GetMessage())
		event.Message = strings.TrimSpace(exitMessage.GetExitReason().String())
	case message.GetUpdateStatus() != nil:
		updateStatus := message.GetUpdateStatus()
		event.Summary = strings.TrimSpace(updateStatus.GetUpdatePhase().String())
		event.Message = strings.TrimSpace(updateStatus.GetNewVersion())
	}

	service.appendEvent(event, counts, nil)
}

func (service *Service) appendEvent(event PushEvent, counts *TaskCount, err error) {
	service.mu.Lock()
	defer service.mu.Unlock()

	service.recentEvents = append([]PushEvent{event}, service.recentEvents...)
	if len(service.recentEvents) > maxRecentEvents {
		service.recentEvents = service.recentEvents[:maxRecentEvents]
	}
	service.watcher.EventCount++
	eventAt := event.OccurredAt.UTC()
	service.watcher.LastEventAt = &eventAt
	if counts != nil {
		service.watcher.ObservedTransferCount = *counts
	}
	if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, io.EOF) {
		service.watcher.LastError = err.Error()
	}
}

func (service *Service) setWatcherStarted() {
	service.mu.Lock()
	defer service.mu.Unlock()

	service.watcher.Started = true
}

func (service *Service) setPushTaskChangeActive(active bool, err error) {
	service.mu.Lock()
	defer service.mu.Unlock()

	service.watcher.PushTaskChangeActive = active
	if active {
		now := time.Now().UTC()
		service.watcher.LastConnectedAt = &now
		service.watcher.LastError = ""
		return
	}
	if err != nil && !shouldIgnoreWatcherError(err) {
		service.watcher.LastError = err.Error()
	}
}

func (service *Service) setPushMessageActive(active bool, err error) {
	service.mu.Lock()
	defer service.mu.Unlock()

	service.watcher.PushMessageActive = active
	if active {
		now := time.Now().UTC()
		service.watcher.LastConnectedAt = &now
		service.watcher.LastError = ""
		return
	}
	if err != nil && !shouldIgnoreWatcherError(err) {
		service.watcher.LastError = err.Error()
	}
}

func (service *Service) authorizedClient(ctx context.Context) (cd2pb.CloudDriveFileSrvClient, cd2client.State, error) {
	if service == nil || service.client == nil {
		return nil, cd2client.State{}, errors.New("CD2 传输任务服务未初始化")
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

func buildStats(tasks []TaskRecord, uploads *cd2pb.GetUploadFileListResult, downloads *cd2pb.GetDownloadFileListResult) Stats {
	stats := Stats{
		GeneratedAt: time.Now().UTC(),
	}
	for _, item := range tasks {
		stats.TotalTasks++
		switch item.Kind {
		case KindUpload:
			stats.UploadTasks++
		case KindDownload:
			stats.DownloadTasks++
		case KindCopy:
			stats.CopyTasks++
		}
		switch item.Status {
		case "queued":
			stats.QueuedTasks++
		case "running":
			stats.RunningTasks++
		case "paused":
			stats.PausedTasks++
		case "failed":
			stats.FailedTasks++
		case "success":
			stats.SuccessTasks++
		case "canceled":
			stats.CanceledTasks++
		case "skipped":
			stats.SkippedTasks++
		}
		stats.TotalBytes += item.TotalBytes
		stats.FinishedBytes += item.FinishedBytes
		stats.BytesPerSecond += item.BytesPerSecond
	}
	if uploads != nil && uploads.GetTotalBytes() > 0 {
		stats.TotalBytes = maxInt64(stats.TotalBytes, int64(uploads.GetTotalBytes()))
		stats.FinishedBytes = maxInt64(stats.FinishedBytes, int64(uploads.GetFinishedBytes()))
	}
	if downloads != nil {
		stats.BytesPerSecond += downloads.GetGlobalBytesPerSecond()
	}
	return stats
}

func sortTaskRecords(tasks []TaskRecord) {
	sort.Slice(tasks, func(i, j int) bool {
		left := tasks[i]
		right := tasks[j]
		if statusWeight(left.Status) != statusWeight(right.Status) {
			return statusWeight(left.Status) < statusWeight(right.Status)
		}
		if !left.LastObservedAt.Equal(right.LastObservedAt) {
			return left.LastObservedAt.After(right.LastObservedAt)
		}
		if left.Kind != right.Kind {
			return left.Kind < right.Kind
		}
		return left.Key < right.Key
	})
}

func uploadTaskFromProto(item *cd2pb.UploadFileInfo, observedAt time.Time) TaskRecord {
	status := mapUploadStatus(item)
	progressPercent := calcProgressInt64(int64(item.GetSize()), int64(item.GetTransferedBytes()))
	return TaskRecord{
		Key:              strings.TrimSpace(item.GetKey()),
		Kind:             KindUpload,
		Title:            chooseTaskTitle(strings.TrimSpace(item.GetDestPath()), "上传任务"),
		Status:           status,
		ProgressPercent:  progressPercent,
		TargetPath:       strings.TrimSpace(item.GetDestPath()),
		FilePath:         strings.TrimSpace(item.GetDestPath()),
		OperatorType:     strings.TrimSpace(item.GetOperatorType().String()),
		RawStatus:        defaultString(strings.TrimSpace(item.GetStatus()), item.GetStatusEnum().String()),
		Paused:           status == "paused",
		CanPause:         status == "queued" || status == "running",
		CanResume:        status == "paused",
		CanCancel:        status == "queued" || status == "running" || status == "paused",
		TotalBytes:       int64(item.GetSize()),
		FinishedBytes:    int64(item.GetTransferedBytes()),
		BytesPerSecond:   0,
		ErrorMessage:     strings.TrimSpace(item.GetErrorMessage()),
		LastObservedAt:   observedAt,
		ControlReference: strings.TrimSpace(item.GetKey()),
	}
}

func downloadTaskFromProto(item *cd2pb.DownloadFileInfo, observedAt time.Time) TaskRecord {
	status := mapDownloadStatus(item)
	progressPercent := calcProcessProgress(item.GetProcess())
	return TaskRecord{
		Key:              strings.TrimSpace(item.GetFilePath()),
		Kind:             KindDownload,
		Title:            chooseTaskTitle(strings.TrimSpace(item.GetFilePath()), "下载任务"),
		Status:           status,
		ProgressPercent:  progressPercent,
		FilePath:         strings.TrimSpace(item.GetFilePath()),
		TargetPath:       strings.TrimSpace(item.GetFilePath()),
		RawStatus:        status,
		Paused:           false,
		CanPause:         false,
		CanResume:        false,
		CanCancel:        false,
		TotalBytes:       int64(item.GetFileLength()),
		FinishedBytes:    0,
		BytesPerSecond:   item.GetBytesPerSecond(),
		BufferUsedBytes:  int64(item.GetTotalBufferUsed()),
		ThreadCount:      int(item.GetDownloadThreadCount()),
		ErrorMessage:     strings.TrimSpace(item.GetLastDownloadError()),
		Detail:           strings.TrimSpace(item.GetDetailDownloadInfo()),
		LastObservedAt:   observedAt,
		ControlReference: strings.TrimSpace(item.GetFilePath()),
	}
}

func copyTaskFromProto(item *cd2pb.CopyTask, observedAt time.Time) TaskRecord {
	status := mapCopyStatus(item)
	progressPercent := calcProgressInt64(int64(item.GetTotalBytes()), int64(item.GetUploadedBytes()))
	startedAt := toTimePointer(item.GetStartTime())
	finishedAt := toTimePointer(item.GetEndTime())
	return TaskRecord{
		Key:              buildCopyTaskKey(item.GetSourcePath(), item.GetDestPath()),
		Kind:             KindCopy,
		Title:            chooseTaskTitle(strings.TrimSpace(item.GetDestPath()), "复制任务"),
		Status:           status,
		ProgressPercent:  progressPercent,
		SourcePath:       strings.TrimSpace(item.GetSourcePath()),
		TargetPath:       strings.TrimSpace(item.GetDestPath()),
		RawStatus:        strings.TrimSpace(item.GetStatus().String()),
		Paused:           item.GetPaused(),
		CanPause:         !item.GetPaused() && (status == "queued" || status == "running"),
		CanResume:        item.GetPaused(),
		CanCancel:        status == "queued" || status == "running" || status == "paused",
		TotalBytes:       int64(item.GetTotalBytes()),
		FinishedBytes:    int64(item.GetUploadedBytes()),
		BytesPerSecond:   0,
		TotalFiles:       int64(item.GetTotalFiles()),
		FinishedFiles:    int64(item.GetUploadedFiles()),
		FailedFiles:      int64(item.GetFailedFiles()),
		CanceledFiles:    int64(item.GetCancelledFiles()),
		SkippedFiles:     int64(item.GetSkippedFiles()),
		ErrorMessage:     flattenTaskErrors(item.GetErrors()),
		StartedAt:        startedAt,
		FinishedAt:       finishedAt,
		LastObservedAt:   observedAt,
		ControlReference: buildCopyTaskKey(item.GetSourcePath(), item.GetDestPath()),
	}
}

func mapUploadStatus(item *cd2pb.UploadFileInfo) string {
	switch item.GetStatusEnum() {
	case cd2pb.UploadFileInfo_WaitforPreprocessing, cd2pb.UploadFileInfo_Preprocessing, cd2pb.UploadFileInfo_Inqueue:
		return "queued"
	case cd2pb.UploadFileInfo_Transfer:
		return "running"
	case cd2pb.UploadFileInfo_Pause:
		return "paused"
	case cd2pb.UploadFileInfo_Finish:
		return "success"
	case cd2pb.UploadFileInfo_Cancelled:
		return "canceled"
	case cd2pb.UploadFileInfo_Skipped, cd2pb.UploadFileInfo_Ignored:
		return "skipped"
	case cd2pb.UploadFileInfo_Error, cd2pb.UploadFileInfo_FatalError:
		return "failed"
	default:
		return "queued"
	}
}

func mapDownloadStatus(item *cd2pb.DownloadFileInfo) string {
	if strings.TrimSpace(item.GetLastDownloadError()) != "" {
		return "failed"
	}
	if item.GetBytesPerSecond() > 0 {
		return "running"
	}
	if item.GetTotalBufferUsed() > 0 || len(item.GetProcess()) > 0 || strings.TrimSpace(item.GetDetailDownloadInfo()) != "" {
		return "queued"
	}
	return "queued"
}

func mapCopyStatus(item *cd2pb.CopyTask) string {
	if item.GetPaused() {
		return "paused"
	}
	switch item.GetStatus() {
	case cd2pb.CopyTask_Pending:
		return "queued"
	case cd2pb.CopyTask_Scanning, cd2pb.CopyTask_Scanned:
		return "running"
	case cd2pb.CopyTask_Completed:
		if len(item.GetErrors()) > 0 || item.GetFailedFiles() > 0 {
			return "failed"
		}
		return "success"
	case cd2pb.CopyTask_Failed:
		return "failed"
	default:
		return "queued"
	}
}

func buildCopyTaskKey(sourcePath, destPath string) string {
	return strings.TrimSpace(sourcePath) + " -> " + strings.TrimSpace(destPath)
}

func flattenTaskErrors(errorsList []*cd2pb.TaskError) string {
	if len(errorsList) == 0 {
		return ""
	}
	parts := make([]string, 0, len(errorsList))
	for _, item := range errorsList {
		if item == nil {
			continue
		}
		message := strings.TrimSpace(item.GetMessage())
		if message == "" {
			continue
		}
		parts = append(parts, message)
	}
	return strings.Join(parts, " | ")
}

func chooseTaskTitle(pathValue string, fallback string) string {
	trimmed := strings.TrimSpace(pathValue)
	if trimmed == "" {
		return fallback
	}
	trimmed = strings.TrimRight(trimmed, "/")
	index := strings.LastIndex(trimmed, "/")
	if index >= 0 && index < len(trimmed)-1 {
		return trimmed[index+1:]
	}
	return trimmed
}

func calcProgressInt64(total, finished int64) int {
	if total <= 0 || finished <= 0 {
		return 0
	}
	if finished >= total {
		return 100
	}
	return int((finished * 100) / total)
}

func calcProcessProgress(process []string) int {
	progress := 0
	for _, item := range process {
		candidate := strings.TrimSpace(strings.TrimSuffix(item, "%"))
		if candidate == "" {
			continue
		}
		var value int
		if _, err := fmt.Sscanf(candidate, "%d", &value); err == nil {
			if value > progress {
				progress = value
			}
		}
	}
	if progress > 100 {
		return 100
	}
	return progress
}

func statusWeight(status string) int {
	switch status {
	case "running":
		return 0
	case "queued":
		return 1
	case "paused":
		return 2
	case "failed":
		return 3
	case "canceled":
		return 4
	case "success":
		return 5
	case "skipped":
		return 6
	default:
		return 10
	}
}

func normalizeKeys(values []string) []string {
	result := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, item := range values {
		key := strings.TrimSpace(item)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, key)
	}
	return result
}

func toTimePointer(value interface{ AsTime() time.Time }) *time.Time {
	if value == nil {
		return nil
	}
	result := value.AsTime().UTC()
	return &result
}

func maxInt64(left, right int64) int64 {
	if left > right {
		return left
	}
	return right
}

func sleepWithContext(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func shouldIgnoreWatcherError(err error) bool {
	if err == nil {
		return true
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, io.EOF) {
		return true
	}
	lower := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(lower, "unimplemented") || strings.Contains(lower, "not implemented")
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return strings.TrimSpace(fallback)
}
