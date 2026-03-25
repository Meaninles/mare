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
	DescribeVector(ctx context.Context, vector []float64, prompts []SearchSemanticPrompt, topK int) (SearchSemanticDescriptionOutput, error)
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

type SearchSemanticPrompt struct {
	Label  string
	Prompt string
}

type SearchSemanticLabel struct {
	Label string
	Score float64
}

type SearchSemanticDescriptionOutput struct {
	ModelName string
	Labels    []SearchSemanticLabel
}

type SearchBridgeError struct {
	Message   string
	ErrorType string
}

func (err *SearchBridgeError) Error() string {
	if err == nil {
		return ""
	}
	return err.Message
}

type pythonSearchBridge struct {
	pythonCmd  string
	scriptPath string
	pythonPath string
}

type searchBridgeRequest struct {
	Operation  string                   `json:"operation"`
	InputPath  string                   `json:"inputPath,omitempty"`
	Text       string                   `json:"text,omitempty"`
	MediaType  string                   `json:"mediaType,omitempty"`
	FFmpegPath string                   `json:"ffmpegPath,omitempty"`
	Vector     []float64                `json:"vector,omitempty"`
	Labels     []searchBridgeLabelInput `json:"labels,omitempty"`
	TopK       int                      `json:"topK,omitempty"`
}

type searchBridgeResponse struct {
	Success     bool                     `json:"success"`
	Transcript  *searchBridgeTranscript  `json:"transcript,omitempty"`
	Embedding   *searchBridgeEmbedding   `json:"embedding,omitempty"`
	Description *searchBridgeDescription `json:"description,omitempty"`
	Error       searchBridgeError        `json:"error,omitempty"`
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

type searchBridgeLabelInput struct {
	Label  string `json:"label"`
	Prompt string `json:"prompt"`
}

type searchBridgeDescription struct {
	ModelName string                    `json:"modelName,omitempty"`
	Labels    []searchBridgeLabelResult `json:"labels,omitempty"`
}

type searchBridgeLabelResult struct {
	Label string  `json:"label"`
	Score float64 `json:"score"`
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

func (bridge *pythonSearchBridge) DescribeVector(
	ctx context.Context,
	vector []float64,
	prompts []SearchSemanticPrompt,
	topK int,
) (SearchSemanticDescriptionOutput, error) {
	if len(vector) == 0 {
		return SearchSemanticDescriptionOutput{}, fmt.Errorf("semantic vector is empty")
	}

	labelInputs := make([]searchBridgeLabelInput, 0, len(prompts))
	for _, prompt := range prompts {
		label := strings.TrimSpace(prompt.Label)
		text := strings.TrimSpace(prompt.Prompt)
		if label == "" || text == "" {
			continue
		}
		labelInputs = append(labelInputs, searchBridgeLabelInput{
			Label:  label,
			Prompt: text,
		})
	}
	if len(labelInputs) == 0 {
		return SearchSemanticDescriptionOutput{}, fmt.Errorf("semantic prompts are empty")
	}

	response, err := bridge.call(ctx, searchBridgeRequest{
		Operation: "describe_vector",
		Vector:    vector,
		Labels:    labelInputs,
		TopK:      topK,
	})
	if err != nil {
		return SearchSemanticDescriptionOutput{}, err
	}
	if response.Description == nil {
		return SearchSemanticDescriptionOutput{}, fmt.Errorf("search ai bridge returned no semantic description")
	}

	labels := make([]SearchSemanticLabel, 0, len(response.Description.Labels))
	for _, label := range response.Description.Labels {
		if strings.TrimSpace(label.Label) == "" {
			continue
		}
		labels = append(labels, SearchSemanticLabel{
			Label: strings.TrimSpace(label.Label),
			Score: label.Score,
		})
	}

	return SearchSemanticDescriptionOutput{
		ModelName: response.Description.ModelName,
		Labels:    labels,
	}, nil
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
		return searchBridgeResponse{}, &SearchBridgeError{
			Message:   fmt.Sprintf("search ai bridge execution failed: %s", message),
			ErrorType: strings.TrimSpace(response.Error.Type),
		}
	}

	if !response.Success {
		message := strings.TrimSpace(response.Error.Message)
		if message == "" {
			message = "search ai bridge failed"
		}
		return searchBridgeResponse{}, &SearchBridgeError{
			Message:   message,
			ErrorType: strings.TrimSpace(response.Error.Type),
		}
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
