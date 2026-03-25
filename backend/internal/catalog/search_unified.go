package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"mam/backend/internal/store"
)

type semanticSearchHit struct {
	AssetID     string
	MatchKind   string
	Score       float64
}

type unifiedSearchAccumulator struct {
	asset             AssetRecord
	score             float64
	matchKinds        map[string]struct{}
	transcriptSnippet *string
	semanticScore     *float64
}

func (service *Service) SearchLibrary(
	ctx context.Context,
	query string,
	mediaType string,
	assetStatus string,
	limit int,
	offset int,
) (UnifiedSearchResponse, error) {
	normalizedQuery := strings.TrimSpace(query)
	if normalizedQuery == "" {
		return UnifiedSearchResponse{
			Query:   normalizedQuery,
			Results: []UnifiedSearchResultRecord{},
		}, nil
	}

	limit = maxInt(limit, 20)
	candidateLimit := maxInt(limit*3, 30)
	response := UnifiedSearchResponse{
		Query:   normalizedQuery,
		Results: []UnifiedSearchResultRecord{},
	}

	accumulators := make(map[string]*unifiedSearchAccumulator)

	textResults, err := service.SearchAssets(ctx, normalizedQuery, mediaType, assetStatus, candidateLimit, 0)
	if err != nil {
		return response, err
	}
	for index, asset := range textResults {
		accumulator := ensureUnifiedSearchAccumulator(accumulators, asset)
		accumulator.score += reciprocalRankScore(index, 1.0)
		accumulator.matchKinds["text"] = struct{}{}
	}

	transcriptHits, err := service.store.SearchTranscriptHits(ctx, store.AssetListOptions{
		Limit:       candidateLimit,
		Offset:      0,
		SearchQuery: normalizedQuery,
		MediaType:   mediaType,
		AssetStatus: assetStatus,
	})
	if err != nil {
		response.Warnings = append(response.Warnings, fmt.Sprintf("转写检索暂时不可用：%s", err.Error()))
	} else if len(transcriptHits) > 0 {
		recordMap, buildErr := service.buildAssetRecordMap(ctx, collectSearchHitAssetIDs(transcriptHits))
		if buildErr != nil {
			return response, buildErr
		}

		for index, hit := range transcriptHits {
			asset, ok := recordMap[hit.AssetID]
			if !ok {
				continue
			}
			accumulator := ensureUnifiedSearchAccumulator(accumulators, asset)
			accumulator.score += reciprocalRankScore(index, 0.88)
			accumulator.matchKinds["transcript"] = struct{}{}
			if accumulator.transcriptSnippet == nil && strings.TrimSpace(hit.Snippet) != "" {
				snippet := strings.TrimSpace(hit.Snippet)
				accumulator.transcriptSnippet = &snippet
			}
		}
	}

	semanticHits, semanticWarning, err := service.searchSemanticHits(ctx, normalizedQuery, mediaType, assetStatus, candidateLimit)
	if err != nil {
		return response, err
	}
	if semanticWarning != "" {
		response.Warnings = append(response.Warnings, semanticWarning)
	}
	if len(semanticHits) > 0 {
		recordMap, buildErr := service.buildAssetRecordMap(ctx, collectSemanticHitAssetIDs(semanticHits))
		if buildErr != nil {
			return response, buildErr
		}

		for _, hit := range semanticHits {
			asset, ok := recordMap[hit.AssetID]
			if !ok {
				continue
			}
			accumulator := ensureUnifiedSearchAccumulator(accumulators, asset)
			accumulator.score += hit.Score * 0.75
			accumulator.matchKinds[hit.MatchKind] = struct{}{}
			if accumulator.semanticScore == nil || hit.Score > *accumulator.semanticScore {
				score := roundFloat(hit.Score, 4)
				accumulator.semanticScore = &score
			}
		}
	}

	results := make([]UnifiedSearchResultRecord, 0, len(accumulators))
	for _, accumulator := range accumulators {
		results = append(results, UnifiedSearchResultRecord{
			Asset:             accumulator.asset,
			MatchKinds:        orderedMatchKinds(accumulator.matchKinds),
			TranscriptSnippet: accumulator.transcriptSnippet,
			SemanticScore:     accumulator.semanticScore,
		})
	}

	sort.Slice(results, func(left, right int) bool {
		leftAccumulator := accumulators[results[left].Asset.ID]
		rightAccumulator := accumulators[results[right].Asset.ID]
		if leftAccumulator.score != rightAccumulator.score {
			return leftAccumulator.score > rightAccumulator.score
		}
		return assetTimestampValue(results[left].Asset) > assetTimestampValue(results[right].Asset)
	})

	if offset > len(results) {
		offset = len(results)
	}

	upperBound := minInt(offset+limit, len(results))
	response.Results = results[offset:upperBound]
	return response, nil
}

func (service *Service) searchSemanticHits(
	ctx context.Context,
	query string,
	mediaType string,
	assetStatus string,
	limit int,
) ([]semanticSearchHit, string, error) {
	featureKinds := semanticFeatureKindsForMediaType(mediaType)
	if len(featureKinds) == 0 {
		return nil, "", nil
	}

	queryEmbedding, err := service.searchBridge.EmbedText(ctx, query)
	if err != nil {
		return nil, fmt.Sprintf("语义检索引擎暂时不可用：%s", err.Error()), nil
	}

	embeddings, err := service.store.ListAssetSemanticEmbeddings(ctx, featureKinds)
	if err != nil {
		return nil, "", err
	}
	if len(embeddings) == 0 {
		return nil, "", nil
	}

	recordMap, err := service.buildAssetRecordMap(ctx, collectEmbeddingAssetIDs(embeddings))
	if err != nil {
		return nil, "", err
	}

	hits := make([]semanticSearchHit, 0, len(embeddings))
	for _, embedding := range embeddings {
		record, ok := recordMap[embedding.AssetID]
		if !ok || !matchesUnifiedSearchFilters(record, mediaType, assetStatus) {
			continue
		}

		var vector []float64
		if err := json.Unmarshal([]byte(embedding.EmbeddingJSON), &vector); err != nil {
			continue
		}

		score := cosineSimilarity(queryEmbedding.Vector, vector)
		if score < 0.18 {
			continue
		}

		hits = append(hits, semanticSearchHit{
			AssetID:   embedding.AssetID,
			MatchKind: semanticMatchKindForFeature(embedding.FeatureKind),
			Score:     score,
		})
	}

	sort.Slice(hits, func(left, right int) bool {
		if hits[left].Score != hits[right].Score {
			return hits[left].Score > hits[right].Score
		}
		return hits[left].AssetID < hits[right].AssetID
	})

	if len(hits) > limit {
		hits = hits[:limit]
	}

	return hits, "", nil
}

func (service *Service) buildAssetRecordMap(ctx context.Context, assetIDs []string) (map[string]AssetRecord, error) {
	assets, err := service.store.GetAssetsByIDs(ctx, assetIDs)
	if err != nil {
		return nil, err
	}

	records, err := service.buildAssetRecords(ctx, assets)
	if err != nil {
		return nil, err
	}

	recordMap := make(map[string]AssetRecord, len(records))
	for _, record := range records {
		recordMap[record.ID] = record
	}
	return recordMap, nil
}

func ensureUnifiedSearchAccumulator(
	accumulators map[string]*unifiedSearchAccumulator,
	asset AssetRecord,
) *unifiedSearchAccumulator {
	if existing, ok := accumulators[asset.ID]; ok {
		return existing
	}

	created := &unifiedSearchAccumulator{
		asset:      asset,
		matchKinds: make(map[string]struct{}),
	}
	accumulators[asset.ID] = created
	return created
}

func orderedMatchKinds(values map[string]struct{}) []string {
	order := []string{"text", "transcript", "semantic_image", "semantic_video"}
	result := make([]string, 0, len(values))
	for _, key := range order {
		if _, ok := values[key]; ok {
			result = append(result, key)
		}
	}
	for key := range values {
		if !containsString(result, key) {
			result = append(result, key)
		}
	}
	return result
}

func reciprocalRankScore(index int, weight float64) float64 {
	return weight / float64(index+1)
}

func collectSearchHitAssetIDs(hits []store.SearchDocumentHit) []string {
	assetIDs := make([]string, 0, len(hits))
	for _, hit := range hits {
		assetIDs = append(assetIDs, hit.AssetID)
	}
	return assetIDs
}

func collectSemanticHitAssetIDs(hits []semanticSearchHit) []string {
	assetIDs := make([]string, 0, len(hits))
	for _, hit := range hits {
		assetIDs = append(assetIDs, hit.AssetID)
	}
	return assetIDs
}

func collectEmbeddingAssetIDs(embeddings []store.AssetSemanticEmbedding) []string {
	assetIDs := make([]string, 0, len(embeddings))
	for _, embedding := range embeddings {
		assetIDs = append(assetIDs, embedding.AssetID)
	}
	return assetIDs
}

func semanticFeatureKindsForMediaType(mediaType string) []string {
	switch strings.ToLower(strings.TrimSpace(mediaType)) {
	case "", "all":
		return []string{semanticFeatureKindImage, semanticFeatureKindVideo}
	case "image":
		return []string{semanticFeatureKindImage}
	case "video":
		return []string{semanticFeatureKindVideo}
	default:
		return nil
	}
}

func semanticMatchKindForFeature(featureKind string) string {
	switch strings.ToLower(strings.TrimSpace(featureKind)) {
	case semanticFeatureKindImage:
		return "semantic_image"
	case semanticFeatureKindVideo:
		return "semantic_video"
	default:
		return "semantic"
	}
}

func matchesUnifiedSearchFilters(asset AssetRecord, mediaType string, assetStatus string) bool {
	if mediaTypeValue := strings.ToLower(strings.TrimSpace(mediaType)); mediaTypeValue != "" && mediaTypeValue != "all" {
		if strings.ToLower(strings.TrimSpace(asset.MediaType)) != mediaTypeValue {
			return false
		}
	}

	statusValue := strings.ToLower(strings.TrimSpace(assetStatus))
	if statusValue == "" || statusValue == "all" {
		return true
	}

	switch normalizedAssetSearchStatus(asset) {
	case statusValue:
		return true
	default:
		return false
	}
}

func normalizedAssetSearchStatus(asset AssetRecord) string {
	switch strings.ToLower(strings.TrimSpace(asset.AssetStatus)) {
	case "processing", "conflict", "pending_delete", "deleted", "partial":
		return strings.ToLower(strings.TrimSpace(asset.AssetStatus))
	}

	if asset.AvailableReplicaCount == 1 {
		return "single"
	}

	return "ready"
}

func cosineSimilarity(left, right []float64) float64 {
	if len(left) == 0 || len(right) == 0 {
		return 0
	}

	limit := minInt(len(left), len(right))
	var (
		dot       float64
		leftNorm  float64
		rightNorm float64
	)

	for index := 0; index < limit; index++ {
		dot += left[index] * right[index]
		leftNorm += left[index] * left[index]
		rightNorm += right[index] * right[index]
	}

	if leftNorm == 0 || rightNorm == 0 {
		return 0
	}

	return dot / (math.Sqrt(leftNorm) * math.Sqrt(rightNorm))
}

func assetTimestampValue(asset AssetRecord) int64 {
	for _, candidate := range []*time.Time{asset.PrimaryTimestamp, &asset.UpdatedAt, &asset.CreatedAt} {
		if candidate == nil {
			continue
		}
		return candidate.UTC().Unix()
	}
	return 0
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func roundFloat(value float64, precision int) float64 {
	pow := math.Pow10(precision)
	return math.Round(value*pow) / pow
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}
