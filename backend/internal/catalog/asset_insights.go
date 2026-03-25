package catalog

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"mam/backend/internal/connectors"
)

var baseSemanticPrompts = []SearchSemanticPrompt{
	{Label: "人像肖像", Prompt: "a portrait photo of a person"},
	{Label: "多人合影", Prompt: "a group photo of several people"},
	{Label: "动物宠物", Prompt: "a photo of an animal or pet"},
	{Label: "风景自然", Prompt: "a natural landscape with mountains, trees, or fields"},
	{Label: "海边水域", Prompt: "a seascape or waterside scene"},
	{Label: "城市建筑", Prompt: "an urban street or modern building"},
	{Label: "室内空间", Prompt: "an indoor room or interior scene"},
	{Label: "夜景低光", Prompt: "a night scene with low light"},
	{Label: "植物花草", Prompt: "flowers, plants, or a garden"},
	{Label: "美食餐饮", Prompt: "food or drinks on a table"},
	{Label: "车辆交通", Prompt: "a car, train, airplane, or traffic scene"},
	{Label: "商品静物", Prompt: "a product photo or still life object"},
	{Label: "文档图表", Prompt: "a document, chart, or printed page"},
	{Label: "屏幕截图", Prompt: "a computer screen, software interface, or screenshot"},
	{Label: "儿童家庭", Prompt: "a family moment with children"},
	{Label: "运动活动", Prompt: "sports or an outdoor activity"},
	{Label: "旅行街拍", Prompt: "travel photography or street photography"},
}

var videoSemanticPrompts = []SearchSemanticPrompt{
	{Label: "会议演讲", Prompt: "a meeting, interview, or presentation"},
	{Label: "舞台演出", Prompt: "a stage performance, concert, or show"},
	{Label: "教学讲解", Prompt: "a person teaching or explaining"},
	{Label: "口播出镜", Prompt: "a talking head video"},
}

func (service *Service) GetAssetInsights(ctx context.Context, assetID string) (AssetInsightsRecord, error) {
	assetID = strings.TrimSpace(assetID)
	if assetID == "" {
		return AssetInsightsRecord{}, errors.New("asset id is required")
	}

	asset, err := service.store.GetAssetByID(ctx, assetID)
	if err != nil {
		return AssetInsightsRecord{}, err
	}

	result := AssetInsightsRecord{}

	if shouldIncludeTranscriptInsights(asset.MediaType) {
		transcript, transcriptErr := service.store.GetAssetTranscriptByAssetID(ctx, asset.ID)
		switch {
		case errors.Is(transcriptErr, sql.ErrNoRows):
		case isSearchInsightsSchemaUnavailable(transcriptErr):
			result.Warnings = append(result.Warnings, "当前资产库还没完成 AI 检索迁移，重启应用后会自动补齐转写与语义表。")
		case transcriptErr != nil:
			return result, transcriptErr
		default:
			text := strings.TrimSpace(transcript.TranscriptText)
			if text != "" {
				result.Transcript = &AssetTranscriptInsightRecord{
					Text:      text,
					Language:  transcript.Language,
					Length:    len([]rune(text)),
					UpdatedAt: transcript.UpdatedAt,
				}
			}
		}
	}

	featureKind := semanticFeatureKindForAssetMediaType(asset.MediaType)
	if featureKind == "" {
		return result, nil
	}

	embedding, embeddingErr := service.store.GetAssetSemanticEmbeddingByAssetAndKind(ctx, asset.ID, featureKind)
	switch {
	case errors.Is(embeddingErr, sql.ErrNoRows):
		return result, nil
	case isSearchInsightsSchemaUnavailable(embeddingErr):
		result.Warnings = append(result.Warnings, "当前资产库还没完成 AI 检索迁移，重启应用后会自动补齐转写与语义表。")
		return result, nil
	case embeddingErr != nil:
		return result, embeddingErr
	}

	var vector []float64
	if err := json.Unmarshal([]byte(embedding.EmbeddingJSON), &vector); err != nil {
		result.Warnings = append(result.Warnings, "语义向量读取失败，暂时无法生成可读标签。")
	} else {
		labels, warning, err := service.resolveSemanticLabels(ctx, featureKind, embedding.ModelName, vector)
		if err != nil {
			return result, err
		}
		if warning != "" {
			result.Warnings = append(result.Warnings, warning)
		}

		result.Semantic = &AssetSemanticInsightRecord{
			FeatureKind: featureKind,
			ModelName:   embedding.ModelName,
			Dimensions:  len(vector),
			Labels:      labels,
			UpdatedAt:   embedding.UpdatedAt,
		}
	}

	if result.Semantic == nil {
		result.Semantic = &AssetSemanticInsightRecord{
			FeatureKind: featureKind,
			ModelName:   embedding.ModelName,
			Dimensions:  len(vector),
			Labels:      []AssetSemanticLabelRecord{},
			UpdatedAt:   embedding.UpdatedAt,
		}
	}

	return result, nil
}

func shouldIncludeTranscriptInsights(mediaType string) bool {
	switch strings.ToLower(strings.TrimSpace(mediaType)) {
	case string(connectors.MediaTypeAudio), string(connectors.MediaTypeVideo):
		return true
	default:
		return false
	}
}

func semanticFeatureKindForAssetMediaType(mediaType string) string {
	switch strings.ToLower(strings.TrimSpace(mediaType)) {
	case string(connectors.MediaTypeImage):
		return semanticFeatureKindImage
	case string(connectors.MediaTypeVideo):
		return semanticFeatureKindVideo
	default:
		return ""
	}
}

func (service *Service) resolveSemanticLabels(
	ctx context.Context,
	featureKind string,
	storedModelName string,
	vector []float64,
) ([]AssetSemanticLabelRecord, string, error) {
	if len(vector) == 0 {
		return nil, "", nil
	}

	description, err := service.searchBridge.DescribeVector(ctx, vector, semanticPromptsForFeatureKind(featureKind), 6)
	if err != nil {
		if warning := buildSemanticInsightWarning(err); warning != "" {
			return nil, warning, nil
		}
		return nil, "", fmt.Errorf("describe semantic labels: %w", err)
	}

	if storedModelName != "" && description.ModelName != "" &&
		!strings.EqualFold(strings.TrimSpace(storedModelName), strings.TrimSpace(description.ModelName)) {
		return nil, "当前语义模型已变更，请重新生成语义特征后再查看标签结果。", nil
	}

	labels := make([]AssetSemanticLabelRecord, 0, len(description.Labels))
	for _, label := range description.Labels {
		if strings.TrimSpace(label.Label) == "" {
			continue
		}
		if label.Score < 0.18 && len(labels) >= 3 {
			continue
		}
		labels = append(labels, AssetSemanticLabelRecord{
			Label: strings.TrimSpace(label.Label),
			Score: roundFloat(label.Score, 4),
		})
	}

	if len(labels) == 0 {
		for index, label := range description.Labels {
			if index >= 3 || strings.TrimSpace(label.Label) == "" {
				break
			}
			labels = append(labels, AssetSemanticLabelRecord{
				Label: strings.TrimSpace(label.Label),
				Score: roundFloat(label.Score, 4),
			})
		}
	}

	return labels, "", nil
}

func semanticPromptsForFeatureKind(featureKind string) []SearchSemanticPrompt {
	prompts := append([]SearchSemanticPrompt{}, baseSemanticPrompts...)
	if strings.EqualFold(strings.TrimSpace(featureKind), semanticFeatureKindVideo) {
		prompts = append(prompts, videoSemanticPrompts...)
	}
	return prompts
}

func isSearchInsightsSchemaUnavailable(err error) bool {
	if err == nil {
		return false
	}

	message := strings.ToLower(err.Error())
	return strings.Contains(message, "no such table: asset_transcripts") ||
		strings.Contains(message, "no such table: asset_search_documents") ||
		strings.Contains(message, "no such table: asset_semantic_embeddings")
}

func buildSemanticInsightWarning(err error) string {
	if err == nil {
		return ""
	}

	message := strings.ToLower(strings.TrimSpace(err.Error()))
	switch {
	case strings.Contains(message, "search_ai.requirements.txt"),
		strings.Contains(message, "transformers/torch"),
		strings.Contains(message, "pillow"):
		return "当前环境缺少语义模型依赖，暂时只能显示已生成的特征元数据。"
	default:
		return ""
	}
}
