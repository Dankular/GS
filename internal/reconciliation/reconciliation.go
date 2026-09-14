package reconciliation

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// RecoverStaleAllocations makes an allocation timeout visible and prevents a
// failed server bootstrap from leaving players permanently matched. The
// allocator itself remains the owner of server shutdown; this transaction
// handles only durable GameService state and notification.
func RecoverStaleAllocations(ctx context.Context, pool *pgxpool.Pool, cutoff time.Time) (int, error) {
	if pool == nil {
		return 0, nil
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `UPDATE match.matches SET state='Failed',state_version=state_version+1,updated_at=now() WHERE state='Allocating' AND updated_at<$1 RETURNING match_id`, cutoff)
	if err != nil {
		return 0, err
	}
	var matchIDs []string
	for rows.Next() {
		var matchID string
		if err := rows.Scan(&matchID); err != nil {
			rows.Close()
			return 0, err
		}
		matchIDs = append(matchIDs, matchID)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()
	for _, matchID := range matchIDs {
		if _, err := tx.Exec(ctx, `UPDATE match.tickets SET status='expired' WHERE match_id=$1 AND status='matched'`, matchID); err != nil {
			return 0, err
		}
		payload, err := json.Marshal(map[string]any{"matchId": matchID, "reason": "allocation_timeout"})
		if err != nil {
			return 0, err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO ops.outbox_events(aggregate_type,aggregate_id,event_type,correlation_id,payload) VALUES('match',$1,'match.failed.v1',$1,$2::jsonb)`, matchID, payload); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return len(matchIDs), nil
}

// RecoverStaleRunningMatches applies the server-crash policy to matches whose
// authenticated heartbeat has stopped. It only changes durable match state;
// Agones remains responsible for the underlying GameServer lifecycle.
func RecoverStaleRunningMatches(ctx context.Context, pool *pgxpool.Pool, cutoff time.Time) (int, error) {
	if pool == nil {
		return 0, nil
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `UPDATE match.matches SET state='Abandoned',state_version=state_version+1,updated_at=now() WHERE state='Running' AND updated_at<$1 RETURNING match_id`, cutoff)
	if err != nil {
		return 0, err
	}
	var matchIDs []string
	for rows.Next() {
		var matchID string
		if err := rows.Scan(&matchID); err != nil {
			rows.Close()
			return 0, err
		}
		matchIDs = append(matchIDs, matchID)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()
	for _, matchID := range matchIDs {
		if _, err := tx.Exec(ctx, `UPDATE match.tickets SET status='expired' WHERE match_id=$1 AND status='matched'`, matchID); err != nil {
			return 0, err
		}
		payload, err := json.Marshal(map[string]any{"matchId": matchID, "reason": "heartbeat_timeout"})
		if err != nil {
			return 0, err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO ops.outbox_events(aggregate_type,aggregate_id,event_type,correlation_id,payload) VALUES('match',$1,'match.abandoned.v1',$1,$2::jsonb)`, matchID, payload); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return len(matchIDs), nil
}

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
