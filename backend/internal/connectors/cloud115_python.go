package connectors

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

const defaultCloud115AppType = "wechatmini"

type Cloud115PythonClient struct {
	credential string
	appType    string
	pythonCmd  string
	scriptPath string
	pythonPath string
}

type Cloud115QRCodeSession struct {
	UID        string `json:"uid"`
	Time       int64  `json:"time"`
	Sign       string `json:"sign"`
	AppType    string `json:"appType"`
	QRCodeURL  string `json:"qrCodeUrl"`
	Status     string `json:"status"`
	StatusCode int    `json:"statusCode"`
	Credential string `json:"credential,omitempty"`
}

type cloud115BridgeRequest struct {
	Operation          string `json:"operation"`
	RootID             string `json:"rootId,omitempty"`
	Cookies            string `json:"cookies,omitempty"`
	AccessToken        string `json:"accessToken,omitempty"`
	AppType            string `json:"appType,omitempty"`
	Path               string `json:"path,omitempty"`
	DestinationPath    string `json:"destinationPath,omitempty"`
	NewName            string `json:"newName,omitempty"`
	Recursive          bool   `json:"recursive,omitempty"`
	IncludeDirectories bool   `json:"includeDirectories,omitempty"`
	MediaOnly          bool   `json:"mediaOnly,omitempty"`
	Limit              int    `json:"limit,omitempty"`
	SourceFile         string `json:"sourceFile,omitempty"`
	DownloadFile       string `json:"downloadFile,omitempty"`
	QRUID              string `json:"qrUid,omitempty"`
	QRTime             int64  `json:"qrTime,omitempty"`
	QRSign             string `json:"qrSign,omitempty"`
}

type cloud115BridgeError struct {
	Message   string `json:"message"`
	Type      string `json:"type"`
	Traceback string `json:"traceback"`
}

type cloud115BridgeResponse struct {
	Success       bool                   `json:"success"`
	HealthStatus  HealthStatus           `json:"healthStatus,omitempty"`
	Entry         *FileEntry             `json:"entry,omitempty"`
	Entries       []FileEntry            `json:"entries,omitempty"`
	DownloadFile  string                 `json:"downloadFile,omitempty"`
	QRCodeSession *Cloud115QRCodeSession `json:"qrCodeSession,omitempty"`
	Error         cloud115BridgeError    `json:"error,omitempty"`
}

func NewCloud115PythonClient(credential string, appType string) *Cloud115PythonClient {
	return &Cloud115PythonClient{
		credential: credential,
		appType:    normalizeCloud115AppType(appType),
		pythonCmd:  defaultString(strings.TrimSpace(os.Getenv("MAM_PYTHON_CMD")), "py"),
		scriptPath: resolveCloud115BridgeScript(),
		pythonPath: resolveCloud115PythonPath(),
	}
}

func normalizeCloud115AppType(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch normalized {
	case "", "desktop", "os_windows", "windows", "mac", "linux":
		return defaultCloud115AppType
	case "wechatmini", "alipaymini", "qandroid", "tv", "android", "ios", "web":
		return normalized
	default:
		return defaultCloud115AppType
	}
}

func StartCloud115QRCodeLogin(ctx context.Context, appType string) (*Cloud115QRCodeSession, error) {
	client := NewCloud115PythonClient("", appType)
	response, err := client.call(ctx, cloud115BridgeRequest{
		Operation: "qrcode_start",
		AppType:   normalizeCloud115AppType(appType),
	}, false)
	if err != nil {
		return nil, err
	}
	if response.QRCodeSession == nil {
		return nil, newConnectorError(EndpointTypeCloud115, "qrcode_start", ErrorCodeUnavailable, "115 bridge returned no qr session", true, nil)
	}
	return response.QRCodeSession, nil
}

func PollCloud115QRCodeLogin(
	ctx context.Context,
	appType string,
	uid string,
	tokenTime int64,
	sign string,
) (*Cloud115QRCodeSession, error) {
	client := NewCloud115PythonClient("", appType)
	response, err := client.call(ctx, cloud115BridgeRequest{
		Operation: "qrcode_poll",
		AppType:   normalizeCloud115AppType(appType),
		QRUID:     strings.TrimSpace(uid),
		QRTime:    tokenTime,
		QRSign:    strings.TrimSpace(sign),
	}, false)
	if err != nil {
		return nil, err
	}
	if response.QRCodeSession == nil {
		return nil, newConnectorError(EndpointTypeCloud115, "qrcode_poll", ErrorCodeUnavailable, "115 bridge returned no qr session", true, nil)
	}
	return response.QRCodeSession, nil
}

func (client *Cloud115PythonClient) HealthCheck(ctx context.Context, rootID string) error {
	_, err := client.call(ctx, cloud115BridgeRequest{
		Operation: "health_check",
		RootID:    rootID,
		Cookies:   client.credential,
		AppType:   client.appType,
	}, true)
	return err
}

func (client *Cloud115PythonClient) ListEntries(ctx context.Context, rootID string, request ListEntriesRequest) ([]FileEntry, error) {
	response, err := client.call(ctx, cloud115BridgeRequest{
		Operation:          "list_entries",
		RootID:             rootID,
		Cookies:            client.credential,
		AppType:            client.appType,
		Path:               request.Path,
		Recursive:          request.Recursive,
		IncludeDirectories: request.IncludeDirectories,
		MediaOnly:          request.MediaOnly,
		Limit:              request.Limit,
	}, true)
	if err != nil {
		return nil, err
	}
	return response.Entries, nil
}

func (client *Cloud115PythonClient) StatEntry(ctx context.Context, rootID string, path string) (FileEntry, error) {
	response, err := client.call(ctx, cloud115BridgeRequest{
		Operation: "stat_entry",
		RootID:    rootID,
		Cookies:   client.credential,
		AppType:   client.appType,
		Path:      path,
	}, true)
	if err != nil {
		return FileEntry{}, err
	}
	if response.Entry == nil {
		return FileEntry{}, newConnectorError(EndpointTypeCloud115, "stat_entry", ErrorCodeUnavailable, "115 bridge returned no entry", true, nil)
	}
	return *response.Entry, nil
}

func (client *Cloud115PythonClient) CopyIn(ctx context.Context, rootID string, destinationPath string, source io.Reader) (FileEntry, error) {
	var (
		tempPath   string
		shouldKeep bool
	)

	if sourceFile, ok := source.(*os.File); ok {
		if stat, statErr := sourceFile.Stat(); statErr == nil && !stat.IsDir() {
			tempPath = sourceFile.Name()
			shouldKeep = true
		}
	}

	if strings.TrimSpace(tempPath) == "" {
		tempFile, err := os.CreateTemp("", "mam-115-upload-*")
		if err != nil {
			return FileEntry{}, newConnectorError(EndpointTypeCloud115, "copy_in", ErrorCodeUnavailable, "unable to create temporary upload file", true, err)
		}
		tempPath = tempFile.Name()
		if !shouldKeep {
			defer os.Remove(tempPath)
		}

		if _, copyErr := io.Copy(tempFile, source); copyErr != nil {
			tempFile.Close()
			return FileEntry{}, newConnectorError(EndpointTypeCloud115, "copy_in", ErrorCodeUnavailable, "unable to stage upload content", true, copyErr)
		}
		if closeErr := tempFile.Close(); closeErr != nil {
			return FileEntry{}, newConnectorError(EndpointTypeCloud115, "copy_in", ErrorCodeUnavailable, "unable to close staged upload content", true, closeErr)
		}
	}

	response, callErr := client.call(ctx, cloud115BridgeRequest{
		Operation:       "copy_in",
		RootID:          rootID,
		Cookies:         client.credential,
		AppType:         client.appType,
		DestinationPath: destinationPath,
		SourceFile:      tempPath,
	}, true)
	if callErr != nil {
		return FileEntry{}, callErr
	}
	if response.Entry == nil {
		return FileEntry{}, newConnectorError(EndpointTypeCloud115, "copy_in", ErrorCodeUnavailable, "115 bridge returned no uploaded entry", true, nil)
	}
	return *response.Entry, nil
}

func (client *Cloud115PythonClient) CopyOut(ctx context.Context, rootID string, sourcePath string, destination io.Writer) error {
	tempFile, err := os.CreateTemp("", "mam-115-download-*")
	if err != nil {
		return newConnectorError(EndpointTypeCloud115, "copy_out", ErrorCodeUnavailable, "unable to create temporary download file", true, err)
	}
	tempPath := tempFile.Name()
	tempFile.Close()
	defer os.Remove(tempPath)

	if _, callErr := client.call(ctx, cloud115BridgeRequest{
		Operation:    "copy_out",
		RootID:       rootID,
		Cookies:      client.credential,
		AppType:      client.appType,
		Path:         sourcePath,
		DownloadFile: tempPath,
	}, true); callErr != nil {
		return callErr
	}

	file, openErr := os.Open(tempPath)
	if openErr != nil {
		return newConnectorError(EndpointTypeCloud115, "copy_out", ErrorCodeUnavailable, "unable to open downloaded temporary file", true, openErr)
	}
	defer file.Close()

	if _, copyErr := io.Copy(destination, file); copyErr != nil {
		return newConnectorError(EndpointTypeCloud115, "copy_out", ErrorCodeUnavailable, "unable to copy downloaded content", true, copyErr)
	}

	return nil
}

func (client *Cloud115PythonClient) DeleteEntry(ctx context.Context, rootID string, path string) error {
	_, err := client.call(ctx, cloud115BridgeRequest{
		Operation: "delete_entry",
		RootID:    rootID,
		Cookies:   client.credential,
		AppType:   client.appType,
		Path:      path,
	}, true)
	return err
}

func (client *Cloud115PythonClient) RenameEntry(ctx context.Context, rootID string, path string, newName string) (FileEntry, error) {
	response, err := client.call(ctx, cloud115BridgeRequest{
		Operation: "rename_entry",
		RootID:    rootID,
		Cookies:   client.credential,
		AppType:   client.appType,
		Path:      path,
		NewName:   newName,
	}, true)
	if err != nil {
		return FileEntry{}, err
	}
	if response.Entry == nil {
		return FileEntry{}, newConnectorError(EndpointTypeCloud115, "rename_entry", ErrorCodeUnavailable, "115 bridge returned no renamed entry", true, nil)
	}
	return *response.Entry, nil
}

func (client *Cloud115PythonClient) MoveEntry(ctx context.Context, rootID string, sourcePath string, destinationPath string) (FileEntry, error) {
	response, err := client.call(ctx, cloud115BridgeRequest{
		Operation:       "move_entry",
		RootID:          rootID,
		Cookies:         client.credential,
		AppType:         client.appType,
		Path:            sourcePath,
		DestinationPath: destinationPath,
	}, true)
	if err != nil {
		return FileEntry{}, err
	}
	if response.Entry == nil {
		return FileEntry{}, newConnectorError(EndpointTypeCloud115, "move_entry", ErrorCodeUnavailable, "115 bridge returned no moved entry", true, nil)
	}
	return *response.Entry, nil
}

func (client *Cloud115PythonClient) MakeDirectory(ctx context.Context, rootID string, path string) (FileEntry, error) {
	response, err := client.call(ctx, cloud115BridgeRequest{
		Operation: "make_directory",
		RootID:    rootID,
		Cookies:   client.credential,
		AppType:   client.appType,
		Path:      path,
	}, true)
	if err != nil {
		return FileEntry{}, err
	}
	if response.Entry == nil {
		return FileEntry{}, newConnectorError(EndpointTypeCloud115, "make_directory", ErrorCodeUnavailable, "115 bridge returned no directory entry", true, nil)
	}
	return *response.Entry, nil
}

func (client *Cloud115PythonClient) call(ctx context.Context, request cloud115BridgeRequest, requireCredential bool) (cloud115BridgeResponse, error) {
	if requireCredential && strings.TrimSpace(client.credential) == "" {
		return cloud115BridgeResponse{}, newConnectorError(EndpointTypeCloud115, request.Operation, ErrorCodeAuthentication, "115 session credential is required", false, nil)
	}
	if strings.TrimSpace(client.scriptPath) == "" {
		return cloud115BridgeResponse{}, newConnectorError(EndpointTypeCloud115, request.Operation, ErrorCodeInvalidConfig, "115 bridge script path is empty", false, nil)
	}
	if !cloud115PathMatches(client.scriptPath, false) {
		return cloud115BridgeResponse{}, newConnectorError(
			EndpointTypeCloud115,
			request.Operation,
			ErrorCodeInvalidConfig,
			"115 bridge script not found: "+client.scriptPath,
			false,
			nil,
		)
	}

	if strings.TrimSpace(request.AppType) == "" {
		request.AppType = client.appType
	}
	request.AppType = normalizeCloud115AppType(request.AppType)

	payload, err := json.Marshal(request)
	if err != nil {
		return cloud115BridgeResponse{}, newConnectorError(EndpointTypeCloud115, request.Operation, ErrorCodeUnavailable, "unable to encode 115 bridge request", true, err)
	}

	command := exec.CommandContext(ctx, client.pythonCmd, client.scriptPath)
	command.Stdin = strings.NewReader(string(payload))

	env := os.Environ()
	env = append(env, "PYTHONIOENCODING=utf-8")
	if strings.TrimSpace(client.pythonPath) != "" && cloud115PathMatches(client.pythonPath, true) {
		existing := os.Getenv("PYTHONPATH")
		if existing != "" {
			env = append(env, "PYTHONPATH="+client.pythonPath+string(os.PathListSeparator)+existing)
		} else {
			env = append(env, "PYTHONPATH="+client.pythonPath)
		}
	}
	command.Env = env

	output, execErr := command.CombinedOutput()

	var response cloud115BridgeResponse
	if len(output) > 0 {
		_ = json.Unmarshal(output, &response)
	}

	if execErr != nil {
		message := strings.TrimSpace(response.Error.Message)
		if message == "" {
			message = strings.TrimSpace(string(output))
		}
		if message == "" {
			message = execErr.Error()
		}
		return cloud115BridgeResponse{}, classifyCloud115BridgeError(request.Operation, message, execErr)
	}

	if !response.Success {
		return cloud115BridgeResponse{}, classifyCloud115BridgeError(request.Operation, response.Error.Message, nil)
	}

	return response, nil
}

func resolveCloud115BridgeScript() string {
	if value := strings.TrimSpace(os.Getenv("MAM_115_BRIDGE_SCRIPT")); value != "" {
		return normalizeCloud115RuntimePath(value)
	}

	return resolveCloud115RuntimePathWithRoots([]string{
		filepath.Join("backend", "tools", "cloud115_bridge.py"),
		filepath.Join("tools", "cloud115_bridge.py"),
		"cloud115_bridge.py",
	}, cloud115RuntimeRoots(), false)
}

func resolveCloud115PythonPath() string {
	if value := strings.TrimSpace(os.Getenv("MAM_115_PYTHONPATH")); value != "" {
		return normalizeCloud115RuntimePath(value)
	}

	return resolveCloud115RuntimePathWithRoots([]string{
		filepath.Join(".tools", "pythonlibs"),
		filepath.Join("backend", "pythonlibs"),
		"pythonlibs",
		filepath.Join("..", ".tools", "pythonlibs"),
	}, cloud115RuntimeRoots(), true)
}

func resolveCloud115RuntimePathWithRoots(candidates []string, roots []string, directory bool) string {
	for _, root := range roots {
		for _, candidate := range candidates {
			fullPath := filepath.Join(root, candidate)
			if cloud115PathMatches(fullPath, directory) {
				return normalizeCloud115RuntimePath(fullPath)
			}
		}
	}
	return ""
}

func cloud115RuntimeRoots() []string {
	executable, _ := os.Executable()
	cwd, _ := os.Getwd()

	roots := []string{
		cwd,
		filepath.Dir(cwd),
		filepath.Dir(filepath.Dir(cwd)),
		filepath.Dir(executable),
		filepath.Dir(filepath.Dir(executable)),
		filepath.Dir(filepath.Dir(filepath.Dir(executable))),
	}

	seen := make(map[string]struct{}, len(roots))
	result := make([]string, 0, len(roots))
	for _, root := range roots {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		cleaned := normalizeCloud115RuntimePath(root)
		if _, ok := seen[cleaned]; ok {
			continue
		}
		seen[cleaned] = struct{}{}
		result = append(result, cleaned)
	}
	return result
}

func cloud115PathMatches(target string, directory bool) bool {
	info, err := os.Stat(target)
	if err != nil {
		return false
	}
	if directory {
		return info.IsDir()
	}
	return !info.IsDir()
}

func normalizeCloud115RuntimePath(value string) string {
	cleaned := filepath.Clean(strings.TrimSpace(value))
	switch {
	case strings.HasPrefix(cleaned, `\\?\UNC\`):
		return `\\` + strings.TrimPrefix(cleaned, `\\?\UNC\`)
	case strings.HasPrefix(cleaned, `\\?\`):
		return strings.TrimPrefix(cleaned, `\\?\`)
	default:
		return cleaned
	}
}

func classifyCloud115BridgeError(operation string, message string, underlying error) error {
	lower := strings.ToLower(strings.TrimSpace(message))
	code := ErrorCodeUnavailable
	temporary := true

	switch {
	case lower == "":
		message = "115 bridge execution failed"
	case strings.Contains(lower, "cookie"), strings.Contains(lower, "token"), strings.Contains(lower, "login"), strings.Contains(lower, "auth"), strings.Contains(lower, "重新登录"):
		code = ErrorCodeAuthentication
		temporary = false
	case strings.Contains(lower, "not found"), strings.Contains(lower, "enoent"), strings.Contains(lower, "不存在"), strings.Contains(lower, "path not found"):
		code = ErrorCodeNotFound
		temporary = false
	case strings.Contains(lower, "permission"), strings.Contains(lower, "denied"), strings.Contains(lower, "forbidden"):
		code = ErrorCodeAccessDenied
		temporary = false
	case strings.Contains(lower, "unsupported"):
		code = ErrorCodeNotSupported
		temporary = false
	}

	return newConnectorError(EndpointTypeCloud115, operation, code, message, temporary, underlying)
}

func ParseCloud115QRTokenTime(value string) int64 {
	parsed, _ := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	return parsed
}
