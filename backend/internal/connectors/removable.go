package connectors

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"strings"
)

type RemovableConfig struct {
	Name   string
	Device DeviceInfo
}

type RemovableConnector struct {
	device            DeviceInfo
	identitySignature string
	local             *LocalConnector
}

func NewRemovableConnector(config RemovableConfig) (*RemovableConnector, error) {
	if strings.TrimSpace(config.Device.MountPoint) == "" {
		return nil, newConnectorError(EndpointTypeRemovable, "configure", ErrorCodeInvalidConfig, "device mount point is required", false, nil)
	}

	local, err := NewLocalConnector(LocalConfig{
		Name:     defaultString(config.Name, defaultString(config.Device.VolumeLabel, "Removable Drive")),
		RootPath: config.Device.MountPoint,
	})
	if err != nil {
		return nil, remapConnectorType(err, EndpointTypeRemovable)
	}

	descriptor := local.Descriptor()
	descriptor.Type = EndpointTypeRemovable
	local.descriptor = descriptor

	return &RemovableConnector{
		device:            config.Device,
		identitySignature: GenerateDeviceIdentity(config.Device),
		local:             local,
	}, nil
}

func (connector *RemovableConnector) Descriptor() Descriptor {
	return connector.local.Descriptor()
}

func (connector *RemovableConnector) Device() DeviceInfo {
	return connector.device
}

func (connector *RemovableConnector) IdentitySignature() string {
	return connector.identitySignature
}

func (connector *RemovableConnector) HealthCheck(ctx context.Context) (HealthStatus, error) {
	status, err := connector.local.HealthCheck(ctx)
	return status, remapConnectorType(err, EndpointTypeRemovable)
}

func (connector *RemovableConnector) ListEntries(ctx context.Context, request ListEntriesRequest) ([]FileEntry, error) {
	entries, err := connector.local.ListEntries(ctx, request)
	return entries, remapConnectorType(err, EndpointTypeRemovable)
}

func (connector *RemovableConnector) StatEntry(ctx context.Context, path string) (FileEntry, error) {
	entry, err := connector.local.StatEntry(ctx, path)
	return entry, remapConnectorType(err, EndpointTypeRemovable)
}

func (connector *RemovableConnector) ReadStream(ctx context.Context, path string) (io.ReadCloser, error) {
	reader, err := connector.local.ReadStream(ctx, path)
	return reader, remapConnectorType(err, EndpointTypeRemovable)
}

func (connector *RemovableConnector) CopyIn(ctx context.Context, destinationPath string, source io.Reader) (FileEntry, error) {
	entry, err := connector.local.CopyIn(ctx, destinationPath, source)
	return entry, remapConnectorType(err, EndpointTypeRemovable)
}

func (connector *RemovableConnector) CopyOut(ctx context.Context, sourcePath string, destination io.Writer) error {
	return remapConnectorType(connector.local.CopyOut(ctx, sourcePath, destination), EndpointTypeRemovable)
}

func (connector *RemovableConnector) DeleteEntry(ctx context.Context, path string) error {
	return remapConnectorType(connector.local.DeleteEntry(ctx, path), EndpointTypeRemovable)
}

func (connector *RemovableConnector) RenameEntry(ctx context.Context, path string, newName string) (FileEntry, error) {
	entry, err := connector.local.RenameEntry(ctx, path, newName)
	return entry, remapConnectorType(err, EndpointTypeRemovable)
}

func (connector *RemovableConnector) MoveEntry(ctx context.Context, sourcePath string, destinationPath string) (FileEntry, error) {
	entry, err := connector.local.MoveEntry(ctx, sourcePath, destinationPath)
	return entry, remapConnectorType(err, EndpointTypeRemovable)
}

func (connector *RemovableConnector) MakeDirectory(ctx context.Context, path string) (FileEntry, error) {
	entry, err := connector.local.MakeDirectory(ctx, path)
	return entry, remapConnectorType(err, EndpointTypeRemovable)
}

func GenerateDeviceIdentity(device DeviceInfo) string {
	parts := []string{
		strings.TrimSpace(device.VolumeSerialNumber),
		strings.TrimSpace(device.FileSystem),
		strings.TrimSpace(device.VolumeLabel),
		strings.TrimSpace(device.Model),
		strings.TrimSpace(device.PNPDeviceID),
	}

	allEmpty := true
	for _, part := range parts {
		if part != "" {
			allEmpty = false
			break
		}
	}
	if allEmpty {
		parts = append(parts, strings.TrimSpace(device.MountPoint))
	}

	sum := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return hex.EncodeToString(sum[:])
}
