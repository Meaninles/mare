package connectors

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveCloud115RuntimePathWithRootsFindsPackagedBridgeFromBackendWorkdir(t *testing.T) {
	t.Parallel()

	resourceRoot := t.TempDir()
	backendWorkdir := filepath.Join(resourceRoot, "backend")
	bridgeScript := filepath.Join(backendWorkdir, "tools", "cloud115_bridge.py")

	if err := os.MkdirAll(filepath.Dir(bridgeScript), 0o755); err != nil {
		t.Fatalf("create bridge script directory: %v", err)
	}
	if err := os.WriteFile(bridgeScript, []byte("print('ok')\n"), 0o644); err != nil {
		t.Fatalf("write bridge script: %v", err)
	}

	resolved := resolveCloud115RuntimePathWithRoots([]string{
		filepath.Join("backend", "tools", "cloud115_bridge.py"),
		filepath.Join("tools", "cloud115_bridge.py"),
		"cloud115_bridge.py",
	}, []string{backendWorkdir, resourceRoot}, false)

	if resolved != filepath.Clean(bridgeScript) {
		t.Fatalf("expected bridge script %q, got %q", bridgeScript, resolved)
	}
}

func TestResolveCloud115RuntimePathWithRootsFindsBundledPythonLibs(t *testing.T) {
	t.Parallel()

	resourceRoot := t.TempDir()
	backendWorkdir := filepath.Join(resourceRoot, "backend")
	pythonLibs := filepath.Join(resourceRoot, "pythonlibs")

	if err := os.MkdirAll(pythonLibs, 0o755); err != nil {
		t.Fatalf("create pythonlibs directory: %v", err)
	}

	resolved := resolveCloud115RuntimePathWithRoots([]string{
		filepath.Join(".tools", "pythonlibs"),
		filepath.Join("backend", "pythonlibs"),
		"pythonlibs",
		filepath.Join("..", ".tools", "pythonlibs"),
	}, []string{backendWorkdir, resourceRoot}, true)

	if resolved != filepath.Clean(pythonLibs) {
		t.Fatalf("expected pythonlibs path %q, got %q", pythonLibs, resolved)
	}
}

func TestNormalizeCloud115RuntimePathStripsWindowsVerbatimPrefix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input    string
		expected string
	}{
		{
			input:    `\\?\B:\codex_project\mam\src-tauri\target\debug\pythonlibs`,
			expected: `B:\codex_project\mam\src-tauri\target\debug\pythonlibs`,
		},
		{
			input:    `\\?\UNC\server\share\pythonlibs`,
			expected: `\\server\share\pythonlibs`,
		},
	}

	for _, testCase := range tests {
		if actual := normalizeCloud115RuntimePath(testCase.input); actual != testCase.expected {
			t.Fatalf("normalizeCloud115RuntimePath(%q) = %q, want %q", testCase.input, actual, testCase.expected)
		}
	}
}
