package matchmaking

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Dankular/GameService/internal/allocation"
	"github.com/Dankular/GameService/internal/matches"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Worker struct {
	Pool                  *pgxpool.Pool
	Allocator             allocation.Allocator
	MatchStore            matches.Store
	Policy                Policy
	Protocol              string
	BatchSize             int
	ServerClaimPrivateKey ed25519.PrivateKey
	ServerClaimTTL        time.Duration
}

// RunOnce claims one compatible batch, allocates a server, and records the
// match before transitioning its tickets out of the queue. The allocation is
// deliberately outside the database transaction; no network call is held
// while locks are open.
func (w Worker) RunOnce(ctx context.Context) (bool, error) {
	if w.Pool == nil || w.Allocator == nil || w.MatchStore.Pool == nil {
		return false, errors.New("matchmaking worker configuration is incomplete")
	}
	tickets, err := w.queued(ctx)
	if err != nil {
		return false, err
	}
	if len(tickets) == 0 {
		return false, nil
	}
	batch := Batch(tickets, w.Policy)
	if len(batch) == 0 {
		return false, nil
	}
	ids := make([]string, len(batch))
	for i := range batch {
		ids[i] = batch[i].ID
	}
	claimed, err := w.claim(ctx, ids)
	if err != nil || !claimed {
		return false, err
	}
	seed := batch[0]
	matchID, err := newID("match")
	if err != nil {
		_ = w.setTicketStatus(ctx, ids, "queued")
		return false, err
	}
	roster := make([]matches.RosterMember, 0, len(batch)*w.Policy.TeamSize)
	for team, ticket := range batch {
		players := append([]string(nil), ticket.PlayerIDs...)
		sort.Strings(players)
		for slot, playerID := range players {
			roster = append(roster, matches.RosterMember{PlayerID: playerID, Slot: team*w.Policy.TeamSize + slot, Team: fmt.Sprintf("team-%d", team)})
		}
	}
	allocationID, err := newID("allocation")
	if err != nil {
		_ = w.setTicketStatus(ctx, ids, "queued")
		return false, err
	}
	rosterParts := make([]string, len(roster))
	for i, member := range roster {
		rosterParts[i] = fmt.Sprintf("%s:%d", member.PlayerID, member.Slot)
	}
	selector := allocation.Selector{
		GameID: seed.GameID, ModeID: seed.ModeID, Build: seed.Build, Region: seed.Region,
		Protocol: w.Protocol, AllocationID: allocationID,
		Metadata: map[string]string{
			"gameservice.io/match-id":      matchID,
			"gameservice.io/allocation-id": allocationID,
			"gameservice.io/server-build":  seed.Build,
			"gameservice.io/match-roster":  strings.Join(rosterParts, ","),
		},
	}
	if len(w.ServerClaimPrivateKey) == ed25519.PrivateKeySize {
		serverToken, err := w.serverClaimToken(matchID, allocationID, seed.Build)
		if err != nil {
			_ = w.setTicketStatus(ctx, ids, "queued")
			return false, fmt.Errorf("sign server claim: %w", err)
		}
		selector.Metadata["gameservice.io/server-token"] = serverToken
	}
	if err := selector.Validate(); err != nil {
		_ = w.setTicketStatus(ctx, ids, "queued")
		return false, err
	}
	allocated, err := w.Allocator.Allocate(ctx, selector)
	if err != nil {
		_ = w.setTicketStatus(ctx, ids, "queued")
		return false, fmt.Errorf("allocate match server: %w", err)
	}
	if _, err := w.MatchStore.Create(ctx, matches.MatchSpec{MatchID: matchID, GameID: seed.GameID, Environment: seed.Environment, ModeID: seed.ModeID, DefinitionRevision: seed.DefinitionRevision, Build: seed.Build, AllocationID: allocated.AllocationID, ServerAddress: allocated.Address, ServerPorts: allocated.Ports}, roster); err != nil {
		_ = w.setTicketStatus(ctx, ids, "queued")
		return false, fmt.Errorf("persist match: %w", err)
	}
	if err := w.MatchStore.MarkAllocating(ctx, matchID); err != nil {
		_ = w.setTicketStatus(ctx, ids, "queued")
		return false, fmt.Errorf("transition match to allocating: %w", err)
	}
	result, err := w.Pool.Exec(ctx, `UPDATE match.tickets SET status='matched',match_id=$2 WHERE ticket_id=ANY($1) AND status='matching'`, ids, matchID)
	if err != nil {
		return false, err
	}
	if int(result.RowsAffected()) != len(ids) {
		return false, fmt.Errorf("match ticket finalization changed: expected %d, updated %d", len(ids), result.RowsAffected())
	}
	return true, nil
}

func (w Worker) serverClaimToken(matchID, allocationID, build string) (string, error) {
	ttl := w.ServerClaimTTL
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	now := time.Now().UTC()
	return matches.SignClaim(matches.JoinClaim{
		Issuer: "control-plane", Audience: "control-plane", Subject: "game-server",
		MatchID: matchID, AllocationID: allocationID, ServerBuild: build,
		IssuedAt: now.Unix(), NotBefore: now.Unix(), ExpiresAt: now.Add(ttl).Unix(),
		JTI: matchID + ":" + allocationID,
	}, w.ServerClaimPrivateKey)
}

func (w Worker) claim(ctx context.Context, ids []string) (bool, error) {
	tx, err := w.Pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	result, err := tx.Exec(ctx, `UPDATE match.tickets SET status='matching' WHERE ticket_id=ANY($1) AND status='queued' AND expires_at>now()`, ids)
	if err != nil {
		return false, err
	}
	if int(result.RowsAffected()) != len(ids) {
		return false, nil
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return true, nil
}

func (w Worker) setTicketStatus(ctx context.Context, ids []string, status string) error {
	_, err := w.Pool.Exec(ctx, `UPDATE match.tickets SET status=$2 WHERE ticket_id=ANY($1) AND status='matching'`, ids, status)
	return err
}

func (w Worker) queued(ctx context.Context) ([]Ticket, error) {
	limit := w.BatchSize
	if limit < 1 {
		limit = 100
	}
	rows, err := w.Pool.Query(ctx, `SELECT t.ticket_id,t.game_id,t.environment,t.mode_id,t.definition_revision,t.build,t.region,t.capacity,COALESCE(t.properties,'{}'::jsonb),EXTRACT(EPOCH FROM t.created_at)::bigint FROM match.tickets t WHERE t.status='queued' AND t.expires_at>now() ORDER BY t.created_at,t.ticket_id LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Ticket
	for rows.Next() {
		var ticket Ticket
		var properties []byte
		if err := rows.Scan(&ticket.ID, &ticket.GameID, &ticket.Environment, &ticket.ModeID, &ticket.DefinitionRevision, &ticket.Build, &ticket.Region, &ticket.Capacity, &properties, &ticket.QueuedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(properties, &ticket.Properties); err != nil {
			return nil, err
		}
		var members []string
		if err := w.Pool.QueryRow(ctx, `SELECT COALESCE(array_agg(player_id ORDER BY player_id),'{}') FROM match.ticket_members WHERE ticket_id=$1`, ticket.ID).Scan(&members); err != nil {
			return nil, err
		}
		ticket.PlayerIDs = members
		result = append(result, ticket)
	}
	return result, rows.Err()
}

func newID(prefix string) (string, error) {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s-%s", prefix, hex.EncodeToString(bytes[:])), nil
}
