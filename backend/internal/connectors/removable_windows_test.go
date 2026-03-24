//go:build windows

package connectors

import "testing"

func TestNormalizeCommandJSONOutputStripsPrefixedNoise(t *testing.T) {
	t.Parallel()

	output, err := normalizeCommandJSONOutput([]byte("Get-CimInstance : access denied\r\n[{\"mountPoint\":\"D:\\\\\"}]"))
	if err != nil {
		t.Fatalf("normalize command output: %v", err)
	}
	if string(output) != "[{\"mountPoint\":\"D:\\\\\"}]" {
		t.Fatalf("unexpected normalized output: %s", string(output))
	}
}

func TestNormalizeCommandJSONOutputDecodesUTF16LE(t *testing.T) {
	t.Parallel()

	raw := []byte{
		0xFF, 0xFE,
		'[', 0x00,
		'{', 0x00,
		'"', 0x00,
		'm', 0x00,
		'o', 0x00,
		'u', 0x00,
		'n', 0x00,
		't', 0x00,
		'P', 0x00,
		'o', 0x00,
		'i', 0x00,
		'n', 0x00,
		't', 0x00,
		'"', 0x00,
		':', 0x00,
		'"', 0x00,
		'D', 0x00,
		':', 0x00,
		'\\', 0x00,
		'\\', 0x00,
		'"', 0x00,
		'}', 0x00,
		']', 0x00,
	}

	output, err := normalizeCommandJSONOutput(raw)
	if err != nil {
		t.Fatalf("normalize utf16 output: %v", err)
	}
	if string(output) != "[{\"mountPoint\":\"D:\\\\\"}]" {
		t.Fatalf("unexpected normalized output: %s", string(output))
	}
}
