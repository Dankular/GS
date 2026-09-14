package economy

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Wallet struct {
	Currency string `json:"currency"`
	Balance  int64  `json:"balance"`
}
type InventoryItem struct {
	ItemID   string `json:"itemId"`
	Quantity int64  `json:"quantity"`
	Version  int64  `json:"version"`
}

func Wallets(ctx context.Context, pool *pgxpool.Pool, playerID string) ([]Wallet, error) {
	if pool == nil || strings.TrimSpace(playerID) == "" {
		return nil, errors.New("economy read query is not configured")
	}
	rows, err := pool.Query(ctx, `SELECT currency,balance FROM economy.wallet_accounts WHERE player_id=$1 ORDER BY currency`, playerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Wallet{}
	for rows.Next() {
		var wallet Wallet
		if err := rows.Scan(&wallet.Currency, &wallet.Balance); err != nil {
			return nil, err
		}
		result = append(result, wallet)
	}
	return result, rows.Err()
}

func Inventory(ctx context.Context, pool *pgxpool.Pool, playerID string) ([]InventoryItem, error) {
	if pool == nil || strings.TrimSpace(playerID) == "" {
		return nil, errors.New("economy read query is not configured")
	}
	rows, err := pool.Query(ctx, `SELECT item_id,quantity,version FROM economy.inventory_stacks WHERE player_id=$1 AND quantity>0 ORDER BY item_id`, playerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []InventoryItem{}
	for rows.Next() {
		var item InventoryItem
		if err := rows.Scan(&item.ItemID, &item.Quantity, &item.Version); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func Snapshot(ctx context.Context, pool *pgxpool.Pool, playerID string) (map[string]any, error) {
	wallets, err := Wallets(ctx, pool, playerID)
	if err != nil {
		return nil, err
	}
	inventory, err := Inventory(ctx, pool, playerID)
	if err != nil {
		return nil, err
	}
	return map[string]any{"playerId": playerID, "wallets": wallets, "inventory": inventory}, nil
}
