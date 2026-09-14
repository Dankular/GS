package matches

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
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
