package matchmaking

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/Dankular/GameService/internal/allocation"
	"github.com/Dankular/GameService/internal/matches"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Worker struct {
	Pool       *pgxpool.Pool
	Allocator  allocation.Allocator
	MatchStore matches.Store
	Policy     Policy
	Protocol   string
	BatchSize  int
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
	seed := batch[0]
	selector := allocation.Selector{GameID: seed.GameID, ModeID: seed.ModeID, Build: seed.Build, Region: seed.Region, Protocol: w.Protocol}
	if err := selector.Validate(); err != nil {
		return false, err
	}
	allocated, err := w.Allocator.Allocate(ctx, selector)
	if err != nil {
		return false, fmt.Errorf("allocate match server: %w", err)
	}
	matchID, err := newID("match")
	if err != nil {
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
	if _, err := w.MatchStore.Create(ctx, matches.MatchSpec{MatchID: matchID, GameID: seed.GameID, Environment: seed.Environment, ModeID: seed.ModeID, DefinitionRevision: seed.DefinitionRevision, Build: seed.Build, AllocationID: allocated.AllocationID, ServerAddress: allocated.Address, ServerPorts: allocated.Ports}, roster); err != nil {
		return false, fmt.Errorf("persist match: %w", err)
	}
	if err := w.MatchStore.MarkAllocating(ctx, matchID); err != nil {
		return false, fmt.Errorf("transition match to allocating: %w", err)
	}
	ids := make([]string, len(batch))
	for i := range batch {
		ids[i] = batch[i].ID
	}
	result, err := w.Pool.Exec(ctx, `UPDATE match.tickets SET status='matched' WHERE ticket_id=ANY($1) AND status='queued' AND expires_at>now()`, ids)
	if err != nil {
		return false, err
	}
	if int(result.RowsAffected()) != len(ids) {
		return false, fmt.Errorf("match ticket claim changed: expected %d, updated %d", len(ids), result.RowsAffected())
	}
	return true, nil
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
