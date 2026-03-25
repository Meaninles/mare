package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type SearchDocumentHit struct {
	AssetID    string
	SourceKind string
	Snippet    string
}

func (store *Store) SaveAssetTranscript(ctx context.Context, transcript AssetTranscript) error {
	existing, err := store.GetAssetTranscriptByAssetID(ctx, transcript.AssetID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	if errors.Is(err, sql.ErrNoRows) {
		_, execErr := store.db.ExecContext(
			ctx,
			`INSERT INTO asset_transcripts
			(asset_id, transcript_text, language, source_version_id, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?)`,
			transcript.AssetID,
			transcript.TranscriptText,
			toNullableString(transcript.Language),
			toNullableString(transcript.SourceVersionID),
			transcript.CreatedAt.UTC().Format(timeLayout),
			transcript.UpdatedAt.UTC().Format(timeLayout),
		)
		if execErr != nil {
			return fmt.Errorf("insert asset transcript: %w", execErr)
		}
		return nil
	}

	existing.TranscriptText = transcript.TranscriptText
	existing.Language = transcript.Language
	existing.SourceVersionID = transcript.SourceVersionID
	existing.UpdatedAt = transcript.UpdatedAt

	_, execErr := store.db.ExecContext(
		ctx,
		`UPDATE asset_transcripts
		 SET transcript_text = ?, language = ?, source_version_id = ?, updated_at = ?
		 WHERE asset_id = ?`,
		existing.TranscriptText,
		toNullableString(existing.Language),
		toNullableString(existing.SourceVersionID),
		existing.UpdatedAt.UTC().Format(timeLayout),
		existing.AssetID,
	)
	if execErr != nil {
		return fmt.Errorf("update asset transcript: %w", execErr)
	}

	return nil
}

func (store *Store) GetAssetTranscriptByAssetID(ctx context.Context, assetID string) (AssetTranscript, error) {
	row := store.db.QueryRowContext(
		ctx,
		`SELECT asset_id, transcript_text, language, source_version_id, created_at, updated_at
		 FROM asset_transcripts WHERE asset_id = ?`,
		assetID,
	)
	return scanAssetTranscript(row)
}

func (store *Store) SaveAssetSearchDocument(ctx context.Context, document AssetSearchDocument) error {
	existing, err := store.GetAssetSearchDocumentByAssetAndKind(ctx, document.AssetID, document.SourceKind)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	if errors.Is(err, sql.ErrNoRows) {
		_, execErr := store.db.ExecContext(
			ctx,
			`INSERT INTO asset_search_documents
			(id, asset_id, source_kind, content, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?)`,
			document.ID,
			document.AssetID,
			document.SourceKind,
			document.Content,
			document.CreatedAt.UTC().Format(timeLayout),
			document.UpdatedAt.UTC().Format(timeLayout),
		)
		if execErr != nil {
			return fmt.Errorf("insert asset search document: %w", execErr)
		}
		return nil
	}

	existing.Content = document.Content
	existing.UpdatedAt = document.UpdatedAt

	_, execErr := store.db.ExecContext(
		ctx,
		`UPDATE asset_search_documents
		 SET content = ?, updated_at = ?
		 WHERE id = ?`,
		existing.Content,
		existing.UpdatedAt.UTC().Format(timeLayout),
		existing.ID,
	)
	if execErr != nil {
		return fmt.Errorf("update asset search document: %w", execErr)
	}

	return nil
}

func (store *Store) GetAssetSearchDocumentByAssetAndKind(
	ctx context.Context,
	assetID string,
	sourceKind string,
) (AssetSearchDocument, error) {
	row := store.db.QueryRowContext(
		ctx,
		`SELECT id, asset_id, source_kind, content, created_at, updated_at
		 FROM asset_search_documents
		 WHERE asset_id = ? AND source_kind = ?
		 LIMIT 1`,
		assetID,
		sourceKind,
	)
	return scanAssetSearchDocument(row)
}

func (store *Store) SearchTranscriptHits(ctx context.Context, options AssetListOptions) ([]SearchDocumentHit, error) {
	resolved := normalizeAssetListOptions(options)
	if resolved.SearchQuery == "" {
		return nil, nil
	}

	ftsQuery := buildAssetSearchFTSQuery(resolved.SearchQuery)
	if ftsQuery == "" {
		return store.searchTranscriptHitsByLike(ctx, resolved)
	}

	likePattern := buildContainsPattern(resolved.SearchQuery)
	rows, err := store.db.QueryContext(
		ctx,
		`WITH replica_counts AS (
			SELECT
				asset_id,
				SUM(CASE WHEN exists_flag = 1 THEN 1 ELSE 0 END) AS available_count
			FROM replicas
			GROUP BY asset_id
		),
		matches AS (
			SELECT
				doc.asset_id,
				doc.source_kind,
				snippet(asset_search_documents_fts, 2, '「', '」', '…', 12) AS snippet,
				0 AS match_order,
				bm25(asset_search_documents_fts) AS match_rank
			FROM asset_search_documents_fts
			INNER JOIN asset_search_documents doc ON doc.rowid = asset_search_documents_fts.rowid
			WHERE asset_search_documents_fts MATCH ?
			  AND doc.source_kind = 'transcript'

			UNION ALL

			SELECT
				doc.asset_id,
				doc.source_kind,
				SUBSTR(doc.content, 1, 160) AS snippet,
				1 AS match_order,
				0.0 AS match_rank
			FROM asset_search_documents doc
			WHERE doc.source_kind = 'transcript'
			  AND LOWER(COALESCE(doc.content, '')) LIKE ? ESCAPE '\'
		)
		SELECT
			matches.asset_id,
			matches.source_kind,
			matches.snippet
		FROM matches
		INNER JOIN assets a ON a.id = matches.asset_id
		LEFT JOIN replica_counts ON replica_counts.asset_id = a.id
		WHERE LOWER(COALESCE(a.asset_status, '')) <> 'deleted'
		  AND (? = '' OR LOWER(COALESCE(a.media_type, '')) = ?)
		  AND (
			? = ''
			OR (? = 'single'
				AND LOWER(COALESCE(a.asset_status, '')) NOT IN ('processing', 'conflict', 'pending_delete', 'deleted', 'partial')
				AND COALESCE(replica_counts.available_count, 0) = 1)
			OR (? = 'ready'
				AND LOWER(COALESCE(a.asset_status, '')) NOT IN ('processing', 'conflict', 'pending_delete', 'deleted', 'partial')
				AND COALESCE(replica_counts.available_count, 0) > 1)
			OR (? NOT IN ('', 'single', 'ready')
				AND LOWER(COALESCE(a.asset_status, '')) = ?)
		  )
		ORDER BY
			matches.match_order ASC,
			matches.match_rank ASC,
			COALESCE(a.primary_timestamp, a.updated_at, a.created_at) DESC,
			a.created_at DESC
		LIMIT ? OFFSET ?`,
		ftsQuery,
		likePattern,
		resolved.MediaType,
		resolved.MediaType,
		resolved.AssetStatus,
		resolved.AssetStatus,
		resolved.AssetStatus,
		resolved.AssetStatus,
		resolved.AssetStatus,
		resolved.Limit,
		resolved.Offset,
	)
	if err != nil {
		return nil, fmt.Errorf("search transcript hits: %w", err)
	}
	defer rows.Close()

	return scanSearchDocumentHits(rows)
}

func (store *Store) searchTranscriptHitsByLike(ctx context.Context, options AssetListOptions) ([]SearchDocumentHit, error) {
	likePattern := buildContainsPattern(options.SearchQuery)
	rows, err := store.db.QueryContext(
		ctx,
		`WITH replica_counts AS (
			SELECT
				asset_id,
				SUM(CASE WHEN exists_flag = 1 THEN 1 ELSE 0 END) AS available_count
			FROM replicas
			GROUP BY asset_id
		)
		SELECT
			doc.asset_id,
			doc.source_kind,
			SUBSTR(doc.content, 1, 160) AS snippet
		FROM asset_search_documents doc
		INNER JOIN assets a ON a.id = doc.asset_id
		LEFT JOIN replica_counts ON replica_counts.asset_id = a.id
		WHERE doc.source_kind = 'transcript'
		  AND LOWER(COALESCE(a.asset_status, '')) <> 'deleted'
		  AND (? = '' OR LOWER(COALESCE(a.media_type, '')) = ?)
		  AND (
			? = ''
			OR (? = 'single'
				AND LOWER(COALESCE(a.asset_status, '')) NOT IN ('processing', 'conflict', 'pending_delete', 'deleted', 'partial')
				AND COALESCE(replica_counts.available_count, 0) = 1)
			OR (? = 'ready'
				AND LOWER(COALESCE(a.asset_status, '')) NOT IN ('processing', 'conflict', 'pending_delete', 'deleted', 'partial')
				AND COALESCE(replica_counts.available_count, 0) > 1)
			OR (? NOT IN ('', 'single', 'ready')
				AND LOWER(COALESCE(a.asset_status, '')) = ?)
		  )
		  AND LOWER(COALESCE(doc.content, '')) LIKE ? ESCAPE '\'
		ORDER BY
			COALESCE(a.primary_timestamp, a.updated_at, a.created_at) DESC,
			a.created_at DESC
		LIMIT ? OFFSET ?`,
		options.MediaType,
		options.MediaType,
		options.AssetStatus,
		options.AssetStatus,
		options.AssetStatus,
		options.AssetStatus,
		options.AssetStatus,
		likePattern,
		options.Limit,
		options.Offset,
	)
	if err != nil {
		return nil, fmt.Errorf("search transcript hits by like: %w", err)
	}
	defer rows.Close()

	return scanSearchDocumentHits(rows)
}

func (store *Store) SaveAssetSemanticEmbedding(ctx context.Context, embedding AssetSemanticEmbedding) error {
	existing, err := store.GetAssetSemanticEmbeddingByAssetAndKind(ctx, embedding.AssetID, embedding.FeatureKind)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	if errors.Is(err, sql.ErrNoRows) {
		_, execErr := store.db.ExecContext(
			ctx,
			`INSERT INTO asset_semantic_embeddings
			(id, asset_id, feature_kind, model_name, embedding_json, source_version_id, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			embedding.ID,
			embedding.AssetID,
			embedding.FeatureKind,
			embedding.ModelName,
			embedding.EmbeddingJSON,
			toNullableString(embedding.SourceVersionID),
			embedding.CreatedAt.UTC().Format(timeLayout),
			embedding.UpdatedAt.UTC().Format(timeLayout),
		)
		if execErr != nil {
			return fmt.Errorf("insert asset semantic embedding: %w", execErr)
		}
		return nil
	}

	existing.ModelName = embedding.ModelName
	existing.EmbeddingJSON = embedding.EmbeddingJSON
	existing.SourceVersionID = embedding.SourceVersionID
	existing.UpdatedAt = embedding.UpdatedAt

	_, execErr := store.db.ExecContext(
		ctx,
		`UPDATE asset_semantic_embeddings
		 SET model_name = ?, embedding_json = ?, source_version_id = ?, updated_at = ?
		 WHERE id = ?`,
		existing.ModelName,
		existing.EmbeddingJSON,
		toNullableString(existing.SourceVersionID),
		existing.UpdatedAt.UTC().Format(timeLayout),
		existing.ID,
	)
	if execErr != nil {
		return fmt.Errorf("update asset semantic embedding: %w", execErr)
	}

	return nil
}

func (store *Store) GetAssetSemanticEmbeddingByAssetAndKind(
	ctx context.Context,
	assetID string,
	featureKind string,
) (AssetSemanticEmbedding, error) {
	row := store.db.QueryRowContext(
		ctx,
		`SELECT id, asset_id, feature_kind, model_name, embedding_json, source_version_id, created_at, updated_at
		 FROM asset_semantic_embeddings
		 WHERE asset_id = ? AND feature_kind = ?
		 LIMIT 1`,
		assetID,
		featureKind,
	)
	return scanAssetSemanticEmbedding(row)
}

func (store *Store) ListAssetSemanticEmbeddings(ctx context.Context, featureKinds []string) ([]AssetSemanticEmbedding, error) {
	if len(featureKinds) == 0 {
		return nil, nil
	}

	normalizedKinds := uniqueLowerStrings(featureKinds)
	placeholders := strings.TrimRight(strings.Repeat("?,", len(normalizedKinds)), ",")
	args := make([]any, 0, len(normalizedKinds))
	for _, kind := range normalizedKinds {
		args = append(args, kind)
	}

	rows, err := store.db.QueryContext(
		ctx,
		fmt.Sprintf(
			`SELECT id, asset_id, feature_kind, model_name, embedding_json, source_version_id, created_at, updated_at
			 FROM asset_semantic_embeddings
			 WHERE LOWER(feature_kind) IN (%s)
			 ORDER BY updated_at DESC`,
			placeholders,
		),
		args...,
	)
	if err != nil {
		return nil, fmt.Errorf("list asset semantic embeddings: %w", err)
	}
	defer rows.Close()

	var embeddings []AssetSemanticEmbedding
	for rows.Next() {
		embedding, scanErr := scanAssetSemanticEmbedding(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		embeddings = append(embeddings, embedding)
	}

	return embeddings, rows.Err()
}

func (store *Store) GetAssetsByIDs(ctx context.Context, assetIDs []string) ([]Asset, error) {
	if len(assetIDs) == 0 {
		return nil, nil
	}

	orderedIDs := uniqueTrimmedStrings(assetIDs)
	placeholders := strings.TrimRight(strings.Repeat("?,", len(orderedIDs)), ",")
	args := make([]any, 0, len(orderedIDs))
	for _, assetID := range orderedIDs {
		args = append(args, assetID)
	}

	rows, err := store.db.QueryContext(
		ctx,
		fmt.Sprintf(
			`SELECT id, logical_path_key, display_name, media_type, asset_status, primary_timestamp, primary_thumbnail_id, created_at, updated_at
			 FROM assets
			 WHERE id IN (%s)`,
			placeholders,
		),
		args...,
	)
	if err != nil {
		return nil, fmt.Errorf("get assets by ids: %w", err)
	}
	defer rows.Close()

	assets, err := scanAssetRows(rows)
	if err != nil {
		return nil, err
	}

	lookup := make(map[string]Asset, len(assets))
	for _, asset := range assets {
		lookup[asset.ID] = asset
	}

	orderedAssets := make([]Asset, 0, len(orderedIDs))
	for _, assetID := range orderedIDs {
		asset, ok := lookup[assetID]
		if !ok {
			continue
		}
		orderedAssets = append(orderedAssets, asset)
	}

	return orderedAssets, nil
}

func scanAssetTranscript(scanner rowScanner) (AssetTranscript, error) {
	var (
		transcript          AssetTranscript
		languageText        sql.NullString
		sourceVersionIDText sql.NullString
		createdAtText       string
		updatedAtText       string
	)

	if err := scanner.Scan(
		&transcript.AssetID,
		&transcript.TranscriptText,
		&languageText,
		&sourceVersionIDText,
		&createdAtText,
		&updatedAtText,
	); err != nil {
		return AssetTranscript{}, fmt.Errorf("scan asset transcript: %w", err)
	}

	createdAt, err := time.Parse(timeLayout, createdAtText)
	if err != nil {
		return AssetTranscript{}, fmt.Errorf("parse asset transcript created_at: %w", err)
	}

	updatedAt, err := time.Parse(timeLayout, updatedAtText)
	if err != nil {
		return AssetTranscript{}, fmt.Errorf("parse asset transcript updated_at: %w", err)
	}

	transcript.Language = parseNullableString(languageText)
	transcript.SourceVersionID = parseNullableString(sourceVersionIDText)
	transcript.CreatedAt = createdAt
	transcript.UpdatedAt = updatedAt
	return transcript, nil
}

func scanAssetSearchDocument(scanner rowScanner) (AssetSearchDocument, error) {
	var (
		document      AssetSearchDocument
		createdAtText string
		updatedAtText string
	)

	if err := scanner.Scan(
		&document.ID,
		&document.AssetID,
		&document.SourceKind,
		&document.Content,
		&createdAtText,
		&updatedAtText,
	); err != nil {
		return AssetSearchDocument{}, fmt.Errorf("scan asset search document: %w", err)
	}

	createdAt, err := time.Parse(timeLayout, createdAtText)
	if err != nil {
		return AssetSearchDocument{}, fmt.Errorf("parse asset search document created_at: %w", err)
	}

	updatedAt, err := time.Parse(timeLayout, updatedAtText)
	if err != nil {
		return AssetSearchDocument{}, fmt.Errorf("parse asset search document updated_at: %w", err)
	}

	document.CreatedAt = createdAt
	document.UpdatedAt = updatedAt
	return document, nil
}

func scanSearchDocumentHits(rows rowIterator) ([]SearchDocumentHit, error) {
	var hits []SearchDocumentHit
	for rows.Next() {
		var hit SearchDocumentHit
		if err := rows.Scan(&hit.AssetID, &hit.SourceKind, &hit.Snippet); err != nil {
			return nil, fmt.Errorf("scan search document hit: %w", err)
		}
		hits = append(hits, hit)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return hits, nil
}

func scanAssetSemanticEmbedding(scanner rowScanner) (AssetSemanticEmbedding, error) {
	var (
		embedding           AssetSemanticEmbedding
		sourceVersionIDText sql.NullString
		createdAtText       string
		updatedAtText       string
	)

	if err := scanner.Scan(
		&embedding.ID,
		&embedding.AssetID,
		&embedding.FeatureKind,
		&embedding.ModelName,
		&embedding.EmbeddingJSON,
		&sourceVersionIDText,
		&createdAtText,
		&updatedAtText,
	); err != nil {
		return AssetSemanticEmbedding{}, fmt.Errorf("scan asset semantic embedding: %w", err)
	}

	createdAt, err := time.Parse(timeLayout, createdAtText)
	if err != nil {
		return AssetSemanticEmbedding{}, fmt.Errorf("parse asset semantic embedding created_at: %w", err)
	}

	updatedAt, err := time.Parse(timeLayout, updatedAtText)
	if err != nil {
		return AssetSemanticEmbedding{}, fmt.Errorf("parse asset semantic embedding updated_at: %w", err)
	}

	embedding.SourceVersionID = parseNullableString(sourceVersionIDText)
	embedding.CreatedAt = createdAt
	embedding.UpdatedAt = updatedAt
	return embedding, nil
}

func decodeEmbeddingJSON(value string) ([]float64, error) {
	var embedding []float64
	if err := json.Unmarshal([]byte(value), &embedding); err != nil {
		return nil, err
	}
	return embedding, nil
}

func uniqueLowerStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		normalized := strings.ToLower(strings.TrimSpace(value))
		if normalized == "" {
			continue
		}
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		result = append(result, normalized)
	}
	return result
}

func uniqueTrimmedStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		normalized := strings.TrimSpace(value)
		if normalized == "" {
			continue
		}
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		result = append(result, normalized)
	}
	return result
}
