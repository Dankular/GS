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
	ErrApprovalRequired  = errors.New("a second actor approval is required")
)

type Store struct{ Pool *pgxpool.Pool }

type Approval struct {
	ID          string `json:"approvalId"`
	GameID      string `json:"gameId"`
	Environment string `json:"environment"`
	Revision    int64  `json:"revision"`
	Digest      string `json:"digest"`
	RequestedBy string `json:"requestedBy"`
	ApprovedBy  string `json:"approvedBy,omitempty"`
	Status      string `json:"status"`
}

func (s Store) RequestOrApprove(ctx context.Context, gameID, environment string, revision int64, actorID, reason string) (Approval, error) {
	if s.Pool == nil {
		return Approval{}, errors.New("definition store is not configured")
	}
	if strings.TrimSpace(gameID) == "" || strings.TrimSpace(environment) == "" || revision < 1 || strings.TrimSpace(actorID) == "" {
		return Approval{}, errors.New("approval data is incomplete")
	}
	if strings.TrimSpace(reason) == "" {
		return Approval{}, ErrReasonRequired
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Approval{}, err
	}
	defer tx.Rollback(ctx)
	var digest string
	if err := tx.QueryRow(ctx, `SELECT digest FROM platform.definition_revisions WHERE game_id=$1 AND revision=$2 AND status IN ('published','activated')`, gameID, revision).Scan(&digest); err != nil {
		return Approval{}, err
	}
	var approval Approval
	err = tx.QueryRow(ctx, `SELECT approval_id::text,requested_by,COALESCE(approved_by,''),status FROM platform.definition_approvals WHERE game_id=$1 AND environment=$2 AND revision=$3 AND status IN ('pending','approved') ORDER BY created_at DESC LIMIT 1`, gameID, environment, revision).Scan(&approval.ID, &approval.RequestedBy, &approval.ApprovedBy, &approval.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		err = tx.QueryRow(ctx, `INSERT INTO platform.definition_approvals(game_id,environment,revision,digest,requested_by,status,reason) VALUES($1,$2,$3,$4,$5,'pending',$6) RETURNING approval_id::text`, gameID, environment, revision, digest, actorID, reason).Scan(&approval.ID)
		if err != nil {
			return Approval{}, err
		}
		approval = Approval{ID: approval.ID, GameID: gameID, Environment: environment, Revision: revision, Digest: digest, RequestedBy: actorID, Status: "pending"}
	} else if err != nil {
		return Approval{}, err
	} else if approval.Status == "pending" {
		if approval.RequestedBy == actorID {
			return Approval{}, ErrApprovalRequired
		}
		if _, err := tx.Exec(ctx, `UPDATE platform.definition_approvals SET status='approved',approved_by=$1,approved_at=now(),reason=$2 WHERE approval_id=$3::uuid AND status='pending'`, actorID, reason, approval.ID); err != nil {
			return Approval{}, err
		}
		approval.ApprovedBy, approval.Status = actorID, "approved"
	}
	approval.GameID, approval.Environment, approval.Revision, approval.Digest = gameID, environment, revision, digest
	if _, err := tx.Exec(ctx, `INSERT INTO ops.audit_log(actor_type,actor_id,action,resource_type,resource_id,correlation_id,details) VALUES('admin',$1,$2,'definition',$3,$4,$5::jsonb)`, actorID, "definition.approval."+approval.Status, fmt.Sprintf("%s:%d", gameID, revision), approval.ID, mustJSON(map[string]string{"environment": environment, "reason": reason})); err != nil {
		return Approval{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Approval{}, err
	}
	return approval, nil
}

func (s Store) HasApprovedActivation(ctx context.Context, gameID, environment string, revision int64, approvalID, actorID string) (bool, error) {
	if s.Pool == nil {
		return false, errors.New("definition store is not configured")
	}
	var valid bool
	err := s.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM platform.definition_approvals WHERE approval_id=$1::uuid AND game_id=$2 AND environment=$3 AND revision=$4 AND status='approved' AND approved_by<>$5 AND digest=(SELECT digest FROM platform.definition_revisions WHERE game_id=$2 AND revision=$4))`, approvalID, gameID, environment, revision, actorID).Scan(&valid)
	return valid, err
}

func publishTx(ctx context.Context, tx pgx.Tx, report compiler.Report, source, actorID, reason string) error {
	if strings.TrimSpace(reason) == "" {
		return ErrReasonRequired
	}
	if strings.TrimSpace(actorID) == "" || report.Definition.Metadata.GameID == "" || report.Definition.Metadata.Revision < 1 || len(report.Canonical) == 0 {
		return errors.New("definition publication data is incomplete")
	}
	reportJSON, err := json.Marshal(report)
	if err != nil {
		return err
	}
	d := report.Definition
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
	_, err = tx.Exec(ctx, `INSERT INTO ops.audit_log(actor_type,actor_id,action,resource_type,resource_id,correlation_id,details) VALUES('admin',$1,'definition.publish','definition',$2,$3,$4::jsonb)`, actorID, fmt.Sprintf("%s:%d", d.Metadata.GameID, d.Metadata.Revision), report.Digest, mustJSON(map[string]string{"reason": reason}))
	return err
}

func (s Store) PublishTx(ctx context.Context, tx pgx.Tx, report compiler.Report, source, actorID, reason string) error {
	return publishTx(ctx, tx, report, source, actorID, reason)
}

func (s Store) ActivateTx(ctx context.Context, tx pgx.Tx, gameID, environment string, revision int64, actorID, reason string, rollback bool) error {
	action := "definition.activate"
	if rollback {
		action = "definition.rollback"
	}
	if strings.TrimSpace(reason) == "" {
		return ErrReasonRequired
	}
	if strings.TrimSpace(gameID) == "" || strings.TrimSpace(environment) == "" || revision < 1 || strings.TrimSpace(actorID) == "" {
		return errors.New("activation data is incomplete")
	}
	var digest string
	if err := tx.QueryRow(ctx, `SELECT digest FROM platform.definition_revisions WHERE game_id=$1 AND revision=$2 AND status IN ('published','activated','superseded')`, gameID, revision).Scan(&digest); err != nil {
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
	_, err := tx.Exec(ctx, `INSERT INTO ops.audit_log(actor_type,actor_id,action,resource_type,resource_id,correlation_id,details) VALUES('admin',$1,$2,'definition',$3,$4,$5::jsonb)`, actorID, action, fmt.Sprintf("%s:%d", gameID, revision), digest, mustJSON(map[string]string{"reason": reason, "environment": environment}))
	return err
}

type AuditRecord struct {
	AuditID       string          `json:"auditId"`
	ActorType     string          `json:"actorType"`
	ActorID       string          `json:"actorId"`
	Action        string          `json:"action"`
	ResourceType  string          `json:"resourceType"`
	ResourceID    string          `json:"resourceId"`
	CorrelationID string          `json:"correlationId"`
	Details       json.RawMessage `json:"details"`
	CreatedAt     string          `json:"createdAt"`
	PreviousHash  string          `json:"previousHash,omitempty"`
	RecordHash    string          `json:"recordHash,omitempty"`
}

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
	return s.setActive(ctx, gameID, environment, revision, actorID, reason, authorized, "definition.activate")
}

func (s Store) Rollback(ctx context.Context, gameID, environment string, revision int64, actorID, reason string, authorized bool) error {
	return s.setActive(ctx, gameID, environment, revision, actorID, reason, authorized, "definition.rollback")
}

func (s Store) setActive(ctx context.Context, gameID, environment string, revision int64, actorID, reason string, authorized bool, action string) error {
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
	if _, err := tx.Exec(ctx, `INSERT INTO ops.audit_log(actor_type,actor_id,action,resource_type,resource_id,correlation_id,details) VALUES('admin',$1,$2,'definition',$3,$4,$5::jsonb)`, actorID, action, fmt.Sprintf("%s:%d", gameID, revision), found, mustJSON(map[string]string{"reason": reason, "environment": environment})); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s Store) Audit(ctx context.Context, limit int) ([]AuditRecord, error) {
	if s.Pool == nil {
		return nil, errors.New("definition store is not configured")
	}
	if limit < 1 || limit > 200 {
		limit = 50
	}
	rows, err := s.Pool.Query(ctx, `SELECT audit_id::text,actor_type,actor_id,action,resource_type,resource_id,correlation_id,details,created_at::text,COALESCE(previous_hash,''),COALESCE(record_hash,'') FROM ops.audit_log ORDER BY created_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []AuditRecord
	for rows.Next() {
		var record AuditRecord
		if err := rows.Scan(&record.AuditID, &record.ActorType, &record.ActorID, &record.Action, &record.ResourceType, &record.ResourceID, &record.CorrelationID, &record.Details, &record.CreatedAt, &record.PreviousHash, &record.RecordHash); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
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
