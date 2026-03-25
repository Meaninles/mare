package store

import (
	"context"
	"fmt"
	"strings"
	"unicode"
)

type AssetListOptions struct {
	Limit       int
	Offset      int
	SearchQuery string
	MediaType   string
	AssetStatus string
}

func (store *Store) SearchAssets(ctx context.Context, options AssetListOptions) ([]Asset, error) {
	resolved := normalizeAssetListOptions(options)
	if resolved.SearchQuery == "" {
		return store.listAssetsByFilters(ctx, resolved)
	}

	ftsQuery := buildAssetSearchFTSQuery(resolved.SearchQuery)
	if ftsQuery == "" {
		return store.searchAssetsByLike(ctx, resolved)
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
		matched_assets AS (
			SELECT
				asset_id,
				0 AS match_order,
				MIN(bm25(asset_search, 1.0, 0.6)) AS match_rank
			FROM asset_search
			WHERE asset_search MATCH ?
			GROUP BY asset_id

			UNION ALL

			SELECT
				id AS asset_id,
				1 AS match_order,
				0.0 AS match_rank
			FROM assets
			WHERE LOWER(COALESCE(display_name, '')) LIKE ? ESCAPE '\'
			   OR LOWER(COALESCE(logical_path_key, '')) LIKE ? ESCAPE '\'
		),
		deduped_matches AS (
			SELECT
				asset_id,
				MIN(match_order) AS match_order,
				MIN(match_rank) AS match_rank
			FROM matched_assets
			GROUP BY asset_id
		)
		SELECT
			a.id,
			a.logical_path_key,
			a.display_name,
			a.media_type,
			a.asset_status,
			a.primary_timestamp,
			a.primary_thumbnail_id,
			a.created_at,
			a.updated_at
		FROM assets a
		INNER JOIN deduped_matches matches ON matches.asset_id = a.id
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
			a.created_at DESC,
			a.display_name COLLATE NOCASE ASC
		LIMIT ? OFFSET ?`,
		ftsQuery,
		likePattern,
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
		return nil, fmt.Errorf("search assets: %w", err)
	}
	defer rows.Close()

	return scanAssetRows(rows)
}

func (store *Store) listAssetsByFilters(ctx context.Context, options AssetListOptions) ([]Asset, error) {
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
			id,
			logical_path_key,
			display_name,
			media_type,
			asset_status,
			primary_timestamp,
			primary_thumbnail_id,
			created_at,
			updated_at
		FROM assets a
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
			COALESCE(a.primary_timestamp, a.updated_at, a.created_at) DESC,
			a.created_at DESC,
			a.display_name COLLATE NOCASE ASC
		LIMIT ? OFFSET ?`,
		options.MediaType,
		options.MediaType,
		options.AssetStatus,
		options.AssetStatus,
		options.AssetStatus,
		options.AssetStatus,
		options.AssetStatus,
		options.Limit,
		options.Offset,
	)
	if err != nil {
		return nil, fmt.Errorf("list assets by filters: %w", err)
	}
	defer rows.Close()

	return scanAssetRows(rows)
}

func (store *Store) searchAssetsByLike(ctx context.Context, options AssetListOptions) ([]Asset, error) {
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
			id,
			logical_path_key,
			display_name,
			media_type,
			asset_status,
			primary_timestamp,
			primary_thumbnail_id,
			created_at,
			updated_at
		FROM assets a
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
		  AND (
			LOWER(COALESCE(a.display_name, '')) LIKE ? ESCAPE '\'
			OR LOWER(COALESCE(a.logical_path_key, '')) LIKE ? ESCAPE '\'
		  )
		ORDER BY
			COALESCE(a.primary_timestamp, a.updated_at, a.created_at) DESC,
			a.created_at DESC,
			a.display_name COLLATE NOCASE ASC
		LIMIT ? OFFSET ?`,
		options.MediaType,
		options.MediaType,
		options.AssetStatus,
		options.AssetStatus,
		options.AssetStatus,
		options.AssetStatus,
		options.AssetStatus,
		likePattern,
		likePattern,
		options.Limit,
		options.Offset,
	)
	if err != nil {
		return nil, fmt.Errorf("search assets with like: %w", err)
	}
	defer rows.Close()

	return scanAssetRows(rows)
}

func scanAssetRows(rows rowIterator) ([]Asset, error) {
	var assets []Asset
	for rows.Next() {
		asset, scanErr := scanAsset(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		assets = append(assets, asset)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return assets, nil
}

type rowIterator interface {
	rowScanner
	Next() bool
	Err() error
}

func normalizeAssetListOptions(options AssetListOptions) AssetListOptions {
	options.Limit = normalizePositiveInt(options.Limit, 200)
	options.Offset = normalizeNonNegativeInt(options.Offset)
	options.SearchQuery = strings.ToLower(strings.TrimSpace(options.SearchQuery))
	options.MediaType = normalizeAssetFilterValue(options.MediaType)
	options.AssetStatus = normalizeAssetFilterValue(options.AssetStatus)
	return options
}

func normalizeAssetFilterValue(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "all" {
		return ""
	}
	return normalized
}

func normalizePositiveInt(value, fallback int) int {
	if value <= 0 {
		return fallback
	}
	return value
}

func normalizeNonNegativeInt(value int) int {
	if value < 0 {
		return 0
	}
	return value
}

func buildAssetSearchFTSQuery(value string) string {
	parts := strings.FieldsFunc(value, func(r rune) bool {
		switch {
		case unicode.IsSpace(r):
			return true
		case strings.ContainsRune(`/\._-:;,+|[]{}()"'<>=!?@#$%^&~`, r):
			return true
		default:
			return false
		}
	})

	seen := make(map[string]struct{}, len(parts))
	tokens := make([]string, 0, len(parts))
	for _, part := range parts {
		token := sanitizeFTSToken(part)
		if token == "" {
			continue
		}
		if _, exists := seen[token]; exists {
			continue
		}
		seen[token] = struct{}{}
		tokens = append(tokens, token+"*")
	}

	return strings.Join(tokens, " AND ")
}

func sanitizeFTSToken(value string) string {
	var builder strings.Builder
	builder.Grow(len(value))
	for _, r := range value {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r):
			builder.WriteRune(unicode.ToLower(r))
		}
	}
	return builder.String()
}

func buildContainsPattern(value string) string {
	replacer := strings.NewReplacer(
		`\`, `\\`,
		`%`, `\%`,
		`_`, `\_`,
	)
	return "%" + replacer.Replace(strings.ToLower(strings.TrimSpace(value))) + "%"
}
