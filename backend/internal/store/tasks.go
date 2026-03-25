package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

func (store *Store) CreateTask(ctx context.Context, task Task) error {
	_, err := store.db.ExecContext(
		ctx,
		`INSERT INTO tasks
		(id, task_type, status, payload, result_summary, error_message, retry_count, created_at, updated_at, started_at, finished_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		task.ID,
		task.TaskType,
		task.Status,
		task.Payload,
		toNullableString(task.ResultSummary),
		toNullableString(task.ErrorMessage),
		task.RetryCount,
		task.CreatedAt.UTC().Format(timeLayout),
		task.UpdatedAt.UTC().Format(timeLayout),
		toNullableTime(task.StartedAt),
		toNullableTime(task.FinishedAt),
	)
	if err != nil {
		return fmt.Errorf("insert task: %w", err)
	}
	return nil
}

func (store *Store) UpdateTaskStatus(ctx context.Context, id string, update TaskStatusUpdate) error {
	_, err := store.db.ExecContext(
		ctx,
		`UPDATE tasks
		 SET status = ?, result_summary = ?, error_message = ?, retry_count = ?, started_at = ?, finished_at = ?, updated_at = ?
		 WHERE id = ?`,
		update.Status,
		toNullableString(update.ResultSummary),
		toNullableString(update.ErrorMessage),
		update.RetryCount,
		toNullableTime(update.StartedAt),
		toNullableTime(update.FinishedAt),
		update.UpdatedAt.UTC().Format(timeLayout),
		id,
	)
	if err != nil {
		return fmt.Errorf("update task status: %w", err)
	}
	return nil
}

func (store *Store) GetTaskByID(ctx context.Context, id string) (Task, error) {
	row := store.db.QueryRowContext(
		ctx,
		`SELECT id, task_type, status, payload, result_summary, error_message, retry_count, created_at, updated_at, started_at, finished_at
		 FROM tasks WHERE id = ?`,
		id,
	)
	return scanTask(row)
}

func (store *Store) DeleteTaskByID(ctx context.Context, id string) error {
	_, err := store.db.ExecContext(ctx, `DELETE FROM tasks WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete task: %w", err)
	}
	return nil
}

func (store *Store) ListTasks(ctx context.Context, limit, offset int) ([]Task, error) {
	rows, err := store.db.QueryContext(
		ctx,
		`SELECT id, task_type, status, payload, result_summary, error_message, retry_count, created_at, updated_at, started_at, finished_at
		 FROM tasks ORDER BY updated_at DESC, created_at DESC LIMIT ? OFFSET ?`,
		limit,
		offset,
	)
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	defer rows.Close()

	var tasks []Task
	for rows.Next() {
		task, scanErr := scanTask(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		tasks = append(tasks, task)
	}

	return tasks, rows.Err()
}

func (store *Store) GetLatestTaskByTypeAndAssetID(ctx context.Context, taskType, assetID string) (Task, error) {
	trimmedTaskType := strings.TrimSpace(taskType)
	trimmedAssetID := strings.TrimSpace(assetID)
	if trimmedTaskType == "" || trimmedAssetID == "" {
		return Task{}, sql.ErrNoRows
	}

	row := store.db.QueryRowContext(
		ctx,
		`SELECT id, task_type, status, payload, result_summary, error_message, retry_count, created_at, updated_at, started_at, finished_at
		 FROM tasks
		 WHERE task_type = ?
		   AND payload LIKE ?
		 ORDER BY updated_at DESC, created_at DESC
		 LIMIT 1`,
		trimmedTaskType,
		`%"assetId":"`+trimmedAssetID+`"%`,
	)
	return scanTask(row)
}

func scanTask(scanner rowScanner) (Task, error) {
	var (
		task              Task
		resultSummaryText sql.NullString
		errorMessageText  sql.NullString
		createdAtText     string
		updatedAtText     string
		startedAtText     sql.NullString
		finishedAtText    sql.NullString
	)

	if err := scanner.Scan(
		&task.ID,
		&task.TaskType,
		&task.Status,
		&task.Payload,
		&resultSummaryText,
		&errorMessageText,
		&task.RetryCount,
		&createdAtText,
		&updatedAtText,
		&startedAtText,
		&finishedAtText,
	); err != nil {
		return Task{}, fmt.Errorf("scan task: %w", err)
	}

	createdAt, err := time.Parse(timeLayout, createdAtText)
	if err != nil {
		return Task{}, fmt.Errorf("parse task created_at: %w", err)
	}

	updatedAt, err := time.Parse(timeLayout, updatedAtText)
	if err != nil {
		return Task{}, fmt.Errorf("parse task updated_at: %w", err)
	}

	startedAt, err := parseNullableTime(startedAtText)
	if err != nil {
		return Task{}, fmt.Errorf("parse task started_at: %w", err)
	}

	finishedAt, err := parseNullableTime(finishedAtText)
	if err != nil {
		return Task{}, fmt.Errorf("parse task finished_at: %w", err)
	}

	task.ResultSummary = parseNullableString(resultSummaryText)
	task.ErrorMessage = parseNullableString(errorMessageText)
	task.CreatedAt = createdAt
	task.UpdatedAt = updatedAt
	task.StartedAt = startedAt
	task.FinishedAt = finishedAt
	return task, nil
}
