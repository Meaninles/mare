package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type SearchAIBridge interface {
	Transcribe(ctx context.Context, inputPath string, mediaType string, ffmpegPath string) (SearchTranscriptOutput, error)
	EmbedImage(ctx context.Context, inputPath string) (SearchEmbeddingOutput, error)
	EmbedVideo(ctx context.Context, inputPath string, ffmpegPath string) (SearchEmbeddingOutput, error)
	EmbedText(ctx context.Context, text string) (SearchEmbeddingOutput, error)
}

type SearchTranscriptOutput struct {
	Text      string
	Language  string
	ModelName string
}

type SearchEmbeddingOutput struct {
	ModelName string
	Vector    []float64
}

type pythonSearchBridge struct {
	pythonCmd  string
	scriptPath string
	pythonPath string
}

type searchBridgeRequest struct {
	Operation  string `json:"operation"`
	InputPath  string `json:"inputPath,omitempty"`
	Text       string `json:"text,omitempty"`
	MediaType  string `json:"mediaType,omitempty"`
	FFmpegPath string `json:"ffmpegPath,omitempty"`
}

type searchBridgeResponse struct {
	Success    bool                    `json:"success"`
	Transcript *searchBridgeTranscript `json:"transcript,omitempty"`
	Embedding  *searchBridgeEmbedding  `json:"embedding,omitempty"`
	Error      searchBridgeError       `json:"error,omitempty"`
}

type searchBridgeTranscript struct {
	Text      string `json:"text"`
	Language  string `json:"language,omitempty"`
	ModelName string `json:"modelName,omitempty"`
}

type searchBridgeEmbedding struct {
	ModelName string    `json:"modelName,omitempty"`
	Vector    []float64 `json:"vector,omitempty"`
}

type searchBridgeError struct {
	Message   string `json:"message"`
	Type      string `json:"type,omitempty"`
	Traceback string `json:"traceback,omitempty"`
}

func NewPythonSearchBridge() SearchAIBridge {
	return &pythonSearchBridge{
		pythonCmd:  defaultString(strings.TrimSpace(os.Getenv("MAM_PYTHON_CMD")), "py"),
		scriptPath: resolveSearchAIScript(),
		pythonPath: resolveSearchAIPythonPath(),
	}
}

func defaultSearchBridge(bridge SearchAIBridge) SearchAIBridge {
	if bridge != nil {
		return bridge
	}
	return NewPythonSearchBridge()
}

func (bridge *pythonSearchBridge) Transcribe(
	ctx context.Context,
	inputPath string,
	mediaType string,
	ffmpegPath string,
) (SearchTranscriptOutput, error) {
	response, err := bridge.call(ctx, searchBridgeRequest{
		Operation:  "transcribe",
		InputPath:  strings.TrimSpace(inputPath),
		MediaType:  strings.TrimSpace(mediaType),
		FFmpegPath: strings.TrimSpace(ffmpegPath),
	})
	if err != nil {
		return SearchTranscriptOutput{}, err
	}
	if response.Transcript == nil {
		return SearchTranscriptOutput{}, fmt.Errorf("search ai bridge returned no transcript")
	}

	return SearchTranscriptOutput{
		Text:      response.Transcript.Text,
		Language:  response.Transcript.Language,
		ModelName: response.Transcript.ModelName,
	}, nil
}

func (bridge *pythonSearchBridge) EmbedImage(ctx context.Context, inputPath string) (SearchEmbeddingOutput, error) {
	return bridge.embed(ctx, searchBridgeRequest{
		Operation: "embed_image",
		InputPath: strings.TrimSpace(inputPath),
	})
}

func (bridge *pythonSearchBridge) EmbedVideo(
	ctx context.Context,
	inputPath string,
	ffmpegPath string,
) (SearchEmbeddingOutput, error) {
	return bridge.embed(ctx, searchBridgeRequest{
		Operation:  "embed_video",
		InputPath:  strings.TrimSpace(inputPath),
		FFmpegPath: strings.TrimSpace(ffmpegPath),
	})
}

func (bridge *pythonSearchBridge) EmbedText(ctx context.Context, text string) (SearchEmbeddingOutput, error) {
	return bridge.embed(ctx, searchBridgeRequest{
		Operation: "embed_text",
		Text:      strings.TrimSpace(text),
	})
}

func (bridge *pythonSearchBridge) embed(ctx context.Context, request searchBridgeRequest) (SearchEmbeddingOutput, error) {
	response, err := bridge.call(ctx, request)
	if err != nil {
		return SearchEmbeddingOutput{}, err
	}
	if response.Embedding == nil {
		return SearchEmbeddingOutput{}, fmt.Errorf("search ai bridge returned no embedding")
	}
	if len(response.Embedding.Vector) == 0 {
		return SearchEmbeddingOutput{}, fmt.Errorf("search ai bridge returned an empty embedding")
	}

	return SearchEmbeddingOutput{
		ModelName: response.Embedding.ModelName,
		Vector:    response.Embedding.Vector,
	}, nil
}

func (bridge *pythonSearchBridge) call(ctx context.Context, request searchBridgeRequest) (searchBridgeResponse, error) {
	if strings.TrimSpace(bridge.scriptPath) == "" {
		return searchBridgeResponse{}, fmt.Errorf("search ai bridge script path is empty")
	}

	payload, err := json.Marshal(request)
	if err != nil {
		return searchBridgeResponse{}, fmt.Errorf("encode search ai request: %w", err)
	}

	command := exec.CommandContext(ctx, bridge.pythonCmd, bridge.scriptPath)
	command.Stdin = strings.NewReader(string(payload))

	env := os.Environ()
	env = append(env, "PYTHONIOENCODING=utf-8")
	if strings.TrimSpace(bridge.pythonPath) != "" {
		existing := os.Getenv("PYTHONPATH")
		if existing != "" {
			env = append(env, "PYTHONPATH="+bridge.pythonPath+string(os.PathListSeparator)+existing)
		} else {
			env = append(env, "PYTHONPATH="+bridge.pythonPath)
		}
	}
	command.Env = env

	output, execErr := command.CombinedOutput()

	var response searchBridgeResponse
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
		return searchBridgeResponse{}, fmt.Errorf("search ai bridge execution failed: %s", message)
	}

	if !response.Success {
		message := strings.TrimSpace(response.Error.Message)
		if message == "" {
			message = "search ai bridge failed"
		}
		return searchBridgeResponse{}, fmt.Errorf(message)
	}

	return response, nil
}

func resolveSearchAIScript() string {
	if value := strings.TrimSpace(os.Getenv("MAM_SEARCH_AI_SCRIPT")); value != "" {
		return value
	}

	candidates := []string{
		filepath.Join("backend", "tools", "search_ai.py"),
		filepath.Join("tools", "search_ai.py"),
	}
	for _, candidate := range candidates {
		if absolute, err := filepath.Abs(candidate); err == nil {
			if _, statErr := os.Stat(absolute); statErr == nil {
				return absolute
			}
		}
	}

	return filepath.Join("backend", "tools", "search_ai.py")
}

func resolveSearchAIPythonPath() string {
	if value := strings.TrimSpace(os.Getenv("MAM_SEARCH_PYTHONPATH")); value != "" {
		return value
	}

	candidates := []string{
		filepath.Join(".tools", "pythonlibs"),
		filepath.Join("..", ".tools", "pythonlibs"),
	}
	for _, candidate := range candidates {
		if absolute, err := filepath.Abs(candidate); err == nil {
			if _, statErr := os.Stat(absolute); statErr == nil {
				return absolute
			}
		}
	}

	return ""
}
