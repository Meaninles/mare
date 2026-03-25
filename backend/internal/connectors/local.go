package connectors

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type LocalConfig struct {
	Name     string
	RootPath string
}

type LocalConnector struct {
	descriptor Descriptor
}

func NewLocalConnector(config LocalConfig) (*LocalConnector, error) {
	if strings.TrimSpace(config.RootPath) == "" {
		return nil, newConnectorError(EndpointTypeLocal, "configure", ErrorCodeInvalidConfig, "root path is required", false, nil)
	}

	rootPath := filepath.Clean(config.RootPath)
	name := config.Name
	if strings.TrimSpace(name) == "" {
		name = "Local"
	}

	return &LocalConnector{
		descriptor: Descriptor{
			Name:     name,
			Type:     EndpointTypeLocal,
			RootPath: rootPath,
			Capabilities: Capabilities{
				CanRead:          true,
				CanWrite:         true,
				CanDelete:        true,
				CanList:          true,
				CanStat:          true,
				CanReadStream:    true,
				CanRename:        true,
				CanMove:          true,
				CanMakeDirectory: true,
			},
		},
	}, nil
}

func (connector *LocalConnector) Descriptor() Descriptor {
	return connector.descriptor
}

func (connector *LocalConnector) HealthCheck(_ context.Context) (HealthStatus, error) {
	info, err := os.Stat(connector.descriptor.RootPath)
	if err != nil {
		return HealthStatusOffline, connector.wrapPathError("health_check", err)
	}

	if !info.IsDir() {
		return HealthStatusOffline, newConnectorError(EndpointTypeLocal, "health_check", ErrorCodeInvalidConfig, "root path is not a directory", false, nil)
	}

	return HealthStatusReady, nil
}

func (connector *LocalConnector) ListEntries(_ context.Context, request ListEntriesRequest) ([]FileEntry, error) {
	root, err := connector.resolvePath(request.Path)
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(root)
	if err != nil {
		return nil, connector.wrapPathError("list_entries", err)
	}
	if !info.IsDir() {
		return nil, newConnectorError(EndpointTypeLocal, "list_entries", ErrorCodeInvalidConfig, "requested path is not a directory", false, nil)
	}

	limit := request.Limit
	if limit <= 0 {
		limit = 0
	}

	entries := make([]FileEntry, 0)
	if request.Recursive {
		err = filepath.WalkDir(root, func(currentPath string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return connector.wrapPathError("list_entries", walkErr)
			}
			if currentPath == root {
				return nil
			}
			if connector.shouldIgnorePath(currentPath) {
				if entry.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}

			fileEntry, entryErr := connector.buildFileEntry(currentPath, entry)
			if entryErr != nil {
				return entryErr
			}

			if !connector.includeEntry(fileEntry, request) {
				return nil
			}

			entries = append(entries, fileEntry)
			if limit > 0 && len(entries) >= limit {
				return io.EOF
			}

			return nil
		})
		if errors.Is(err, io.EOF) {
			err = nil
		}
	} else {
		dirEntries, readErr := os.ReadDir(root)
		if readErr != nil {
			return nil, connector.wrapPathError("list_entries", readErr)
		}

		for _, entry := range dirEntries {
			currentPath := filepath.Join(root, entry.Name())
			if connector.shouldIgnorePath(currentPath) {
				continue
			}
			fileEntry, entryErr := connector.buildFileEntry(currentPath, entry)
			if entryErr != nil {
				return nil, entryErr
			}
			if !connector.includeEntry(fileEntry, request) {
				continue
			}

			entries = append(entries, fileEntry)
			if limit > 0 && len(entries) >= limit {
				break
			}
		}
	}

	if err != nil {
		return nil, err
	}

	return entries, nil
}

func (connector *LocalConnector) StatEntry(_ context.Context, path string) (FileEntry, error) {
	resolvedPath, err := connector.resolvePath(path)
	if err != nil {
		return FileEntry{}, err
	}

	entry, statErr := os.Stat(resolvedPath)
	if statErr != nil {
		return FileEntry{}, connector.wrapPathError("stat_entry", statErr)
	}

	return connector.fileInfoToEntry(resolvedPath, entry), nil
}

func (connector *LocalConnector) ReadStream(_ context.Context, path string) (io.ReadCloser, error) {
	resolvedPath, err := connector.resolvePath(path)
	if err != nil {
		return nil, err
	}

	reader, openErr := os.Open(resolvedPath)
	if openErr != nil {
		return nil, connector.wrapPathError("read_stream", openErr)
	}

	return reader, nil
}

func (connector *LocalConnector) CopyIn(_ context.Context, destinationPath string, source io.Reader) (FileEntry, error) {
	resolvedPath, err := connector.resolvePath(destinationPath)
	if err != nil {
		return FileEntry{}, err
	}

	if mkdirErr := os.MkdirAll(filepath.Dir(resolvedPath), 0o755); mkdirErr != nil {
		return FileEntry{}, connector.wrapPathError("copy_in", mkdirErr)
	}

	file, createErr := os.Create(resolvedPath)
	if createErr != nil {
		return FileEntry{}, connector.wrapPathError("copy_in", createErr)
	}
	defer file.Close()

	if _, copyErr := io.Copy(file, source); copyErr != nil {
		return FileEntry{}, connector.wrapPathError("copy_in", copyErr)
	}

	info, statErr := os.Stat(resolvedPath)
	if statErr != nil {
		return FileEntry{}, connector.wrapPathError("copy_in", statErr)
	}

	return connector.fileInfoToEntry(resolvedPath, info), nil
}

func (connector *LocalConnector) CopyOut(ctx context.Context, sourcePath string, destination io.Writer) error {
	reader, err := connector.ReadStream(ctx, sourcePath)
	if err != nil {
		return err
	}
	defer reader.Close()

	if _, copyErr := io.Copy(destination, reader); copyErr != nil {
		return connector.wrapPathError("copy_out", copyErr)
	}

	return nil
}

func (connector *LocalConnector) DeleteEntry(_ context.Context, path string) error {
	resolvedPath, err := connector.resolvePath(path)
	if err != nil {
		return err
	}

	if removeErr := os.RemoveAll(resolvedPath); removeErr != nil {
		return connector.wrapPathError("delete_entry", removeErr)
	}

	return nil
}

func (connector *LocalConnector) RenameEntry(_ context.Context, path string, newName string) (FileEntry, error) {
	if strings.TrimSpace(newName) == "" {
		return FileEntry{}, newConnectorError(EndpointTypeLocal, "rename_entry", ErrorCodeInvalidConfig, "new name is required", false, nil)
	}

	resolvedPath, err := connector.resolvePath(path)
	if err != nil {
		return FileEntry{}, err
	}

	targetPath := filepath.Join(filepath.Dir(resolvedPath), newName)
	if renameErr := os.Rename(resolvedPath, targetPath); renameErr != nil {
		return FileEntry{}, connector.wrapPathError("rename_entry", renameErr)
	}

	info, statErr := os.Stat(targetPath)
	if statErr != nil {
		return FileEntry{}, connector.wrapPathError("rename_entry", statErr)
	}

	return connector.fileInfoToEntry(targetPath, info), nil
}

func (connector *LocalConnector) MoveEntry(_ context.Context, sourcePath string, destinationPath string) (FileEntry, error) {
	sourceResolvedPath, err := connector.resolvePath(sourcePath)
	if err != nil {
		return FileEntry{}, err
	}

	destinationResolvedPath, err := connector.resolvePath(destinationPath)
	if err != nil {
		return FileEntry{}, err
	}

	if mkdirErr := os.MkdirAll(filepath.Dir(destinationResolvedPath), 0o755); mkdirErr != nil {
		return FileEntry{}, connector.wrapPathError("move_entry", mkdirErr)
	}

	if renameErr := os.Rename(sourceResolvedPath, destinationResolvedPath); renameErr != nil {
		return FileEntry{}, connector.wrapPathError("move_entry", renameErr)
	}

	info, statErr := os.Stat(destinationResolvedPath)
	if statErr != nil {
		return FileEntry{}, connector.wrapPathError("move_entry", statErr)
	}

	return connector.fileInfoToEntry(destinationResolvedPath, info), nil
}

func (connector *LocalConnector) MakeDirectory(_ context.Context, path string) (FileEntry, error) {
	resolvedPath, err := connector.resolvePath(path)
	if err != nil {
		return FileEntry{}, err
	}

	if mkdirErr := os.MkdirAll(resolvedPath, 0o755); mkdirErr != nil {
		return FileEntry{}, connector.wrapPathError("make_directory", mkdirErr)
	}

	info, statErr := os.Stat(resolvedPath)
	if statErr != nil {
		return FileEntry{}, connector.wrapPathError("make_directory", statErr)
	}

	return connector.fileInfoToEntry(resolvedPath, info), nil
}

func (connector *LocalConnector) resolvePath(path string) (string, error) {
	relativePath := strings.TrimSpace(path)
	if relativePath == "" || relativePath == "." {
		return connector.descriptor.RootPath, nil
	}

	rootPath := filepath.Clean(connector.descriptor.RootPath)
	var resolvedPath string
	if filepath.IsAbs(relativePath) {
		resolvedPath = filepath.Clean(relativePath)
	} else {
		resolvedPath = filepath.Join(rootPath, filepath.Clean(relativePath))
	}
	relativeToRoot, err := filepath.Rel(rootPath, resolvedPath)
	if err != nil {
		return "", newConnectorError(EndpointTypeLocal, "resolve_path", ErrorCodeInvalidConfig, "unable to resolve path", false, err)
	}
	if relativeToRoot == ".." || strings.HasPrefix(relativeToRoot, ".."+string(os.PathSeparator)) {
		return "", newConnectorError(EndpointTypeLocal, "resolve_path", ErrorCodeAccessDenied, "path escapes connector root", false, nil)
	}

	return resolvedPath, nil
}

func (connector *LocalConnector) buildFileEntry(path string, entry os.DirEntry) (FileEntry, error) {
	info, err := entry.Info()
	if err != nil {
		return FileEntry{}, connector.wrapPathError("list_entries", err)
	}

	return connector.fileInfoToEntry(path, info), nil
}

func (connector *LocalConnector) fileInfoToEntry(path string, info os.FileInfo) FileEntry {
	relativePath, _ := filepath.Rel(connector.descriptor.RootPath, path)
	modifiedAt := info.ModTime().UTC()
	isDir := info.IsDir()

	return FileEntry{
		Path:         path,
		RelativePath: filepath.ToSlash(relativePath),
		Name:         info.Name(),
		Kind:         kindForEntry(isDir),
		MediaType:    DetectMediaType(path, isDir),
		Size:         info.Size(),
		ModifiedAt:   &modifiedAt,
		IsDir:        isDir,
	}
}

func (connector *LocalConnector) includeEntry(entry FileEntry, request ListEntriesRequest) bool {
	if ShouldIgnoreAssetPath(entry.RelativePath) {
		return false
	}
	if entry.IsDir {
		return request.IncludeDirectories
	}
	if request.MediaOnly {
		return entry.MediaType != MediaTypeUnknown
	}
	return true
}

func (connector *LocalConnector) shouldIgnorePath(path string) bool {
	relativePath, err := filepath.Rel(connector.descriptor.RootPath, path)
	if err != nil {
		return ShouldIgnoreAssetPath(path)
	}
	return ShouldIgnoreAssetPath(filepath.ToSlash(relativePath))
}

func (connector *LocalConnector) wrapPathError(operation string, err error) error {
	if errors.Is(err, os.ErrNotExist) {
		return newConnectorError(EndpointTypeLocal, operation, ErrorCodeNotFound, "path does not exist", false, err)
	}
	if errors.Is(err, os.ErrPermission) {
		return newConnectorError(EndpointTypeLocal, operation, ErrorCodeAccessDenied, "access denied", false, err)
	}
	return newConnectorError(EndpointTypeLocal, operation, ErrorCodeUnavailable, fmt.Sprintf("local filesystem operation failed: %v", err), true, err)
}

func kindForEntry(isDir bool) EntryKind {
	if isDir {
		return EntryKindDirectory
	}
	return EntryKindFile
}
