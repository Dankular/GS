package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Dankular/GameService/internal/commands"
	"github.com/jackc/pgx/v5"
)

type Handler func(context.Context, pgx.Tx, commands.Envelope) (commands.Result, error)

type CommandService struct{ Handler Handler }

func (s CommandService) Handle(ctx context.Context, tx pgx.Tx, e commands.Envelope) (commands.Result, error) {
	if tx == nil {
		return commands.Result{}, errors.New("admin command transaction is required")
	}
	if e.Actor.Type != "admin" {
		return rejected(e, "ADMIN_ACTOR_REQUIRED", "admin commands require an admin actor"), nil
	}
	switch e.Spec.Operation {
	case "admin.execute_command":
		nestedOperation := stringArg(e.Spec.Arguments, "operation")
		if !allowedNestedOperation(nestedOperation) {
			return rejected(e, "NESTED_OPERATION_FORBIDDEN", "operation is not allowed for admin execution"), nil
		}
		target := stringArg(e.Spec.Arguments, "targetPlayerId")
		if target == "" {
			return rejected(e, "INVALID_ARGUMENT", "targetPlayerId is required"), nil
		}
		nestedArguments, ok := e.Spec.Arguments["arguments"].(map[string]any)
		if !ok {
			return rejected(e, "INVALID_ARGUMENT", "arguments must be an object"), nil
		}
		nestedArguments = cloneArguments(nestedArguments)
		nestedArguments["playerId"] = target
		if s.Handler == nil {
			return commands.Result{}, errors.New("admin command handler is not configured")
		}
		nested := commands.Envelope{APIVersion: "game.platform/v1alpha1", Kind: "Command", Metadata: commands.Metadata{RequestID: e.Metadata.RequestID + ":admin", CorrelationID: e.Metadata.CorrelationID, GameID: e.Metadata.GameID, Environment: e.Metadata.Environment, DefinitionRevision: e.Metadata.DefinitionRevision}, Actor: commands.Actor{Type: "admin", ID: e.Actor.ID}, Spec: commands.Spec{Operation: nestedOperation, Arguments: nestedArguments}}
		result, err := s.Handler(ctx, tx, nested)
		if err != nil {
			return commands.Result{}, err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO ops.audit_log(actor_type,actor_id,action,resource_type,resource_id,correlation_id,details) VALUES('admin',$1,'admin.execute_command','player',$2,$3,$4::jsonb)`, e.Actor.ID, target, e.Metadata.CorrelationID, mapJSON(map[string]string{"operation": nestedOperation, "reason": stringArg(e.Spec.Arguments, "reason")})); err != nil {
			return commands.Result{}, err
		}
		if result.Status != "succeeded" {
			return rejected(e, "NESTED_COMMAND_REJECTED", "nested command was rejected"), nil
		}
		return succeeded(e, map[string]any{"targetPlayerId": target, "operation": nestedOperation, "result": result.Result}), nil
	case "admin.player_snapshot":
		playerID := stringArg(e.Spec.Arguments, "playerId")
		if playerID == "" {
			return rejected(e, "INVALID_ARGUMENT", "playerId is required"), nil
		}
		snapshot, err := snapshot(ctx, tx, playerID)
		if err != nil {
			return commands.Result{}, err
		}
		return succeeded(e, map[string]any{"playerId": playerID, "snapshot": snapshot}), nil
	case "admin.audit_search":
		limit := 50
		if value, ok := e.Spec.Arguments["limit"].(json.Number); ok {
			parsed, err := value.Int64()
			if err != nil || parsed < 1 || parsed > 200 {
				return rejected(e, "INVALID_ARGUMENT", "limit must be between 1 and 200"), nil
			}
			limit = int(parsed)
		}
		action := stringArg(e.Spec.Arguments, "action")
		rows, err := tx.Query(ctx, `SELECT audit_id::text,actor_type,actor_id,action,resource_type,resource_id,correlation_id,details,created_at::text FROM ops.audit_log WHERE ($1='' OR action=$1) ORDER BY created_at DESC LIMIT $2`, action, limit)
		if err != nil {
			return commands.Result{}, err
		}
		defer rows.Close()
		records := []map[string]any{}
		for rows.Next() {
			var id, actorType, actorID, recordAction, resourceType, resourceID, correlationID, createdAt string
			var details []byte
			if err := rows.Scan(&id, &actorType, &actorID, &recordAction, &resourceType, &resourceID, &correlationID, &details, &createdAt); err != nil {
				return commands.Result{}, err
			}
			var decoded any
			if err := json.Unmarshal(details, &decoded); err != nil {
				return commands.Result{}, err
			}
			records = append(records, map[string]any{"auditId": id, "actorType": actorType, "actorId": actorID, "action": recordAction, "resourceType": resourceType, "resourceId": resourceID, "correlationId": correlationID, "details": decoded, "createdAt": createdAt})
		}
		if err := rows.Err(); err != nil {
			return commands.Result{}, err
		}
		return succeeded(e, map[string]any{"records": records}), nil
	default:
		return commands.Result{}, fmt.Errorf("unsupported admin operation: %s", e.Spec.Operation)
	}
}

func allowedNestedOperation(operation string) bool {
	switch operation {
	case "inventory.grant", "inventory.consume", "wallet.credit", "wallet.debit", "entitlement.grant", "entitlement.revoke", "progression.add_xp", "progression.complete_objective", "reward.claim":
		return true
	default:
		return false
	}
}

func cloneArguments(input map[string]any) map[string]any {
	output := make(map[string]any, len(input)+1)
	for key, value := range input {
		output[key] = value
	}
	return output
}
func mapJSON(value map[string]string) []byte { data, _ := json.Marshal(value); return data }

func snapshot(ctx context.Context, tx pgx.Tx, playerID string) (map[string]any, error) {
	wallets := []map[string]any{}
	rows, err := tx.Query(ctx, `SELECT currency,balance FROM economy.wallet_accounts WHERE player_id=$1 ORDER BY currency`, playerID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var currency string
		var balance int64
		if err := rows.Scan(&currency, &balance); err != nil {
			rows.Close()
			return nil, err
		}
		wallets = append(wallets, map[string]any{"currency": currency, "balance": balance})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	items := []map[string]any{}
	rows, err = tx.Query(ctx, `SELECT item_id,quantity,version FROM economy.inventory_stacks WHERE player_id=$1 ORDER BY item_id`, playerID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var item string
		var quantity, version int64
		if err := rows.Scan(&item, &quantity, &version); err != nil {
			rows.Close()
			return nil, err
		}
		items = append(items, map[string]any{"itemId": item, "quantity": quantity, "version": version})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	return map[string]any{"wallets": wallets, "inventory": items}, nil
}

func stringArg(args map[string]any, key string) string {
	value, _ := args[key].(string)
	return strings.TrimSpace(value)
}
func succeeded(e commands.Envelope, result map[string]any) commands.Result {
	return commands.Result{RequestID: e.Metadata.RequestID, CorrelationID: e.Metadata.CorrelationID, Status: "succeeded", Operation: e.Spec.Operation, Result: result, Events: []string{"admin.command.accepted.v1"}}
}
func rejected(e commands.Envelope, code, message string) commands.Result {
	return commands.Result{RequestID: e.Metadata.RequestID, CorrelationID: e.Metadata.CorrelationID, Status: "rejected", Operation: e.Spec.Operation, Error: &commands.CommandError{Code: code, Message: message}}
}
