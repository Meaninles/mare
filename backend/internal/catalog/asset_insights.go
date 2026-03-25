package catalog

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"mam/backend/internal/connectors"
)

type semanticPromptGroup struct {
	Label   string
	Prompts []string
}

type semanticLabelAggregate struct {
	Label string
	Total float64
	Max   float64
	Count int
}

var baseSemanticPromptGroups = []semanticPromptGroup{
	{Label: "人像肖像", Prompts: []string{"a portrait photo of a person", "a close-up portrait of a human face", "a selfie or headshot photo of one person"}},
	{Label: "多人合影", Prompts: []string{"a group photo of several people", "friends or family posing together for a photo", "multiple people standing together for a picture"}},
	{Label: "猫咪特写", Prompts: []string{"a close-up photo of a cat", "a portrait of a domestic cat", "a kitten or pet cat as the main subject"}},
	{Label: "狗狗特写", Prompts: []string{"a close-up photo of a dog", "a portrait of a domestic dog", "a puppy or pet dog as the main subject"}},
	{Label: "动物宠物", Prompts: []string{"a pet or animal as the main subject", "an animal photo taken up close", "a photo of a pet at home or outdoors"}},
	{Label: "风景自然", Prompts: []string{"a natural landscape with mountains, trees, or fields", "an outdoor scenic landscape", "nature photography with sky and land"}},
	{Label: "海边水域", Prompts: []string{"a beach, seaside, river, or lake scene", "water, ocean, or shoreline in the scene", "a waterside landscape"}},
	{Label: "城市建筑", Prompts: []string{"an urban street or modern building", "city architecture or skyline", "a street scene in a city"}},
	{Label: "室内空间", Prompts: []string{"an indoor room or interior scene", "inside a house, office, or room", "interior photography"}},
	{Label: "夜景低光", Prompts: []string{"a night scene with low light", "a dark environment with artificial lights", "night photography"}},
	{Label: "花草植物", Prompts: []string{"flowers, plants, leaves, or a garden", "a close-up of flowers or plants", "botanical or garden photography"}},
	{Label: "美食餐饮", Prompts: []string{"food or drinks on a table", "a meal, dish, or beverage photo", "food photography"}},
	{Label: "交通工具", Prompts: []string{"a car, train, airplane, bicycle, or traffic scene", "a vehicle as the main subject", "transportation or road traffic"}},
	{Label: "商品静物", Prompts: []string{"a product photo or still life object", "an object arranged for display", "a close-up of a product or item"}},
	{Label: "文档图表", Prompts: []string{"a document, chart, printed page, or presentation slide", "text-heavy paper or chart", "a page with writing, tables, or diagrams"}},
	{Label: "屏幕截图", Prompts: []string{"a computer screen, software interface, or screenshot", "a phone or computer user interface", "an app, website, or desktop screenshot"}},
	{Label: "儿童家庭", Prompts: []string{"a family moment with children", "parents and children together", "a child or family lifestyle photo"}},
	{Label: "运动活动", Prompts: []string{"sports or an outdoor activity", "a person exercising or playing sports", "action photography of movement"}},
	{Label: "旅行街拍", Prompts: []string{"travel photography or street photography", "a candid travel street scene", "daily life captured while traveling"}},
}

var videoSemanticPromptGroups = []semanticPromptGroup{
	{Label: "会议演讲", Prompts: []string{"a meeting, interview, or presentation", "a person speaking in a meeting room", "a presentation or talk in front of people"}},
	{Label: "舞台演出", Prompts: []string{"a stage performance, concert, or show", "people performing on stage", "concert lights and stage action"}},
	{Label: "教学讲解", Prompts: []string{"a person teaching or explaining", "an educational demonstration video", "instructional or tutorial video"}},
	{Label: "口播出镜", Prompts: []string{"a talking head video", "one person speaking directly to the camera", "a vlog or direct-to-camera speaking video"}},
}

var semanticLabelSuppressions = map[string][]string{
	"猫咪特写": {"动物宠物"},
	"狗狗特写": {"动物宠物"},
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
			result.Warnings = append(result.Warnings, "当前资产库还没完成 AI 搜索迁移，重启应用后会自动补齐转写与语义表。")
		case transcriptErr != nil:
			return result, transcriptErr
		default:
			text := strings.TrimSpace(transcript.TranscriptText)
			result.Transcript = &AssetTranscriptInsightRecord{
				Text:      text,
				Language:  transcript.Language,
				Length:    len([]rune(text)),
				UpdatedAt: transcript.UpdatedAt,
			}
			if text == "" {
				result.Warnings = append(result.Warnings, "未检测到可用转写文本。")
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
		result.Warnings = append(result.Warnings, "当前资产库还没完成 AI 搜索迁移，重启应用后会自动补齐转写与语义表。")
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

	prompts := semanticPromptsForFeatureKind(featureKind)
	if len(prompts) == 0 {
		return nil, "", nil
	}

	description, err := service.searchBridge.DescribeVector(ctx, vector, prompts, len(prompts))
	if err != nil {
		if warning := buildSemanticInsightWarning(err); warning != "" {
			return nil, warning, nil
		}
		return nil, "", fmt.Errorf("describe semantic labels: %w", err)
	}

	if storedModelName != "" && description.ModelName != "" &&
		!strings.EqualFold(strings.TrimSpace(storedModelName), strings.TrimSpace(description.ModelName)) {
		return nil, "当前语义模型已经变化，请重新生成语义特征后再查看标签结果。", nil
	}

	aggregated := aggregateSemanticLabelScores(description.Labels)
	labels := make([]AssetSemanticLabelRecord, 0, 6)
	for _, label := range aggregated {
		if strings.TrimSpace(label.Label) == "" {
			continue
		}
		if isSuppressedSemanticLabel(label.Label, labels) {
			continue
		}
		if label.Score < 0.18 && len(labels) >= 3 {
			continue
		}
		labels = append(labels, label)
		if len(labels) >= 6 {
			break
		}
	}

	if len(labels) == 0 {
		for _, label := range aggregated {
			if strings.TrimSpace(label.Label) == "" || isSuppressedSemanticLabel(label.Label, labels) {
				continue
			}
			labels = append(labels, label)
			if len(labels) >= 3 {
				break
			}
		}
	}

	return labels, "", nil
}

func semanticPromptsForFeatureKind(featureKind string) []SearchSemanticPrompt {
	groups := append([]semanticPromptGroup{}, baseSemanticPromptGroups...)
	if strings.EqualFold(strings.TrimSpace(featureKind), semanticFeatureKindVideo) {
		groups = append(groups, videoSemanticPromptGroups...)
	}

	prompts := make([]SearchSemanticPrompt, 0, len(groups)*3)
	for _, group := range groups {
		label := strings.TrimSpace(group.Label)
		if label == "" {
			continue
		}
		for _, prompt := range group.Prompts {
			text := strings.TrimSpace(prompt)
			if text == "" {
				continue
			}
			prompts = append(prompts, SearchSemanticPrompt{
				Label:  label,
				Prompt: text,
			})
		}
	}

	return prompts
}

func aggregateSemanticLabelScores(raw []SearchSemanticLabel) []AssetSemanticLabelRecord {
	if len(raw) == 0 {
		return nil
	}

	accumulators := make(map[string]*semanticLabelAggregate, len(raw))
	for _, item := range raw {
		label := strings.TrimSpace(item.Label)
		if label == "" {
			continue
		}

		accumulator, ok := accumulators[label]
		if !ok {
			accumulator = &semanticLabelAggregate{
				Label: label,
				Max:   item.Score,
			}
			accumulators[label] = accumulator
		}

		accumulator.Total += item.Score
		accumulator.Count++
		if item.Score > accumulator.Max {
			accumulator.Max = item.Score
		}
	}

	labels := make([]AssetSemanticLabelRecord, 0, len(accumulators))
	for _, accumulator := range accumulators {
		if accumulator.Count <= 0 {
			continue
		}

		score := accumulator.Total / float64(accumulator.Count)
		if accumulator.Count > 1 {
			score = (score + accumulator.Max) / 2
		}

		labels = append(labels, AssetSemanticLabelRecord{
			Label: accumulator.Label,
			Score: roundFloat(score, 4),
		})
	}

	sort.Slice(labels, func(left, right int) bool {
		if labels[left].Score == labels[right].Score {
			return labels[left].Label < labels[right].Label
		}
		return labels[left].Score > labels[right].Score
	})

	return labels
}

func isSuppressedSemanticLabel(candidate string, selected []AssetSemanticLabelRecord) bool {
	for _, item := range selected {
		for _, suppressed := range semanticLabelSuppressions[strings.TrimSpace(item.Label)] {
			if strings.EqualFold(strings.TrimSpace(candidate), strings.TrimSpace(suppressed)) {
				return true
			}
		}
	}
	return false
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
