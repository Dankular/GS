package economy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/Dankular/GameService/internal/commands"
	"github.com/jackc/pgx/v5"
)

var ErrInvalidArgument = errors.New("invalid economy argument")

type Service struct{}

func (Service) Handle(ctx context.Context, tx pgx.Tx, e commands.Envelope) (commands.Result, error) {
	args := e.Spec.Arguments
	player, err := stringArg(args, "playerId", e.Actor.ID)
	if err != nil {
		return commands.Result{}, err
	}
	switch e.Spec.Operation {
	case "wallet.get":
		return walletGet(ctx, tx, player, e)
	case "wallet.credit":
		return walletChange(ctx, tx, player, args, e, true)
	case "wallet.debit":
		return walletChange(ctx, tx, player, args, e, false)
	case "inventory.list":
		return inventoryList(ctx, tx, player, e)
	case "inventory.grant":
		return inventoryChange(ctx, tx, player, args, e, true)
	case "inventory.consume":
		return inventoryChange(ctx, tx, player, args, e, false)
	default:
		return commands.Result{RequestID: e.Metadata.RequestID, CorrelationID: e.Metadata.CorrelationID, Operation: e.Spec.Operation, Status: "succeeded", Result: map[string]any{"accepted": true}}, nil
	}
}

func stringArg(args map[string]any, key, fallback string) (string, error) {
	if v, ok := args[key]; ok {
		s, ok := v.(string)
		if !ok || strings.TrimSpace(s) == "" {
			return "", fmt.Errorf("%w: %s", ErrInvalidArgument, key)
		}
		return s, nil
	}
	if fallback == "" {
		return "", fmt.Errorf("%w: %s is required", ErrInvalidArgument, key)
	}
	return fallback, nil
}
func intArg(args map[string]any, key string) (int64, error) {
	v, ok := args[key]
	if !ok {
		return 0, fmt.Errorf("%w: %s is required", ErrInvalidArgument, key)
	}
	var n int64
	switch x := v.(type) {
	case json.Number:
		var err error
		n, err = x.Int64()
		if err != nil {
			return 0, fmt.Errorf("%w: %s", ErrInvalidArgument, key)
		}
	case float64:
		if x != math.Trunc(x) || x > math.MaxInt64 || x < math.MinInt64 {
			return 0, fmt.Errorf("%w: %s", ErrInvalidArgument, key)
		}
		n = int64(x)
	default:
		return 0, fmt.Errorf("%w: %s", ErrInvalidArgument, key)
	}
	if n <= 0 {
		return 0, fmt.Errorf("%w: %s must be positive", ErrInvalidArgument, key)
	}
	return n, nil
}

func base(e commands.Envelope, result map[string]any, event string) commands.Result {
	return commands.Result{RequestID: e.Metadata.RequestID, CorrelationID: e.Metadata.CorrelationID, Status: "succeeded", Operation: e.Spec.Operation, Result: result, Events: []string{event}}
}

func walletGet(ctx context.Context, tx pgx.Tx, player string, e commands.Envelope) (commands.Result, error) {
	currency, err := stringArg(e.Spec.Arguments, "currency", "")
	if err != nil {
		return commands.Result{}, err
	}
	var balance int64
	err = tx.QueryRow(ctx, `SELECT balance FROM economy.wallet_accounts WHERE player_id=$1 AND currency=$2`, player, currency).Scan(&balance)
	if errors.Is(err, pgx.ErrNoRows) {
		balance = 0
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return commands.Result{}, err
	}
	return base(e, map[string]any{"playerId": player, "currency": currency, "balance": balance}, "wallet.read.v1"), nil
}

func walletChange(ctx context.Context, tx pgx.Tx, player string, args map[string]any, e commands.Envelope, credit bool) (commands.Result, error) {
	currency, err := stringArg(args, "currency", "")
	if err != nil {
		return commands.Result{}, err
	}
	amount, err := intArg(args, "amount")
	if err != nil {
		return commands.Result{}, err
	}
	var balance int64
	if credit {
		err = tx.QueryRow(ctx, `INSERT INTO economy.wallet_accounts(player_id,currency,balance) VALUES($1,$2,$3) ON CONFLICT(player_id,currency) DO UPDATE SET balance=economy.wallet_accounts.balance+$3 RETURNING balance`, player, currency, amount).Scan(&balance)
	} else {
		err = tx.QueryRow(ctx, `UPDATE economy.wallet_accounts SET balance=balance-$3 WHERE player_id=$1 AND currency=$2 AND balance >= $3 RETURNING balance`, player, currency, amount).Scan(&balance)
		if errors.Is(err, pgx.ErrNoRows) {
			return commands.Result{}, fmt.Errorf("INSUFFICIENT_FUNDS")
		}
	}
	if err != nil {
		return commands.Result{}, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO economy.ledger_transactions(request_id,reason) VALUES($1,$2)`, e.Metadata.RequestID, e.Spec.Operation)
	if err != nil {
		return commands.Result{}, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO economy.ledger_entries(request_id,player_id,currency,amount) VALUES($1,$2,$3,$4)`, e.Metadata.RequestID, player, currency, func() int64 {
		if credit {
			return amount
		}
		return -amount
	}())
	if err != nil {
		return commands.Result{}, err
	}
	event := "wallet.debited.v1"
	if credit {
		event = "wallet.credited.v1"
	}
	return base(e, map[string]any{"playerId": player, "currency": currency, "amount": amount, "balance": balance}, event), nil
}

func inventoryList(ctx context.Context, tx pgx.Tx, player string, e commands.Envelope) (commands.Result, error) {
	rows, err := tx.Query(ctx, `SELECT item_id,quantity,version FROM economy.inventory_stacks WHERE player_id=$1 AND quantity>0 ORDER BY item_id`, player)
	if err != nil {
		return commands.Result{}, err
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id string
		var q, v int64
		if err := rows.Scan(&id, &q, &v); err != nil {
			return commands.Result{}, err
		}
		items = append(items, map[string]any{"itemId": id, "quantity": q, "version": v})
	}
	if err := rows.Err(); err != nil {
		return commands.Result{}, err
	}
	return base(e, map[string]any{"playerId": player, "items": items}, "inventory.listed.v1"), nil
}

func inventoryChange(ctx context.Context, tx pgx.Tx, player string, args map[string]any, e commands.Envelope, grant bool) (commands.Result, error) {
	item, err := stringArg(args, "itemId", "")
	if err != nil {
		return commands.Result{}, err
	}
	q, err := intArg(args, "quantity")
	if err != nil {
		return commands.Result{}, err
	}
	var quantity, version int64
	if grant {
		err = tx.QueryRow(ctx, `INSERT INTO economy.inventory_stacks(player_id,item_id,quantity,version) VALUES($1,$2,$3,1) ON CONFLICT(player_id,item_id) DO UPDATE SET quantity=economy.inventory_stacks.quantity+$3,version=economy.inventory_stacks.version+1 RETURNING quantity,version`, player, item, q).Scan(&quantity, &version)
	} else {
		err = tx.QueryRow(ctx, `UPDATE economy.inventory_stacks SET quantity=quantity-$3,version=version+1 WHERE player_id=$1 AND item_id=$2 AND quantity >= $3 RETURNING quantity,version`, player, item, q).Scan(&quantity, &version)
		if errors.Is(err, pgx.ErrNoRows) {
			return commands.Result{}, fmt.Errorf("INSUFFICIENT_ITEMS")
		}
	}
	if err != nil {
		return commands.Result{}, err
	}
	event := "inventory.consumed.v1"
	if grant {
		event = "inventory.granted.v1"
	}
	return base(e, map[string]any{"playerId": player, "itemId": item, "quantity": q, "newQuantity": quantity, "version": version}, event), nil
}
