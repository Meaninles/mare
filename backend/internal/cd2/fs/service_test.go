package fs

import (
	"bytes"
	"context"
	"net"
	"strconv"
	"strings"
	"sync"
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

type fakeFileServiceServer struct {
	cd2pb.UnimplementedCloudDriveFileSrvServer
	mu            sync.Mutex
	files         map[string]*cd2pb.CloudDriveFile
	nextHandle    uint64
	openUploads   map[uint64]*fakeOpenUpload
	nextUploadID  uint64
	remoteUploads map[string]*fakeRemoteUpload
}

type fakeOpenUpload struct {
	parentPath string
	fileName   string
	buffer     bytes.Buffer
}

type fakeRemoteUpload struct {
	filePath string
	fileSize uint64
	buffer   bytes.Buffer
	events   chan *cd2pb.RemoteUploadChannelReply
}

func (server *fakeFileServiceServer) GetSystemInfo(context.Context, *emptypb.Empty) (*cd2pb.CloudDriveSystemInfo, error) {
	return &cd2pb.CloudDriveSystemInfo{SystemReady: true}, nil
}

func (server *fakeFileServiceServer) GetApiTokenInfo(_ context.Context, request *cd2pb.StringValue) (*cd2pb.TokenInfo, error) {
	if request == nil || strings.TrimSpace(request.GetValue()) != "good-token" {
		return nil, status.Error(codes.PermissionDenied, "bad token")
	}
	return &cd2pb.TokenInfo{Token: "good-token", FriendlyName: "mam-backend"}, nil
}

func (server *fakeFileServiceServer) GetSubFiles(request *cd2pb.ListSubFileRequest, stream grpc.ServerStreamingServer[cd2pb.SubFilesReply]) error {
	path := normalizePath(request.GetPath())
	children := make([]*cd2pb.CloudDriveFile, 0)
	for _, item := range server.files {
		parent, _ := splitPath(item.GetFullPathName())
		if parent == path {
			children = append(children, item)
		}
	}
	return stream.Send(&cd2pb.SubFilesReply{SubFiles: children})
}

func (server *fakeFileServiceServer) GetSearchResults(request *cd2pb.SearchRequest, stream grpc.ServerStreamingServer[cd2pb.SubFilesReply]) error {
	path := normalizePath(request.GetPath())
	query := strings.ToLower(strings.TrimSpace(request.GetSearchFor()))
	children := make([]*cd2pb.CloudDriveFile, 0)
	for _, item := range server.files {
		if !strings.HasPrefix(item.GetFullPathName(), path) {
			continue
		}
		if query == "" || strings.Contains(strings.ToLower(item.GetName()), query) {
			children = append(children, item)
		}
	}
	return stream.Send(&cd2pb.SubFilesReply{SubFiles: children})
}

func (server *fakeFileServiceServer) FindFileByPath(_ context.Context, request *cd2pb.FindFileByPathRequest) (*cd2pb.CloudDriveFile, error) {
	fullPath := normalizePath(strings.TrimRight(request.GetParentPath(), "/") + "/" + request.GetPath())
	if request.GetParentPath() == "/" {
		fullPath = normalizePath("/" + request.GetPath())
	}
	item, ok := server.files[fullPath]
	if !ok {
		return nil, status.Error(codes.NotFound, "file not found")
	}
	return item, nil
}

func (server *fakeFileServiceServer) CreateFolder(_ context.Context, request *cd2pb.CreateFolderRequest) (*cd2pb.CreateFolderResult, error) {
	fullPath := normalizePath(request.GetParentPath() + "/" + request.GetFolderName())
	item := newProtoFile(fullPath, true)
	server.files[fullPath] = item
	return &cd2pb.CreateFolderResult{
		FolderCreated: item,
		Result: &cd2pb.FileOperationResult{
			Success:         true,
			ResultFilePaths: []string{fullPath},
		},
	}, nil
}

func (server *fakeFileServiceServer) RenameFile(_ context.Context, request *cd2pb.RenameFileRequest) (*cd2pb.FileOperationResult, error) {
	current := normalizePath(request.GetTheFilePath())
	item, ok := server.files[current]
	if !ok {
		return &cd2pb.FileOperationResult{Success: false, ErrorMessage: "file not found"}, nil
	}
	parent, _ := splitPath(current)
	next := normalizePath(parent + "/" + request.GetNewName())
	delete(server.files, current)
	item.Name = request.GetNewName()
	item.FullPathName = next
	server.files[next] = item
	return &cd2pb.FileOperationResult{Success: true, ResultFilePaths: []string{next}}, nil
}

func (server *fakeFileServiceServer) MoveFile(_ context.Context, request *cd2pb.MoveFileRequest) (*cd2pb.FileOperationResult, error) {
	resultPaths := make([]string, 0, len(request.GetTheFilePaths()))
	for _, path := range request.GetTheFilePaths() {
		current := normalizePath(path)
		item, ok := server.files[current]
		if !ok {
			continue
		}
		next := normalizePath(request.GetDestPath() + "/" + item.GetName())
		delete(server.files, current)
		item.FullPathName = next
		server.files[next] = item
		resultPaths = append(resultPaths, next)
	}
	return &cd2pb.FileOperationResult{Success: true, ResultFilePaths: resultPaths}, nil
}

func (server *fakeFileServiceServer) CopyFile(_ context.Context, request *cd2pb.CopyFileRequest) (*cd2pb.FileOperationResult, error) {
	resultPaths := make([]string, 0, len(request.GetTheFilePaths()))
	for _, path := range request.GetTheFilePaths() {
		current := normalizePath(path)
		item, ok := server.files[current]
		if !ok {
			continue
		}
		next := normalizePath(request.GetDestPath() + "/" + item.GetName())
		server.files[next] = newProtoFile(next, item.GetIsDirectory())
		resultPaths = append(resultPaths, next)
	}
	return &cd2pb.FileOperationResult{Success: true, ResultFilePaths: resultPaths}, nil
}

func (server *fakeFileServiceServer) DeleteFile(_ context.Context, request *cd2pb.FileRequest) (*cd2pb.FileOperationResult, error) {
	path := normalizePath(request.GetPath())
	delete(server.files, path)
	return &cd2pb.FileOperationResult{Success: true, ResultFilePaths: []string{path}}, nil
}

func (server *fakeFileServiceServer) DeleteFiles(_ context.Context, request *cd2pb.MultiFileRequest) (*cd2pb.FileOperationResult, error) {
	for _, path := range request.GetPath() {
		delete(server.files, normalizePath(path))
	}
	return &cd2pb.FileOperationResult{Success: true, ResultFilePaths: request.GetPath()}, nil
}

func (server *fakeFileServiceServer) DeleteFilesPermanently(_ context.Context, request *cd2pb.MultiFileRequest) (*cd2pb.FileOperationResult, error) {
	for _, path := range request.GetPath() {
		delete(server.files, normalizePath(path))
	}
	return &cd2pb.FileOperationResult{Success: true, ResultFilePaths: request.GetPath()}, nil
}

func (server *fakeFileServiceServer) GetFileDetailProperties(_ context.Context, request *cd2pb.FileRequest) (*cd2pb.FileDetailProperties, error) {
	path := normalizePath(request.GetPath())
	item, ok := server.files[path]
	if !ok {
		return nil, status.Error(codes.NotFound, "file not found")
	}
	if item.GetIsDirectory() {
		return &cd2pb.FileDetailProperties{
			TotalFileCount:   3,
			TotalFolderCount: 1,
			TotalSize:        4096,
			OriginalPath:     item.GetOriginalPath(),
		}, nil
	}
	return &cd2pb.FileDetailProperties{
		TotalFileCount: 0,
		TotalSize:      item.GetSize(),
		OriginalPath:   item.GetOriginalPath(),
	}, nil
}

func (server *fakeFileServiceServer) GetDownloadUrlPath(_ context.Context, request *cd2pb.GetDownloadUrlPathRequest) (*cd2pb.DownloadUrlPathInfo, error) {
	path := normalizePath(request.GetPath())
	if _, ok := server.files[path]; !ok {
		return nil, status.Error(codes.NotFound, "file not found")
	}
	expires := uint64(600)
	directURL := "https://example.test/download" + path
	return &cd2pb.DownloadUrlPathInfo{
		DownloadUrlPath: "/static/http/127.0.0.1/path" + path,
		ExpiresIn:       &expires,
		DirectUrl:       &directURL,
		AdditionalHeaders: map[string]string{
			"X-Test": "ok",
		},
	}, nil
}

func (server *fakeFileServiceServer) StartRemoteUpload(_ context.Context, request *cd2pb.StartRemoteUploadRequest) (*cd2pb.RemoteUploadStarted, error) {
	server.mu.Lock()
	defer server.mu.Unlock()

	server.nextUploadID++
	uploadID := "upload-" + strconv.FormatUint(server.nextUploadID, 10)
	if server.remoteUploads == nil {
		server.remoteUploads = map[string]*fakeRemoteUpload{}
	}

	upload := &fakeRemoteUpload{
		filePath: normalizePath(request.GetFilePath()),
		fileSize: request.GetFileSize(),
		events:   make(chan *cd2pb.RemoteUploadChannelReply, 4),
	}
	server.remoteUploads[uploadID] = upload

	upload.events <- &cd2pb.RemoteUploadChannelReply{
		UploadId: uploadID,
		Request: &cd2pb.RemoteUploadChannelReply_StatusChanged{
			StatusChanged: &cd2pb.RemoteUploadStatusChanged{Status: cd2pb.UploadFileInfo_Transfer},
		},
	}
	upload.events <- &cd2pb.RemoteUploadChannelReply{
		UploadId: uploadID,
		Request: &cd2pb.RemoteUploadChannelReply_ReadData{
			ReadData: &cd2pb.RemoteReadDataRequest{
				Offset: 0,
				Length: request.GetFileSize(),
			},
		},
	}

	return &cd2pb.RemoteUploadStarted{UploadId: uploadID}, nil
}

func (server *fakeFileServiceServer) RemoteUploadChannel(_ *cd2pb.RemoteUploadChannelRequest, stream grpc.ServerStreamingServer[cd2pb.RemoteUploadChannelReply]) error {
	for {
		server.mu.Lock()
		var upload *fakeRemoteUpload
		for _, candidate := range server.remoteUploads {
			upload = candidate
			break
		}
		server.mu.Unlock()

		if upload == nil {
			select {
			case <-stream.Context().Done():
				return stream.Context().Err()
			case <-time.After(10 * time.Millisecond):
			}
			continue
		}

		for {
			select {
			case <-stream.Context().Done():
				return stream.Context().Err()
			case event, ok := <-upload.events:
				if !ok {
					return nil
				}
				if err := stream.Send(event); err != nil {
					return err
				}
			}
		}
	}
}

func (server *fakeFileServiceServer) RemoteReadData(_ context.Context, request *cd2pb.RemoteReadDataUpload) (*cd2pb.RemoteReadDataReply, error) {
	server.mu.Lock()
	defer server.mu.Unlock()

	upload, ok := server.remoteUploads[request.GetUploadId()]
	if !ok {
		return nil, status.Error(codes.NotFound, "remote upload not found")
	}

	if len(request.GetData()) > 0 {
		if _, err := upload.buffer.Write(request.GetData()); err != nil {
			return nil, err
		}
	}

	if request.GetIsLastChunk() {
		server.files[upload.filePath] = newSizedProtoFile(upload.filePath, false, int64(upload.buffer.Len()))
		upload.events <- &cd2pb.RemoteUploadChannelReply{
			UploadId: request.GetUploadId(),
			Request: &cd2pb.RemoteUploadChannelReply_StatusChanged{
				StatusChanged: &cd2pb.RemoteUploadStatusChanged{Status: cd2pb.UploadFileInfo_Finish},
			},
		}
		close(upload.events)
		delete(server.remoteUploads, request.GetUploadId())
	}

	return &cd2pb.RemoteReadDataReply{
		Success:       true,
		BytesReceived: uint64(len(request.GetData())),
		IsLastChunk:   request.GetIsLastChunk(),
	}, nil
}

func (server *fakeFileServiceServer) RemoteHashProgress(context.Context, *cd2pb.RemoteHashProgressUpload) (*cd2pb.RemoteHashProgressReply, error) {
	return &cd2pb.RemoteHashProgressReply{}, nil
}

func (server *fakeFileServiceServer) RemoteUploadControl(_ context.Context, request *cd2pb.RemoteUploadControlRequest) (*emptypb.Empty, error) {
	server.mu.Lock()
	defer server.mu.Unlock()

	if upload, ok := server.remoteUploads[request.GetUploadId()]; ok {
		close(upload.events)
		delete(server.remoteUploads, request.GetUploadId())
	}
	return &emptypb.Empty{}, nil
}

func TestListSearchStatAndDetail(t *testing.T) {
	t.Parallel()

	service, cleanup := newFileService(t)
	defer cleanup()

	entries, path, err := service.List(context.Background(), "/115open", false)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if path != "/115open" || len(entries) != 2 {
		t.Fatalf("unexpected list result path=%q entries=%+v", path, entries)
	}

	searchEntries, searchPath, err := service.Search(context.Background(), SearchRequest{
		Path:  "/115open",
		Query: "demo",
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if searchPath != "/115open" || len(searchEntries) != 1 || searchEntries[0].Name != "demo.txt" {
		t.Fatalf("unexpected search result path=%q entries=%+v", searchPath, searchEntries)
	}

	entry, err := service.Stat(context.Background(), "/115open/demo.txt")
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if entry.Name != "demo.txt" || entry.CloudName != "115open" {
		t.Fatalf("unexpected stat result: %+v", entry)
	}

	detail, err := service.GetDetailProperties(context.Background(), "/115open/media", false)
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	if detail.TotalFileCount != 3 || detail.TotalFolderCount != 1 {
		t.Fatalf("unexpected detail: %+v", detail)
	}
}

func (server *fakeFileServiceServer) CreateFile(_ context.Context, request *cd2pb.CreateFileRequest) (*cd2pb.CreateFileResult, error) {
	server.nextHandle++
	if server.openUploads == nil {
		server.openUploads = map[uint64]*fakeOpenUpload{}
	}
	server.openUploads[server.nextHandle] = &fakeOpenUpload{
		parentPath: normalizePath(request.GetParentPath()),
		fileName:   request.GetFileName(),
	}
	return &cd2pb.CreateFileResult{FileHandle: server.nextHandle}, nil
}

func (server *fakeFileServiceServer) WriteToFile(_ context.Context, request *cd2pb.WriteFileRequest) (*cd2pb.WriteFileResult, error) {
	upload, ok := server.openUploads[request.GetFileHandle()]
	if !ok {
		return nil, status.Error(codes.NotFound, "file handle not found")
	}
	if _, err := upload.buffer.Write(request.GetBuffer()); err != nil {
		return nil, err
	}
	return &cd2pb.WriteFileResult{BytesWritten: uint64(len(request.GetBuffer()))}, nil
}

func (server *fakeFileServiceServer) WriteToFileStream(stream grpc.ClientStreamingServer[cd2pb.WriteFileRequest, cd2pb.WriteFileResult]) error {
	for {
		request, err := stream.Recv()
		if err != nil {
			return err
		}
		upload, ok := server.openUploads[request.GetFileHandle()]
		if !ok {
			return status.Error(codes.NotFound, "file handle not found")
		}
		if len(request.GetBuffer()) > 0 {
			if _, err := upload.buffer.Write(request.GetBuffer()); err != nil {
				return err
			}
		}
		if request.GetCloseFile() {
			fullPath := normalizePath(upload.parentPath + "/" + upload.fileName)
			server.files[fullPath] = newSizedProtoFile(fullPath, false, int64(upload.buffer.Len()))
			delete(server.openUploads, request.GetFileHandle())
			return stream.SendAndClose(&cd2pb.WriteFileResult{BytesWritten: uint64(upload.buffer.Len())})
		}
	}
}

func (server *fakeFileServiceServer) CloseFile(_ context.Context, request *cd2pb.CloseFileRequest) (*cd2pb.FileOperationResult, error) {
	upload, ok := server.openUploads[request.GetFileHandle()]
	if !ok {
		return &cd2pb.FileOperationResult{Success: false, ErrorMessage: "file handle not found"}, nil
	}
	fullPath := normalizePath(upload.parentPath + "/" + upload.fileName)
	server.files[fullPath] = newSizedProtoFile(fullPath, false, int64(upload.buffer.Len()))
	delete(server.openUploads, request.GetFileHandle())
	return &cd2pb.FileOperationResult{
		Success:         true,
		ResultFilePaths: []string{fullPath},
	}, nil
}

func TestCreateRenameMoveCopyDeleteAndDownload(t *testing.T) {
	t.Parallel()

	service, cleanup := newFileService(t)
	defer cleanup()

	created, operation, err := service.CreateFolder(context.Background(), CreateFolderRequest{
		ParentPath: "/115open",
		FolderName: "new-folder",
	})
	if err != nil {
		t.Fatalf("create folder: %v", err)
	}
	if !operation.Success || created.FullPathName != "/115open/new-folder" {
		t.Fatalf("unexpected create result: %+v %+v", created, operation)
	}

	renameResult, err := service.Rename(context.Background(), RenameRequest{
		Path:    "/115open/new-folder",
		NewName: "renamed-folder",
	})
	if err != nil {
		t.Fatalf("rename: %v", err)
	}
	if !renameResult.Success || len(renameResult.ResultFilePaths) != 1 || renameResult.ResultFilePaths[0] != "/115open/renamed-folder" {
		t.Fatalf("unexpected rename result: %+v", renameResult)
	}

	copyResult, err := service.Copy(context.Background(), CopyRequest{
		Paths:          []string{"/115open/demo.txt"},
		DestPath:       "/115open/renamed-folder",
		ConflictPolicy: "overwrite",
	})
	if err != nil {
		t.Fatalf("copy: %v", err)
	}
	if !copyResult.Success || len(copyResult.ResultFilePaths) != 1 {
		t.Fatalf("unexpected copy result: %+v", copyResult)
	}

	moveResult, err := service.Move(context.Background(), MoveRequest{
		Paths:          []string{"/115open/demo.txt"},
		DestPath:       "/115open/renamed-folder",
		ConflictPolicy: "rename",
	})
	if err != nil {
		t.Fatalf("move: %v", err)
	}
	if !moveResult.Success || len(moveResult.ResultFilePaths) != 1 {
		t.Fatalf("unexpected move result: %+v", moveResult)
	}

	downloadInfo, err := service.GetDownloadURL(context.Background(), DownloadURLRequest{
		Path:      "/115open/renamed-folder/demo.txt",
		GetDirect: true,
	})
	if err != nil {
		t.Fatalf("get download url: %v", err)
	}
	if downloadInfo.DirectURL == "" || downloadInfo.DownloadURLPath == "" {
		t.Fatalf("unexpected download info: %+v", downloadInfo)
	}

	deleteResult, err := service.Delete(context.Background(), DeleteRequest{
		Paths: []string{"/115open/renamed-folder/demo.txt"},
	})
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if !deleteResult.Success {
		t.Fatalf("unexpected delete result: %+v", deleteResult)
	}
}

func TestUpload(t *testing.T) {
	t.Parallel()

	service, cleanup := newFileService(t)
	defer cleanup()

	result, err := service.Upload(context.Background(), "/115open/media", "new-upload.txt", strings.NewReader("hello-cd2"))
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	if result.FullPathName != "/115open/media/new-upload.txt" {
		t.Fatalf("unexpected upload path: %+v", result)
	}
	if result.BytesWritten != uint64(len("hello-cd2")) {
		t.Fatalf("unexpected bytes written: %+v", result)
	}
	if result.Entry == nil || result.Entry.Name != "new-upload.txt" {
		t.Fatalf("expected uploaded entry, got %+v", result)
	}
}

func newFileService(t *testing.T) (*Service, func()) {
	t.Helper()

	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	fake := &fakeFileServiceServer{
		files: map[string]*cd2pb.CloudDriveFile{
			"/115open":          newProtoFile("/115open", true),
			"/115open/demo.txt": newProtoFile("/115open/demo.txt", false),
			"/115open/media":    newProtoFile("/115open/media", true),
		},
		openUploads:   map[uint64]*fakeOpenUpload{},
		remoteUploads: map[string]*fakeRemoteUpload{},
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

func newProtoFile(path string, isDirectory bool) *cd2pb.CloudDriveFile {
	return newSizedProtoFile(path, isDirectory, 2048)
}

func newSizedProtoFile(path string, isDirectory bool, size int64) *cd2pb.CloudDriveFile {
	normalized := normalizePath(path)
	_, name := splitPath(normalized)
	if normalized == "/" {
		name = "/"
	}
	return &cd2pb.CloudDriveFile{
		Id:           normalized,
		Name:         name,
		FullPathName: normalized,
		Size:         size,
		FileType: func() cd2pb.CloudDriveFile_FileType {
			if isDirectory {
				return cd2pb.CloudDriveFile_Directory
			}
			return cd2pb.CloudDriveFile_File
		}(),
		CreateTime:           timestamppb.New(time.Date(2026, 3, 29, 10, 0, 0, 0, time.UTC)),
		WriteTime:            timestamppb.New(time.Date(2026, 3, 29, 10, 30, 0, 0, time.UTC)),
		OriginalPath:         normalized,
		IsDirectory:          isDirectory,
		IsCloudDirectory:     isDirectory,
		IsCloudFile:          !isDirectory,
		CanSearch:            isDirectory,
		HasDetailProperties:  true,
		CanContentSearch:     true,
		CanDeletePermanently: true,
		CloudAPI: &cd2pb.CloudAPI{
			Name:     "115open",
			UserName: "102399770",
			NickName: "102399770",
			Path:     stringPointer("/115open"),
		},
	}
}

func stringPointer(value string) *string {
	copied := value
	return &copied
}
