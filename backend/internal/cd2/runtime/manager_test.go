package cd2runtime

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestProbeExternalRuntimeReady(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("<html><head><title>CloudDrive</title></head><body>ok</body></html>"))
		case "/public/manifest.json":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"name":"CloudDrive2","short_name":"CloudDrive"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	manager := NewManager(Config{
		Enabled:      true,
		Mode:         ModeExternal,
		BaseURL:      server.URL,
		ExpectedName: "CloudDrive2",
		ProbeTimeout: 2 * time.Second,
	})

	state := manager.Probe(context.Background())
	if !state.Reachable {
		t.Fatalf("expected runtime to be reachable")
	}
	if !state.Ready {
		t.Fatalf("expected runtime to be ready")
	}
	if state.ProductName != "CloudDrive2" {
		t.Fatalf("expected product name CloudDrive2, got %q", state.ProductName)
	}
	if !state.NameMatched {
		t.Fatalf("expected product name to match")
	}
	if state.VersionCheckStatus != "skipped" {
		t.Fatalf("expected version check to be skipped, got %q", state.VersionCheckStatus)
	}
}

func TestProbeExternalRuntimeUnavailable(t *testing.T) {
	t.Parallel()

	manager := NewManager(Config{
		Enabled:      true,
		Mode:         ModeExternal,
		BaseURL:      "http://127.0.0.1:1",
		ExpectedName: "CloudDrive2",
		ProbeTimeout: 300 * time.Millisecond,
	})

	state := manager.Probe(context.Background())
	if state.Reachable {
		t.Fatalf("expected runtime to be unreachable")
	}
	if state.Ready {
		t.Fatalf("expected runtime to be not ready")
	}
	if state.LastError == "" {
		t.Fatalf("expected runtime error message")
	}
}
