package reconciliation

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Finding is a projection mismatch. Reconciliation is deliberately read-only:
// corrections must be compensating ledger transactions, never projection edits.
type Finding struct {
	PlayerID      string `json:"playerId"`
	Currency      string `json:"currency"`
	WalletBalance int64  `json:"walletBalance"`
	LedgerBalance int64  `json:"ledgerBalance"`
	Difference    int64  `json:"difference"`
}

// Scan compares every wallet projection with the sum of its player ledger
// entries. It also reports ledger accounts that have no wallet projection.
func Scan(ctx context.Context, pool *pgxpool.Pool) ([]Finding, error) {
	rows, err := pool.Query(ctx, `
		WITH ledger AS (
			SELECT player_id, currency, SUM(amount)::bigint AS balance
			FROM economy.ledger_entries
			WHERE player_id <> '__system__'
			GROUP BY player_id, currency
		)
		SELECT COALESCE(w.player_id, l.player_id),
		       COALESCE(w.currency, l.currency),
		       COALESCE(w.balance, 0),
		       COALESCE(l.balance, 0)
		FROM economy.wallet_accounts w
		FULL OUTER JOIN ledger l ON l.player_id=w.player_id AND l.currency=w.currency
		WHERE COALESCE(w.balance, 0) <> COALESCE(l.balance, 0)
		ORDER BY 1, 2`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var findings []Finding
	for rows.Next() {
		var finding Finding
		if err := rows.Scan(&finding.PlayerID, &finding.Currency, &finding.WalletBalance, &finding.LedgerBalance); err != nil {
			return nil, err
		}
		finding.Difference = finding.WalletBalance - finding.LedgerBalance
		findings = append(findings, finding)
	}
	return findings, rows.Err()
}
