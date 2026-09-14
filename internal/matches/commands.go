package matches

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Dankular/GameService/internal/commands"
	"github.com/jackc/pgx/v5"
)

// CommandService handles match operations that must share the command
// reservation transaction with their match-state mutation.
type CommandService struct {
	Store Store
}

func (s CommandService) Handle(ctx context.Context, tx pgx.Tx, e commands.Envelope) (commands.Result, error) {
	if tx == nil {
		return commands.Result{}, errors.New("match command transaction is required")
	}
	matchID, err := requiredString(e.Spec.Arguments, "matchId")
	if err != nil {
		return rejected(e, "INVALID_ARGUMENT", err.Error()), nil
	}
	switch e.Spec.Operation {
	case "match.get":
		record, err := s.Store.GetTx(ctx, tx, matchID, e.Actor.ID)
		if err != nil {
			return rejected(e, "MATCH_NOT_FOUND", "match was not found for this actor"), nil
		}
		return succeeded(e, map[string]any{"match": record}), nil
	case "match.issue_join_claim":
		token, err := s.Store.IssueJoinClaimTx(ctx, tx, matchID, e.Actor.ID, time.Now())
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return rejected(e, "MATCH_NOT_FOUND", "match was not found for this actor"), nil
			}
			if strings.HasPrefix(err.Error(), "match is not joinable") {
				return rejected(e, "MATCH_NOT_JOINABLE", err.Error()), nil
			}
			return commands.Result{}, err
		}
		return succeeded(e, map[string]any{"matchId": matchID, "token": token}), nil
	case "match.abandon":
		result, err := tx.Exec(ctx, `UPDATE match.matches SET state='Abandoned',state_version=state_version+1,updated_at=now() WHERE match_id=$1 AND state IN ('Ready','Running') AND EXISTS (SELECT 1 FROM match.roster_members WHERE match_id=$1 AND player_id=$2)`, matchID, e.Actor.ID)
		if err != nil {
			return commands.Result{}, err
		}
		if result.RowsAffected() == 0 {
			return rejected(e, "MATCH_NOT_ABANDONABLE", "match was not found, not active, or actor is not a roster member"), nil
		}
		return succeeded(e, map[string]any{"matchId": matchID, "state": Abandoned}), nil
	default:
		return commands.Result{}, fmt.Errorf("unsupported match operation: %s", e.Spec.Operation)
	}
}

func requiredString(arguments map[string]any, name string) (string, error) {
	value, ok := arguments[name].(string)
	if !ok || strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("%s is required", name)
	}
	return value, nil
}

func succeeded(e commands.Envelope, result map[string]any) commands.Result {
	return commands.Result{RequestID: e.Metadata.RequestID, CorrelationID: e.Metadata.CorrelationID, Status: "succeeded", Operation: e.Spec.Operation, Result: result, Events: []string{"match.command.accepted.v1"}}
}

func rejected(e commands.Envelope, code, message string) commands.Result {
	return commands.Result{RequestID: e.Metadata.RequestID, CorrelationID: e.Metadata.CorrelationID, Status: "rejected", Operation: e.Spec.Operation, Error: &commands.CommandError{Code: code, Message: message}}
}
