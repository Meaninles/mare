package connectors

import (
	"context"
	"io"
	"strings"
)

type Cloud115Config struct {
	Name        string
	RootID      string
	AccessToken string
	AppType     string
}

type Cloud115Client interface {
	HealthCheck(ctx context.Context, rootID string) error
	ListEntries(ctx context.Context, rootID string, request ListEntriesRequest) ([]FileEntry, error)
	StatEntry(ctx context.Context, rootID string, path string) (FileEntry, error)
	CopyIn(ctx context.Context, rootID string, destinationPath string, source io.Reader) (FileEntry, error)
	CopyOut(ctx context.Context, rootID string, sourcePath string, destination io.Writer) error
	DeleteEntry(ctx context.Context, rootID string, path string) error
	RenameEntry(ctx context.Context, rootID string, path string, newName string) (FileEntry, error)
	MoveEntry(ctx context.Context, rootID string, sourcePath string, destinationPath string) (FileEntry, error)
	MakeDirectory(ctx context.Context, rootID string, path string) (FileEntry, error)
}

type Cloud115Connector struct {
	config     Cloud115Config
	client     Cloud115Client
	descriptor Descriptor
}

func NewCloud115Connector(config Cloud115Config, client Cloud115Client) (*Cloud115Connector, error) {
	if strings.TrimSpace(config.RootID) == "" {
		return nil, newConnectorError(EndpointTypeCloud115, "configure", ErrorCodeInvalidConfig, "root id is required", false, nil)
	}

	if client == nil {
		client = NewCloud115PythonClient(config.AccessToken, config.AppType)
	}

	return &Cloud115Connector{
		config: config,
		client: client,
		descriptor: Descriptor{
			Name:     defaultString(config.Name, "115 Cloud"),
			Type:     EndpointTypeCloud115,
			RootPath: config.RootID,
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

func (connector *Cloud115Connector) Descriptor() Descriptor {
	return connector.descriptor
}

func (connector *Cloud115Connector) HealthCheck(ctx context.Context) (HealthStatus, error) {
	if strings.TrimSpace(connector.config.AccessToken) == "" {
		return HealthStatusOffline, newConnectorError(EndpointTypeCloud115, "health_check", ErrorCodeAuthentication, "115 cookies or access token is required", false, nil)
	}
	if err := connector.client.HealthCheck(ctx, connector.config.RootID); err != nil {
		return HealthStatusOffline, remapConnectorType(err, EndpointTypeCloud115)
	}
	return HealthStatusReady, nil
}

func (connector *Cloud115Connector) ListEntries(ctx context.Context, request ListEntriesRequest) ([]FileEntry, error) {
	if strings.TrimSpace(connector.config.AccessToken) == "" {
		return nil, newConnectorError(EndpointTypeCloud115, "list_entries", ErrorCodeAuthentication, "115 cookies or access token is required", false, nil)
	}
	entries, err := connector.client.ListEntries(ctx, connector.config.RootID, request)
	return entries, remapConnectorType(err, EndpointTypeCloud115)
}

func (connector *Cloud115Connector) StatEntry(ctx context.Context, path string) (FileEntry, error) {
	if strings.TrimSpace(connector.config.AccessToken) == "" {
		return FileEntry{}, newConnectorError(EndpointTypeCloud115, "stat_entry", ErrorCodeAuthentication, "115 cookies or access token is required", false, nil)
	}
	entry, err := connector.client.StatEntry(ctx, connector.config.RootID, path)
	return entry, remapConnectorType(err, EndpointTypeCloud115)
}

func (connector *Cloud115Connector) ReadStream(ctx context.Context, path string) (io.ReadCloser, error) {
	return openReadStreamFromCopyOut(ctx, connector, path)
}

func (connector *Cloud115Connector) CopyIn(ctx context.Context, destinationPath string, source io.Reader) (FileEntry, error) {
	if strings.TrimSpace(connector.config.AccessToken) == "" {
		return FileEntry{}, newConnectorError(EndpointTypeCloud115, "copy_in", ErrorCodeAuthentication, "115 cookies or access token is required", false, nil)
	}
	entry, err := connector.client.CopyIn(ctx, connector.config.RootID, destinationPath, source)
	return entry, remapConnectorType(err, EndpointTypeCloud115)
}

func (connector *Cloud115Connector) CopyOut(ctx context.Context, sourcePath string, destination io.Writer) error {
	if strings.TrimSpace(connector.config.AccessToken) == "" {
		return newConnectorError(EndpointTypeCloud115, "copy_out", ErrorCodeAuthentication, "115 cookies or access token is required", false, nil)
	}
	return remapConnectorType(connector.client.CopyOut(ctx, connector.config.RootID, sourcePath, destination), EndpointTypeCloud115)
}

func (connector *Cloud115Connector) DeleteEntry(ctx context.Context, path string) error {
	if strings.TrimSpace(connector.config.AccessToken) == "" {
		return newConnectorError(EndpointTypeCloud115, "delete_entry", ErrorCodeAuthentication, "115 cookies or access token is required", false, nil)
	}
	return remapConnectorType(connector.client.DeleteEntry(ctx, connector.config.RootID, path), EndpointTypeCloud115)
}

func (connector *Cloud115Connector) RenameEntry(ctx context.Context, path string, newName string) (FileEntry, error) {
	if strings.TrimSpace(connector.config.AccessToken) == "" {
		return FileEntry{}, newConnectorError(EndpointTypeCloud115, "rename_entry", ErrorCodeAuthentication, "115 cookies or access token is required", false, nil)
	}
	entry, err := connector.client.RenameEntry(ctx, connector.config.RootID, path, newName)
	return entry, remapConnectorType(err, EndpointTypeCloud115)
}

func (connector *Cloud115Connector) MoveEntry(ctx context.Context, sourcePath string, destinationPath string) (FileEntry, error) {
	if strings.TrimSpace(connector.config.AccessToken) == "" {
		return FileEntry{}, newConnectorError(EndpointTypeCloud115, "move_entry", ErrorCodeAuthentication, "115 cookies or access token is required", false, nil)
	}
	entry, err := connector.client.MoveEntry(ctx, connector.config.RootID, sourcePath, destinationPath)
	return entry, remapConnectorType(err, EndpointTypeCloud115)
}

func (connector *Cloud115Connector) MakeDirectory(ctx context.Context, path string) (FileEntry, error) {
	if strings.TrimSpace(connector.config.AccessToken) == "" {
		return FileEntry{}, newConnectorError(EndpointTypeCloud115, "make_directory", ErrorCodeAuthentication, "115 cookies or access token is required", false, nil)
	}
	entry, err := connector.client.MakeDirectory(ctx, connector.config.RootID, path)
	return entry, remapConnectorType(err, EndpointTypeCloud115)
}

type UnsupportedCloud115Client struct{}

func (UnsupportedCloud115Client) HealthCheck(context.Context, string) error {
	return newConnectorError(EndpointTypeCloud115, "health_check", ErrorCodeNotSupported, "115 API client is not configured", false, nil)
}

func (UnsupportedCloud115Client) ListEntries(context.Context, string, ListEntriesRequest) ([]FileEntry, error) {
	return nil, newConnectorError(EndpointTypeCloud115, "list_entries", ErrorCodeNotSupported, "115 API client is not configured", false, nil)
}

func (UnsupportedCloud115Client) StatEntry(context.Context, string, string) (FileEntry, error) {
	return FileEntry{}, newConnectorError(EndpointTypeCloud115, "stat_entry", ErrorCodeNotSupported, "115 API client is not configured", false, nil)
}

func (UnsupportedCloud115Client) CopyIn(context.Context, string, string, io.Reader) (FileEntry, error) {
	return FileEntry{}, newConnectorError(EndpointTypeCloud115, "copy_in", ErrorCodeNotSupported, "115 API client is not configured", false, nil)
}

func (UnsupportedCloud115Client) CopyOut(context.Context, string, string, io.Writer) error {
	return newConnectorError(EndpointTypeCloud115, "copy_out", ErrorCodeNotSupported, "115 API client is not configured", false, nil)
}

func (UnsupportedCloud115Client) DeleteEntry(context.Context, string, string) error {
	return newConnectorError(EndpointTypeCloud115, "delete_entry", ErrorCodeNotSupported, "115 API client is not configured", false, nil)
}

func (UnsupportedCloud115Client) RenameEntry(context.Context, string, string, string) (FileEntry, error) {
	return FileEntry{}, newConnectorError(EndpointTypeCloud115, "rename_entry", ErrorCodeNotSupported, "115 API client is not configured", false, nil)
}

func (UnsupportedCloud115Client) MoveEntry(context.Context, string, string, string) (FileEntry, error) {
	return FileEntry{}, newConnectorError(EndpointTypeCloud115, "move_entry", ErrorCodeNotSupported, "115 API client is not configured", false, nil)
}

func (UnsupportedCloud115Client) MakeDirectory(context.Context, string, string) (FileEntry, error) {
	return FileEntry{}, newConnectorError(EndpointTypeCloud115, "make_directory", ErrorCodeNotSupported, "115 API client is not configured", false, nil)
}
