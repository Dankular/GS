package matches

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Dankular/GameService/internal/commands"
	"github.com/Dankular/GameService/internal/compiler"
	"github.com/Dankular/GameService/internal/economy"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrInvalidResult        = errors.New("invalid match result")
	ErrResultDigestMismatch = errors.New("match result digest mismatch")
	ErrMatchNotRunning      = errors.New("match is not running")
)

type ResultSubmission struct {
	MatchID       string
	Sequence      int64
	Payload       json.RawMessage
	PayloadDigest string
	CorrelationID string
}

// RecordResultConflict persists evidence of a conflicting duplicate after the
// transaction that attempted the result has been rolled back. Conflicts are
// security telemetry and put the aggregate into a reviewable state.
func RecordResultConflict(ctx context.Context, pool *pgxpool.Pool, matchID string, sequence int64, acceptedDigest, conflictingDigest, correlationID string) error {
	if pool == nil || matchID == "" || sequence < 1 || acceptedDigest == "" || conflictingDigest == "" {
		return fmt.Errorf("invalid result conflict")
	}
	if correlationID == "" {
		correlationID = matchID
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `INSERT INTO match.result_conflicts(match_id,result_sequence,accepted_digest,conflicting_digest,correlation_id) VALUES($1,$2,$3,$4,$5)`, matchID, sequence, acceptedDigest, conflictingDigest, correlationID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE match.matches SET state='Disputed',state_version=state_version+1,updated_at=now() WHERE match_id=$1 AND state IN ('Running','Finalizing','Completed')`, matchID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r ResultSubmission) Validate() ([]byte, string, error) {
	if r.MatchID == "" || r.Sequence < 1 || len(r.Payload) == 0 {
		return nil, "", fmt.Errorf("%w: match ID, positive sequence, and payload are required", ErrInvalidResult)
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, r.Payload); err != nil {
		return nil, "", fmt.Errorf("%w: payload must be JSON", ErrInvalidResult)
	}
	canonical := compact.Bytes()
	digest := Digest(canonical)
	if r.PayloadDigest != "" && r.PayloadDigest != digest {
		return nil, "", ErrResultDigestMismatch
	}
	return canonical, digest, nil
}

// SubmitResult records an authoritative result and advances a running match
// to Finalizing in the same transaction. A duplicate with the same digest is
// successful and does not mutate the result; a conflicting duplicate fails.
func SubmitResult(ctx context.Context, tx pgx.Tx, submission ResultSubmission) (duplicate bool, digest string, err error) {
	canonical, digest, err := submission.Validate()
	if err != nil {
		return false, "", err
	}
	var storedDigest string
	err = tx.QueryRow(ctx, `INSERT INTO match.results(match_id,result_sequence,payload_digest,payload) VALUES($1,$2,$3,$4::jsonb) ON CONFLICT(match_id,result_sequence) DO NOTHING RETURNING payload_digest`, submission.MatchID, submission.Sequence, digest, canonical).Scan(&storedDigest)
	if errors.Is(err, pgx.ErrNoRows) {
		if err := tx.QueryRow(ctx, `SELECT payload_digest FROM match.results WHERE match_id=$1 AND result_sequence=$2`, submission.MatchID, submission.Sequence).Scan(&storedDigest); err != nil {
			return false, "", err
		}
		if storedDigest != digest {
			return false, storedDigest, ErrResultDigestMismatch
		}
		return true, digest, nil
	}
	if err != nil {
		return false, "", err
	}
	if err := tx.QueryRow(ctx, `UPDATE match.matches SET state='Finalizing',state_version=state_version+1,updated_at=now() WHERE match_id=$1 AND state IN ('Running','Finalizing') RETURNING match_id`, submission.MatchID).Scan(new(string)); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, digest, ErrMatchNotRunning
		}
		return false, digest, err
	}
	correlationID := submission.CorrelationID
	if correlationID == "" {
		correlationID = submission.MatchID
	}
	eventPayload, err := json.Marshal(map[string]any{"matchId": submission.MatchID, "sequence": submission.Sequence, "payloadDigest": digest})
	if err != nil {
		return false, digest, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO ops.outbox_events(aggregate_type,aggregate_id,event_type,correlation_id,payload) VALUES('match',$1,'match.result.accepted.v1',$2,$3::jsonb)`, submission.MatchID, correlationID, eventPayload); err != nil {
		return false, digest, err
	}
	return false, digest, nil
}

// ResultFinalizer commits an authoritative result and all definition-driven
// post-match effects in the same PostgreSQL transaction. External delivery is
// represented by outbox events and happens only after this transaction commits.
type ResultFinalizer struct{ Economy economy.Service }

func (f ResultFinalizer) Submit(ctx context.Context, tx pgx.Tx, submission ResultSubmission) (bool, string, error) {
	var gameID, environment, modeID string
	var revision int64
	var canonical []byte
	if err := tx.QueryRow(ctx, `SELECT game_id,environment,mode_id,definition_revision FROM match.matches WHERE match_id=$1`, submission.MatchID).Scan(&gameID, &environment, &modeID, &revision); err != nil {
		return false, "", err
	}
	if err := tx.QueryRow(ctx, `SELECT canonical FROM platform.definition_revisions WHERE game_id=$1 AND revision=$2`, gameID, revision).Scan(&canonical); err != nil {
		return false, "", err
	}
	var definition compiler.Definition
	if err := json.Unmarshal(canonical, &definition); err != nil {
		return false, "", fmt.Errorf("decode match definition: %w", err)
	}
	mode, ok := findMatchMode(definition, modeID)
	if !ok {
		return false, "", fmt.Errorf("match mode not found: %s", modeID)
	}
	canonicalResult, _, err := submission.Validate()
	if err != nil {
		return false, "", err
	}
	players, err := resultPlayers(canonicalResult)
	if err != nil {
		return false, "", err
	}
	if err := validateResultRoster(ctx, tx, submission.MatchID, players); err != nil {
		return false, "", err
	}
	duplicate, digest, err := SubmitResult(ctx, tx, submission)
	if err != nil || duplicate {
		return duplicate, digest, err
	}
	if mode.ResultPolicy.RewardID != "" {
		var roster []string
		rows, err := tx.Query(ctx, `SELECT player_id FROM match.roster_members WHERE match_id=$1 ORDER BY player_id`, submission.MatchID)
		if err != nil {
			return false, digest, err
		}
		for rows.Next() {
			var player string
			if err := rows.Scan(&player); err != nil {
				rows.Close()
				return false, digest, err
			}
			roster = append(roster, player)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return false, digest, err
		}
		rows.Close()
		for _, player := range roster {
			envelope := commands.Envelope{Metadata: commands.Metadata{RequestID: fmt.Sprintf("%s:%d:reward:%s", submission.MatchID, submission.Sequence, player), CorrelationID: submission.CorrelationID, GameID: gameID, Environment: environment, DefinitionRevision: revision}, Actor: commands.Actor{Type: "match-server", ID: player}, Spec: commands.Spec{Operation: "reward.claim", Arguments: map[string]any{"rewardId": mode.ResultPolicy.RewardID, "sourceId": fmt.Sprintf("match:%s:%d", submission.MatchID, submission.Sequence)}}}
			result, err := f.Economy.Handle(ctx, tx, envelope)
			if err != nil {
				return false, digest, err
			}
			if result.Status != "succeeded" {
				return false, digest, fmt.Errorf("match reward rejected: %s", result.Error.Message)
			}
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE match.matches SET state='Completed',state_version=state_version+1,updated_at=now() WHERE match_id=$1 AND state='Finalizing'`, submission.MatchID); err != nil {
		return false, digest, err
	}
	eventPayload, err := json.Marshal(map[string]any{"matchId": submission.MatchID, "sequence": submission.Sequence, "payloadDigest": digest})
	if err != nil {
		return false, digest, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO ops.outbox_events(aggregate_type,aggregate_id,event_type,correlation_id,payload) VALUES('match',$1,'match.completed.v1',$2,$3::jsonb)`, submission.MatchID, submission.CorrelationID, eventPayload); err != nil {
		return false, digest, err
	}
	return false, digest, nil
}

func findMatchMode(definition compiler.Definition, modeID string) (compiler.MatchMode, bool) {
	for _, mode := range definition.Spec.MatchModes {
		if mode.ID == modeID {
			return mode, true
		}
	}
	return compiler.MatchMode{}, false
}

func resultPlayers(payload []byte) ([]string, error) {
	var result struct {
		Players []struct {
			PlayerID string `json:"playerId"`
		} `json:"players"`
	}
	if err := json.Unmarshal(payload, &result); err != nil || len(result.Players) == 0 {
		return nil, fmt.Errorf("match result has no players")
	}
	players := make([]string, 0, len(result.Players))
	seen := map[string]struct{}{}
	for _, player := range result.Players {
		if player.PlayerID == "" {
			return nil, fmt.Errorf("match result contains an empty player ID")
		}
		if _, ok := seen[player.PlayerID]; ok {
			return nil, fmt.Errorf("match result contains duplicate player ID")
		}
		seen[player.PlayerID] = struct{}{}
		players = append(players, player.PlayerID)
	}
	return players, nil
}

func validateResultRoster(ctx context.Context, tx pgx.Tx, matchID string, players []string) error {
	for _, player := range players {
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM match.roster_members WHERE match_id=$1 AND player_id=$2)`, matchID, player).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("match result contains non-roster player: %s", player)
		}
	}
	return nil
}
