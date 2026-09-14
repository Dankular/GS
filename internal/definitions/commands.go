package definitions

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Dankular/GameService/internal/commands"
	"github.com/Dankular/GameService/internal/compiler"
	"github.com/jackc/pgx/v5"
)

type CommandService struct{ Store Store }

func (s CommandService) Handle(ctx context.Context, tx pgx.Tx, e commands.Envelope) (commands.Result, error) {
	if tx == nil {
		return commands.Result{}, errors.New("definition command transaction is required")
	}
	if e.Actor.Type != "admin" {
		return definitionReject(e, "ADMIN_ACTOR_REQUIRED", "definition commands require an admin actor"), nil
	}
	args := e.Spec.Arguments
	switch e.Spec.Operation {
	case "definition.validate":
		report, err := compileSource(args)
		if err != nil {
			return definitionReject(e, "INVALID_DEFINITION", err.Error()), nil
		}
		return definitionSuccess(e, map[string]any{"report": report}), nil
	case "definition.diff":
		report, err := compileSource(args)
		if err != nil {
			return definitionReject(e, "INVALID_DEFINITION", err.Error()), nil
		}
		gameID := stringArg(args, "gameId", report.Definition.Metadata.GameID)
		revision, err := int64Arg(args, "revision", 0)
		if err != nil {
			return definitionReject(e, "INVALID_ARGUMENT", err.Error()), nil
		}
		var canonical []byte
		if err := tx.QueryRow(ctx, `SELECT canonical FROM platform.definition_revisions WHERE game_id=$1 AND revision=$2`, gameID, revision).Scan(&canonical); err != nil {
			return definitionReject(e, "DEFINITION_NOT_FOUND", "target definition revision was not found"), nil
		}
		var target compiler.Definition
		if err := json.Unmarshal(canonical, &target); err != nil {
			return commands.Result{}, fmt.Errorf("decode target definition: %w", err)
		}
		targetReport := compiler.Report{Definition: target, Canonical: canonical, Digest: compilerDigest(canonical)}
		return definitionSuccess(e, map[string]any{"diff": compiler.Diff(targetReport, report), "fromDigest": targetReport.Digest, "toDigest": report.Digest}), nil
	case "definition.publish":
		report, err := compileSource(args)
		if err != nil {
			return definitionReject(e, "INVALID_DEFINITION", err.Error()), nil
		}
		if err := s.Store.PublishTx(ctx, tx, report, stringArg(args, "source", ""), e.Actor.ID, stringArg(args, "reason", "")); err != nil {
			return definitionStoreError(e, err), nil
		}
		return definitionSuccess(e, map[string]any{"gameId": report.Definition.Metadata.GameID, "revision": report.Definition.Metadata.Revision, "digest": report.Digest, "status": "published"}), nil
	case "definition.activate", "definition.rollback":
		gameID := stringArg(args, "gameId", "")
		environment := stringArg(args, "environment", "")
		revision, err := int64Arg(args, "revision", 0)
		if err != nil {
			return definitionReject(e, "INVALID_ARGUMENT", err.Error()), nil
		}
		if err := s.Store.ActivateTx(ctx, tx, gameID, environment, revision, e.Actor.ID, stringArg(args, "reason", ""), e.Spec.Operation == "definition.rollback"); err != nil {
			return definitionStoreError(e, err), nil
		}
		return definitionSuccess(e, map[string]any{"gameId": gameID, "environment": environment, "revision": revision, "status": "activated"}), nil
	default:
		return commands.Result{}, fmt.Errorf("unsupported definition operation: %s", e.Spec.Operation)
	}
}

func compileSource(args map[string]any) (compiler.Report, error) {
	source := stringArg(args, "source", "")
	if strings.TrimSpace(source) == "" {
		return compiler.Report{}, errors.New("source is required")
	}
	return compiler.Compile(strings.NewReader(source))
}

func compilerDigest(canonical []byte) string {
	digest := sha256.Sum256(canonical)
	return fmt.Sprintf("sha256:%x", digest)
}

func stringArg(args map[string]any, key, fallback string) string {
	if value, ok := args[key].(string); ok {
		return strings.TrimSpace(value)
	}
	return fallback
}

func int64Arg(args map[string]any, key string, fallback int64) (int64, error) {
	value, ok := args[key]
	if !ok {
		if fallback != 0 {
			return fallback, nil
		}
		return 0, fmt.Errorf("%s is required", key)
	}
	switch v := value.(type) {
	case json.Number:
		n, err := v.Int64()
		if err != nil {
			return 0, fmt.Errorf("%s must be an integer", key)
		}
		return n, nil
	case float64:
		if v != float64(int64(v)) {
			return 0, fmt.Errorf("%s must be an integer", key)
		}
		return int64(v), nil
	default:
		return 0, fmt.Errorf("%s must be an integer", key)
	}
}

func definitionSuccess(e commands.Envelope, result map[string]any) commands.Result {
	return commands.Result{RequestID: e.Metadata.RequestID, CorrelationID: e.Metadata.CorrelationID, Status: "succeeded", Operation: e.Spec.Operation, Result: result, Events: []string{"definition.command.accepted.v1"}}
}
func definitionReject(e commands.Envelope, code, message string) commands.Result {
	return commands.Result{RequestID: e.Metadata.RequestID, CorrelationID: e.Metadata.CorrelationID, Status: "rejected", Operation: e.Spec.Operation, Error: &commands.CommandError{Code: code, Message: message}}
}
func definitionStoreError(e commands.Envelope, err error) commands.Result {
	code := "DEFINITION_OPERATION_FAILED"
	if errors.Is(err, ErrImmutableConflict) {
		code = "IMMUTABLE_CONFLICT"
	}
	if errors.Is(err, ErrReasonRequired) {
		code = "REASON_REQUIRED"
	}
	if errors.Is(err, pgx.ErrNoRows) {
		code = "DEFINITION_NOT_FOUND"
	}
	return definitionReject(e, code, err.Error())
}
