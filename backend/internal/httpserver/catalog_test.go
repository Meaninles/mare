package httpserver

import (
	"strings"
	"testing"
)

func TestDecodeEndpointRequestPayloadSupportsFlatNetworkStoragePayload(t *testing.T) {
	payload := `{
		"name":"115_test",
		"roleMode":"MANAGED",
		"availabilityStatus":"AVAILABLE",
		"provider":"115",
		"loginMethod":"qrcode",
		"rootFolderId":"0",
		"appType":"android",
		"credential":"cookie-value"
	}`

	decoded, err := decodeEndpointRequestPayload(strings.NewReader(payload))
	if err != nil {
		t.Fatalf("decode endpoint payload: %v", err)
	}
	if decoded.Name != "115_test" {
		t.Fatalf("unexpected name: %q", decoded.Name)
	}
	if len(decoded.ConnectionConfig) == 0 {
		t.Fatalf("expected synthesized connection config")
	}
	configText := string(decoded.ConnectionConfig)
	if !strings.Contains(configText, `"provider":"115"`) {
		t.Fatalf("expected provider in connection config, got %s", configText)
	}
	if !strings.Contains(configText, `"rootFolderId":"0"`) {
		t.Fatalf("expected rootFolderId in connection config, got %s", configText)
	}
}

func TestDecodeEndpointRequestPayloadSupportsAlternateEndpointTypeFields(t *testing.T) {
	payload := `{
		"name":"115_test",
		"type":"NETWORK_STORAGE",
		"connection_config":{"provider":"115"}
	}`

	decoded, err := decodeEndpointRequestPayload(strings.NewReader(payload))
	if err != nil {
		t.Fatalf("decode endpoint payload: %v", err)
	}
	if decoded.EndpointType != "NETWORK_STORAGE" {
		t.Fatalf("unexpected endpoint type: %q", decoded.EndpointType)
	}
	if !strings.Contains(string(decoded.ConnectionConfig), `"provider":"115"`) {
		t.Fatalf("unexpected connection config: %s", string(decoded.ConnectionConfig))
	}
}
