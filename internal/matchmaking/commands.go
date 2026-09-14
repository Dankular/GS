package matchmaking

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Dankular/GameService/internal/commands"
	"github.com/jackc/pgx/v5"
)

type CommandService struct{ Store Store }

func (s CommandService) Handle(ctx context.Context, tx pgx.Tx, e commands.Envelope) (commands.Result, error) {
	if tx == nil {
		return commands.Result{}, errors.New("matchmaking command transaction is required")
	}
	args := e.Spec.Arguments
	switch e.Spec.Operation {
	case "matchmaking.enqueue":
		var restricted bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM platform.player_restrictions WHERE player_id=$1 AND (expires_at IS NULL OR expires_at>now()) AND kind IN ('ban','queue'))`, e.Actor.ID).Scan(&restricted); err != nil {
			return commands.Result{}, err
		}
		if restricted {
			return reject(e, "PLAYER_RESTRICTED", "player is not allowed to enter matchmaking"), nil
		}
		expires, err := time.Parse(time.RFC3339, stringArg(args, "expiresAt"))
		if err != nil {
			return reject(e, "INVALID_ARGUMENT", "expiresAt must be RFC3339"), nil
		}
		players, ok := stringSlice(args["playerIds"])
		if !ok {
			return reject(e, "INVALID_ARGUMENT", "playerIds must be an array of strings"), nil
		}
		request := TicketRequest{GameID: stringArg(args, "gameId"), Environment: stringArg(args, "environment"), ModeID: stringArg(args, "modeId"), DefinitionRevision: int64Arg(args, "definitionRevision"), Build: stringArg(args, "build"), Region: stringArg(args, "region"), Capacity: intArg(args, "capacity"), PlayerIDs: players, Properties: mapArg(args["properties"]), ExpiresAt: expires}
		if e.Actor.Type == "player" {
			if err := request.ValidatePlayerTicket(e.Actor.ID, time.Now()); err != nil {
				return reject(e, "INVALID_TICKET", err.Error()), nil
			}
		}
		record, err := s.Store.CreateTx(ctx, tx, request, e.Actor.ID, time.Now())
		if err != nil {
			if errors.Is(err, ErrActiveTicket) {
				return reject(e, "ACTIVE_TICKET", err.Error()), nil
			}
			if errors.Is(err, ErrInvalidTicket) {
				return reject(e, "INVALID_TICKET", err.Error()), nil
			}
			return commands.Result{}, err
		}
		return success(e, map[string]any{"ticket": record}), nil
	case "matchmaking.status":
		ticketID := stringArg(args, "ticketId")
		record, err := s.Store.GetTx(ctx, tx, ticketID, e.Actor.ID)
		if err != nil {
			return reject(e, "TICKET_NOT_FOUND", "ticket was not found for this actor"), nil
		}
		return success(e, map[string]any{"ticket": record}), nil
	case "matchmaking.cancel":
		if err := s.Store.CancelTx(ctx, tx, stringArg(args, "ticketId"), e.Actor.ID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return reject(e, "TICKET_NOT_CANCELLABLE", "ticket was not found or is not queued for this actor"), nil
			}
			return commands.Result{}, err
		}
		return success(e, map[string]any{"cancelled": true}), nil
	default:
		return commands.Result{}, fmt.Errorf("unsupported matchmaking operation: %s", e.Spec.Operation)
	}
}

func stringArg(args map[string]any, name string) string {
	value, _ := args[name].(string)
	return strings.TrimSpace(value)
}
func intArg(args map[string]any, name string) int {
	switch value := args[name].(type) {
	case int:
		return value
	case float64:
		return int(value)
	case fmt.Stringer:
		var n int
		_, _ = fmt.Sscan(value.String(), &n)
		return n
	}
	return 0
}

func int64Arg(args map[string]any, name string) int64 { return int64(intArg(args, name)) }
func stringSlice(value any) ([]string, bool) {
	raw, ok := value.([]any)
	if !ok {
		if typed, yes := value.([]string); yes {
			return typed, true
		}
		return nil, false
	}
	result := make([]string, len(raw))
	for i, item := range raw {
		var yes bool
		result[i], yes = item.(string)
		if !yes {
			return nil, false
		}
	}
	return result, true
}
func mapArg(value any) map[string]any { result, _ := value.(map[string]any); return result }
func success(e commands.Envelope, data map[string]any) commands.Result {
	return commands.Result{RequestID: e.Metadata.RequestID, CorrelationID: e.Metadata.CorrelationID, Status: "succeeded", Operation: e.Spec.Operation, Result: data, Events: []string{"matchmaking.command.accepted.v1"}}
}
func reject(e commands.Envelope, code, message string) commands.Result {
	return commands.Result{RequestID: e.Metadata.RequestID, CorrelationID: e.Metadata.CorrelationID, Status: "rejected", Operation: e.Spec.Operation, Error: &commands.CommandError{Code: code, Message: message}}
}
