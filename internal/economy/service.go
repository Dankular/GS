package economy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/Dankular/GameService/internal/commands"
	"github.com/Dankular/GameService/internal/compiler"
	"github.com/jackc/pgx/v5"
)

var ErrInvalidArgument = errors.New("invalid economy argument")

type Service struct{}

func (Service) Handle(ctx context.Context, tx pgx.Tx, e commands.Envelope) (result commands.Result, err error) {
	defer func() {
		if err == nil {
			return
		}
		code, ok := businessErrorCode(err)
		if !ok {
			return
		}
		result = commands.Result{RequestID: e.Metadata.RequestID, CorrelationID: e.Metadata.CorrelationID, Operation: e.Spec.Operation, Status: "rejected", Error: &commands.CommandError{Code: code, Message: err.Error(), Retryable: false}}
		err = nil
	}()
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
	case "wallet.transfer":
		return walletTransfer(ctx, tx, player, args, e)
	case "inventory.list":
		return inventoryList(ctx, tx, player, e)
	case "inventory.grant":
		return inventoryChange(ctx, tx, player, args, e, true)
	case "inventory.consume":
		return inventoryChange(ctx, tx, player, args, e, false)
	case "inventory.transfer":
		return inventoryTransfer(ctx, tx, player, args, e)
	case "entitlement.list":
		return entitlementList(ctx, tx, player, e)
	case "entitlement.grant":
		return entitlementChange(ctx, tx, player, args, e, true)
	case "entitlement.revoke":
		return entitlementChange(ctx, tx, player, args, e, false)
	case "progression.get":
		return progressionGet(ctx, tx, player, args, e)
	case "progression.add_xp":
		return progressionAddXP(ctx, tx, player, args, e)
	case "progression.complete_objective":
		return objectiveComplete(ctx, tx, player, args, e)
	case "reward.preview":
		return rewardPreview(ctx, tx, player, args, e)
	case "reward.claim":
		return rewardClaim(ctx, tx, player, args, e)
	default:
		return commands.Result{RequestID: e.Metadata.RequestID, CorrelationID: e.Metadata.CorrelationID, Operation: e.Spec.Operation, Status: "rejected", Error: &commands.CommandError{Code: "UNSUPPORTED_OPERATION", Message: "operation is registered but not implemented", Retryable: false}}, nil
	}
}

func businessErrorCode(err error) (string, bool) {
	if errors.Is(err, ErrInvalidArgument) {
		return "INVALID_ARGUMENT", true
	}
	message := err.Error()
	switch {
	case strings.Contains(message, "INSUFFICIENT_FUNDS"):
		return "INSUFFICIENT_FUNDS", true
	case strings.Contains(message, "INSUFFICIENT_ITEMS"):
		return "INSUFFICIENT_ITEMS", true
	case strings.HasPrefix(message, "reward not found"):
		return "REWARD_NOT_FOUND", true
	case strings.HasPrefix(message, "invalid reward"):
		return "INVALID_REWARD", true
	case strings.HasPrefix(message, "unknown currency"):
		return "UNKNOWN_CURRENCY", true
	case strings.HasPrefix(message, "unknown item"):
		return "UNKNOWN_ITEM", true
	case strings.Contains(message, "currency bounds"):
		return "CURRENCY_LIMIT", true
	case strings.Contains(message, "stack limit"):
		return "STACK_LIMIT", true
	default:
		return "", false
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

const systemLedgerAccount = "__system__"

type ledgerEntry struct {
	Account string
	Amount  int64
}

func balancedLedgerEntries(player string, amount int64) []ledgerEntry {
	return []ledgerEntry{{Account: player, Amount: amount}, {Account: systemLedgerAccount, Amount: -amount}}
}

func appendLedgerEntries(ctx context.Context, tx pgx.Tx, requestID, player, currency string, amount int64) error {
	for _, entry := range balancedLedgerEntries(player, amount) {
		if _, err := tx.Exec(ctx, `INSERT INTO economy.ledger_entries(request_id,player_id,currency,amount) VALUES($1,$2,$3,$4)`, requestID, entry.Account, currency, entry.Amount); err != nil {
			return err
		}
	}
	return nil
}

func appendTransferLedgerEntries(ctx context.Context, tx pgx.Tx, requestID, from, to, currency string, amount int64) error {
	entries := []ledgerEntry{{Account: from, Amount: -amount}, {Account: to, Amount: amount}}
	for _, entry := range entries {
		if _, err := tx.Exec(ctx, `INSERT INTO economy.ledger_entries(request_id,player_id,currency,amount) VALUES($1,$2,$3,$4)`, requestID, entry.Account, currency, entry.Amount); err != nil {
			return err
		}
	}
	return nil
}

func transferTarget(args map[string]any, player string) (string, error) {
	target, err := stringArg(args, "targetPlayerId", "")
	if err != nil {
		return "", err
	}
	if target == player {
		return "", fmt.Errorf("%w: targetPlayerId must differ from playerId", ErrInvalidArgument)
	}
	return target, nil
}

func walletGet(ctx context.Context, tx pgx.Tx, player string, e commands.Envelope) (commands.Result, error) {
	currency, err := stringArg(e.Spec.Arguments, "currency", "")
	if err != nil {
		return commands.Result{}, err
	}
	definition, err := loadDefinition(ctx, tx, e)
	if err != nil {
		return commands.Result{}, err
	}
	currencySpec, ok := findCurrency(definition, currency)
	if !ok {
		return commands.Result{}, fmt.Errorf("unknown currency: %s", currency)
	}
	var balance int64
	err = tx.QueryRow(ctx, `SELECT balance FROM economy.wallet_accounts WHERE player_id=$1 AND currency=$2`, player, currency).Scan(&balance)
	if errors.Is(err, pgx.ErrNoRows) {
		balance = 0
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return commands.Result{}, err
	}
	if balance < currencySpec.MinBalance || balance > currencySpec.MaxBalance {
		return commands.Result{}, fmt.Errorf("wallet balance violates currency bounds")
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
	definition, err := loadDefinition(ctx, tx, e)
	if err != nil {
		return commands.Result{}, err
	}
	currencySpec, ok := findCurrency(definition, currency)
	if !ok {
		return commands.Result{}, fmt.Errorf("unknown currency: %s", currency)
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
	if balance < currencySpec.MinBalance || balance > currencySpec.MaxBalance {
		return commands.Result{}, fmt.Errorf("wallet balance violates currency bounds")
	}
	_, err = tx.Exec(ctx, `INSERT INTO economy.ledger_transactions(request_id,reason) VALUES($1,$2)`, e.Metadata.RequestID, e.Spec.Operation)
	if err != nil {
		return commands.Result{}, err
	}
	amountSigned := amount
	if !credit {
		amountSigned = -amount
	}
	if err = appendLedgerEntries(ctx, tx, e.Metadata.RequestID, player, currency, amountSigned); err != nil {
		return commands.Result{}, err
	}
	event := "wallet.debited.v1"
	if credit {
		event = "wallet.credited.v1"
	}
	return base(e, map[string]any{"playerId": player, "currency": currency, "amount": amount, "balance": balance}, event), nil
}

func walletTransfer(ctx context.Context, tx pgx.Tx, player string, args map[string]any, e commands.Envelope) (commands.Result, error) {
	target, err := transferTarget(args, player)
	if err != nil {
		return commands.Result{}, err
	}
	currency, err := stringArg(args, "currency", "")
	if err != nil {
		return commands.Result{}, err
	}
	amount, err := intArg(args, "amount")
	if err != nil {
		return commands.Result{}, err
	}
	definition, err := loadDefinition(ctx, tx, e)
	if err != nil {
		return commands.Result{}, err
	}
	currencySpec, ok := findCurrency(definition, currency)
	if !ok {
		return commands.Result{}, fmt.Errorf("unknown currency: %s", currency)
	}
	rows, err := tx.Query(ctx, `SELECT player_id,balance FROM economy.wallet_accounts WHERE currency=$1 AND player_id=ANY($2) ORDER BY player_id FOR UPDATE`, currency, []string{player, target})
	if err != nil {
		return commands.Result{}, err
	}
	balances := map[string]int64{}
	for rows.Next() {
		var id string
		var balance int64
		if err := rows.Scan(&id, &balance); err != nil {
			rows.Close()
			return commands.Result{}, err
		}
		balances[id] = balance
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return commands.Result{}, err
	}
	rows.Close()
	fromBalance, ok := balances[player]
	if !ok || fromBalance < amount {
		return commands.Result{}, fmt.Errorf("INSUFFICIENT_FUNDS")
	}
	toBalance := balances[target] + amount
	if toBalance < currencySpec.MinBalance || toBalance > currencySpec.MaxBalance {
		return commands.Result{}, fmt.Errorf("wallet balance violates currency bounds")
	}
	if _, err := tx.Exec(ctx, `UPDATE economy.wallet_accounts SET balance=balance-$3 WHERE player_id=$1 AND currency=$2`, player, currency, amount); err != nil {
		return commands.Result{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO economy.wallet_accounts(player_id,currency,balance) VALUES($1,$2,$3) ON CONFLICT(player_id,currency) DO UPDATE SET balance=economy.wallet_accounts.balance+$3`, target, currency, amount); err != nil {
		return commands.Result{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO economy.ledger_transactions(request_id,reason) VALUES($1,$2)`, e.Metadata.RequestID, e.Spec.Operation); err != nil {
		return commands.Result{}, err
	}
	if err := appendTransferLedgerEntries(ctx, tx, e.Metadata.RequestID, player, target, currency, amount); err != nil {
		return commands.Result{}, err
	}
	return base(e, map[string]any{"fromPlayerId": player, "targetPlayerId": target, "currency": currency, "amount": amount, "fromBalance": fromBalance - amount, "targetBalance": toBalance}, "wallet.transferred.v1"), nil
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
	definition, err := loadDefinition(ctx, tx, e)
	if err != nil {
		return commands.Result{}, err
	}
	itemSpec, ok := findItem(definition, item)
	if !ok {
		return commands.Result{}, fmt.Errorf("unknown item: %s", item)
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
	if quantity > itemSpec.StackLimit {
		return commands.Result{}, fmt.Errorf("item stack limit exceeded")
	}
	event := "inventory.consumed.v1"
	if grant {
		event = "inventory.granted.v1"
	}
	return base(e, map[string]any{"playerId": player, "itemId": item, "quantity": q, "newQuantity": quantity, "version": version}, event), nil
}

func inventoryTransfer(ctx context.Context, tx pgx.Tx, player string, args map[string]any, e commands.Envelope) (commands.Result, error) {
	target, err := transferTarget(args, player)
	if err != nil {
		return commands.Result{}, err
	}
	item, err := stringArg(args, "itemId", "")
	if err != nil {
		return commands.Result{}, err
	}
	quantity, err := intArg(args, "quantity")
	if err != nil {
		return commands.Result{}, err
	}
	definition, err := loadDefinition(ctx, tx, e)
	if err != nil {
		return commands.Result{}, err
	}
	itemSpec, ok := findItem(definition, item)
	if !ok {
		return commands.Result{}, fmt.Errorf("unknown item: %s", item)
	}
	rows, err := tx.Query(ctx, `SELECT player_id,quantity FROM economy.inventory_stacks WHERE item_id=$1 AND player_id=ANY($2) ORDER BY player_id FOR UPDATE`, item, []string{player, target})
	if err != nil {
		return commands.Result{}, err
	}
	quantities := map[string]int64{}
	for rows.Next() {
		var id string
		var value int64
		if err := rows.Scan(&id, &value); err != nil {
			rows.Close()
			return commands.Result{}, err
		}
		quantities[id] = value
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return commands.Result{}, err
	}
	rows.Close()
	fromQuantity, ok := quantities[player]
	if !ok || fromQuantity < quantity {
		return commands.Result{}, fmt.Errorf("INSUFFICIENT_ITEMS")
	}
	targetQuantity := quantities[target] + quantity
	if targetQuantity > itemSpec.StackLimit {
		return commands.Result{}, fmt.Errorf("item stack limit exceeded")
	}
	if _, err := tx.Exec(ctx, `UPDATE economy.inventory_stacks SET quantity=quantity-$3,version=version+1 WHERE player_id=$1 AND item_id=$2`, player, item, quantity); err != nil {
		return commands.Result{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO economy.inventory_stacks(player_id,item_id,quantity,version) VALUES($1,$2,$3,1) ON CONFLICT(player_id,item_id) DO UPDATE SET quantity=economy.inventory_stacks.quantity+$3,version=economy.inventory_stacks.version+1`, target, item, quantity); err != nil {
		return commands.Result{}, err
	}
	return base(e, map[string]any{"fromPlayerId": player, "targetPlayerId": target, "itemId": item, "quantity": quantity, "fromQuantity": fromQuantity - quantity, "targetQuantity": targetQuantity}, "inventory.transferred.v1"), nil
}

func entitlementList(ctx context.Context, tx pgx.Tx, player string, e commands.Envelope) (commands.Result, error) {
	rows, err := tx.Query(ctx, `SELECT entitlement_id,COALESCE(expires_at::text,'') FROM economy.entitlements WHERE player_id=$1 AND revoked_at IS NULL AND (expires_at IS NULL OR expires_at>now()) ORDER BY entitlement_id`, player)
	if err != nil {
		return commands.Result{}, err
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id string
		var expiresAt string
		if err := rows.Scan(&id, &expiresAt); err != nil {
			return commands.Result{}, err
		}
		item := map[string]any{"entitlementId": id}
		if expiresAt != "" {
			item["expiresAt"] = expiresAt
		}
		items = append(items, item)
	}
	return base(e, map[string]any{"playerId": player, "entitlements": items}, "entitlement.listed.v1"), rows.Err()
}

func entitlementChange(ctx context.Context, tx pgx.Tx, player string, args map[string]any, e commands.Envelope, grant bool) (commands.Result, error) {
	id, err := stringArg(args, "entitlementId", "")
	if err != nil {
		return commands.Result{}, err
	}
	if grant {
		_, err = tx.Exec(ctx, `INSERT INTO economy.entitlements(player_id,entitlement_id) VALUES($1,$2) ON CONFLICT(player_id,entitlement_id) DO UPDATE SET revoked_at=NULL`, player, id)
	} else {
		_, err = tx.Exec(ctx, `UPDATE economy.entitlements SET revoked_at=now() WHERE player_id=$1 AND entitlement_id=$2`, player, id)
	}
	if err != nil {
		return commands.Result{}, err
	}
	event := "entitlement.revoked.v1"
	if grant {
		event = "entitlement.granted.v1"
	}
	return base(e, map[string]any{"playerId": player, "entitlementId": id}, event), nil
}

func progressionGet(ctx context.Context, tx pgx.Tx, player string, args map[string]any, e commands.Envelope) (commands.Result, error) {
	track, err := stringArg(args, "trackId", "default")
	if err != nil {
		return commands.Result{}, err
	}
	var xp, level int64
	err = tx.QueryRow(ctx, `SELECT xp,level FROM progression.player_progress WHERE player_id=$1 AND track_id=$2`, player, track).Scan(&xp, &level)
	if errors.Is(err, pgx.ErrNoRows) {
		xp, level = 0, 1
		err = nil
	}
	if err != nil {
		return commands.Result{}, err
	}
	return base(e, map[string]any{"playerId": player, "trackId": track, "xp": xp, "level": level}, "progression.read.v1"), nil
}

func progressionAddXP(ctx context.Context, tx pgx.Tx, player string, args map[string]any, e commands.Envelope) (commands.Result, error) {
	track, err := stringArg(args, "trackId", "default")
	if err != nil {
		return commands.Result{}, err
	}
	xp, err := intArg(args, "amount")
	if err != nil {
		return commands.Result{}, err
	}
	var total, level int64
	err = tx.QueryRow(ctx, `INSERT INTO progression.player_progress(player_id,track_id,xp,level,version) VALUES($1,$2,$3,($3/100)+1,1) ON CONFLICT(player_id,track_id) DO UPDATE SET xp=progression.player_progress.xp+$3,level=((progression.player_progress.xp+$3)/100)+1,version=progression.player_progress.version+1 RETURNING xp,level`, player, track, xp).Scan(&total, &level)
	if err != nil {
		return commands.Result{}, err
	}
	return base(e, map[string]any{"playerId": player, "trackId": track, "amount": xp, "xp": total, "level": level}, "progression.updated.v1"), nil
}

func objectiveComplete(ctx context.Context, tx pgx.Tx, player string, args map[string]any, e commands.Envelope) (commands.Result, error) {
	objective, err := stringArg(args, "objectiveId", "")
	if err != nil {
		return commands.Result{}, err
	}
	source, err := stringArg(args, "sourceId", e.Metadata.RequestID)
	if err != nil {
		return commands.Result{}, err
	}
	result, err := tx.Exec(ctx, `INSERT INTO progression.objective_completions(player_id,objective_id,source_id) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, player, objective, source)
	if err != nil {
		return commands.Result{}, err
	}
	return base(e, map[string]any{"playerId": player, "objectiveId": objective, "sourceId": source, "duplicate": result.RowsAffected() == 0}, "progression.objective.completed.v1"), nil
}

func rewardPreview(ctx context.Context, tx pgx.Tx, player string, args map[string]any, e commands.Envelope) (commands.Result, error) {
	reward, err := loadReward(ctx, tx, e, args)
	if err != nil {
		return commands.Result{}, err
	}
	return base(e, map[string]any{"playerId": player, "rewardId": reward.ID, "grants": reward.Grants, "oncePerPlayer": reward.OncePerPlayer}, "reward.previewed.v1"), nil
}

func rewardClaim(ctx context.Context, tx pgx.Tx, player string, args map[string]any, e commands.Envelope) (commands.Result, error) {
	reward, err := loadReward(ctx, tx, e, args)
	if err != nil {
		return commands.Result{}, err
	}
	source, err := stringArg(args, "sourceId", "")
	if err != nil {
		return commands.Result{}, err
	}
	if reward.OncePerPlayer {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, player+":"+reward.ID); err != nil {
			return commands.Result{}, err
		}
		var claimed bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM economy.reward_claims WHERE player_id=$1 AND reward_id=$2)`, player, reward.ID).Scan(&claimed); err != nil {
			return commands.Result{}, err
		}
		if claimed {
			return base(e, map[string]any{"playerId": player, "rewardId": reward.ID, "duplicate": true}, "reward.claimed.v1"), nil
		}
	}
	result, err := tx.Exec(ctx, `INSERT INTO economy.reward_claims(player_id,reward_id,source_id,request_id) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING`, player, reward.ID, source, e.Metadata.RequestID)
	if err != nil {
		return commands.Result{}, err
	}
	if result.RowsAffected() == 0 {
		return base(e, map[string]any{"playerId": player, "rewardId": reward.ID, "duplicate": true}, "reward.claimed.v1"), nil
	}
	for index, grant := range reward.Grants {
		if grant.Currency != "" {
			amount := grant.Amount
			if amount < 1 {
				return commands.Result{}, fmt.Errorf("invalid reward currency grant")
			}
			if _, err := tx.Exec(ctx, `INSERT INTO economy.wallet_accounts(player_id,currency,balance) VALUES($1,$2,$3) ON CONFLICT(player_id,currency) DO UPDATE SET balance=economy.wallet_accounts.balance+$3`, player, grant.Currency, amount); err != nil {
				return commands.Result{}, err
			}
			requestID := fmt.Sprintf("%s:reward:%d", e.Metadata.RequestID, index)
			if _, err := tx.Exec(ctx, `INSERT INTO economy.ledger_transactions(request_id,reason) VALUES($1,$2)`, requestID, "reward.claim"); err != nil {
				return commands.Result{}, err
			}
			if err := appendLedgerEntries(ctx, tx, requestID, player, grant.Currency, amount); err != nil {
				return commands.Result{}, err
			}
		} else if grant.Item != "" {
			if _, err := tx.Exec(ctx, `INSERT INTO economy.inventory_stacks(player_id,item_id,quantity,version) VALUES($1,$2,$3,1) ON CONFLICT(player_id,item_id) DO UPDATE SET quantity=economy.inventory_stacks.quantity+$3,version=economy.inventory_stacks.version+1`, player, grant.Item, grant.Quantity); err != nil {
				return commands.Result{}, err
			}
		}
	}
	return base(e, map[string]any{"playerId": player, "rewardId": reward.ID, "duplicate": false}, "reward.claimed.v1"), nil
}

func loadReward(ctx context.Context, tx pgx.Tx, e commands.Envelope, args map[string]any) (compiler.Reward, error) {
	rewardID, err := stringArg(args, "rewardId", "")
	if err != nil {
		return compiler.Reward{}, err
	}
	definition, err := loadDefinition(ctx, tx, e)
	if err != nil {
		return compiler.Reward{}, err
	}
	for _, reward := range definition.Spec.Rewards {
		if reward.ID == rewardID {
			return reward, nil
		}
	}
	return compiler.Reward{}, fmt.Errorf("reward not found: %s", rewardID)
}

func loadDefinition(ctx context.Context, tx pgx.Tx, e commands.Envelope) (compiler.Definition, error) {
	var canonical []byte
	if err := tx.QueryRow(ctx, `SELECT canonical FROM platform.definition_revisions WHERE game_id=$1 AND revision=$2`, e.Metadata.GameID, e.Metadata.DefinitionRevision).Scan(&canonical); err != nil {
		return compiler.Definition{}, err
	}
	var definition compiler.Definition
	if err := json.Unmarshal(canonical, &definition); err != nil {
		return compiler.Definition{}, fmt.Errorf("decode published definition: %w", err)
	}
	return definition, nil
}

func findCurrency(definition compiler.Definition, id string) (compiler.Currency, bool) {
	for _, currency := range definition.Spec.Catalog.Currencies {
		if currency.ID == id {
			return currency, true
		}
	}
	return compiler.Currency{}, false
}

func findItem(definition compiler.Definition, id string) (compiler.Item, bool) {
	for _, item := range definition.Spec.Catalog.Items {
		if item.ID == id {
			return item, true
		}
	}
	return compiler.Item{}, false
}
