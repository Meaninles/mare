package store

import "time"

type StorageEndpoint struct {
	ID                 string
	Name               string
	Note               string
	EndpointType       string
	RootPath           string
	RoleMode           string
	IdentitySignature  string
	AvailabilityStatus string
	ConnectionConfig   string
	CredentialRef      string
	CredentialHint     string
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type LibraryMetadata struct {
	LibraryID     string
	LibraryName   string
	FileExtension string
	SchemaFamily  string
	CreatedAt     time.Time
	UpdatedAt     time.Time
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
	AssetID         string
	DurationSeconds *float64
	CodecName       *string
	SampleRateHz    *int
	ChannelCount    *int
	SourceVersionID *string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type AssetTranscript struct {
	AssetID         string
	TranscriptText  string
	Language        *string
	SourceVersionID *string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type AssetSearchDocument struct {
	ID         string
	AssetID    string
	SourceKind string
	Content    string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type AssetSemanticEmbedding struct {
	ID              string
	AssetID         string
	FeatureKind     string
	ModelName       string
	EmbeddingJSON   string
	SourceVersionID *string
	CreatedAt       time.Time
	UpdatedAt       time.Time
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
	ID            string     `json:"id"`
	TaskType      string     `json:"taskType"`
	Status        string     `json:"status"`
	Payload       string     `json:"payload"`
	ResultSummary *string    `json:"resultSummary,omitempty"`
	ErrorMessage  *string    `json:"errorMessage,omitempty"`
	RetryCount    int        `json:"retryCount"`
	CreatedAt     time.Time  `json:"createdAt"`
	UpdatedAt     time.Time  `json:"updatedAt"`
	StartedAt     *time.Time `json:"startedAt,omitempty"`
	FinishedAt    *time.Time `json:"finishedAt,omitempty"`
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

type ImportRule struct {
	ID                string
	RuleType          string
	MatchValue        string
	TargetEndpointIDs string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}
