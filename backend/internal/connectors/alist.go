package connectors

import (
	"context"
	"fmt"
	"io"
	"path"
	"strings"

	sidecaralist "mam/backend/internal/sidecars/alist"
)

type AListConfig struct {
	Name            string
	RootPath        string
	MountPath       string
	Driver          string
	Addition        string
	Password        string
	Remark          string
	CacheExpiration int
}

type AListConnector struct {
	config     AListConfig
	runtime    *sidecaralist.Runtime
	descriptor Descriptor
}

func NewAListConnector(config AListConfig, runtime *sidecaralist.Runtime) (*AListConnector, error) {
	rootPath := normalizeAListConnectorPath(defaultString(config.RootPath, config.MountPath))
	if rootPath == "" || rootPath == "/" {
		return nil, newConnectorError(EndpointTypeAList, "configure", ErrorCodeInvalidConfig, "alist root path is required", false, nil)
	}
	if runtime == nil {
		return nil, newConnectorError(EndpointTypeAList, "configure", ErrorCodeInvalidConfig, "alist runtime is required", false, nil)
	}

	return &AListConnector{
		config:  config,
		runtime: runtime,
		descriptor: Descriptor{
			Name:     defaultString(config.Name, "AList"),
			Type:     EndpointTypeAList,
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

func (connector *AListConnector) Descriptor() Descriptor {
	return connector.descriptor
}

func (connector *AListConnector) HealthCheck(ctx context.Context) (HealthStatus, error) {
	if _, err := connector.runtime.ListEntries(ctx, connector.descriptor.RootPath, connector.config.Password, false); err != nil {
		return HealthStatusOffline, remapConnectorType(err, EndpointTypeAList)
	}
	return HealthStatusReady, nil
}

func (connector *AListConnector) ListEntries(ctx context.Context, request ListEntriesRequest) ([]FileEntry, error) {
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

		entries, err := connector.runtime.ListEntries(ctx, currentPath, connector.config.Password, false)
		if err != nil {
			return nil, remapConnectorType(err, EndpointTypeAList)
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

func (connector *AListConnector) StatEntry(ctx context.Context, targetPath string) (FileEntry, error) {
	resolvedPath, err := connector.resolvePath(targetPath)
	if err != nil {
		return FileEntry{}, err
	}

	entry, err := connector.runtime.StatEntry(ctx, resolvedPath, connector.config.Password, true)
	if err != nil {
		return FileEntry{}, remapConnectorType(err, EndpointTypeAList)
	}
	return connector.toFileEntry(entry), nil
}

func (connector *AListConnector) ReadStream(ctx context.Context, targetPath string) (io.ReadCloser, error) {
	resolvedPath, err := connector.resolvePath(targetPath)
	if err != nil {
		return nil, err
	}

	reader, err := connector.runtime.OpenReadStream(ctx, resolvedPath, connector.config.Password)
	if err != nil {
		return nil, remapConnectorType(err, EndpointTypeAList)
	}
	return reader, nil
}

func (connector *AListConnector) CopyIn(ctx context.Context, destinationPath string, source io.Reader) (FileEntry, error) {
	resolvedPath, err := connector.resolvePath(destinationPath)
	if err != nil {
		return FileEntry{}, err
	}
	if err := connector.ensureDirectory(ctx, path.Dir(resolvedPath)); err != nil {
		return FileEntry{}, err
	}

	if err := connector.runtime.CopyIn(ctx, resolvedPath, connector.config.Password, true, source); err != nil {
		return FileEntry{}, remapConnectorType(err, EndpointTypeAList)
	}
	return connector.StatEntry(ctx, resolvedPath)
}

func (connector *AListConnector) CopyOut(ctx context.Context, sourcePath string, destination io.Writer) error {
	reader, err := connector.ReadStream(ctx, sourcePath)
	if err != nil {
		return err
	}
	defer reader.Close()

	if _, err := io.Copy(destination, reader); err != nil {
		return remapConnectorType(err, EndpointTypeAList)
	}
	return nil
}

func (connector *AListConnector) DeleteEntry(ctx context.Context, targetPath string) error {
	resolvedPath, err := connector.resolvePath(targetPath)
	if err != nil {
		return err
	}
	parentDir, name := path.Split(resolvedPath)
	if err := connector.runtime.RemoveEntry(ctx, parentDir, []string{name}); err != nil {
		return remapConnectorType(err, EndpointTypeAList)
	}
	return nil
}

func (connector *AListConnector) RenameEntry(ctx context.Context, targetPath string, newName string) (FileEntry, error) {
	resolvedPath, err := connector.resolvePath(targetPath)
	if err != nil {
		return FileEntry{}, err
	}

	if err := connector.runtime.RenameEntry(ctx, resolvedPath, strings.TrimSpace(newName), true); err != nil {
		return FileEntry{}, remapConnectorType(err, EndpointTypeAList)
	}
	return connector.StatEntry(ctx, path.Join(path.Dir(resolvedPath), strings.TrimSpace(newName)))
}

func (connector *AListConnector) MoveEntry(ctx context.Context, sourcePath string, destinationPath string) (FileEntry, error) {
	sourceResolvedPath, err := connector.resolvePath(sourcePath)
	if err != nil {
		return FileEntry{}, err
	}
	destinationResolvedPath, err := connector.resolvePath(destinationPath)
	if err != nil {
		return FileEntry{}, err
	}

	sourceDir, sourceName := path.Split(sourceResolvedPath)
	destinationDir, destinationName := path.Split(destinationResolvedPath)
	if err := connector.ensureDirectory(ctx, destinationDir); err != nil {
		return FileEntry{}, err
	}
	if err := connector.runtime.MoveEntry(ctx, sourceDir, destinationDir, []string{sourceName}, true); err != nil {
		return FileEntry{}, remapConnectorType(err, EndpointTypeAList)
	}
	if destinationName != sourceName {
		if err := connector.runtime.RenameEntry(ctx, path.Join(destinationDir, sourceName), destinationName, true); err != nil {
			return FileEntry{}, remapConnectorType(err, EndpointTypeAList)
		}
	}
	return connector.StatEntry(ctx, destinationResolvedPath)
}

func (connector *AListConnector) MakeDirectory(ctx context.Context, targetPath string) (FileEntry, error) {
	resolvedPath, err := connector.resolvePath(targetPath)
	if err != nil {
		return FileEntry{}, err
	}

	if err := connector.runtime.MakeDirectory(ctx, resolvedPath); err != nil {
		return FileEntry{}, remapConnectorType(err, EndpointTypeAList)
	}
	return connector.StatEntry(ctx, resolvedPath)
}

func (connector *AListConnector) resolvePath(value string) (string, error) {
	rootPath := normalizeAListConnectorPath(connector.descriptor.RootPath)
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || trimmed == "." || trimmed == "/" {
		return rootPath, nil
	}

	if strings.HasPrefix(strings.ReplaceAll(trimmed, `\`, "/"), "/") {
		candidate := normalizeAListConnectorPath(trimmed)
		if candidate == rootPath || strings.HasPrefix(candidate, rootPath+"/") {
			return candidate, nil
		}
		return "", newConnectorError(EndpointTypeAList, "resolve_path", ErrorCodeAccessDenied, "path escapes connector root", false, nil)
	}

	candidate := normalizeAListConnectorPath(path.Join(rootPath, trimmed))
	if candidate != rootPath && !strings.HasPrefix(candidate, rootPath+"/") {
		return "", newConnectorError(EndpointTypeAList, "resolve_path", ErrorCodeAccessDenied, "path escapes connector root", false, nil)
	}
	return candidate, nil
}

func (connector *AListConnector) relativePath(fullPath string) string {
	rootPath := normalizeAListConnectorPath(connector.descriptor.RootPath)
	fullPath = normalizeAListConnectorPath(fullPath)
	if fullPath == rootPath {
		return ""
	}
	return strings.TrimPrefix(strings.TrimPrefix(fullPath, rootPath), "/")
}

func (connector *AListConnector) toFileEntry(entry sidecaralist.Entry) FileEntry {
	fullPath := normalizeAListConnectorPath(entry.Path)
	relativePath := connector.relativePath(fullPath)
	isDir := entry.IsDir

	return FileEntry{
		Path:         fullPath,
		RelativePath: relativePath,
		Name:         defaultString(strings.TrimSpace(entry.Name), path.Base(fullPath)),
		Kind:         kindForEntry(isDir),
		MediaType:    DetectMediaType(fullPath, isDir),
		Size:         entry.Size,
		ModifiedAt:   entry.ModifiedAt,
		IsDir:        isDir,
	}
}

func (connector *AListConnector) includeEntry(entry FileEntry, request ListEntriesRequest) bool {
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

func (connector *AListConnector) ensureDirectory(ctx context.Context, targetPath string) error {
	normalized := normalizeAListConnectorPath(targetPath)
	if normalized == "" || normalized == "/" || normalized == normalizeAListConnectorPath(connector.descriptor.RootPath) {
		return nil
	}
	if err := connector.runtime.MakeDirectory(ctx, normalized); err != nil {
		return remapConnectorType(err, EndpointTypeAList)
	}
	return nil
}

func normalizeAListConnectorPath(value string) string {
	normalized := strings.TrimSpace(strings.ReplaceAll(value, `\`, "/"))
	if normalized == "" {
		return ""
	}
	if !strings.HasPrefix(normalized, "/") {
		normalized = "/" + normalized
	}
	cleaned := path.Clean(normalized)
	if cleaned == "." {
		return ""
	}
	return cleaned
}

func (connector *AListConnector) String() string {
	return fmt.Sprintf("AList(%s)", connector.descriptor.RootPath)
}
