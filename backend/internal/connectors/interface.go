package connectors

import (
	"context"
	"io"
	"time"
)

type Connector interface {
	Descriptor() Descriptor
	HealthCheck(ctx context.Context) (HealthStatus, error)
	ListEntries(ctx context.Context, request ListEntriesRequest) ([]FileEntry, error)
	StatEntry(ctx context.Context, path string) (FileEntry, error)
	ReadStream(ctx context.Context, path string) (io.ReadCloser, error)
	CopyIn(ctx context.Context, destinationPath string, source io.Reader) (FileEntry, error)
	CopyOut(ctx context.Context, sourcePath string, destination io.Writer) error
	DeleteEntry(ctx context.Context, path string) error
	RenameEntry(ctx context.Context, path string, newName string) (FileEntry, error)
	MoveEntry(ctx context.Context, sourcePath string, destinationPath string) (FileEntry, error)
	MakeDirectory(ctx context.Context, path string) (FileEntry, error)
}

type Descriptor struct {
	Name         string
	Type         EndpointType
	RootPath     string
	Capabilities Capabilities
}

type Capabilities struct {
	CanRead          bool
	CanWrite         bool
	CanDelete        bool
	CanList          bool
	CanStat          bool
	CanReadStream    bool
	CanChangeNotify  bool
	CanRename        bool
	CanMove          bool
	CanMakeDirectory bool
}

type EndpointType string

const (
	EndpointTypeLocal     EndpointType = "LOCAL"
	EndpointTypeQNAP      EndpointType = "QNAP_SMB"
	EndpointTypeNetwork   EndpointType = "NETWORK_STORAGE"
	EndpointTypeRemovable EndpointType = "REMOVABLE"
)

type EntryKind string

const (
	EntryKindFile      EntryKind = "file"
	EntryKindDirectory EntryKind = "directory"
)

type MediaType string

const (
	MediaTypeUnknown MediaType = "unknown"
	MediaTypeImage   MediaType = "image"
	MediaTypeVideo   MediaType = "video"
	MediaTypeAudio   MediaType = "audio"
)

type HealthStatus string

const (
	HealthStatusReady    HealthStatus = "ready"
	HealthStatusDegraded HealthStatus = "degraded"
	HealthStatusOffline  HealthStatus = "offline"
)

type ListEntriesRequest struct {
	Path               string
	Recursive          bool
	IncludeDirectories bool
	MediaOnly          bool
	Limit              int
}

type FileEntry struct {
	Path         string     `json:"path"`
	RelativePath string     `json:"relativePath"`
	Name         string     `json:"name"`
	Kind         EntryKind  `json:"kind"`
	MediaType    MediaType  `json:"mediaType"`
	Size         int64      `json:"size"`
	ModifiedAt   *time.Time `json:"modifiedAt,omitempty"`
	IsDir        bool       `json:"isDir"`
}
