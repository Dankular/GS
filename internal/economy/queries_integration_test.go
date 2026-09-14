//go:build integration

package economy

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestQueriesArePlayerScoped(t *testing.T) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL is required for integration tests")
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	ctx := context.Background()
	_, err = pool.Exec(ctx, `INSERT INTO economy.wallet_accounts(player_id,currency,balance) VALUES('query-player','coins',9) ON CONFLICT(player_id,currency) DO UPDATE SET balance=9`)
	if err != nil {
		t.Fatal(err)
	}
	wallets, err := Wallets(ctx, pool, "query-player")
	if err != nil {
		t.Fatal(err)
	}
	if len(wallets) != 1 || wallets[0].Balance != 9 {
		t.Fatalf("unexpected wallets: %#v", wallets)
	}
	other, err := Wallets(ctx, pool, "other-query-player")
	if err != nil {
		t.Fatal(err)
	}
	if len(other) != 0 {
		t.Fatalf("query leaked another player: %#v", other)
	}
	_, _ = pool.Exec(ctx, `DELETE FROM economy.wallet_accounts WHERE player_id='query-player'`)
}
