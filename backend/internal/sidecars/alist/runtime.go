package alist

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
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultAdminUsername   = "admin"
	defaultStorageCacheTTL = 30
	startupTimeout         = 20 * time.Second
)

type Runtime struct {
	stateRoot string

	mu         sync.Mutex
	httpClient *http.Client
	binaryPath string
	process    *exec.Cmd
	logFile    *os.File
	baseURL    string
	adminPass  string
	port       int
}

type runtimeState struct {
	AdminPassword string `json:"adminPassword"`
	Port          int    `json:"port"`
}

type StorageSpec struct {
	ID              int64
	MountPath       string
	Driver          string
	Addition        string
	Remark          string
	CacheExpiration int
	Order           int
	Disabled        bool
}

type Storage struct {
	ID              int64  `json:"id"`
	MountPath       string `json:"mount_path"`
	Driver          string `json:"driver"`
	Addition        string `json:"addition"`
	Remark          string `json:"remark"`
	Status          string `json:"status"`
	CacheExpiration int    `json:"cache_expiration"`
	Order           int    `json:"order"`
	Disabled        bool   `json:"disabled"`
}

type Entry struct {
	Name       string
	Path       string
	RawPath    string
	RawURL     string
	IsDir      bool
	Size       int64
	ModifiedAt *time.Time
}

type LinkInfo struct {
	URL     string
	Headers map[string][]string
}

type TaskInfo struct {
	ID         string
	Name       string
	State      int
	Status     string
	Progress   float64
	TotalBytes int64
	Error      string
	StartTime  *time.Time
	EndTime    *time.Time
}

type listResponse struct {
	Content []entryResponse `json:"content"`
}

type entryResponse struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	RawURL   string `json:"raw_url"`
	IsDir    bool   `json:"is_dir"`
	Size     int64  `json:"size"`
	Modified string `json:"modified"`
}

type linkResponse struct {
	URL    string              `json:"url"`
	Header map[string][]string `json:"header"`
}

type taskResponse struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	State      int     `json:"state"`
	Status     string  `json:"status"`
	Progress   float64 `json:"progress"`
	TotalBytes int64   `json:"total_bytes"`
	Error      string  `json:"error"`
	StartTime  string  `json:"start_time"`
	EndTime    string  `json:"end_time"`
}

type copyTaskCreateResponse struct {
	Tasks []struct {
		ID string `json:"id"`
	} `json:"tasks"`
}

type apiEnvelope[T any] struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    T      `json:"data"`
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

func (runtime *Runtime) EnsureStorage(ctx context.Context, spec StorageSpec) (Storage, error) {
	spec, err := normalizeStorageSpec(spec)
	if err != nil {
		return Storage{}, err
	}

	token, err := runtime.ensureAuthenticated(ctx)
	if err != nil {
		return Storage{}, err
	}

	storages, err := runtime.listStorages(ctx, token)
	if err != nil {
		return Storage{}, err
	}

	for _, existing := range storages {
		if !strings.EqualFold(existing.MountPath, spec.MountPath) {
			continue
		}
		spec.ID = existing.ID
		if storageEquivalent(existing, spec) {
			return existing, nil
		}
		if err := runtime.updateStorage(ctx, token, spec); err != nil {
			if recovered, ok, recoverErr := runtime.recoverStorageAfterMutationError(ctx, token, spec, err); recoverErr == nil && ok {
				return recovered, nil
			}
			return Storage{}, err
		}
		updated, ok, err := runtime.findStorageByMountPath(ctx, token, spec.MountPath)
		if err != nil {
			return Storage{}, err
		}
		if ok {
			return updated, nil
		}
		return Storage{}, fmt.Errorf("alist storage %q updated but could not be reloaded", spec.MountPath)
	}

	if err := runtime.createStorage(ctx, token, spec); err != nil {
		if recovered, ok, recoverErr := runtime.recoverStorageAfterMutationError(ctx, token, spec, err); recoverErr == nil && ok {
			return recovered, nil
		}
		return Storage{}, err
	}

	created, ok, err := runtime.findStorageByMountPath(ctx, token, spec.MountPath)
	if err != nil {
		return Storage{}, err
	}
	if ok {
		return created, nil
	}
	return Storage{}, fmt.Errorf("alist storage %q created but could not be found", spec.MountPath)
}

func (runtime *Runtime) ListEntries(ctx context.Context, targetPath string, password string, refresh bool) ([]Entry, error) {
	token, err := runtime.ensureAuthenticated(ctx)
	if err != nil {
		return nil, err
	}

	payload := map[string]any{
		"path":     normalizeAListPath(targetPath),
		"password": strings.TrimSpace(password),
		"page":     1,
		"per_page": 1000,
		"refresh":  refresh,
	}
	var response listResponse
	if err := runtime.doJSON(ctx, token, http.MethodPost, "/api/fs/list", payload, &response); err != nil {
		return nil, err
	}

	result := make([]Entry, 0, len(response.Content))
	for _, item := range response.Content {
		result = append(result, mapEntryResponse(normalizeAListPath(targetPath), item))
	}
	return result, nil
}

func (runtime *Runtime) StatEntry(ctx context.Context, targetPath string, password string, refresh bool) (Entry, error) {
	token, err := runtime.ensureAuthenticated(ctx)
	if err != nil {
		return Entry{}, err
	}

	payload := map[string]any{
		"path":     normalizeAListPath(targetPath),
		"password": strings.TrimSpace(password),
		"refresh":  refresh,
	}
	var response entryResponse
	if err := runtime.doJSON(ctx, token, http.MethodPost, "/api/fs/get", payload, &response); err != nil {
		return Entry{}, err
	}

	return mapEntryResponse(normalizeAListPath(targetPath), response), nil
}

func (runtime *Runtime) OpenReadStream(ctx context.Context, targetPath string, password string) (io.ReadCloser, error) {
	linkInfo, err := runtime.LinkEntry(ctx, targetPath, password)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(linkInfo.URL) == "" {
		return nil, fmt.Errorf("alist entry %q did not expose a raw url", targetPath)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, linkInfo.URL, nil)
	if err != nil {
		return nil, err
	}
	applyHeaderValues(request.Header, linkInfo.Headers)

	response, err := runtime.httpClient.Do(request)
	if err != nil {
		return nil, err
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		defer response.Body.Close()
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return nil, fmt.Errorf("alist raw request failed: status=%d body=%s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	return response.Body, nil
}

func (runtime *Runtime) LinkEntry(ctx context.Context, targetPath string, password string) (LinkInfo, error) {
	token, err := runtime.ensureAuthenticated(ctx)
	if err != nil {
		return LinkInfo{}, err
	}

	payload := map[string]any{
		"path":     normalizeAListPath(targetPath),
		"password": strings.TrimSpace(password),
	}
	var response linkResponse
	if err := runtime.doJSON(ctx, token, http.MethodPost, "/api/fs/link", payload, &response); err != nil {
		return LinkInfo{}, err
	}
	if strings.TrimSpace(response.URL) == "" {
		return LinkInfo{}, fmt.Errorf("alist link for %q did not return a url", targetPath)
	}
	return LinkInfo{
		URL:     strings.TrimSpace(response.URL),
		Headers: response.Header,
	}, nil
}

func (runtime *Runtime) CopyIn(ctx context.Context, targetPath string, password string, overwrite bool, source io.Reader) error {
	token, err := runtime.ensureAuthenticated(ctx)
	if err != nil {
		return err
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPut, runtime.baseURL+"/api/fs/put", source)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", token)
	request.Header.Set("File-Path", urlEncodePath(targetPath))
	request.Header.Set("Password", strings.TrimSpace(password))
	request.Header.Set("Overwrite", strconv.FormatBool(overwrite))

	response, err := runtime.httpClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return fmt.Errorf("alist copy in failed: status=%d body=%s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

func (runtime *Runtime) MakeDirectory(ctx context.Context, targetPath string) error {
	token, err := runtime.ensureAuthenticated(ctx)
	if err != nil {
		return err
	}
	return runtime.doJSON(ctx, token, http.MethodPost, "/api/fs/mkdir", map[string]any{
		"path": normalizeAListPath(targetPath),
	}, nil)
}

func (runtime *Runtime) RenameEntry(ctx context.Context, targetPath string, newName string, overwrite bool) error {
	token, err := runtime.ensureAuthenticated(ctx)
	if err != nil {
		return err
	}
	return runtime.doJSON(ctx, token, http.MethodPost, "/api/fs/rename", map[string]any{
		"path":      normalizeAListPath(targetPath),
		"name":      strings.TrimSpace(newName),
		"overwrite": overwrite,
	}, nil)
}

func (runtime *Runtime) MoveEntry(ctx context.Context, sourceDir string, destinationDir string, names []string, overwrite bool) error {
	token, err := runtime.ensureAuthenticated(ctx)
	if err != nil {
		return err
	}
	return runtime.doJSON(ctx, token, http.MethodPost, "/api/fs/move", map[string]any{
		"src_dir":   normalizeAListDir(sourceDir),
		"dst_dir":   normalizeAListDir(destinationDir),
		"names":     names,
		"overwrite": overwrite,
	}, nil)
}

func (runtime *Runtime) RemoveEntry(ctx context.Context, dir string, names []string) error {
	token, err := runtime.ensureAuthenticated(ctx)
	if err != nil {
		return err
	}
	return runtime.doJSON(ctx, token, http.MethodPost, "/api/fs/remove", map[string]any{
		"dir":   normalizeAListDir(dir),
		"names": names,
	}, nil)
}

func (runtime *Runtime) CreateCopyTask(ctx context.Context, srcDir string, dstDir string, names []string, overwrite bool) (TaskInfo, error) {
	token, err := runtime.ensureAuthenticated(ctx)
	if err != nil {
		return TaskInfo{}, err
	}

	var response copyTaskCreateResponse
	if err := runtime.doJSON(ctx, token, http.MethodPost, "/api/fs/copy", map[string]any{
		"src_dir":   normalizeAListDir(srcDir),
		"dst_dir":   normalizeAListDir(dstDir),
		"names":     names,
		"overwrite": overwrite,
	}, &response); err != nil {
		return TaskInfo{}, err
	}
	if len(response.Tasks) == 0 || strings.TrimSpace(response.Tasks[0].ID) == "" {
		return TaskInfo{}, errors.New("alist copy task did not return a task id")
	}

	return runtime.GetCopyTask(ctx, response.Tasks[0].ID)
}

func (runtime *Runtime) GetCopyTask(ctx context.Context, taskID string) (TaskInfo, error) {
	token, err := runtime.ensureAuthenticated(ctx)
	if err != nil {
		return TaskInfo{}, err
	}

	var response taskResponse
	endpoint := fmt.Sprintf("/api/admin/task/copy/info?tid=%s", taskID)
	if err := runtime.doJSON(ctx, token, http.MethodPost, endpoint, nil, &response); err != nil {
		return TaskInfo{}, err
	}
	return mapTaskInfo(response), nil
}

func (runtime *Runtime) CancelCopyTask(ctx context.Context, taskID string) error {
	token, err := runtime.ensureAuthenticated(ctx)
	if err != nil {
		return err
	}
	return runtime.doJSON(ctx, token, http.MethodPost, fmt.Sprintf("/api/admin/task/copy/cancel?tid=%s", taskID), nil, nil)
}

func (runtime *Runtime) RetryCopyTask(ctx context.Context, taskID string) error {
	token, err := runtime.ensureAuthenticated(ctx)
	if err != nil {
		return err
	}
	return runtime.doJSON(ctx, token, http.MethodPost, fmt.Sprintf("/api/admin/task/copy/retry?tid=%s", taskID), nil, nil)
}

func (runtime *Runtime) ensureAuthenticated(ctx context.Context) (string, error) {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	if err := runtime.ensureRunningLocked(ctx); err != nil {
		return "", err
	}
	return runtime.loginLocked(ctx)
}

func (runtime *Runtime) ensureRunningLocked(ctx context.Context) error {
	if runtime.httpClient == nil {
		runtime.httpClient = &http.Client{Timeout: 60 * time.Second}
	}

	binaryPath, err := runtime.resolveBinaryPath()
	if err != nil {
		return err
	}
	runtime.binaryPath = binaryPath

	if err := os.MkdirAll(runtime.dataDir(), 0o755); err != nil {
		return err
	}

	state, err := runtime.loadOrCreateState()
	if err != nil {
		return err
	}
	runtime.adminPass = state.AdminPassword
	runtime.port = state.Port
	runtime.baseURL = fmt.Sprintf("http://127.0.0.1:%d", runtime.port)

	if err := runtime.initializeAdminPassword(ctx); err != nil {
		return err
	}
	if err := runtime.ensureConfigFile(); err != nil {
		return err
	}

	if _, err := runtime.loginLocked(ctx); err == nil {
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

	command := exec.CommandContext(context.Background(), runtime.binaryPath, "--data", runtime.dataDir(), "--no-prefix", "--log-std", "server")
	command.Stdout = logFile
	command.Stderr = logFile
	command.Dir = filepath.Dir(runtime.binaryPath)

	if err := command.Start(); err != nil {
		_ = logFile.Close()
		return err
	}
	runtime.process = command
	runtime.logFile = logFile

	deadline := time.Now().Add(startupTimeout)
	for time.Now().Before(deadline) {
		if _, err := runtime.loginLocked(ctx); err == nil {
			return nil
		}
		if command.ProcessState != nil && command.ProcessState.Exited() {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}

	return fmt.Errorf("alist sidecar failed to start within %s", startupTimeout)
}

func (runtime *Runtime) loginLocked(ctx context.Context) (string, error) {
	payload := map[string]string{
		"username": defaultAdminUsername,
		"password": runtime.adminPass,
	}
	var response struct {
		Token string `json:"token"`
	}
	if err := runtime.doJSONWithBaseURL(ctx, "", runtime.baseURL, http.MethodPost, "/api/auth/login", payload, &response); err != nil {
		return "", err
	}
	if strings.TrimSpace(response.Token) == "" {
		return "", errors.New("alist login did not return a token")
	}
	return response.Token, nil
}

func (runtime *Runtime) listStorages(ctx context.Context, token string) ([]Storage, error) {
	var response struct {
		Content []Storage `json:"content"`
	}
	if err := runtime.doJSON(ctx, token, http.MethodGet, "/api/admin/storage/list", nil, &response); err != nil {
		return nil, err
	}
	return response.Content, nil
}

func (runtime *Runtime) createStorage(ctx context.Context, token string, spec StorageSpec) error {
	return runtime.doJSON(ctx, token, http.MethodPost, "/api/admin/storage/create", specToPayload(spec), nil)
}

func (runtime *Runtime) updateStorage(ctx context.Context, token string, spec StorageSpec) error {
	return runtime.doJSON(ctx, token, http.MethodPost, "/api/admin/storage/update", specToPayload(spec), nil)
}

func (runtime *Runtime) findStorageByMountPath(ctx context.Context, token string, mountPath string) (Storage, bool, error) {
	storages, err := runtime.listStorages(ctx, token)
	if err != nil {
		return Storage{}, false, err
	}
	for _, storage := range storages {
		if strings.EqualFold(storage.MountPath, mountPath) {
			return storage, true, nil
		}
	}
	return Storage{}, false, nil
}

func (runtime *Runtime) recoverStorageAfterMutationError(
	ctx context.Context,
	token string,
	spec StorageSpec,
	mutationErr error,
) (Storage, bool, error) {
	if !isRecoverableStorageMutationError(mutationErr) {
		return Storage{}, false, nil
	}

	storage, ok, err := runtime.findStorageByMountPath(ctx, token, spec.MountPath)
	if err != nil || !ok {
		return Storage{}, ok, err
	}
	if err := runtime.verifyStorageAccessible(ctx, token, storage.MountPath); err != nil {
		return Storage{}, false, nil
	}
	return storage, true, nil
}

func isRecoverableStorageMutationError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(message, "failed init storage") || strings.Contains(message, "storage is already created")
}

func (runtime *Runtime) verifyStorageAccessible(ctx context.Context, token string, mountPath string) error {
	return runtime.doJSON(ctx, token, http.MethodPost, "/api/fs/list", map[string]any{
		"path":     normalizeAListPath(mountPath),
		"password": "",
		"page":     1,
		"per_page": 1,
		"refresh":  true,
	}, nil)
}

func (runtime *Runtime) doJSON(ctx context.Context, token string, method string, requestPath string, payload any, destination any) error {
	return runtime.doJSONWithBaseURL(ctx, token, runtime.baseURL, method, requestPath, payload, destination)
}

func (runtime *Runtime) doJSONWithBaseURL(ctx context.Context, token string, baseURL string, method string, requestPath string, payload any, destination any) error {
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(encoded)
	}

	request, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(baseURL, "/")+requestPath, body)
	if err != nil {
		return err
	}
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if strings.TrimSpace(token) != "" {
		request.Header.Set("Authorization", token)
	}

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
		return fmt.Errorf("alist request failed: status=%d body=%s", response.StatusCode, strings.TrimSpace(string(raw)))
	}

	var envelope apiEnvelope[json.RawMessage]
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return fmt.Errorf("alist decode response: %w", err)
	}
	if envelope.Code != 200 {
		return fmt.Errorf("alist api error: %s", defaultMessage(envelope.Message, string(raw)))
	}

	if destination == nil {
		return nil
	}
	if len(envelope.Data) == 0 || string(envelope.Data) == "null" {
		return nil
	}
	if err := json.Unmarshal(envelope.Data, destination); err != nil {
		return fmt.Errorf("alist decode payload: %w", err)
	}
	return nil
}

func (runtime *Runtime) initializeAdminPassword(ctx context.Context) error {
	command := exec.CommandContext(ctx, runtime.binaryPath, "--data", runtime.dataDir(), "--no-prefix", "admin", "set", runtime.adminPass)
	command.Dir = filepath.Dir(runtime.binaryPath)
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("initialize alist admin password: %w (%s)", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func (runtime *Runtime) ensureConfigFile() error {
	configPath := filepath.Join(runtime.dataDir(), "config.json")
	config := map[string]any{}

	if raw, err := os.ReadFile(configPath); err == nil && len(bytes.TrimSpace(raw)) > 0 {
		if err := json.Unmarshal(raw, &config); err != nil {
			return err
		}
	}

	config["site_url"] = ""
	config["cdn"] = ""
	config["temp_dir"] = filepath.Join(runtime.dataDir(), "temp")
	config["bleve_dir"] = filepath.Join(runtime.dataDir(), "bleve")
	config["dist_dir"] = ""
	config["log"] = map[string]any{
		"enable":      true,
		"name":        filepath.Join(runtime.dataDir(), "log", "log.log"),
		"max_size":    50,
		"max_backups": 30,
		"max_age":     28,
		"compress":    false,
	}
	config["scheme"] = map[string]any{
		"address":        "127.0.0.1",
		"http_port":      runtime.port,
		"https_port":     -1,
		"force_https":    false,
		"cert_file":      "",
		"key_file":       "",
		"unix_file":      "",
		"unix_file_perm": "",
		"enable_h2c":     false,
	}
	config["cors"] = map[string]any{
		"allow_origins": []string{"*"},
		"allow_methods": []string{"*"},
		"allow_headers": []string{"*"},
	}

	tasks := map[string]any{
		"download":             map[string]any{"workers": 5, "max_retry": 1, "task_persistant": true},
		"transfer":             map[string]any{"workers": 5, "max_retry": 2, "task_persistant": true},
		"upload":               map[string]any{"workers": 5, "max_retry": 0, "task_persistant": true},
		"copy":                 map[string]any{"workers": 5, "max_retry": 2, "task_persistant": true},
		"allow_retry_canceled": true,
	}
	config["tasks"] = mergeTaskConfig(config["tasks"], tasks)

	encoded, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configPath, encoded, 0o644)
}

func (runtime *Runtime) loadOrCreateState() (runtimeState, error) {
	statePath := filepath.Join(runtime.dataDir(), "runtime-state.json")
	if raw, err := os.ReadFile(statePath); err == nil {
		var state runtimeState
		if err := json.Unmarshal(raw, &state); err == nil &&
			strings.TrimSpace(state.AdminPassword) != "" &&
			state.Port > 0 {
			return state, nil
		}
	}

	port, err := allocateFreePort()
	if err != nil {
		return runtimeState{}, err
	}

	state := runtimeState{
		AdminPassword: randomSecret(24),
		Port:          port,
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

	if configured := strings.TrimSpace(os.Getenv("MAM_ALIST_BINARY")); configured != "" {
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
	candidates := []string{
		filepath.Join(".tools", "runtime", "alist", "extracted", "alist.exe"),
		filepath.Join("tools", "runtime", "alist", "alist.exe"),
		filepath.Join("vendor", "alist", "alist.exe"),
		filepath.Join("alist", "alist.exe"),
	}

	for _, root := range roots {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		for _, candidate := range candidates {
			fullPath := filepath.Join(root, candidate)
			if pathExists(fullPath) {
				return filepath.Clean(fullPath), nil
			}
		}
	}

	return "", errors.New("alist binary not found; checked workspace runtime locations")
}

func (runtime *Runtime) dataDir() string {
	return filepath.Join(runtime.stateRoot, "sidecars", "alist")
}

func (runtime *Runtime) closeLogFileLocked() {
	if runtime.logFile != nil {
		_ = runtime.logFile.Close()
		runtime.logFile = nil
	}
}

func normalizeStorageSpec(spec StorageSpec) (StorageSpec, error) {
	spec.MountPath = normalizeAListPath(spec.MountPath)
	spec.Driver = strings.TrimSpace(spec.Driver)
	spec.Addition = strings.TrimSpace(spec.Addition)
	spec.Remark = strings.TrimSpace(spec.Remark)
	if spec.MountPath == "" || spec.MountPath == "/" {
		return StorageSpec{}, errors.New("alist mount path is required")
	}
	if spec.Driver == "" {
		return StorageSpec{}, errors.New("alist storage driver is required")
	}
	if spec.CacheExpiration <= 0 {
		spec.CacheExpiration = defaultStorageCacheTTL
	}
	if spec.Addition == "" {
		spec.Addition = "{}"
	}
	return spec, nil
}

func specToPayload(spec StorageSpec) map[string]any {
	payload := map[string]any{
		"mount_path":       spec.MountPath,
		"driver":           spec.Driver,
		"addition":         spec.Addition,
		"remark":           spec.Remark,
		"cache_expiration": spec.CacheExpiration,
		"order":            spec.Order,
		"disabled":         spec.Disabled,
	}
	if spec.ID > 0 {
		payload["id"] = spec.ID
	}
	return payload
}

func storageEquivalent(existing Storage, spec StorageSpec) bool {
	return strings.EqualFold(existing.MountPath, spec.MountPath) &&
		strings.EqualFold(existing.Driver, spec.Driver) &&
		normalizeJSONText(existing.Addition) == normalizeJSONText(spec.Addition) &&
		strings.TrimSpace(existing.Remark) == strings.TrimSpace(spec.Remark) &&
		existing.CacheExpiration == spec.CacheExpiration &&
		existing.Order == spec.Order &&
		existing.Disabled == spec.Disabled
}

func normalizeJSONText(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "{}"
	}
	var payload any
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return trimmed
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return trimmed
	}
	return string(encoded)
}

func mergeTaskConfig(current any, defaults map[string]any) map[string]any {
	merged := make(map[string]any, len(defaults))
	for key, value := range defaults {
		merged[key] = value
	}

	existing, ok := current.(map[string]any)
	if !ok {
		return merged
	}
	for key, value := range existing {
		merged[key] = value
	}
	for key, value := range defaults {
		if nestedExisting, ok := merged[key].(map[string]any); ok {
			if nestedDefaults, ok := value.(map[string]any); ok {
				for nestedKey, nestedValue := range nestedDefaults {
					nestedExisting[nestedKey] = nestedValue
				}
				merged[key] = nestedExisting
			}
		}
	}
	return merged
}

func mapEntryResponse(requestPath string, item entryResponse) Entry {
	modifiedAt := parseTimestamp(item.Modified)
	fullPath := normalizeAListPath(requestPath)
	if requestPath != "" && requestPath != "/" {
		basePath := normalizeAListPath(path.Dir(requestPath))
		if path.Base(requestPath) != item.Name {
			basePath = normalizeAListPath(requestPath)
		}
		fullPath = normalizeAListPath(path.Join(basePath, item.Name))
	}
	if strings.TrimSpace(item.Name) == "" && requestPath != "" {
		item.Name = path.Base(requestPath)
	}
	return Entry{
		Name:       item.Name,
		Path:       fullPath,
		RawPath:    item.Path,
		RawURL:     item.RawURL,
		IsDir:      item.IsDir,
		Size:       item.Size,
		ModifiedAt: modifiedAt,
	}
}

func applyHeaderValues(destination http.Header, values map[string][]string) {
	for key, items := range values {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		for _, item := range items {
			trimmed := strings.TrimSpace(item)
			if trimmed == "" {
				continue
			}
			destination.Add(key, trimmed)
		}
	}
}

func mapTaskInfo(item taskResponse) TaskInfo {
	return TaskInfo{
		ID:         item.ID,
		Name:       item.Name,
		State:      item.State,
		Status:     item.Status,
		Progress:   item.Progress,
		TotalBytes: item.TotalBytes,
		Error:      item.Error,
		StartTime:  parseTimestamp(item.StartTime),
		EndTime:    parseTimestamp(item.EndTime),
	}
}

func parseTimestamp(value string) *time.Time {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if parsed, err := time.Parse(layout, trimmed); err == nil {
			utc := parsed.UTC()
			return &utc
		}
	}
	return nil
}

func normalizeAListPath(value string) string {
	normalized := strings.TrimSpace(strings.ReplaceAll(value, `\`, "/"))
	if normalized == "" {
		return "/"
	}
	if !strings.HasPrefix(normalized, "/") {
		normalized = "/" + normalized
	}
	cleaned := path.Clean(normalized)
	if cleaned == "." {
		return "/"
	}
	return cleaned
}

func normalizeAListDir(value string) string {
	cleaned := normalizeAListPath(value)
	if !strings.HasSuffix(cleaned, "/") {
		cleaned += "/"
	}
	return cleaned
}

func randomSecret(length int) string {
	if length <= 0 {
		length = 24
	}
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	buffer := make([]byte, length)
	if _, err := rand.Read(buffer); err != nil {
		return fmt.Sprintf("alist-%d", time.Now().UnixNano())
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

func defaultMessage(primary string, fallback string) string {
	if strings.TrimSpace(primary) != "" {
		return strings.TrimSpace(primary)
	}
	return strings.TrimSpace(fallback)
}

func urlEncodePath(value string) string {
	replacer := strings.NewReplacer(
		"%", "%25",
		" ", "%20",
		"#", "%23",
		"?", "%3F",
	)
	return replacer.Replace(normalizeAListPath(value))
}
