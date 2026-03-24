package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
)

type RetryTaskSummary struct {
	OriginalTaskID string `json:"originalTaskId"`
	NewTaskID      string `json:"newTaskId,omitempty"`
	TaskType       string `json:"taskType"`
	Status         string `json:"status"`
	Message        string `json:"message"`
}

func (service *Service) RetryTask(ctx context.Context, taskID string) (RetryTaskSummary, error) {
	task, err := service.store.GetTaskByID(ctx, taskID)
	if err != nil {
		return RetryTaskSummary{}, err
	}

	summary := RetryTaskSummary{
		OriginalTaskID: task.ID,
		TaskType:       task.TaskType,
		Status:         taskStatusFailed,
	}

	if !isFailedTaskStatus(task.Status) {
		return summary, fmt.Errorf("only failed tasks can be retried")
	}

	slog.Info("retrying task", "taskId", task.ID, "taskType", task.TaskType)

	switch strings.TrimSpace(task.TaskType) {
	case taskTypeRestoreAsset:
		var request RestoreAssetRequest
		if err := json.Unmarshal([]byte(task.Payload), &request); err != nil {
			return summary, fmt.Errorf("decode restore task payload: %w", err)
		}

		result, retryErr := service.RestoreAsset(ctx, request)
		summary.NewTaskID = result.TaskID
		if retryErr != nil {
			summary.Message = retryErr.Error()
			return summary, retryErr
		}

		summary.Status = taskStatusSuccess
		summary.Message = "restore task retried successfully"
		return summary, nil
	case taskTypeRestoreBatch:
		var request BatchRestoreRequest
		if err := json.Unmarshal([]byte(task.Payload), &request); err != nil {
			return summary, fmt.Errorf("decode batch restore task payload: %w", err)
		}

		result, retryErr := service.RestoreAssetsToEndpoint(ctx, request)
		summary.NewTaskID = result.TaskID
		if retryErr != nil {
			summary.Message = retryErr.Error()
			return summary, retryErr
		}

		summary.Status = taskStatusSuccess
		summary.Message = "batch restore task retried successfully"
		return summary, nil
	case "scan_endpoint":
		var request struct {
			EndpointID string `json:"endpointId"`
		}
		if err := json.Unmarshal([]byte(task.Payload), &request); err != nil {
			return summary, fmt.Errorf("decode scan task payload: %w", err)
		}

		result, retryErr := service.RescanEndpoint(ctx, request.EndpointID)
		summary.NewTaskID = result.TaskID
		if retryErr != nil {
			summary.Message = retryErr.Error()
			return summary, retryErr
		}

		summary.Status = taskStatusSuccess
		summary.Message = "scan task retried successfully"
		return summary, nil
	case taskTypeImportExecute:
		var request ExecuteImportRequest
		if err := json.Unmarshal([]byte(task.Payload), &request); err != nil {
			return summary, fmt.Errorf("decode import task payload: %w", err)
		}

		result, retryErr := service.ExecuteImport(ctx, request)
		summary.NewTaskID = result.TaskID
		if retryErr != nil {
			summary.Message = retryErr.Error()
			return summary, retryErr
		}

		summary.Status = taskStatusSuccess
		summary.Message = "import task retried successfully"
		return summary, nil
	case mediaTaskThumbnail, mediaTaskVideoCover, mediaTaskAudioMetadata:
		var payload struct {
			AssetID string `json:"assetId"`
		}
		if err := json.Unmarshal([]byte(task.Payload), &payload); err != nil {
			return summary, fmt.Errorf("decode media task payload: %w", err)
		}

		newTask, retryErr := service.startMediaTask(ctx, payload.AssetID, task.TaskType)
		summary.NewTaskID = newTask.ID
		if retryErr != nil {
			summary.Message = retryErr.Error()
			return summary, retryErr
		}

		summary.Status = taskStatusSuccess
		summary.Message = "media task retried successfully"
		return summary, nil
	default:
		return summary, fmt.Errorf("task type %q does not support retry", task.TaskType)
	}
}

func (service *Service) RetrySyncTask(ctx context.Context, taskID string) (RetryTaskSummary, error) {
	return service.RetryTask(ctx, taskID)
}

func isFailedTaskStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case taskStatusFailed, "error":
		return true
	default:
		return false
	}
}
