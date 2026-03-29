package fs

import (
	"context"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"path"
	"strings"
	"time"

	cd2client "mam/backend/internal/cd2/client"
	cd2pb "mam/backend/internal/cd2/pb"
)

type FileEntry struct {
	ID                   string                `json:"id"`
	Name                 string                `json:"name"`
	FullPathName         string                `json:"fullPathName"`
	Size                 int64                 `json:"size"`
	FileType             string                `json:"fileType"`
	CreateTime           string                `json:"createTime,omitempty"`
	WriteTime            string                `json:"writeTime,omitempty"`
	AccessTime           string                `json:"accessTime,omitempty"`
	CloudName            string                `json:"cloudName,omitempty"`
	CloudUserName        string                `json:"cloudUserName,omitempty"`
	CloudNickName        string                `json:"cloudNickName,omitempty"`
	ThumbnailURL         string                `json:"thumbnailUrl,omitempty"`
	PreviewURL           string                `json:"previewUrl,omitempty"`
	OriginalPath         string                `json:"originalPath,omitempty"`
	IsDirectory          bool                  `json:"isDirectory"`
	IsRoot               bool                  `json:"isRoot"`
	IsCloudRoot          bool                  `json:"isCloudRoot"`
	IsCloudDirectory     bool                  `json:"isCloudDirectory"`
	IsCloudFile          bool                  `json:"isCloudFile"`
	IsSearchResult       bool                  `json:"isSearchResult"`
	IsForbidden          bool                  `json:"isForbidden"`
	IsLocal              bool                  `json:"isLocal"`
	CanSearch            bool                  `json:"canSearch"`
	HasDetailProperties  bool                  `json:"hasDetailProperties"`
	CanContentSearch     bool                  `json:"canContentSearch"`
	CanDeletePermanently bool                  `json:"canDeletePermanently"`
	DetailProperties     *FileDetailProperties `json:"detailProperties,omitempty"`
	DownloadURLPath      *DownloadURLInfo      `json:"downloadUrlPath,omitempty"`
}

type FileDetailProperties struct {
	TotalFileCount   int64  `json:"totalFileCount"`
	TotalFolderCount int64  `json:"totalFolderCount"`
	TotalSize        int64  `json:"totalSize"`
	IsFaved          bool   `json:"isFaved"`
	IsShared         bool   `json:"isShared"`
	OriginalPath     string `json:"originalPath,omitempty"`
}

type DownloadURLInfo struct {
	DownloadURLPath  string            `json:"downloadUrlPath"`
	ExpiresIn        *uint64           `json:"expiresIn,omitempty"`
	DirectURL        string            `json:"directUrl,omitempty"`
	UserAgent        string            `json:"userAgent,omitempty"`
	AdditionalHeader map[string]string `json:"additionalHeaders,omitempty"`
}

type SearchRequest struct {
	Path          string `json:"path"`
	Query         string `json:"query"`
	ForceRefresh  bool   `json:"forceRefresh"`
	FuzzyMatch    bool   `json:"fuzzyMatch"`
	ContentSearch bool   `json:"contentSearch"`
}

type CreateFolderRequest struct {
	ParentPath string `json:"parentPath"`
	FolderName string `json:"folderName"`
}

type RenameRequest struct {
	Path    string `json:"path"`
	NewName string `json:"newName"`
}

type MoveRequest struct {
	Paths                     []string `json:"paths"`
	DestPath                  string   `json:"destPath"`
	ConflictPolicy            string   `json:"conflictPolicy"`
	MoveAcrossClouds          bool     `json:"moveAcrossClouds"`
	HandleConflictRecursively bool     `json:"handleConflictRecursively"`
}

type CopyRequest struct {
	Paths                     []string `json:"paths"`
	DestPath                  string   `json:"destPath"`
	ConflictPolicy            string   `json:"conflictPolicy"`
	HandleConflictRecursively bool     `json:"handleConflictRecursively"`
}

type DeleteRequest struct {
	Paths     []string `json:"paths"`
	Permanent bool     `json:"permanent"`
}

type DownloadURLRequest struct {
	Path      string `json:"path"`
	Preview   bool   `json:"preview"`
	LazyRead  bool   `json:"lazyRead"`
	GetDirect bool   `json:"getDirect"`
}

type FileOperationResult struct {
	Success         bool     `json:"success"`
	ErrorMessage    string   `json:"errorMessage,omitempty"`
	ResultFilePaths []string `json:"resultFilePaths,omitempty"`
}

type UploadResult struct {
	FileName     string     `json:"fileName"`
	ParentPath   string     `json:"parentPath"`
	FullPathName string     `json:"fullPathName"`
	BytesWritten uint64     `json:"bytesWritten"`
	Entry        *FileEntry `json:"entry,omitempty"`
}

type Service struct {
	client *cd2client.Manager
}

type uploadSource struct {
	readerAt io.ReaderAt
	size     uint64
	close    func() error
}

type remoteUploadRecvResult struct {
	reply *cd2pb.RemoteUploadChannelReply
	err   error
}

func (source *uploadSource) Close() error {
	if source == nil || source.close == nil {
		return nil
	}
	return source.close()
}

func NewService(client *cd2client.Manager) *Service {
	return &Service{client: client}
}

func (service *Service) List(ctx context.Context, path string, forceRefresh bool) ([]FileEntry, string, error) {
	client, _, err := service.authorizedClient(ctx)
	if err != nil {
		return nil, "", err
	}

	normalizedPath := normalizePath(path)
	requestCtx, cancel := service.client.WithRequestTimeout(ctx)
	defer cancel()

	stream, err := client.GetSubFiles(requestCtx, &cd2pb.ListSubFileRequest{
		Path:         normalizedPath,
		ForceRefresh: forceRefresh,
	})
	if err != nil {
		return nil, normalizedPath, err
	}

	entries, err := collectSubFiles(stream)
	if err != nil {
		return nil, normalizedPath, err
	}
	return entries, normalizedPath, nil
}

func (service *Service) Search(ctx context.Context, request SearchRequest) ([]FileEntry, string, error) {
	client, _, err := service.authorizedClient(ctx)
	if err != nil {
		return nil, "", err
	}

	query := strings.TrimSpace(request.Query)
	if query == "" {
		return nil, "", errors.New("搜索关键词不能为空")
	}
	normalizedPath := normalizePath(request.Path)

	requestCtx, cancel := service.client.WithRequestTimeout(ctx)
	defer cancel()

	stream, err := client.GetSearchResults(requestCtx, &cd2pb.SearchRequest{
		Path:          normalizedPath,
		SearchFor:     query,
		ForceRefresh:  request.ForceRefresh,
		FuzzyMatch:    request.FuzzyMatch,
		ContentSearch: optionalBool(request.ContentSearch),
	})
	if err != nil {
		return nil, normalizedPath, err
	}

	entries, err := collectSubFiles(stream)
	if err != nil {
		return nil, normalizedPath, err
	}
	return entries, normalizedPath, nil
}

func (service *Service) Stat(ctx context.Context, path string) (FileEntry, error) {
	client, _, err := service.authorizedClient(ctx)
	if err != nil {
		return FileEntry{}, err
	}

	normalizedPath := normalizePath(path)
	if normalizedPath == "/" {
		return FileEntry{
			Name:                "/",
			FullPathName:        "/",
			FileType:            cd2pb.CloudDriveFile_Directory.String(),
			IsDirectory:         true,
			IsRoot:              true,
			CanSearch:           true,
			HasDetailProperties: false,
		}, nil
	}

	parentPath, filePath := splitPath(normalizedPath)
	requestCtx, cancel := service.client.WithRequestTimeout(ctx)
	defer cancel()

	result, err := client.FindFileByPath(requestCtx, &cd2pb.FindFileByPathRequest{
		ParentPath: parentPath,
		Path:       filePath,
	})
	if err != nil {
		return FileEntry{}, err
	}
	return fileEntryFromProto(result), nil
}

func (service *Service) GetDetailProperties(ctx context.Context, path string, forceRefresh bool) (FileDetailProperties, error) {
	client, _, err := service.authorizedClient(ctx)
	if err != nil {
		return FileDetailProperties{}, err
	}

	requestCtx, cancel := service.client.WithRequestTimeout(ctx)
	defer cancel()

	result, err := client.GetFileDetailProperties(requestCtx, &cd2pb.FileRequest{
		Path:         normalizePath(path),
		ForceRefresh: optionalBool(forceRefresh),
	})
	if err != nil {
		return FileDetailProperties{}, err
	}
	return fileDetailFromProto(result), nil
}

func (service *Service) CreateFolder(ctx context.Context, request CreateFolderRequest) (FileEntry, FileOperationResult, error) {
	client, _, err := service.authorizedClient(ctx)
	if err != nil {
		return FileEntry{}, FileOperationResult{}, err
	}

	parentPath := normalizePath(request.ParentPath)
	folderName := strings.TrimSpace(request.FolderName)
	if folderName == "" {
		return FileEntry{}, FileOperationResult{}, errors.New("目录名称不能为空")
	}

	requestCtx, cancel := service.client.WithRequestTimeout(ctx)
	defer cancel()

	result, err := client.CreateFolder(requestCtx, &cd2pb.CreateFolderRequest{
		ParentPath: parentPath,
		FolderName: folderName,
	})
	if err != nil {
		return FileEntry{}, FileOperationResult{}, err
	}
	if result == nil {
		return FileEntry{}, FileOperationResult{}, errors.New("CD2 没有返回建目录结果")
	}

	operation := operationResultFromProto(result.GetResult())
	if !operation.Success {
		return FileEntry{}, operation, errors.New(defaultString(operation.ErrorMessage, "CD2 创建目录失败"))
	}

	return fileEntryFromProto(result.GetFolderCreated()), operation, nil
}

func (service *Service) Rename(ctx context.Context, request RenameRequest) (FileOperationResult, error) {
	client, _, err := service.authorizedClient(ctx)
	if err != nil {
		return FileOperationResult{}, err
	}

	path := normalizePath(request.Path)
	newName := strings.TrimSpace(request.NewName)
	if path == "/" {
		return FileOperationResult{}, errors.New("根目录不支持重命名")
	}
	if newName == "" {
		return FileOperationResult{}, errors.New("新名称不能为空")
	}

	requestCtx, cancel := service.client.WithRequestTimeout(ctx)
	defer cancel()

	result, err := client.RenameFile(requestCtx, &cd2pb.RenameFileRequest{
		TheFilePath: path,
		NewName:     newName,
	})
	if err != nil {
		return FileOperationResult{}, err
	}
	operation := operationResultFromProto(result)
	if !operation.Success {
		return operation, errors.New(defaultString(operation.ErrorMessage, "CD2 重命名失败"))
	}
	return operation, nil
}

func (service *Service) Move(ctx context.Context, request MoveRequest) (FileOperationResult, error) {
	client, _, err := service.authorizedClient(ctx)
	if err != nil {
		return FileOperationResult{}, err
	}

	paths := normalizePathList(request.Paths)
	if len(paths) == 0 {
		return FileOperationResult{}, errors.New("至少要选择一个文件或目录")
	}
	destPath := normalizePath(request.DestPath)

	conflictPolicy, err := parseMoveConflictPolicy(request.ConflictPolicy)
	if err != nil {
		return FileOperationResult{}, err
	}

	requestCtx, cancel := service.client.WithRequestTimeout(ctx)
	defer cancel()

	result, err := client.MoveFile(requestCtx, &cd2pb.MoveFileRequest{
		TheFilePaths:              paths,
		DestPath:                  destPath,
		ConflictPolicy:            conflictPolicy.Enum(),
		MoveAcrossClouds:          optionalBool(request.MoveAcrossClouds),
		HandleConflictRecursively: optionalBool(request.HandleConflictRecursively),
	})
	if err != nil {
		return FileOperationResult{}, err
	}

	operation := operationResultFromProto(result)
	if !operation.Success {
		return operation, errors.New(defaultString(operation.ErrorMessage, "CD2 移动失败"))
	}
	return operation, nil
}

func (service *Service) Copy(ctx context.Context, request CopyRequest) (FileOperationResult, error) {
	client, _, err := service.authorizedClient(ctx)
	if err != nil {
		return FileOperationResult{}, err
	}

	paths := normalizePathList(request.Paths)
	if len(paths) == 0 {
		return FileOperationResult{}, errors.New("至少要选择一个文件或目录")
	}
	destPath := normalizePath(request.DestPath)

	conflictPolicy, err := parseCopyConflictPolicy(request.ConflictPolicy)
	if err != nil {
		return FileOperationResult{}, err
	}

	requestCtx, cancel := service.client.WithRequestTimeout(ctx)
	defer cancel()

	result, err := client.CopyFile(requestCtx, &cd2pb.CopyFileRequest{
		TheFilePaths:              paths,
		DestPath:                  destPath,
		ConflictPolicy:            conflictPolicy.Enum(),
		HandleConflictRecursively: optionalBool(request.HandleConflictRecursively),
	})
	if err != nil {
		return FileOperationResult{}, err
	}

	operation := operationResultFromProto(result)
	if !operation.Success {
		return operation, errors.New(defaultString(operation.ErrorMessage, "CD2 复制失败"))
	}
	return operation, nil
}

func (service *Service) Delete(ctx context.Context, request DeleteRequest) (FileOperationResult, error) {
	client, _, err := service.authorizedClient(ctx)
	if err != nil {
		return FileOperationResult{}, err
	}

	paths := normalizePathList(request.Paths)
	if len(paths) == 0 {
		return FileOperationResult{}, errors.New("至少要选择一个文件或目录")
	}

	requestCtx, cancel := service.client.WithRequestTimeout(ctx)
	defer cancel()

	var result *cd2pb.FileOperationResult
	if request.Permanent || len(paths) > 1 {
		requestPayload := &cd2pb.MultiFileRequest{Path: paths}
		if request.Permanent {
			result, err = client.DeleteFilesPermanently(requestCtx, requestPayload)
		} else {
			result, err = client.DeleteFiles(requestCtx, requestPayload)
		}
	} else {
		result, err = client.DeleteFile(requestCtx, &cd2pb.FileRequest{Path: paths[0]})
	}
	if err != nil {
		return FileOperationResult{}, err
	}

	operation := operationResultFromProto(result)
	if !operation.Success {
		return operation, errors.New(defaultString(operation.ErrorMessage, "CD2 删除失败"))
	}
	return operation, nil
}

func (service *Service) GetDownloadURL(ctx context.Context, request DownloadURLRequest) (DownloadURLInfo, error) {
	client, _, err := service.authorizedClient(ctx)
	if err != nil {
		return DownloadURLInfo{}, err
	}

	path := normalizePath(request.Path)
	requestCtx, cancel := service.client.WithRequestTimeout(ctx)
	defer cancel()

	result, err := client.GetDownloadUrlPath(requestCtx, &cd2pb.GetDownloadUrlPathRequest{
		Path:         path,
		Preview:      request.Preview,
		LazyRead:     request.LazyRead,
		GetDirectUrl: request.GetDirect,
	})
	if err != nil {
		return DownloadURLInfo{}, err
	}
	return downloadURLFromProto(result), nil
}

func (service *Service) legacyUpload(ctx context.Context, parentPath string, fileName string, reader io.Reader) (UploadResult, error) {
	client, _, err := service.authorizedClient(ctx)
	if err != nil {
		return UploadResult{}, err
	}

	normalizedParent := normalizePath(parentPath)
	normalizedFileName := strings.TrimSpace(path.Base(strings.ReplaceAll(fileName, "\\", "/")))
	if normalizedFileName == "" || normalizedFileName == "." || normalizedFileName == "/" {
		return UploadResult{}, errors.New("文件名不能为空")
	}
	if reader == nil {
		return UploadResult{}, errors.New("上传流不能为空")
	}

	createCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	createResult, err := client.CreateFile(createCtx, &cd2pb.CreateFileRequest{
		ParentPath: normalizedParent,
		FileName:   normalizedFileName,
	})
	if err != nil {
		return UploadResult{}, err
	}
	if createResult == nil || createResult.GetFileHandle() == 0 {
		return UploadResult{}, errors.New("CD2 没有返回可用的上传句柄")
	}

	fileHandle := createResult.GetFileHandle()
	closed := false
	defer func() {
		if closed {
			return
		}
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		_, _ = client.CloseFile(cleanupCtx, &cd2pb.CloseFileRequest{FileHandle: fileHandle})
	}()

	buffer := make([]byte, 4*1024*1024)
	var startPos uint64
	var bytesWritten uint64
	streamCtx, streamCancel := context.WithTimeout(ctx, 30*time.Minute)
	defer streamCancel()

	stream, err := client.WriteToFileStream(streamCtx)
	if err != nil {
		return UploadResult{}, err
	}

	for {
		n, readErr := reader.Read(buffer)
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, buffer[:n])
			if err := stream.Send(&cd2pb.WriteFileRequest{
				FileHandle: fileHandle,
				StartPos:   startPos,
				Length:     uint64(n),
				Buffer:     chunk,
				CloseFile:  false,
			}); err != nil {
				return UploadResult{}, err
			}
			bytesWritten += uint64(n)
			startPos += uint64(n)
		}

		if errors.Is(readErr, io.EOF) {
			if err := stream.Send(&cd2pb.WriteFileRequest{
				FileHandle: fileHandle,
				StartPos:   startPos,
				Length:     0,
				CloseFile:  true,
			}); err != nil {
				return UploadResult{}, err
			}
			if _, err := stream.CloseAndRecv(); err != nil {
				return UploadResult{}, err
			}
			closed = true
			break
		}
		if readErr != nil {
			return UploadResult{}, readErr
		}
	}

	fullPath := normalizePath(normalizedParent + "/" + normalizedFileName)
	uploaded := UploadResult{
		FileName:     normalizedFileName,
		ParentPath:   normalizedParent,
		FullPathName: fullPath,
		BytesWritten: bytesWritten,
	}
	entry, statErr := service.Stat(ctx, fullPath)
	if statErr == nil {
		uploaded.Entry = &entry
	}
	return uploaded, nil
}

func (service *Service) Upload(ctx context.Context, parentPath string, fileName string, reader io.Reader) (UploadResult, error) {
	client, _, err := service.authorizedClient(ctx)
	if err != nil {
		return UploadResult{}, err
	}

	normalizedParent := normalizePath(parentPath)
	normalizedFileName := strings.TrimSpace(path.Base(strings.ReplaceAll(fileName, "\\", "/")))
	if normalizedFileName == "" || normalizedFileName == "." || normalizedFileName == "/" {
		return UploadResult{}, errors.New("upload file name is required")
	}
	if reader == nil {
		return UploadResult{}, errors.New("upload source is required")
	}

	source, err := prepareUploadSource(reader)
	if err != nil {
		return UploadResult{}, err
	}
	defer func() { _ = source.Close() }()

	fullPath := normalizePath(normalizedParent + "/" + normalizedFileName)
	bytesWritten, err := service.remoteUpload(ctx, client, fullPath, source)
	if err != nil {
		return UploadResult{}, err
	}

	uploaded := UploadResult{
		FileName:     normalizedFileName,
		ParentPath:   normalizedParent,
		FullPathName: fullPath,
		BytesWritten: bytesWritten,
	}
	entry, statErr := service.Stat(ctx, fullPath)
	if statErr == nil {
		uploaded.Entry = &entry
	}
	return uploaded, nil
}

func (service *Service) remoteUpload(ctx context.Context, client cd2pb.CloudDriveFileSrvClient, filePath string, source *uploadSource) (uint64, error) {
	if client == nil {
		return 0, errors.New("cd2 upload client is not initialized")
	}
	if source == nil || source.readerAt == nil {
		return 0, errors.New("upload source is not initialized")
	}

	channelCtx, channelCancel := context.WithTimeout(ctx, 30*time.Minute)
	defer channelCancel()

	channel, err := client.RemoteUploadChannel(channelCtx, &cd2pb.RemoteUploadChannelRequest{
		DeviceId: remoteUploadDeviceID(),
	})
	if err != nil {
		return 0, err
	}

	startCtx, startCancel := context.WithTimeout(ctx, 2*time.Minute)
	defer startCancel()

	started, err := client.StartRemoteUpload(startCtx, &cd2pb.StartRemoteUploadRequest{
		FilePath:                 filePath,
		FileSize:                 source.size,
		ClientCanCalculateHashes: false,
	})
	if err != nil {
		return 0, err
	}

	uploadID := strings.TrimSpace(started.GetUploadId())
	if uploadID == "" {
		return 0, errors.New("cd2 did not return a remote upload id")
	}

	replyCh := make(chan remoteUploadRecvResult, 1)
	go func() {
		for {
			reply, recvErr := channel.Recv()
			replyCh <- remoteUploadRecvResult{reply: reply, err: recvErr}
			if recvErr != nil {
				close(replyCh)
				return
			}
		}
	}()

	var bytesWritten uint64
	awaitFileCommit := false
	commitDeadline := time.Time{}
	for {
		var timeout <-chan time.Time
		if awaitFileCommit {
			timeout = time.After(500 * time.Millisecond)
		}

		select {
		case result, ok := <-replyCh:
			if !ok {
				if awaitFileCommit {
					committed, err := service.remoteUploadFileCommitted(ctx, client, filePath, source.size)
					if err == nil && committed {
						return source.size, nil
					}
				}
				return 0, errors.New("cd2 remote upload channel closed before upload finished")
			}
			if result.err != nil {
				if errors.Is(result.err, io.EOF) && awaitFileCommit {
					committed, err := service.remoteUploadFileCommitted(ctx, client, filePath, source.size)
					if err == nil && committed {
						return source.size, nil
					}
				}
				if errors.Is(result.err, io.EOF) {
					return 0, errors.New("cd2 remote upload channel closed before upload finished")
				}
				return 0, result.err
			}

			reply := result.reply
			if reply == nil || strings.TrimSpace(reply.GetUploadId()) != uploadID {
				continue
			}

			if readRequest := reply.GetReadData(); readRequest != nil {
				sent, reachedFileEnd, err := service.remoteUploadRead(ctx, client, uploadID, source, readRequest)
				if err != nil {
					_ = cancelRemoteUpload(context.Background(), client, uploadID)
					return 0, err
				}
				bytesWritten += sent
				if reachedFileEnd {
					awaitFileCommit = true
					if commitDeadline.IsZero() {
						commitDeadline = time.Now().Add(20 * time.Second)
					}
				}
				continue
			}

			if hashRequest := reply.GetHashData(); hashRequest != nil {
				if err := service.remoteUploadHash(ctx, client, uploadID, source, hashRequest); err != nil {
					_ = cancelRemoteUpload(context.Background(), client, uploadID)
					return 0, err
				}
				continue
			}

			if statusChange := reply.GetStatusChanged(); statusChange != nil {
				switch statusChange.GetStatus() {
				case cd2pb.UploadFileInfo_Finish, cd2pb.UploadFileInfo_Skipped:
					return source.size, nil
				case cd2pb.UploadFileInfo_Cancelled, cd2pb.UploadFileInfo_Error, cd2pb.UploadFileInfo_FatalError:
					return 0, errors.New(defaultString(strings.TrimSpace(statusChange.GetErrorMessage()), "cd2 remote upload failed"))
				}
			}
		case <-timeout:
			committed, err := service.remoteUploadFileCommitted(ctx, client, filePath, source.size)
			if err == nil && committed {
				return source.size, nil
			}
			if !commitDeadline.IsZero() && time.Now().After(commitDeadline) {
				return 0, errors.New("timed out waiting for cd2 to finalize uploaded file")
			}
		}
	}
}

func (service *Service) remoteUploadRead(ctx context.Context, client cd2pb.CloudDriveFileSrvClient, uploadID string, source *uploadSource, request *cd2pb.RemoteReadDataRequest) (uint64, bool, error) {
	if request == nil {
		return 0, false, nil
	}

	offset := request.GetOffset()
	if offset > source.size {
		return 0, false, fmt.Errorf("cd2 requested invalid upload offset %d beyond file size %d", offset, source.size)
	}

	requestedLength := request.GetLength()
	if requestedLength == 0 || offset+requestedLength > source.size {
		requestedLength = source.size - offset
	}

	if requestedLength == 0 {
		requestCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()

		reply, err := client.RemoteReadData(requestCtx, &cd2pb.RemoteReadDataUpload{
			UploadId:    uploadID,
			Offset:      offset,
			Length:      0,
			LazyRead:    request.GetLazyRead(),
			IsLastChunk: true,
		})
		if err != nil {
			return 0, false, err
		}
		if !reply.GetSuccess() {
			return 0, false, errors.New(defaultString(strings.TrimSpace(reply.GetErrorMessage()), "cd2 rejected zero-length upload chunk"))
		}
		return 0, true, nil
	}

	const chunkSize = 4 * 1024 * 1024
	buffer := make([]byte, chunkSize)
	var sent uint64

	for sent < requestedLength {
		nextSize := minUint64(chunkSize, requestedLength-sent)
		n, readErr := source.readerAt.ReadAt(buffer[:nextSize], int64(offset+sent))
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return sent, false, readErr
		}
		if n <= 0 {
			return sent, false, io.ErrUnexpectedEOF
		}

		payload := make([]byte, n)
		copy(payload, buffer[:n])

		requestCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		reply, err := client.RemoteReadData(requestCtx, &cd2pb.RemoteReadDataUpload{
			UploadId:    uploadID,
			Offset:      offset + sent,
			Length:      uint64(n),
			LazyRead:    request.GetLazyRead(),
			Data:        payload,
			IsLastChunk: sent+uint64(n) >= requestedLength,
		})
		cancel()
		if err != nil {
			return sent, false, err
		}
		if !reply.GetSuccess() {
			return sent, false, errors.New(defaultString(strings.TrimSpace(reply.GetErrorMessage()), "cd2 rejected uploaded data"))
		}

		sent += uint64(n)
		if errors.Is(readErr, io.EOF) {
			break
		}
	}

	return sent, offset+requestedLength >= source.size, nil
}

func (service *Service) remoteUploadFileCommitted(ctx context.Context, client cd2pb.CloudDriveFileSrvClient, filePath string, expectedSize uint64) (bool, error) {
	if client == nil {
		return false, errors.New("cd2 upload client is not initialized")
	}

	parentPath, fileName := splitPath(filePath)
	requestCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	file, err := client.FindFileByPath(requestCtx, &cd2pb.FindFileByPathRequest{
		ParentPath: parentPath,
		Path:       fileName,
	})
	if err != nil {
		return false, err
	}
	if file == nil {
		return false, nil
	}
	if expectedSize == 0 {
		return true, nil
	}
	return uint64(file.GetSize()) == expectedSize, nil
}

func (service *Service) remoteUploadHash(ctx context.Context, client cd2pb.CloudDriveFileSrvClient, uploadID string, source *uploadSource, request *cd2pb.RemoteHashDataRequest) error {
	if request == nil {
		return nil
	}

	hashType := cd2pb.CloudDriveFile_HashType(request.GetHashType())
	hashValue, blockHashes, err := calculateRemoteHash(source, hashType, request.GetBlockSize())
	if err != nil {
		return err
	}

	requestCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	payload := &cd2pb.RemoteHashProgressUpload{
		UploadId:    uploadID,
		BytesHashed: source.size,
		TotalBytes:  source.size,
		HashType:    hashType,
		BlockHashes: blockHashes,
	}
	if hashValue != "" {
		payload.HashValue = &hashValue
	}

	_, err = client.RemoteHashProgress(requestCtx, payload)
	return err
}

func calculateRemoteHash(source *uploadSource, hashType cd2pb.CloudDriveFile_HashType, blockSize uint32) (string, []string, error) {
	if source == nil || source.readerAt == nil {
		return "", nil, errors.New("upload source is not initialized")
	}

	switch hashType {
	case cd2pb.CloudDriveFile_Md5:
		return hashFromSource(source, md5.New, blockSize)
	case cd2pb.CloudDriveFile_Sha1:
		return hashFromSource(source, sha1.New, 0)
	default:
		return "", nil, fmt.Errorf("unsupported remote hash type: %s", hashType.String())
	}
}

func hashFromSource(source *uploadSource, builder func() hash.Hash, blockSize uint32) (string, []string, error) {
	reader := io.NewSectionReader(source.readerAt, 0, int64(source.size))
	digest := builder()
	if _, err := io.Copy(digest, reader); err != nil {
		return "", nil, err
	}

	var blockHashes []string
	if blockSize > 0 {
		blockHashes = make([]string, 0, source.size/uint64(blockSize)+1)
		buffer := make([]byte, blockSize)
		for offset := uint64(0); offset < source.size; offset += uint64(blockSize) {
			nextSize := minUint64(uint64(blockSize), source.size-offset)
			n, err := source.readerAt.ReadAt(buffer[:nextSize], int64(offset))
			if err != nil && !errors.Is(err, io.EOF) {
				return "", nil, err
			}
			blockDigest := builder()
			if _, err := blockDigest.Write(buffer[:n]); err != nil {
				return "", nil, err
			}
			blockHashes = append(blockHashes, hex.EncodeToString(blockDigest.Sum(nil)))
		}
	}

	return hex.EncodeToString(digest.Sum(nil)), blockHashes, nil
}

func prepareUploadSource(reader io.Reader) (*uploadSource, error) {
	if existing, ok := reader.(interface {
		io.ReaderAt
		io.Seeker
	}); ok {
		size, err := existing.Seek(0, io.SeekEnd)
		if err != nil {
			return nil, err
		}
		if _, err := existing.Seek(0, io.SeekStart); err != nil {
			return nil, err
		}
		return &uploadSource{
			readerAt: existing,
			size:     uint64(size),
		}, nil
	}

	tempFile, err := os.CreateTemp("", "mam-cd2-upload-*")
	if err != nil {
		return nil, err
	}

	written, copyErr := io.Copy(tempFile, reader)
	if copyErr != nil {
		name := tempFile.Name()
		_ = tempFile.Close()
		_ = os.Remove(name)
		return nil, copyErr
	}
	if _, err := tempFile.Seek(0, io.SeekStart); err != nil {
		name := tempFile.Name()
		_ = tempFile.Close()
		_ = os.Remove(name)
		return nil, err
	}

	name := tempFile.Name()
	return &uploadSource{
		readerAt: tempFile,
		size:     uint64(written),
		close: func() error {
			closeErr := tempFile.Close()
			removeErr := os.Remove(name)
			if closeErr != nil {
				return closeErr
			}
			return removeErr
		},
	}, nil
}

func remoteUploadDeviceID() string {
	buffer := make([]byte, 12)
	if _, err := rand.Read(buffer); err != nil {
		return fmt.Sprintf("mam-backend-%d", time.Now().UnixNano())
	}
	return "mam-backend-" + hex.EncodeToString(buffer)
}

func cancelRemoteUpload(ctx context.Context, client cd2pb.CloudDriveFileSrvClient, uploadID string) error {
	if client == nil || strings.TrimSpace(uploadID) == "" {
		return nil
	}

	cancelCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	_, err := client.RemoteUploadControl(cancelCtx, &cd2pb.RemoteUploadControlRequest{
		UploadId: uploadID,
		Control:  &cd2pb.RemoteUploadControlRequest_Cancel{Cancel: &cd2pb.CancelRemoteUpload{}},
	})
	return err
}

func minUint64(left, right uint64) uint64 {
	if left < right {
		return left
	}
	return right
}

func (service *Service) authorizedClient(ctx context.Context) (cd2pb.CloudDriveFileSrvClient, cd2client.State, error) {
	if service == nil || service.client == nil {
		return nil, cd2client.State{}, errors.New("CD2 文件服务未初始化")
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

func collectSubFiles(stream interface {
	Recv() (*cd2pb.SubFilesReply, error)
}) ([]FileEntry, error) {
	entries := make([]FileEntry, 0)
	for {
		reply, err := stream.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return entries, nil
			}
			return nil, err
		}
		for _, item := range reply.GetSubFiles() {
			entries = append(entries, fileEntryFromProto(item))
		}
	}
}

func fileEntryFromProto(value *cd2pb.CloudDriveFile) FileEntry {
	if value == nil {
		return FileEntry{}
	}

	entry := FileEntry{
		ID:                   strings.TrimSpace(value.GetId()),
		Name:                 strings.TrimSpace(value.GetName()),
		FullPathName:         strings.TrimSpace(value.GetFullPathName()),
		Size:                 value.GetSize(),
		FileType:             value.GetFileType().String(),
		CreateTime:           formatTimestamp(value.GetCreateTime()),
		WriteTime:            formatTimestamp(value.GetWriteTime()),
		AccessTime:           formatTimestamp(value.GetAccessTime()),
		ThumbnailURL:         strings.TrimSpace(value.GetThumbnailUrl()),
		PreviewURL:           strings.TrimSpace(value.GetPreviewUrl()),
		OriginalPath:         strings.TrimSpace(value.GetOriginalPath()),
		IsDirectory:          value.GetIsDirectory(),
		IsRoot:               value.GetIsRoot(),
		IsCloudRoot:          value.GetIsCloudRoot(),
		IsCloudDirectory:     value.GetIsCloudDirectory(),
		IsCloudFile:          value.GetIsCloudFile(),
		IsSearchResult:       value.GetIsSearchResult(),
		IsForbidden:          value.GetIsForbidden(),
		IsLocal:              value.GetIsLocal(),
		CanSearch:            value.GetCanSearch(),
		HasDetailProperties:  value.GetHasDetailProperties(),
		CanContentSearch:     value.GetCanContentSearch(),
		CanDeletePermanently: value.GetCanDeletePermanently(),
	}

	if cloud := value.GetCloudAPI(); cloud != nil {
		entry.CloudName = strings.TrimSpace(cloud.GetName())
		entry.CloudUserName = strings.TrimSpace(cloud.GetUserName())
		entry.CloudNickName = strings.TrimSpace(cloud.GetNickName())
	}
	if detail := value.GetDetailProperties(); detail != nil {
		converted := fileDetailFromProto(detail)
		entry.DetailProperties = &converted
	}
	if info := value.GetDownloadUrlPath(); info != nil {
		converted := downloadURLFromProto(info)
		entry.DownloadURLPath = &converted
	}
	if entry.Name == "" && entry.FullPathName == "/" {
		entry.Name = "/"
	}
	return entry
}

func fileDetailFromProto(value *cd2pb.FileDetailProperties) FileDetailProperties {
	if value == nil {
		return FileDetailProperties{}
	}
	return FileDetailProperties{
		TotalFileCount:   value.GetTotalFileCount(),
		TotalFolderCount: value.GetTotalFolderCount(),
		TotalSize:        value.GetTotalSize(),
		IsFaved:          value.GetIsFaved(),
		IsShared:         value.GetIsShared(),
		OriginalPath:     strings.TrimSpace(value.GetOriginalPath()),
	}
}

func downloadURLFromProto(value *cd2pb.DownloadUrlPathInfo) DownloadURLInfo {
	if value == nil {
		return DownloadURLInfo{}
	}
	info := DownloadURLInfo{
		DownloadURLPath:  strings.TrimSpace(value.GetDownloadUrlPath()),
		AdditionalHeader: map[string]string{},
	}
	if value.ExpiresIn != nil {
		expiresIn := value.GetExpiresIn()
		info.ExpiresIn = &expiresIn
	}
	if value.DirectUrl != nil {
		info.DirectURL = strings.TrimSpace(value.GetDirectUrl())
	}
	if value.UserAgent != nil {
		info.UserAgent = strings.TrimSpace(value.GetUserAgent())
	}
	for key, item := range value.GetAdditionalHeaders() {
		info.AdditionalHeader[key] = item
	}
	if len(info.AdditionalHeader) == 0 {
		info.AdditionalHeader = nil
	}
	return info
}

func operationResultFromProto(value *cd2pb.FileOperationResult) FileOperationResult {
	if value == nil {
		return FileOperationResult{}
	}
	return FileOperationResult{
		Success:         value.GetSuccess(),
		ErrorMessage:    strings.TrimSpace(value.GetErrorMessage()),
		ResultFilePaths: append([]string(nil), value.GetResultFilePaths()...),
	}
}

func normalizePath(value string) string {
	trimmed := strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	if trimmed == "" {
		return "/"
	}
	if !strings.HasPrefix(trimmed, "/") {
		trimmed = "/" + trimmed
	}
	for strings.Contains(trimmed, "//") {
		trimmed = strings.ReplaceAll(trimmed, "//", "/")
	}
	if len(trimmed) > 1 {
		trimmed = strings.TrimRight(trimmed, "/")
		if trimmed == "" {
			return "/"
		}
	}
	return trimmed
}

func normalizePathList(values []string) []string {
	paths := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, item := range values {
		path := normalizePath(item)
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		paths = append(paths, path)
	}
	return paths
}

func splitPath(path string) (parentPath string, filePath string) {
	normalized := normalizePath(path)
	if normalized == "/" {
		return "/", ""
	}
	index := strings.LastIndex(normalized, "/")
	if index <= 0 {
		return "/", strings.TrimPrefix(normalized, "/")
	}
	parent := normalized[:index]
	if parent == "" {
		parent = "/"
	}
	return parent, normalized[index+1:]
}

func parseMoveConflictPolicy(value string) (cd2pb.MoveFileRequest_ConflictPolicy, error) {
	switch strings.ToLower(strings.TrimSpace(defaultString(value, "overwrite"))) {
	case "overwrite":
		return cd2pb.MoveFileRequest_Overwrite, nil
	case "rename":
		return cd2pb.MoveFileRequest_Rename, nil
	case "skip":
		return cd2pb.MoveFileRequest_Skip, nil
	default:
		return cd2pb.MoveFileRequest_Overwrite, errors.New("不支持的移动冲突策略，仅支持 overwrite / rename / skip")
	}
}

func parseCopyConflictPolicy(value string) (cd2pb.CopyFileRequest_ConflictPolicy, error) {
	switch strings.ToLower(strings.TrimSpace(defaultString(value, "overwrite"))) {
	case "overwrite":
		return cd2pb.CopyFileRequest_Overwrite, nil
	case "rename":
		return cd2pb.CopyFileRequest_Rename, nil
	case "skip":
		return cd2pb.CopyFileRequest_Skip, nil
	default:
		return cd2pb.CopyFileRequest_Overwrite, errors.New("不支持的复制冲突策略，仅支持 overwrite / rename / skip")
	}
}

func formatTimestamp(value interface{ AsTime() time.Time }) string {
	if value == nil {
		return ""
	}
	return value.AsTime().UTC().Format(time.RFC3339Nano)
}

func optionalBool(value bool) *bool {
	copied := value
	return &copied
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return strings.TrimSpace(fallback)
}
