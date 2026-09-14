package definitions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Dankular/GameService/internal/compiler"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrUnauthorized      = errors.New("definition operation is unauthorized")
	ErrImmutableConflict = errors.New("definition revision is immutable and conflicts with existing content")
	ErrReasonRequired    = errors.New("definition operation reason is required")
)

type Store struct{ Pool *pgxpool.Pool }

func (s Store) Publish(ctx context.Context, report compiler.Report, source, actorID, reason string, authorized bool) error {
	if !authorized {
		return ErrUnauthorized
	}
	if strings.TrimSpace(reason) == "" {
		return ErrReasonRequired
	}
	if s.Pool == nil {
		return errors.New("definition store is not configured")
	}
	if strings.TrimSpace(actorID) == "" || report.Definition.Metadata.GameID == "" || report.Definition.Metadata.Revision < 1 || len(report.Canonical) == 0 {
		return errors.New("definition publication data is incomplete")
	}
	reportJSON, err := json.Marshal(report)
	if err != nil {
		return err
	}
	d := report.Definition
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `INSERT INTO platform.games(game_id) VALUES($1) ON CONFLICT DO NOTHING`, d.Metadata.GameID); err != nil {
		return err
	}
	result, err := tx.Exec(ctx, `INSERT INTO platform.definition_revisions(game_id,revision,digest,source_yaml,canonical,compiled,validation_report,actor_id,status) VALUES($1,$2,$3,$4,$5::jsonb,$6::jsonb,$7::jsonb,$8,'published') ON CONFLICT(game_id,revision) DO NOTHING`, d.Metadata.GameID, d.Metadata.Revision, report.Digest, source, report.Canonical, report.Canonical, reportJSON, actorID)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		var digest string
		if err := tx.QueryRow(ctx, `SELECT digest FROM platform.definition_revisions WHERE game_id=$1 AND revision=$2`, d.Metadata.GameID, d.Metadata.Revision).Scan(&digest); err != nil {
			return err
		}
		if digest != report.Digest {
			return ErrImmutableConflict
		}
	}
	if _, err := tx.Exec(ctx, `INSERT INTO ops.audit_log(actor_type,actor_id,action,resource_type,resource_id,correlation_id,details) VALUES('admin',$1,'definition.publish','definition',$2,$3,$4::jsonb)`, actorID, fmt.Sprintf("%s:%d", d.Metadata.GameID, d.Metadata.Revision), report.Digest, mustJSON(map[string]string{"reason": reason})); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s Store) Activate(ctx context.Context, gameID, environment string, revision int64, actorID, reason string, authorized bool) error {
	if !authorized {
		return ErrUnauthorized
	}
	if strings.TrimSpace(reason) == "" {
		return ErrReasonRequired
	}
	if s.Pool == nil {
		return errors.New("definition store is not configured")
	}
	if strings.TrimSpace(gameID) == "" || strings.TrimSpace(environment) == "" || revision < 1 || strings.TrimSpace(actorID) == "" {
		return errors.New("activation data is incomplete")
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var found string
	if err := tx.QueryRow(ctx, `SELECT digest FROM platform.definition_revisions WHERE game_id=$1 AND revision=$2 AND status IN ('published','activated')`, gameID, revision).Scan(&found); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO platform.environments(game_id,environment) VALUES($1,$2) ON CONFLICT DO NOTHING`, gameID, environment); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE platform.definition_revisions SET status='superseded' WHERE game_id=$1 AND revision <> $2 AND status='activated'`, gameID, revision); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE platform.definition_revisions SET status='activated' WHERE game_id=$1 AND revision=$2`, gameID, revision); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO platform.definition_activations(game_id,environment,revision,activated_by) VALUES($1,$2,$3,$4) ON CONFLICT(game_id,environment) DO UPDATE SET revision=EXCLUDED.revision,activated_by=EXCLUDED.activated_by,activated_at=now()`, gameID, environment, revision, actorID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO ops.audit_log(actor_type,actor_id,action,resource_type,resource_id,correlation_id,details) VALUES('admin',$1,'definition.activate','definition',$2,$3,$4::jsonb)`, actorID, fmt.Sprintf("%s:%d", gameID, revision), found, mustJSON(map[string]string{"reason": reason, "environment": environment})); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func mustJSON(value any) []byte { data, _ := json.Marshal(value); return data }

func (s Store) Active(ctx context.Context, gameID, environment string) (int64, error) {
	if s.Pool == nil {
		return 0, errors.New("definition store is not configured")
	}
	var revision int64
	err := s.Pool.QueryRow(ctx, `SELECT revision FROM platform.definition_activations WHERE game_id=$1 AND environment=$2`, gameID, environment).Scan(&revision)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, fmt.Errorf("active definition not found: %w", err)
	}
	return revision, err
}
