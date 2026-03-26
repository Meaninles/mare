package aria2

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const startupTimeout = 20 * time.Second

type Runtime struct {
	stateRoot string

	mu         sync.Mutex
	httpClient *http.Client
	binaryPath string
	process    *exec.Cmd
	logFile    *os.File
	baseURL    string
	rpcSecret  string
	port       int
}

type runtimeState struct {
	RPCSecret string `json:"rpcSecret"`
	Port      int    `json:"port"`
}

type AddRequest struct {
	URIs           []string
	Dir            string
	Out            string
	Headers        []string
	Continue       bool
	AllowOverwrite bool
}

type Status struct {
	GID             string
	Status          string
	TotalLength     int64
	CompletedLength int64
	DownloadSpeed   int64
	ErrorCode       string
	ErrorMessage    string
	Files           []StatusFile
}

type StatusFile struct {
	Path string
	URIs []string
}

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	ID      string `json:"id"`
	Params  []any  `json:"params,omitempty"`
}

type rpcResponse struct {
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type statusResponse struct {
	GID             string `json:"gid"`
	Status          string `json:"status"`
	TotalLength     string `json:"totalLength"`
	CompletedLength string `json:"completedLength"`
	DownloadSpeed   string `json:"downloadSpeed"`
	ErrorCode       string `json:"errorCode"`
	ErrorMessage    string `json:"errorMessage"`
	Files           []struct {
		Path string `json:"path"`
		URIs []struct {
			URI string `json:"uri"`
		} `json:"uris"`
	} `json:"files"`
}

func NewRuntime(stateRoot string) *Runtime {
	return &Runtime{
		stateRoot: filepath.Clean(stateRoot),
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

func (runtime *Runtime) EnsureRunning(ctx context.Context) error {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	return runtime.ensureRunningLocked(ctx)
}

func (runtime *Runtime) Shutdown(ctx context.Context) error {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	if runtime.process == nil || runtime.process.Process == nil {
		runtime.closeLogFileLocked()
		return nil
	}

	done := make(chan error, 1)
	go func() {
		done <- runtime.process.Wait()
	}()

	_ = runtime.process.Process.Kill()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		runtime.process = nil
		runtime.closeLogFileLocked()
		return nil
	case <-time.After(3 * time.Second):
		runtime.process = nil
		runtime.closeLogFileLocked()
		return nil
	}
}

func (runtime *Runtime) AddURI(ctx context.Context, request AddRequest) (string, error) {
	if err := runtime.EnsureRunning(ctx); err != nil {
		return "", err
	}

	options := map[string]any{
		"dir":                filepath.Clean(request.Dir),
		"out":                strings.TrimSpace(request.Out),
		"continue":           strconv.FormatBool(request.Continue),
		"allow-overwrite":    strconv.FormatBool(request.AllowOverwrite),
		"auto-file-renaming": "false",
		"file-allocation":    "none",
	}
	if len(request.Headers) > 0 {
		options["header"] = request.Headers
	}

	var gid string
	if err := runtime.rpcCall(ctx, "aria2.addUri", &gid, request.URIs, options); err != nil {
		return "", err
	}
	return strings.TrimSpace(gid), nil
}

func (runtime *Runtime) TellStatus(ctx context.Context, gid string) (Status, error) {
	if err := runtime.EnsureRunning(ctx); err != nil {
		return Status{}, err
	}

	var response statusResponse
	if err := runtime.rpcCall(ctx, "aria2.tellStatus", &response, strings.TrimSpace(gid)); err != nil {
		return Status{}, err
	}
	return mapStatus(response), nil
}

func (runtime *Runtime) Pause(ctx context.Context, gid string) error {
	return runtime.simpleCall(ctx, "aria2.pause", strings.TrimSpace(gid))
}

func (runtime *Runtime) Unpause(ctx context.Context, gid string) error {
	return runtime.simpleCall(ctx, "aria2.unpause", strings.TrimSpace(gid))
}

func (runtime *Runtime) Remove(ctx context.Context, gid string) error {
	if err := runtime.simpleCall(ctx, "aria2.remove", strings.TrimSpace(gid)); err != nil {
		return err
	}
	_ = runtime.simpleCall(ctx, "aria2.removeDownloadResult", strings.TrimSpace(gid))
	return nil
}

func (runtime *Runtime) ensureRunningLocked(ctx context.Context) error {
	binaryPath, err := runtime.resolveBinaryPath()
	if err != nil {
		return err
	}
	runtime.binaryPath = binaryPath

	if err := os.MkdirAll(runtime.dataDir(), 0o755); err != nil {
		return err
	}
	if err := runtime.ensureSessionFile(); err != nil {
		return err
	}
	state, err := runtime.loadOrCreateState()
	if err != nil {
		return err
	}
	runtime.rpcSecret = state.RPCSecret
	runtime.port = state.Port
	runtime.baseURL = fmt.Sprintf("http://127.0.0.1:%d/jsonrpc", runtime.port)

	if err := runtime.healthcheckLocked(ctx); err == nil {
		return nil
	}

	if runtime.process != nil && runtime.process.Process != nil {
		_ = runtime.process.Process.Kill()
		runtime.process = nil
		runtime.closeLogFileLocked()
	}

	logFile, err := os.OpenFile(filepath.Join(runtime.dataDir(), "sidecar.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}

	command := exec.CommandContext(
		context.Background(),
		runtime.binaryPath,
		"--enable-rpc=true",
		"--rpc-listen-all=false",
		fmt.Sprintf("--rpc-listen-port=%d", runtime.port),
		fmt.Sprintf("--rpc-secret=%s", runtime.rpcSecret),
		"--rpc-allow-origin-all=true",
		"--continue=true",
		fmt.Sprintf("--save-session=%s", runtime.sessionPath()),
		fmt.Sprintf("--input-file=%s", runtime.sessionPath()),
		"--save-session-interval=1",
		"--auto-file-renaming=false",
		"--file-allocation=none",
	)
	command.Stdout = logFile
	command.Stderr = logFile
	command.Dir = filepath.Dir(runtime.binaryPath)
	command.Env = sanitizedProcessEnvironment()

	if err := command.Start(); err != nil {
		_ = logFile.Close()
		return err
	}
	runtime.process = command
	runtime.logFile = logFile

	deadline := time.Now().Add(startupTimeout)
	for time.Now().Before(deadline) {
		if err := runtime.healthcheckLocked(ctx); err == nil {
			return nil
		}
		if command.ProcessState != nil && command.ProcessState.Exited() {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}

	return fmt.Errorf("aria2 sidecar failed to start within %s", startupTimeout)
}

func (runtime *Runtime) ensureSessionFile() error {
	sessionPath := runtime.sessionPath()
	file, err := os.OpenFile(sessionPath, os.O_CREATE, 0o644)
	if err != nil {
		return err
	}
	return file.Close()
}

func (runtime *Runtime) healthcheckLocked(ctx context.Context) error {
	var result map[string]any
	return runtime.rpcCall(ctx, "aria2.getVersion", &result)
}

func (runtime *Runtime) simpleCall(ctx context.Context, method string, params ...any) error {
	if err := runtime.EnsureRunning(ctx); err != nil {
		return err
	}
	return runtime.rpcCall(ctx, method, nil, params...)
}

func (runtime *Runtime) rpcCall(ctx context.Context, method string, destination any, params ...any) error {
	payload := rpcRequest{
		JSONRPC: "2.0",
		Method:  method,
		ID:      fmt.Sprintf("mare-%d", time.Now().UnixNano()),
	}
	if strings.TrimSpace(runtime.rpcSecret) != "" {
		payload.Params = append(payload.Params, "token:"+runtime.rpcSecret)
	}
	payload.Params = append(payload.Params, params...)

	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, runtime.baseURL, bytes.NewReader(encoded))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")

	response, err := runtime.httpClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	raw, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("aria2 rpc failed: status=%d body=%s", response.StatusCode, strings.TrimSpace(string(raw)))
	}

	var rpcResult rpcResponse
	if err := json.Unmarshal(raw, &rpcResult); err != nil {
		return fmt.Errorf("aria2 decode response: %w", err)
	}
	if rpcResult.Error != nil {
		return fmt.Errorf("aria2 rpc error: %s", strings.TrimSpace(rpcResult.Error.Message))
	}
	if destination == nil || len(rpcResult.Result) == 0 || string(rpcResult.Result) == "null" {
		return nil
	}
	return json.Unmarshal(rpcResult.Result, destination)
}

func (runtime *Runtime) loadOrCreateState() (runtimeState, error) {
	statePath := filepath.Join(runtime.dataDir(), "runtime-state.json")
	if raw, err := os.ReadFile(statePath); err == nil {
		var state runtimeState
		if err := json.Unmarshal(raw, &state); err == nil &&
			strings.TrimSpace(state.RPCSecret) != "" &&
			state.Port > 0 {
			return state, nil
		}
	}

	port, err := allocateFreePort()
	if err != nil {
		return runtimeState{}, err
	}

	state := runtimeState{
		RPCSecret: randomSecret(32),
		Port:      port,
	}

	encoded, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return runtimeState{}, err
	}
	if err := os.WriteFile(statePath, encoded, 0o644); err != nil {
		return runtimeState{}, err
	}
	return state, nil
}

func (runtime *Runtime) resolveBinaryPath() (string, error) {
	if strings.TrimSpace(runtime.binaryPath) != "" {
		return runtime.binaryPath, nil
	}

	if configured := strings.TrimSpace(os.Getenv("MAM_ARIA2_BINARY")); configured != "" {
		if pathExists(configured) {
			return filepath.Clean(configured), nil
		}
	}

	executable, _ := os.Executable()
	cwd, _ := os.Getwd()
	roots := []string{
		cwd,
		filepath.Dir(cwd),
		filepath.Dir(executable),
		filepath.Dir(filepath.Dir(executable)),
	}
	patterns := []string{
		filepath.Join(".tools", "runtime", "aria2", "extracted", "*", "aria2c.exe"),
		filepath.Join("tools", "runtime", "aria2", "*", "aria2c.exe"),
		filepath.Join("vendor", "aria2", "*", "aria2c.exe"),
		filepath.Join("aria2", "aria2c.exe"),
	}

	for _, root := range roots {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		for _, pattern := range patterns {
			matches, err := filepath.Glob(filepath.Join(root, pattern))
			if err != nil {
				continue
			}
			for _, match := range matches {
				if pathExists(match) {
					return filepath.Clean(match), nil
				}
			}
		}
	}

	return "", errors.New("aria2 binary not found; checked workspace runtime locations")
}

func (runtime *Runtime) dataDir() string {
	return filepath.Join(runtime.stateRoot, "sidecars", "aria2")
}

func (runtime *Runtime) sessionPath() string {
	return filepath.Join(runtime.dataDir(), "aria2.session")
}

func (runtime *Runtime) closeLogFileLocked() {
	if runtime.logFile != nil {
		_ = runtime.logFile.Close()
		runtime.logFile = nil
	}
}

func mapStatus(response statusResponse) Status {
	files := make([]StatusFile, 0, len(response.Files))
	for _, file := range response.Files {
		uris := make([]string, 0, len(file.URIs))
		for _, uri := range file.URIs {
			if strings.TrimSpace(uri.URI) != "" {
				uris = append(uris, strings.TrimSpace(uri.URI))
			}
		}
		files = append(files, StatusFile{
			Path: strings.TrimSpace(file.Path),
			URIs: uris,
		})
	}

	return Status{
		GID:             strings.TrimSpace(response.GID),
		Status:          strings.TrimSpace(response.Status),
		TotalLength:     parseInt64(response.TotalLength),
		CompletedLength: parseInt64(response.CompletedLength),
		DownloadSpeed:   parseInt64(response.DownloadSpeed),
		ErrorCode:       strings.TrimSpace(response.ErrorCode),
		ErrorMessage:    strings.TrimSpace(response.ErrorMessage),
		Files:           files,
	}
}

func parseInt64(value string) int64 {
	parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil {
		return 0
	}
	return parsed
}

func randomSecret(length int) string {
	if length <= 0 {
		length = 24
	}
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	buffer := make([]byte, length)
	if _, err := rand.Read(buffer); err != nil {
		return fmt.Sprintf("aria2-%d", time.Now().UnixNano())
	}
	for index := range buffer {
		buffer[index] = alphabet[int(buffer[index])%len(alphabet)]
	}
	return string(buffer)
}

func allocateFreePort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()

	address, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		return 0, errors.New("unexpected tcp listener address type")
	}
	return address.Port, nil
}

func pathExists(target string) bool {
	info, err := os.Stat(target)
	return err == nil && !info.IsDir()
}

func sanitizedProcessEnvironment() []string {
	values := os.Environ()
	result := make([]string, 0, len(values))
	for _, entry := range values {
		key, _, found := strings.Cut(entry, "=")
		if found && isProxyEnvironmentKey(key) {
			continue
		}
		result = append(result, entry)
	}
	return result
}

func isProxyEnvironmentKey(key string) bool {
	switch strings.ToUpper(strings.TrimSpace(key)) {
	case "ALL_PROXY", "HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY":
		return true
	default:
		return false
	}
}
