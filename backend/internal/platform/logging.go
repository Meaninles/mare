package platform

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

type LogEntry struct {
	Timestamp  string         `json:"timestamp"`
	Level      string         `json:"level"`
	Message    string         `json:"message"`
	Attributes map[string]any `json:"attributes,omitempty"`
}

func NewLogger(environment string, logFilePath string) *slog.Logger {
	level := slog.LevelInfo
	if environment == "development" {
		level = slog.LevelDebug
	}

	writer := io.Writer(os.Stdout)
	if strings.TrimSpace(logFilePath) != "" {
		if err := os.MkdirAll(filepath.Dir(logFilePath), 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "create log directory failed: %v\n", err)
		} else if file, err := os.OpenFile(logFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "open log file failed: %v\n", err)
		} else {
			writer = io.MultiWriter(os.Stdout, file)
		}
	}

	return slog.New(slog.NewJSONHandler(writer, &slog.HandlerOptions{Level: level}))
}

func ReadRecentLogEntries(logFilePath string, limit int, level string) ([]LogEntry, error) {
	if strings.TrimSpace(logFilePath) == "" {
		return []LogEntry{}, nil
	}
	if limit <= 0 {
		limit = 100
	}

	file, err := os.Open(logFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return []LogEntry{}, nil
		}
		return nil, fmt.Errorf("open log file: %w", err)
	}
	defer file.Close()

	filterLevel := normalizeLogLevel(level)
	buffer := make([]LogEntry, 0, limit)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		entry, ok := parseLogEntry(scanner.Bytes())
		if !ok {
			continue
		}
		if filterLevel != "" && normalizeLogLevel(entry.Level) != filterLevel {
			continue
		}
		if len(buffer) == limit {
			copy(buffer, buffer[1:])
			buffer[len(buffer)-1] = entry
			continue
		}
		buffer = append(buffer, entry)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan log file: %w", err)
	}

	return buffer, nil
}

func parseLogEntry(line []byte) (LogEntry, bool) {
	var payload map[string]any
	if err := json.Unmarshal(line, &payload); err != nil {
		return LogEntry{}, false
	}

	entry := LogEntry{
		Timestamp:  stringValue(payload["time"]),
		Level:      stringValue(payload["level"]),
		Message:    stringValue(payload["msg"]),
		Attributes: map[string]any{},
	}

	for key, value := range payload {
		switch key {
		case "time", "level", "msg":
			continue
		default:
			entry.Attributes[key] = value
		}
	}

	if len(entry.Attributes) == 0 {
		entry.Attributes = nil
	}
	return entry, true
}

func stringValue(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	return ""
}

func normalizeLogLevel(level string) string {
	return strings.ToLower(strings.TrimSpace(level))
}
