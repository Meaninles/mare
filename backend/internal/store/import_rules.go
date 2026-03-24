package store

import (
	"context"
	"fmt"
	"time"
)

func (store *Store) ListImportRules(ctx context.Context) ([]ImportRule, error) {
	rows, err := store.db.QueryContext(
		ctx,
		`SELECT id, rule_type, match_value, target_endpoint_ids, created_at, updated_at
		 FROM import_rules
		 ORDER BY rule_type ASC, match_value ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list import rules: %w", err)
	}
	defer rows.Close()

	var rules []ImportRule
	for rows.Next() {
		rule, scanErr := scanImportRule(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		rules = append(rules, rule)
	}

	return rules, rows.Err()
}

func (store *Store) ReplaceImportRules(ctx context.Context, rules []ImportRule) error {
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin import rule transaction: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM import_rules`); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("delete import rules: %w", err)
	}

	for _, rule := range rules {
		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO import_rules
			(id, rule_type, match_value, target_endpoint_ids, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?)`,
			rule.ID,
			rule.RuleType,
			rule.MatchValue,
			rule.TargetEndpointIDs,
			rule.CreatedAt.UTC().Format(timeLayout),
			rule.UpdatedAt.UTC().Format(timeLayout),
		); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("insert import rule: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit import rules: %w", err)
	}

	return nil
}

func scanImportRule(scanner rowScanner) (ImportRule, error) {
	var (
		rule          ImportRule
		createdAtText string
		updatedAtText string
	)

	if err := scanner.Scan(
		&rule.ID,
		&rule.RuleType,
		&rule.MatchValue,
		&rule.TargetEndpointIDs,
		&createdAtText,
		&updatedAtText,
	); err != nil {
		return ImportRule{}, fmt.Errorf("scan import rule: %w", err)
	}

	createdAt, err := time.Parse(timeLayout, createdAtText)
	if err != nil {
		return ImportRule{}, fmt.Errorf("parse import rule created_at: %w", err)
	}

	updatedAt, err := time.Parse(timeLayout, updatedAtText)
	if err != nil {
		return ImportRule{}, fmt.Errorf("parse import rule updated_at: %w", err)
	}

	rule.CreatedAt = createdAt
	rule.UpdatedAt = updatedAt
	return rule, nil
}
