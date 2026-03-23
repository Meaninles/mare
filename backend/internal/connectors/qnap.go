package connectors

import (
	"context"
	"io"
)

type QNAPConfig struct {
	Name      string
	SharePath string
}

type QNAPConnector struct {
	local *LocalConnector
}

func NewQNAPConnector(config QNAPConfig) (*QNAPConnector, error) {
	local, err := NewLocalConnector(LocalConfig{
		Name:     defaultString(config.Name, "QNAP SMB"),
		RootPath: config.SharePath,
	})
	if err != nil {
		return nil, remapConnectorType(err, EndpointTypeQNAP)
	}

	descriptor := local.Descriptor()
	descriptor.Type = EndpointTypeQNAP
	local.descriptor = descriptor

	return &QNAPConnector{local: local}, nil
}

func (connector *QNAPConnector) Descriptor() Descriptor {
	return connector.local.Descriptor()
}

func (connector *QNAPConnector) HealthCheck(ctx context.Context) (HealthStatus, error) {
	status, err := connector.local.HealthCheck(ctx)
	return status, remapConnectorType(err, EndpointTypeQNAP)
}

func (connector *QNAPConnector) ListEntries(ctx context.Context, request ListEntriesRequest) ([]FileEntry, error) {
	entries, err := connector.local.ListEntries(ctx, request)
	return entries, remapConnectorType(err, EndpointTypeQNAP)
}

func (connector *QNAPConnector) StatEntry(ctx context.Context, path string) (FileEntry, error) {
	entry, err := connector.local.StatEntry(ctx, path)
	return entry, remapConnectorType(err, EndpointTypeQNAP)
}

func (connector *QNAPConnector) ReadStream(ctx context.Context, path string) (io.ReadCloser, error) {
	reader, err := connector.local.ReadStream(ctx, path)
	return reader, remapConnectorType(err, EndpointTypeQNAP)
}

func (connector *QNAPConnector) CopyIn(ctx context.Context, destinationPath string, source io.Reader) (FileEntry, error) {
	entry, err := connector.local.CopyIn(ctx, destinationPath, source)
	return entry, remapConnectorType(err, EndpointTypeQNAP)
}

func (connector *QNAPConnector) CopyOut(ctx context.Context, sourcePath string, destination io.Writer) error {
	return remapConnectorType(connector.local.CopyOut(ctx, sourcePath, destination), EndpointTypeQNAP)
}

func (connector *QNAPConnector) DeleteEntry(ctx context.Context, path string) error {
	return remapConnectorType(connector.local.DeleteEntry(ctx, path), EndpointTypeQNAP)
}

func (connector *QNAPConnector) RenameEntry(ctx context.Context, path string, newName string) (FileEntry, error) {
	entry, err := connector.local.RenameEntry(ctx, path, newName)
	return entry, remapConnectorType(err, EndpointTypeQNAP)
}

func (connector *QNAPConnector) MoveEntry(ctx context.Context, sourcePath string, destinationPath string) (FileEntry, error) {
	entry, err := connector.local.MoveEntry(ctx, sourcePath, destinationPath)
	return entry, remapConnectorType(err, EndpointTypeQNAP)
}

func (connector *QNAPConnector) MakeDirectory(ctx context.Context, path string) (FileEntry, error) {
	entry, err := connector.local.MakeDirectory(ctx, path)
	return entry, remapConnectorType(err, EndpointTypeQNAP)
}
