package alist

import (
	"encoding/json"
	"testing"
)

func TestUploadTaskStateRawSupportsNumberAndString(t *testing.T) {
	testCases := []struct {
		name       string
		payload    string
		wantState  int
		wantStatus string
	}{
		{
			name:       "numeric state",
			payload:    `{"id":"task-1","state":2,"status":"","progress":100}`,
			wantState:  2,
			wantStatus: "complete",
		},
		{
			name:       "string state",
			payload:    `{"id":"task-2","state":"running","status":"","progress":42}`,
			wantState:  1,
			wantStatus: "running",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			var response uploadTaskResponse
			if err := json.Unmarshal([]byte(testCase.payload), &response); err != nil {
				t.Fatalf("unmarshal upload task response: %v", err)
			}

			info := mapUploadTaskInfo(response)
			if info.State != testCase.wantState {
				t.Fatalf("expected mapped state %d, got %d", testCase.wantState, info.State)
			}
			if info.Status != testCase.wantStatus {
				t.Fatalf("expected mapped status %q, got %q", testCase.wantStatus, info.Status)
			}
		})
	}
}

func TestParseUploadTaskInfoPayloadSupportsObjectAndArray(t *testing.T) {
	testCases := []struct {
		name       string
		payload    string
		wantID     string
		wantState  int
		wantStatus string
	}{
		{
			name:       "object payload",
			payload:    `{"id":"task-object","state":1,"status":"running","progress":42}`,
			wantID:     "task-object",
			wantState:  1,
			wantStatus: "running",
		},
		{
			name:       "array payload",
			payload:    `[{"id":"task-array","state":2,"status":"","progress":100}]`,
			wantID:     "task-array",
			wantState:  2,
			wantStatus: "complete",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			info, err := parseUploadTaskInfoPayload(json.RawMessage(testCase.payload))
			if err != nil {
				t.Fatalf("parse upload task payload: %v", err)
			}
			if info.ID != testCase.wantID {
				t.Fatalf("expected task id %q, got %q", testCase.wantID, info.ID)
			}
			if info.State != testCase.wantState {
				t.Fatalf("expected mapped state %d, got %d", testCase.wantState, info.State)
			}
			if info.Status != testCase.wantStatus {
				t.Fatalf("expected mapped status %q, got %q", testCase.wantStatus, info.Status)
			}
		})
	}
}

func TestURLEncodePathEscapesUnicodeSegments(t *testing.T) {
	encoded := urlEncodePath("/upload/摄影备份/dsc_1420.jpg")
	expected := "/upload/%E6%91%84%E5%BD%B1%E5%A4%87%E4%BB%BD/dsc_1420.jpg"
	if encoded != expected {
		t.Fatalf("expected encoded path %q, got %q", expected, encoded)
	}
}
