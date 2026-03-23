package store

import "time"

type StorageEndpoint struct {
	ID                 string
	Name               string
	EndpointType       string
	RootPath           string
	RoleMode           string
	IdentitySignature  string
	AvailabilityStatus string
	ConnectionConfig   string
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type Asset struct {
	ID                 string
	LogicalPathKey     string
	DisplayName        string
	MediaType          string
	AssetStatus        string
	PrimaryTimestamp   *time.Time
	PrimaryThumbnailID *string
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type AssetPreview struct {
	ID              string
	AssetID         string
	Kind            string
	FilePath        string
	MIMEType        *string
	Width           *int
	Height          *int
	SourceVersionID *string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type AssetMediaMetadata struct {
	AssetID          string
	DurationSeconds  *float64
	CodecName        *string
	SampleRateHz     *int
	ChannelCount     *int
	SourceVersionID  *string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type ReplicaVersion struct {
	ID             string
	Size           int64
	MTime          *time.Time
	CTime          *time.Time
	ChecksumQuick  *string
	ChecksumFull   *string
	MediaSignature *string
	ScanRevision   *string
	CreatedAt      time.Time
}

type Replica struct {
	ID            string
	AssetID       string
	EndpointID    string
	PhysicalPath  string
	ReplicaStatus string
	ExistsFlag    bool
	VersionID     *string
	LastSeenAt    *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type Task struct {
	ID            string
	TaskType      string
	Status        string
	Payload       string
	ResultSummary *string
	ErrorMessage  *string
	RetryCount    int
	CreatedAt     time.Time
	UpdatedAt     time.Time
	StartedAt     *time.Time
	FinishedAt    *time.Time
}

type TaskStatusUpdate struct {
	Status        string
	ResultSummary *string
	ErrorMessage  *string
	RetryCount    int
	StartedAt     *time.Time
	FinishedAt    *time.Time
	UpdatedAt     time.Time
}
