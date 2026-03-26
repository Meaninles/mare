package catalog

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"mam/backend/internal/connectors"
)

func TestExplainNetworkStorageListErrorHighlightsRootFolderID(t *testing.T) {
	err := explainNetworkStorageListError(
		networkStorageEndpointConfig{RootFolderID: "3392605958421007941"},
		errors.New("alist api error: failed get objs: failed to list objs: unexpected error"),
	)
	if err == nil {
		t.Fatal("expected wrapped error")
	}
	if !strings.Contains(err.Error(), "\u76ee\u5f55 ID 3392605958421007941 \u65e0\u6cd5\u8bbf\u95ee") {
		t.Fatalf("expected error to mention inaccessible root folder id, got %q", err.Error())
	}
}

func TestExplainNetworkStorageListErrorHighlightsCredentialOnRootAccess(t *testing.T) {
	err := explainNetworkStorageListError(
		networkStorageEndpointConfig{RootFolderID: "0"},
		errors.New("alist api error: failed get objs: failed to list objs: unexpected error"),
	)
	if err == nil {
		t.Fatal("expected wrapped error")
	}
	if !strings.Contains(err.Error(), "115 \u7f51\u76d8\u6839\u76ee\u5f55\u8bbf\u95ee\u5931\u8d25") {
		t.Fatalf("expected error to mention root access failure, got %q", err.Error())
	}
}

func TestNormalizeConnectionConfigForNetworkStorageExtractsCredentialAndBuildsInternalAListFields(t *testing.T) {
	normalized, extracted, err := normalizeConnectionConfig(
		string(connectors.EndpointTypeNetwork),
		json.RawMessage(`{"provider":"115","rootFolderId":"0","credential":"token-123"}`),
	)
	if err != nil {
		t.Fatalf("normalize connection config: %v", err)
	}
	if extracted == nil {
		t.Fatal("expected credential extraction result")
	}
	if extracted.Secret != "token-123" {
		t.Fatalf("expected extracted secret token-123, got %q", extracted.Secret)
	}

	var payload map[string]any
	if err := json.Unmarshal(normalized, &payload); err != nil {
		t.Fatalf("decode normalized config: %v", err)
	}

	if _, exists := payload["credential"]; exists {
		t.Fatalf("expected normalized config to exclude raw credential, got %s", string(normalized))
	}
	if payload["provider"] != "115" {
		t.Fatalf("expected provider 115, got %#v", payload["provider"])
	}
	if payload["driver"] != networkStorageDriver115Cloud {
		t.Fatalf("expected driver %q, got %#v", networkStorageDriver115Cloud, payload["driver"])
	}
	if strings.TrimSpace(payload["storageKey"].(string)) == "" {
		t.Fatalf("expected storageKey to be generated, got %s", string(normalized))
	}
	if strings.TrimSpace(payload["mountPath"].(string)) == "" {
		t.Fatalf("expected mountPath to be generated, got %s", string(normalized))
	}
	if payload["loginMethod"] != networkStorageLoginModeManual {
		t.Fatalf("expected login method %q, got %#v", networkStorageLoginModeManual, payload["loginMethod"])
	}
}

func TestBuildNetworkStorageStorageSpecEnablesLocalProxyDownload(t *testing.T) {
	spec, err := buildNetworkStorageStorageSpec(networkStorageEndpointConfig{
		Provider:     networkStorageProvider115,
		StorageKey:   "test-storage",
		MountPath:    "/network/115/test-storage",
		Driver:       networkStorageDriver115Cloud,
		RootFolderID: "0",
		Credential:   "token-123",
	})
	if err != nil {
		t.Fatalf("build network storage spec: %v", err)
	}
	if !spec.WebProxy {
		t.Fatal("expected network storage spec to enable web proxy for download compatibility")
	}
	if !spec.ProxyRange {
		t.Fatal("expected network storage spec to enable proxy range for resumable downloads")
	}
}

func TestResolveRequestedEndpointTypeRejectsLegacy115DirectType(t *testing.T) {
	resolved, err := resolveRequestedEndpointType("115", nil)
	if err == nil {
		t.Fatal("expected legacy 115 direct endpoint type to be rejected")
	}
	if resolved != "" {
		t.Fatalf("expected no resolved endpoint type, got %q", resolved)
	}
}
