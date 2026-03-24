package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"time"

	"mam/backend/internal/store"
)

const taskTypeDeleteReplica = "delete_replica"

type DeleteReplicaRequest struct {
	AssetID          string `json:"assetId"`
	TargetEndpointID string `json:"targetEndpointId"`
}

type DeleteReplicaSummary struct {
	TaskID                   string    `json:"taskId"`
	AssetID                  string    `json:"assetId"`
	DisplayName              string    `json:"displayName"`
	TargetEndpointID         string    `json:"targetEndpointId"`
	TargetEndpointName       string    `json:"targetEndpointName"`
	TargetPhysicalPath       string    `json:"targetPhysicalPath"`
	Status                   string    `json:"status"`
	ReplicaDeleted           bool      `json:"replicaDeleted"`
	AssetRemoved             bool      `json:"assetRemoved"`
	RemainingAvailableCopies int       `json:"remainingAvailableCopies"`
	AssetStatus              string    `json:"assetStatus"`
	StartedAt                time.Time `json:"startedAt"`
	FinishedAt               time.Time `json:"finishedAt"`
	Error                    string    `json:"error,omitempty"`
}

type deleteExecutionResult struct {
	asset                    store.Asset
	targetEndpoint           store.StorageEndpoint
	targetPhysicalPath       string
	remainingAvailableCopies int
	assetRemoved             bool
}

func (service *Service) DeleteReplica(ctx context.Context, request DeleteReplicaRequest) (DeleteReplicaSummary, error) {
	task, err := service.createCatalogTask(ctx, taskTypeDeleteReplica, request)
	if err != nil {
		return DeleteReplicaSummary{}, err
	}

	startedAt := time.Now().UTC()
	summary := DeleteReplicaSummary{
		TaskID:           task.ID,
		Status:           taskStatusRunning,
		StartedAt:        startedAt,
		AssetID:          strings.TrimSpace(request.AssetID),
		TargetEndpointID: strings.TrimSpace(request.TargetEndpointID),
	}

	if err := service.store.UpdateTaskStatus(ctx, task.ID, store.TaskStatusUpdate{
		Status:     taskStatusRunning,
		RetryCount: task.RetryCount,
		StartedAt:  &startedAt,
		UpdatedAt:  startedAt,
	}); err != nil {
		return summary, err
	}

	result, deleteErr := service.executeDeleteReplica(ctx, request)
	finishedAt := time.Now().UTC()
	summary.FinishedAt = finishedAt

	if deleteErr != nil {
		summary.Status = taskStatusFailed
		summary.Error = deleteErr.Error()

		resultText, marshalErr := json.Marshal(summary)
		if marshalErr != nil {
			return summary, marshalErr
		}
		errorText := deleteErr.Error()
		resultSummary := string(resultText)
		if err := service.store.UpdateTaskStatus(ctx, task.ID, store.TaskStatusUpdate{
			Status:        taskStatusFailed,
			ResultSummary: &resultSummary,
			ErrorMessage:  &errorText,
			RetryCount:    task.RetryCount,
			StartedAt:     &startedAt,
			FinishedAt:    &finishedAt,
			UpdatedAt:     finishedAt,
		}); err != nil {
			return summary, err
		}

		return summary, deleteErr
	}

	summary.AssetID = result.asset.ID
	summary.DisplayName = result.asset.DisplayName
	summary.TargetEndpointID = result.targetEndpoint.ID
	summary.TargetEndpointName = result.targetEndpoint.Name
	summary.TargetPhysicalPath = result.targetPhysicalPath
	summary.ReplicaDeleted = true
	summary.AssetRemoved = result.assetRemoved
	summary.RemainingAvailableCopies = result.remainingAvailableCopies
	summary.AssetStatus = result.asset.AssetStatus
	summary.Status = taskStatusSuccess

	resultText, marshalErr := json.Marshal(summary)
	if marshalErr != nil {
		return summary, marshalErr
	}
	resultSummary := string(resultText)
	if err := service.store.UpdateTaskStatus(ctx, task.ID, store.TaskStatusUpdate{
		Status:        taskStatusSuccess,
		ResultSummary: &resultSummary,
		RetryCount:    task.RetryCount,
		StartedAt:     &startedAt,
		FinishedAt:    &finishedAt,
		UpdatedAt:     finishedAt,
	}); err != nil {
		return summary, err
	}

	slog.Info(
		"replica deleted",
		"taskId", task.ID,
		"assetId", summary.AssetID,
		"targetEndpointId", summary.TargetEndpointID,
		"assetRemoved", summary.AssetRemoved,
		"remainingAvailableCopies", summary.RemainingAvailableCopies,
	)
	return summary, nil
}

func (service *Service) executeDeleteReplica(ctx context.Context, request DeleteReplicaRequest) (deleteExecutionResult, error) {
	assetID := strings.TrimSpace(request.AssetID)
	targetEndpointID := strings.TrimSpace(request.TargetEndpointID)
	if assetID == "" || targetEndpointID == "" {
		return deleteExecutionResult{}, errors.New("assetId and targetEndpointId are required")
	}

	if _, err := service.store.GetAssetByID(ctx, assetID); err != nil {
		return deleteExecutionResult{}, err
	}

	targetEndpoint, err := service.store.GetStorageEndpointByID(ctx, targetEndpointID)
	if err != nil {
		return deleteExecutionResult{}, err
	}

	replica, err := service.store.GetReplicaByAssetAndEndpoint(ctx, assetID, targetEndpointID)
	if err != nil {
		return deleteExecutionResult{}, err
	}
	if !replica.ExistsFlag || normalizeReplicaLifecycle(replica.ReplicaStatus, replica.ExistsFlag) == replicaLifecycleDeleted {
		return deleteExecutionResult{}, errors.New("target replica is not available for deletion")
	}

	connector, err := service.buildConnector(targetEndpoint)
	if err != nil {
		return deleteExecutionResult{}, err
	}
	if !connector.Descriptor().Capabilities.CanDelete {
		return deleteExecutionResult{}, errors.New("target endpoint does not support delete")
	}

	if err := connector.DeleteEntry(ctx, replica.PhysicalPath); err != nil {
		return deleteExecutionResult{}, err
	}

	now := time.Now().UTC()
	replica.ExistsFlag = false
	replica.ReplicaStatus = string(ReplicaStatusDeleted)
	replica.UpdatedAt = now
	if err := service.store.UpdateReplica(ctx, replica); err != nil {
		return deleteExecutionResult{}, err
	}

	if err := service.syncAssetStatus(ctx, assetID); err != nil {
		return deleteExecutionResult{}, err
	}

	updatedAsset, err := service.store.GetAssetByID(ctx, assetID)
	if err != nil {
		return deleteExecutionResult{}, err
	}

	replicas, err := service.store.ListReplicasByAssetID(ctx, assetID)
	if err != nil {
		return deleteExecutionResult{}, err
	}

	remainingAvailableCopies := 0
	for _, candidate := range replicas {
		if candidate.ExistsFlag {
			remainingAvailableCopies++
		}
	}

	return deleteExecutionResult{
		asset:                    updatedAsset,
		targetEndpoint:           targetEndpoint,
		targetPhysicalPath:       replica.PhysicalPath,
		remainingAvailableCopies: remainingAvailableCopies,
		assetRemoved:             remainingAvailableCopies == 0,
	}, nil
}
