package transfers

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
	"google.golang.org/protobuf/types/known/timestamppb"
)

type fakeTransferServer struct {
	cd2pb.UnimplementedCloudDriveFileSrvServer
	uploadFiles   map[string]*cd2pb.UploadFileInfo
	downloadFiles []*cd2pb.DownloadFileInfo
	copyTasks     map[string]*cd2pb.CopyTask
}

func (server *fakeTransferServer) GetSystemInfo(context.Context, *emptypb.Empty) (*cd2pb.CloudDriveSystemInfo, error) {
	return &cd2pb.CloudDriveSystemInfo{SystemReady: true}, nil
}

func (server *fakeTransferServer) GetApiTokenInfo(_ context.Context, request *cd2pb.StringValue) (*cd2pb.TokenInfo, error) {
	if request == nil || strings.TrimSpace(request.GetValue()) != "good-token" {
		return nil, status.Error(codes.PermissionDenied, "bad token")
	}
	return &cd2pb.TokenInfo{Token: "good-token", FriendlyName: "mam-backend"}, nil
}

func (server *fakeTransferServer) GetDownloadFileList(context.Context, *emptypb.Empty) (*cd2pb.GetDownloadFileListResult, error) {
	return &cd2pb.GetDownloadFileListResult{
		GlobalBytesPerSecond: 256,
		DownloadFiles:        server.downloadFiles,
	}, nil
}

func (server *fakeTransferServer) GetUploadFileList(_ context.Context, _ *cd2pb.GetUploadFileListRequest) (*cd2pb.GetUploadFileListResult, error) {
	files := make([]*cd2pb.UploadFileInfo, 0, len(server.uploadFiles))
	var totalBytes uint64
	var finishedBytes uint64
	for _, item := range server.uploadFiles {
		files = append(files, item)
		totalBytes += item.GetSize()
		finishedBytes += item.GetTransferedBytes()
	}
	return &cd2pb.GetUploadFileListResult{
		TotalCount:           uint32(len(files)),
		UploadFiles:          files,
		GlobalBytesPerSecond: 512,
		TotalBytes:           totalBytes,
		FinishedBytes:        finishedBytes,
	}, nil
}

func (server *fakeTransferServer) GetCopyTasks(context.Context, *emptypb.Empty) (*cd2pb.GetCopyTaskResult, error) {
	tasks := make([]*cd2pb.CopyTask, 0, len(server.copyTasks))
	for _, item := range server.copyTasks {
		tasks = append(tasks, item)
	}
	return &cd2pb.GetCopyTaskResult{CopyTasks: tasks}, nil
}

func (server *fakeTransferServer) PauseUploadFiles(_ context.Context, request *cd2pb.MultpleUploadFileKeyRequest) (*emptypb.Empty, error) {
	for _, key := range request.GetKeys() {
		if item, ok := server.uploadFiles[key]; ok {
			item.StatusEnum = cd2pb.UploadFileInfo_Pause
			item.Status = "Pause"
		}
	}
	return &emptypb.Empty{}, nil
}

func (server *fakeTransferServer) ResumeUploadFiles(_ context.Context, request *cd2pb.MultpleUploadFileKeyRequest) (*emptypb.Empty, error) {
	for _, key := range request.GetKeys() {
		if item, ok := server.uploadFiles[key]; ok {
			item.StatusEnum = cd2pb.UploadFileInfo_Transfer
			item.Status = "Transfer"
		}
	}
	return &emptypb.Empty{}, nil
}

func (server *fakeTransferServer) CancelUploadFiles(_ context.Context, request *cd2pb.MultpleUploadFileKeyRequest) (*emptypb.Empty, error) {
	for _, key := range request.GetKeys() {
		if item, ok := server.uploadFiles[key]; ok {
			item.StatusEnum = cd2pb.UploadFileInfo_Cancelled
			item.Status = "Cancelled"
		}
	}
	return &emptypb.Empty{}, nil
}

func (server *fakeTransferServer) PauseAllUploadFiles(context.Context, *emptypb.Empty) (*emptypb.Empty, error) {
	for _, item := range server.uploadFiles {
		item.StatusEnum = cd2pb.UploadFileInfo_Pause
		item.Status = "Pause"
	}
	return &emptypb.Empty{}, nil
}

func (server *fakeTransferServer) ResumeAllUploadFiles(context.Context, *emptypb.Empty) (*emptypb.Empty, error) {
	for _, item := range server.uploadFiles {
		item.StatusEnum = cd2pb.UploadFileInfo_Transfer
		item.Status = "Transfer"
	}
	return &emptypb.Empty{}, nil
}

func (server *fakeTransferServer) PauseCopyTask(_ context.Context, request *cd2pb.PauseCopyTaskRequest) (*emptypb.Empty, error) {
	key := buildCopyTaskKey(request.GetSourcePath(), request.GetDestPath())
	if item, ok := server.copyTasks[key]; ok {
		item.Paused = request.GetPause()
	}
	return &emptypb.Empty{}, nil
}

func (server *fakeTransferServer) CancelCopyTask(_ context.Context, request *cd2pb.CopyTaskRequest) (*emptypb.Empty, error) {
	key := buildCopyTaskKey(request.GetSourcePath(), request.GetDestPath())
	if item, ok := server.copyTasks[key]; ok {
		item.Status = cd2pb.CopyTask_Failed
		item.Errors = []*cd2pb.TaskError{{Message: "cancelled by test"}}
	}
	return &emptypb.Empty{}, nil
}

func (server *fakeTransferServer) PauseAllCopyTasks(_ context.Context, request *cd2pb.PauseAllCopyTasksRequest) (*cd2pb.BatchOperationResult, error) {
	for _, item := range server.copyTasks {
		item.Paused = request.GetPause()
	}
	return &cd2pb.BatchOperationResult{Success: true, AffectedCount: uint32(len(server.copyTasks))}, nil
}

func (server *fakeTransferServer) ResumeAllCopyTasks(context.Context, *emptypb.Empty) (*cd2pb.BatchOperationResult, error) {
	for _, item := range server.copyTasks {
		item.Paused = false
	}
	return &cd2pb.BatchOperationResult{Success: true, AffectedCount: uint32(len(server.copyTasks))}, nil
}

func (server *fakeTransferServer) PushTaskChange(_ *emptypb.Empty, stream grpc.ServerStreamingServer[cd2pb.GetAllTasksCountResult]) error {
	return stream.Send(&cd2pb.GetAllTasksCountResult{
		UploadCount:   uint32(len(server.uploadFiles)),
		DownloadCount: uint32(len(server.downloadFiles)),
		CopyTaskCount: uint32(len(server.copyTasks)),
		UploadFileStatusChanges: []*cd2pb.UploadFileInfo{
			server.uploadFiles["upload-1"],
		},
	})
}

func (server *fakeTransferServer) PushMessage(_ *emptypb.Empty, stream grpc.ServerStreamingServer[cd2pb.CloudDrivePushMessage]) error {
	return stream.Send(&cd2pb.CloudDrivePushMessage{
		MessageType: cd2pb.CloudDrivePushMessage_UPLOADER_COUNT,
		Data: &cd2pb.CloudDrivePushMessage_TransferTaskStatus{
			TransferTaskStatus: &cd2pb.TransferTaskStatus{
				UploadCount:   uint32(len(server.uploadFiles)),
				DownloadCount: uint32(len(server.downloadFiles)),
				CopyTaskCount: uint32(len(server.copyTasks)),
			},
		},
	})
}

func TestListBuildsTaskSnapshotsAndWatcherEvents(t *testing.T) {
	t.Parallel()

	service, cleanup := newTransferService(t)
	defer cleanup()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		result, err := service.List(context.Background())
		if err != nil {
			t.Fatalf("list transfers: %v", err)
		}
		if len(result.Tasks) != 3 {
			t.Fatalf("expected 3 tasks, got %+v", result.Tasks)
		}
		if result.Stats.UploadTasks != 1 || result.Stats.DownloadTasks != 1 || result.Stats.CopyTasks != 1 {
			t.Fatalf("unexpected stats: %+v", result.Stats)
		}
		if len(result.RecentEvents) > 0 && result.Watcher.EventCount > 0 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("expected watcher events to arrive")
}

func TestApplyActionControlsUploadAndCopyTasks(t *testing.T) {
	t.Parallel()

	service, cleanup := newTransferService(t)
	defer cleanup()

	summary, err := service.ApplyAction(context.Background(), ActionRequest{
		Kind:   KindUpload,
		Action: ActionPause,
		Keys:   []string{"upload-1"},
	})
	if err != nil {
		t.Fatalf("pause upload: %v", err)
	}
	if summary.Updated != 1 {
		t.Fatalf("unexpected pause summary: %+v", summary)
	}

	result, err := service.List(context.Background())
	if err != nil {
		t.Fatalf("list after pause: %v", err)
	}
	uploadTask := findTask(result.Tasks, KindUpload)
	if uploadTask == nil || uploadTask.Status != "paused" {
		t.Fatalf("expected paused upload task, got %+v", uploadTask)
	}

	if _, err := service.ApplyAction(context.Background(), ActionRequest{
		Kind:       KindCopy,
		Action:     ActionCancel,
		SourcePath: "/115open/source",
		DestPath:   "/115open/dest",
	}); err != nil {
		t.Fatalf("cancel copy: %v", err)
	}

	result, err = service.List(context.Background())
	if err != nil {
		t.Fatalf("list after cancel copy: %v", err)
	}
	copyTask := findTask(result.Tasks, KindCopy)
	if copyTask == nil || copyTask.Status != "failed" {
		t.Fatalf("expected failed copy task, got %+v", copyTask)
	}
}

func newTransferService(t *testing.T) (*Service, func()) {
	t.Helper()

	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	fake := &fakeTransferServer{
		uploadFiles: map[string]*cd2pb.UploadFileInfo{
			"upload-1": {
				Key:             "upload-1",
				DestPath:        "/115open/uploads/demo.mp4",
				Size:            100,
				TransferedBytes: 25,
				Status:          "Transfer",
				StatusEnum:      cd2pb.UploadFileInfo_Transfer,
				OperatorType:    cd2pb.UploadFileInfo_Copy,
			},
		},
		downloadFiles: []*cd2pb.DownloadFileInfo{
			{
				FilePath:            "/115open/downloads/demo.mp4",
				FileLength:          120,
				DownloadThreadCount: 4,
				BytesPerSecond:      128,
				Process:             []string{"33%"},
			},
		},
		copyTasks: map[string]*cd2pb.CopyTask{
			buildCopyTaskKey("/115open/source", "/115open/dest"): {
				SourcePath:    "/115open/source",
				DestPath:      "/115open/dest",
				Status:        cd2pb.CopyTask_Scanning,
				TotalFiles:    4,
				UploadedFiles: 1,
				TotalBytes:    200,
				UploadedBytes: 50,
				StartTime:     timestamppb.New(time.Now().Add(-time.Minute)),
			},
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

	service := NewService(manager)
	cleanup := func() {
		service.Close()
		_ = manager.Close()
		server.Stop()
		_ = listener.Close()
	}
	return service, cleanup
}

func findTask(tasks []TaskRecord, kind string) *TaskRecord {
	for _, item := range tasks {
		if item.Kind == kind {
			copied := item
			return &copied
		}
	}
	return nil
}
