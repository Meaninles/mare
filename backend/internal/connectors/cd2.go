package connectors

import (
	"context"
	"io"
	"path"
	"strings"
	"time"

	cd2fs "mam/backend/internal/cd2/fs"
)

type CD2Config struct {
	Name     string
	RootPath string
	Service  *cd2fs.Service
}

type CD2Connector struct {
	service    *cd2fs.Service
	descriptor Descriptor
}

func NewCD2Connector(config CD2Config) (*CD2Connector, error) {
	rootPath := normalizeCD2Path(config.RootPath)
	if rootPath == "" || rootPath == "/" {
		return nil, newConnectorError(EndpointTypeCD2, "configure", ErrorCodeInvalidConfig, "cd2 root path is required", false, nil)
	}
	if config.Service == nil {
		return nil, newConnectorError(EndpointTypeCD2, "configure", ErrorCodeInvalidConfig, "cd2 service is required", false, nil)
	}

	return &CD2Connector{
		service: config.Service,
		descriptor: Descriptor{
			Name:     defaultString(strings.TrimSpace(config.Name), "CD2 云盘目录"),
			Type:     EndpointTypeCD2,
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

func (connector *CD2Connector) Descriptor() Descriptor {
	return connector.descriptor
}

func (connector *CD2Connector) HealthCheck(ctx context.Context) (HealthStatus, error) {
	if _, err := connector.service.Stat(ctx, connector.descriptor.RootPath); err != nil {
		return HealthStatusOffline, err
	}
	return HealthStatusReady, nil
}

func (connector *CD2Connector) ListEntries(ctx context.Context, request ListEntriesRequest) ([]FileEntry, error) {
	rootPath, err := connector.resolvePath(request.Path)
	if err != nil {
		return nil, err
	}

	queue := []string{rootPath}
	result := make([]FileEntry, 0)
	limit := request.Limit

	for len(queue) > 0 {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		currentPath := queue[0]
		queue = queue[1:]

		entries, _, err := connector.service.List(ctx, currentPath, false)
		if err != nil {
			return nil, err
		}

		for _, entry := range entries {
			fileEntry := connector.toFileEntry(entry)
			if fileEntry.IsDir && request.Recursive {
				queue = append(queue, fileEntry.Path)
			}
			if !connector.includeEntry(fileEntry, request) {
				continue
			}
			result = append(result, fileEntry)
			if limit > 0 && len(result) >= limit {
				return result, nil
			}
		}
	}

	return result, nil
}

func (connector *CD2Connector) StatEntry(ctx context.Context, targetPath string) (FileEntry, error) {
	resolvedPath, err := connector.resolvePath(targetPath)
	if err != nil {
		return FileEntry{}, err
	}

	entry, err := connector.service.Stat(ctx, resolvedPath)
	if err != nil {
		return FileEntry{}, err
	}
	return connector.toFileEntry(entry), nil
}

func (connector *CD2Connector) ReadStream(ctx context.Context, targetPath string) (io.ReadCloser, error) {
	resolvedPath, err := connector.resolvePath(targetPath)
	if err != nil {
		return nil, err
	}
	return connector.service.OpenReadStream(ctx, resolvedPath)
}

func (connector *CD2Connector) CopyIn(ctx context.Context, destinationPath string, source io.Reader) (FileEntry, error) {
	resolvedPath, err := connector.resolvePath(destinationPath)
	if err != nil {
		return FileEntry{}, err
	}

	parentPath := normalizeCD2Path(path.Dir(resolvedPath))
	fileName := strings.TrimSpace(path.Base(resolvedPath))
	if fileName == "" || fileName == "." || fileName == "/" {
		return FileEntry{}, newConnectorError(EndpointTypeCD2, "copy_in", ErrorCodeInvalidConfig, "destination file name is required", false, nil)
	}

	result, err := connector.service.Upload(ctx, parentPath, fileName, source)
	if err != nil {
		return FileEntry{}, err
	}
	if result.Entry != nil {
		return connector.toFileEntry(*result.Entry), nil
	}
	return connector.StatEntry(ctx, resolvedPath)
}

func (connector *CD2Connector) CopyOut(ctx context.Context, sourcePath string, destination io.Writer) error {
	reader, err := connector.ReadStream(ctx, sourcePath)
	if err != nil {
		return err
	}
	defer reader.Close()

	_, err = io.Copy(destination, reader)
	return err
}

func (connector *CD2Connector) DeleteEntry(ctx context.Context, targetPath string) error {
	resolvedPath, err := connector.resolvePath(targetPath)
	if err != nil {
		return err
	}

	_, err = connector.service.Delete(ctx, cd2fs.DeleteRequest{
		Paths: []string{resolvedPath},
	})
	return err
}

func (connector *CD2Connector) RenameEntry(ctx context.Context, targetPath string, newName string) (FileEntry, error) {
	resolvedPath, err := connector.resolvePath(targetPath)
	if err != nil {
		return FileEntry{}, err
	}

	if _, err := connector.service.Rename(ctx, cd2fs.RenameRequest{
		Path:    resolvedPath,
		NewName: strings.TrimSpace(newName),
	}); err != nil {
		return FileEntry{}, err
	}

	return connector.StatEntry(ctx, path.Join(path.Dir(resolvedPath), strings.TrimSpace(newName)))
}

func (connector *CD2Connector) MoveEntry(ctx context.Context, sourcePath string, destinationPath string) (FileEntry, error) {
	sourceResolvedPath, err := connector.resolvePath(sourcePath)
	if err != nil {
		return FileEntry{}, err
	}
	destinationResolvedPath, err := connector.resolvePath(destinationPath)
	if err != nil {
		return FileEntry{}, err
	}

	sourceName := path.Base(sourceResolvedPath)
	destinationName := path.Base(destinationResolvedPath)
	destinationDir := normalizeCD2Path(path.Dir(destinationResolvedPath))

	if _, err := connector.service.Move(ctx, cd2fs.MoveRequest{
		Paths:    []string{sourceResolvedPath},
		DestPath: destinationDir,
	}); err != nil {
		return FileEntry{}, err
	}

	finalPath := normalizeCD2Path(path.Join(destinationDir, sourceName))
	if destinationName != sourceName {
		if _, err := connector.service.Rename(ctx, cd2fs.RenameRequest{
			Path:    finalPath,
			NewName: destinationName,
		}); err != nil {
			return FileEntry{}, err
		}
		finalPath = normalizeCD2Path(path.Join(destinationDir, destinationName))
	}

	return connector.StatEntry(ctx, finalPath)
}

func (connector *CD2Connector) MakeDirectory(ctx context.Context, targetPath string) (FileEntry, error) {
	resolvedPath, err := connector.resolvePath(targetPath)
	if err != nil {
		return FileEntry{}, err
	}

	parentPath := normalizeCD2Path(path.Dir(resolvedPath))
	folderName := strings.TrimSpace(path.Base(resolvedPath))
	entry, _, err := connector.service.CreateFolder(ctx, cd2fs.CreateFolderRequest{
		ParentPath: parentPath,
		FolderName: folderName,
	})
	if err != nil {
		return FileEntry{}, err
	}
	return connector.toFileEntry(entry), nil
}

func (connector *CD2Connector) resolvePath(value string) (string, error) {
	rootPath := normalizeCD2Path(connector.descriptor.RootPath)
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || trimmed == "." || trimmed == "/" {
		return rootPath, nil
	}

	normalized := strings.ReplaceAll(trimmed, `\`, "/")
	var candidate string
	if strings.HasPrefix(normalized, "/") {
		candidate = normalizeCD2Path(normalized)
	} else {
		candidate = normalizeCD2Path(path.Join(rootPath, normalized))
	}

	if candidate != rootPath && !strings.HasPrefix(candidate, rootPath+"/") {
		return "", newConnectorError(EndpointTypeCD2, "resolve_path", ErrorCodeAccessDenied, "path escapes cd2 connector root", false, nil)
	}
	return candidate, nil
}

func (connector *CD2Connector) includeEntry(entry FileEntry, request ListEntriesRequest) bool {
	if request.MediaOnly && entry.IsDir {
		return false
	}
	if !request.IncludeDirectories && entry.IsDir {
		return false
	}
	if request.MediaOnly && entry.MediaType == MediaTypeUnknown {
		return false
	}
	return true
}

func (connector *CD2Connector) toFileEntry(entry cd2fs.FileEntry) FileEntry {
	resolved := normalizeCD2Path(entry.FullPathName)
	return FileEntry{
		Path:         resolved,
		RelativePath: connector.relativePath(resolved),
		Name:         entry.Name,
		Kind:         resolveCD2EntryKind(entry),
		MediaType:    DetectMediaType(entry.Name, entry.IsDirectory),
		Size:         entry.Size,
		ModifiedAt:   parseCD2Time(entry.WriteTime),
		IsDir:        entry.IsDirectory,
	}
}

func (connector *CD2Connector) relativePath(fullPath string) string {
	rootPath := normalizeCD2Path(connector.descriptor.RootPath)
	fullPath = normalizeCD2Path(fullPath)
	if fullPath == rootPath {
		return ""
	}
	return strings.TrimPrefix(strings.TrimPrefix(fullPath, rootPath), "/")
}

func resolveCD2EntryKind(entry cd2fs.FileEntry) EntryKind {
	if entry.IsDirectory {
		return EntryKindDirectory
	}
	return EntryKindFile
}

func normalizeCD2Path(value string) string {
	normalized := strings.TrimSpace(strings.ReplaceAll(value, `\`, "/"))
	if normalized == "" {
		return "/"
	}
	cleaned := path.Clean(normalized)
	if !strings.HasPrefix(cleaned, "/") {
		cleaned = "/" + cleaned
	}
	if cleaned == "." {
		return "/"
	}
	return cleaned
}

func parseCD2Time(value string) *time.Time {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
	}
	for _, layout := range layouts {
		if parsed, err := time.Parse(layout, trimmed); err == nil {
			utc := parsed.UTC()
			return &utc
		}
	}
	return nil
}

var _ Connector = (*CD2Connector)(nil)
