package catalog

import (
	"encoding/json"
	"time"

	"mam/backend/internal/store"
)

type ScanResult struct {
	EndpointID     string     `json:"endpointId"`
	PhysicalPath   string     `json:"physicalPath"`
	LogicalPathKey string     `json:"logicalPathKey"`
	Size           int64      `json:"size"`
	MTime          *time.Time `json:"mtime,omitempty"`
	MediaType      string     `json:"mediaType"`
	IsDir          bool       `json:"isDir"`
}

type MergeStats struct {
	AssetsCreated   int `json:"assetsCreated"`
	AssetsUpdated   int `json:"assetsUpdated"`
	ReplicasCreated int `json:"replicasCreated"`
	ReplicasUpdated int `json:"replicasUpdated"`
}

type EndpointScanSummary struct {
	TaskID          string    `json:"taskId"`
	EndpointID      string    `json:"endpointId"`
	EndpointName    string    `json:"endpointName"`
	EndpointType    string    `json:"endpointType"`
	Status          string    `json:"status"`
	FilesScanned    int       `json:"filesScanned"`
	BatchCount      int       `json:"batchCount"`
	AssetsCreated   int       `json:"assetsCreated"`
	AssetsUpdated   int       `json:"assetsUpdated"`
	ReplicasCreated int       `json:"replicasCreated"`
	ReplicasUpdated int       `json:"replicasUpdated"`
	MissingReplicas int       `json:"missingReplicas"`
	StartedAt       time.Time `json:"startedAt"`
	FinishedAt      time.Time `json:"finishedAt"`
	Error           string    `json:"error,omitempty"`
}

type FullScanSummary struct {
	StartedAt         time.Time             `json:"startedAt"`
	FinishedAt        time.Time             `json:"finishedAt"`
	EndpointCount     int                   `json:"endpointCount"`
	SuccessCount      int                   `json:"successCount"`
	FailedCount       int                   `json:"failedCount"`
	EndpointSummaries []EndpointScanSummary `json:"endpointSummaries"`
}

type RegisterEndpointRequest struct {
	Name               string          `json:"name"`
	Note               string          `json:"note"`
	EndpointType       string          `json:"endpointType"`
	RootPath           string          `json:"rootPath"`
	RoleMode           string          `json:"roleMode"`
	AvailabilityStatus string          `json:"availabilityStatus"`
	IdentitySignature  string          `json:"identitySignature"`
	ConnectionConfig   json.RawMessage `json:"connectionConfig"`
}

type UpdateEndpointRequest struct {
	Name               string          `json:"name"`
	Note               string          `json:"note"`
	EndpointType       string          `json:"endpointType"`
	RootPath           string          `json:"rootPath"`
	RoleMode           string          `json:"roleMode"`
	AvailabilityStatus string          `json:"availabilityStatus"`
	IdentitySignature  string          `json:"identitySignature"`
	ConnectionConfig   json.RawMessage `json:"connectionConfig"`
}

type EndpointRecord struct {
	ID                 string          `json:"id"`
	Name               string          `json:"name"`
	Note               string          `json:"note"`
	EndpointType       string          `json:"endpointType"`
	RootPath           string          `json:"rootPath"`
	RoleMode           string          `json:"roleMode"`
	IdentitySignature  string          `json:"identitySignature"`
	AvailabilityStatus string          `json:"availabilityStatus"`
	ConnectionConfig   json.RawMessage `json:"connectionConfig"`
	CreatedAt          time.Time       `json:"createdAt"`
	UpdatedAt          time.Time       `json:"updatedAt"`
}

type DeleteEndpointSummary struct {
	EndpointID             string    `json:"endpointId"`
	EndpointName           string    `json:"endpointName"`
	EndpointType           string    `json:"endpointType"`
	RemovedReplicaCount    int       `json:"removedReplicaCount"`
	AffectedAssetCount     int       `json:"affectedAssetCount"`
	DeletedAssetCount      int       `json:"deletedAssetCount"`
	UpdatedImportRuleCount int       `json:"updatedImportRuleCount"`
	DeletedAt              time.Time `json:"deletedAt"`
}

type AssetVersionRecord struct {
	ID           string     `json:"id"`
	Size         int64      `json:"size"`
	MTime        *time.Time `json:"mtime,omitempty"`
	CreatedAt    time.Time  `json:"createdAt"`
	ScanRevision *string    `json:"scanRevision,omitempty"`
}

type AssetPreviewRecord struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	URL       string    `json:"url"`
	MIMEType  *string   `json:"mimeType,omitempty"`
	Width     *int      `json:"width,omitempty"`
	Height    *int      `json:"height,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type AssetAudioMetadataRecord struct {
	DurationSeconds *float64 `json:"durationSeconds,omitempty"`
	CodecName       *string  `json:"codecName,omitempty"`
	SampleRateHz    *int     `json:"sampleRateHz,omitempty"`
	ChannelCount    *int     `json:"channelCount,omitempty"`
}

type ReplicaRecord struct {
	ID            string              `json:"id"`
	EndpointID    string              `json:"endpointId"`
	PhysicalPath  string              `json:"physicalPath"`
	ReplicaStatus string              `json:"replicaStatus"`
	ExistsFlag    bool                `json:"existsFlag"`
	LastSeenAt    *time.Time          `json:"lastSeenAt,omitempty"`
	Version       *AssetVersionRecord `json:"version,omitempty"`
}

type AssetRecord struct {
	ID                    string                    `json:"id"`
	LogicalPathKey        string                    `json:"logicalPathKey"`
	DisplayName           string                    `json:"displayName"`
	MediaType             string                    `json:"mediaType"`
	AssetStatus           string                    `json:"assetStatus"`
	PrimaryTimestamp      *time.Time                `json:"primaryTimestamp,omitempty"`
	Poster                *AssetPreviewRecord       `json:"poster,omitempty"`
	PreviewURL            *string                   `json:"previewUrl,omitempty"`
	AudioMetadata         *AssetAudioMetadataRecord `json:"audioMetadata,omitempty"`
	CreatedAt             time.Time                 `json:"createdAt"`
	UpdatedAt             time.Time                 `json:"updatedAt"`
	AvailableReplicaCount int                       `json:"availableReplicaCount"`
	MissingReplicaCount   int                       `json:"missingReplicaCount"`
	Replicas              []ReplicaRecord           `json:"replicas"`
}

type TaskRecord = store.Task
