package matchmaking

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type TicketRequest struct {
	GameID             string         `json:"gameId"`
	Environment        string         `json:"environment"`
	ModeID             string         `json:"modeId"`
	DefinitionRevision int64          `json:"definitionRevision"`
	Build              string         `json:"build"`
	Region             string         `json:"region"`
	Capacity           int            `json:"capacity"`
	PlayerIDs          []string       `json:"playerIds"`
	Properties         map[string]any `json:"properties"`
	ExpiresAt          time.Time      `json:"expiresAt"`
}

type TicketRecord struct {
	TicketID           string         `json:"ticketId"`
	GameID             string         `json:"gameId"`
	Environment        string         `json:"environment"`
	ModeID             string         `json:"modeId"`
	DefinitionRevision int64          `json:"definitionRevision"`
	Status             string         `json:"status"`
	Build              string         `json:"build"`
	Region             string         `json:"region"`
	Capacity           int            `json:"capacity"`
	PlayerIDs          []string       `json:"playerIds"`
	Properties         map[string]any `json:"properties"`
	CreatedAt          time.Time      `json:"createdAt"`
	ExpiresAt          time.Time      `json:"expiresAt"`
}

var (
	ErrInvalidTicket = errors.New("invalid matchmaking ticket")
	ErrActiveTicket  = errors.New("player already has an active matchmaking ticket")
)

func (r TicketRequest) Validate(actorID string, now time.Time) error {
	for name, value := range map[string]string{"gameId": r.GameID, "environment": r.Environment, "modeId": r.ModeID, "build": r.Build, "region": r.Region} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%w: %s is required", ErrInvalidTicket, name)
		}
	}
	if r.DefinitionRevision < 1 || r.Capacity < 1 || len(r.PlayerIDs) == 0 || len(r.PlayerIDs) > r.Capacity {
		return fmt.Errorf("%w: invalid revision, capacity, or players", ErrInvalidTicket)
	}
	seen := map[string]struct{}{}
	for _, playerID := range r.PlayerIDs {
		if strings.TrimSpace(playerID) == "" {
			return fmt.Errorf("%w: empty player ID", ErrInvalidTicket)
		}
		if _, ok := seen[playerID]; ok {
			return fmt.Errorf("%w: duplicate player ID", ErrInvalidTicket)
		}
		seen[playerID] = struct{}{}
	}
	if _, ok := seen[actorID]; !ok {
		return fmt.Errorf("%w: actor must be a ticket member", ErrInvalidTicket)
	}
	if r.ExpiresAt.IsZero() {
		return fmt.Errorf("%w: expiresAt is required", ErrInvalidTicket)
	}
	if !r.ExpiresAt.After(now) || r.ExpiresAt.After(now.Add(30*time.Minute)) {
		return fmt.Errorf("%w: expiresAt outside allowed window", ErrInvalidTicket)
	}
	return nil
}

type Store struct{ Pool *pgxpool.Pool }

func (s Store) CreateTx(ctx context.Context, tx pgx.Tx, request TicketRequest, actorID string, now time.Time) (TicketRecord, error) {
	if err := request.Validate(actorID, now); err != nil {
		return TicketRecord{}, err
	}
	id, err := ticketID()
	if err != nil {
		return TicketRecord{}, err
	}
	properties, err := json.Marshal(request.Properties)
	if err != nil {
		return TicketRecord{}, fmt.Errorf("encode ticket properties: %w", err)
	}
	players := append([]string(nil), request.PlayerIDs...)
	sort.Strings(players)
	for _, playerID := range players {
		if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, playerID); err != nil {
			return TicketRecord{}, fmt.Errorf("lock ticket member: %w", err)
		}
		var active bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM match.ticket_members m JOIN match.tickets t ON t.ticket_id=m.ticket_id WHERE m.player_id=$1 AND t.status='queued' AND t.expires_at>now())`, playerID).Scan(&active); err != nil {
			return TicketRecord{}, fmt.Errorf("check active ticket: %w", err)
		}
		if active {
			return TicketRecord{}, fmt.Errorf("%w: %s", ErrActiveTicket, playerID)
		}
	}
	if _, err = tx.Exec(ctx, `INSERT INTO match.tickets(ticket_id,game_id,environment,mode_id,definition_revision,build,region,capacity,status,properties,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,'queued',$9,$10)`, id, request.GameID, request.Environment, request.ModeID, request.DefinitionRevision, request.Build, request.Region, request.Capacity, properties, request.ExpiresAt); err != nil {
		return TicketRecord{}, fmt.Errorf("create ticket: %w", err)
	}
	for _, playerID := range request.PlayerIDs {
		if _, err = tx.Exec(ctx, `INSERT INTO match.ticket_members(ticket_id,player_id) VALUES($1,$2)`, id, playerID); err != nil {
			return TicketRecord{}, fmt.Errorf("create ticket member: %w", err)
		}
	}
	return TicketRecord{TicketID: id, GameID: request.GameID, Environment: request.Environment, ModeID: request.ModeID, DefinitionRevision: request.DefinitionRevision, Status: "queued", Build: request.Build, Region: request.Region, Capacity: request.Capacity, PlayerIDs: append([]string(nil), request.PlayerIDs...), Properties: request.Properties, CreatedAt: now, ExpiresAt: request.ExpiresAt}, nil
}

func (s Store) GetTx(ctx context.Context, tx pgx.Tx, ticketID, actorID string) (TicketRecord, error) {
	var r TicketRecord
	var properties []byte
	if err := tx.QueryRow(ctx, `SELECT t.ticket_id,t.game_id,t.environment,t.mode_id,t.definition_revision,t.status,t.build,t.region,t.capacity,t.properties,t.created_at,t.expires_at FROM match.tickets t JOIN match.ticket_members m ON m.ticket_id=t.ticket_id WHERE t.ticket_id=$1 AND m.player_id=$2`, ticketID, actorID).Scan(&r.TicketID, &r.GameID, &r.Environment, &r.ModeID, &r.DefinitionRevision, &r.Status, &r.Build, &r.Region, &r.Capacity, &properties, &r.CreatedAt, &r.ExpiresAt); err != nil {
		return TicketRecord{}, err
	}
	if err := json.Unmarshal(properties, &r.Properties); err != nil {
		return TicketRecord{}, err
	}
	rows, err := tx.Query(ctx, `SELECT player_id FROM match.ticket_members WHERE ticket_id=$1 ORDER BY player_id`, ticketID)
	if err != nil {
		return TicketRecord{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var playerID string
		if err := rows.Scan(&playerID); err != nil {
			return TicketRecord{}, err
		}
		r.PlayerIDs = append(r.PlayerIDs, playerID)
	}
	return r, rows.Err()
}

func (s Store) CancelTx(ctx context.Context, tx pgx.Tx, ticketID, actorID string) error {
	result, err := tx.Exec(ctx, `UPDATE match.tickets SET status='cancelled' WHERE ticket_id=$1 AND status='queued' AND EXISTS (SELECT 1 FROM match.ticket_members WHERE ticket_id=$1 AND player_id=$2)`, ticketID, actorID)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (s Store) Create(ctx context.Context, request TicketRequest, actorID string, now time.Time) (TicketRecord, error) {
	if s.Pool == nil {
		return TicketRecord{}, errors.New("matchmaking store is not configured")
	}
	if err := request.Validate(actorID, now); err != nil {
		return TicketRecord{}, err
	}
	id, err := ticketID()
	if err != nil {
		return TicketRecord{}, err
	}
	properties, err := json.Marshal(request.Properties)
	if err != nil {
		return TicketRecord{}, fmt.Errorf("encode ticket properties: %w", err)
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return TicketRecord{}, err
	}
	defer tx.Rollback(ctx)
	// Serialize active-ticket checks per player so concurrent ticket submissions
	// cannot both pass the check before either transaction commits.
	players := append([]string(nil), request.PlayerIDs...)
	sort.Strings(players)
	for _, playerID := range players {
		if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, playerID); err != nil {
			return TicketRecord{}, fmt.Errorf("lock ticket member: %w", err)
		}
		var active bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM match.ticket_members m JOIN match.tickets t ON t.ticket_id=m.ticket_id WHERE m.player_id=$1 AND t.status='queued' AND t.expires_at>now())`, playerID).Scan(&active); err != nil {
			return TicketRecord{}, fmt.Errorf("check active ticket: %w", err)
		}
		if active {
			return TicketRecord{}, fmt.Errorf("%w: %s", ErrActiveTicket, playerID)
		}
	}
	_, err = tx.Exec(ctx, `INSERT INTO match.tickets(ticket_id,game_id,environment,mode_id,definition_revision,build,region,capacity,status,properties,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,'queued',$9,$10)`, id, request.GameID, request.Environment, request.ModeID, request.DefinitionRevision, request.Build, request.Region, request.Capacity, properties, request.ExpiresAt)
	if err != nil {
		return TicketRecord{}, fmt.Errorf("create ticket: %w", err)
	}
	for _, playerID := range request.PlayerIDs {
		if _, err = tx.Exec(ctx, `INSERT INTO match.ticket_members(ticket_id,player_id) VALUES($1,$2)`, id, playerID); err != nil {
			return TicketRecord{}, fmt.Errorf("create ticket member: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return TicketRecord{}, err
	}
	return TicketRecord{TicketID: id, GameID: request.GameID, Environment: request.Environment, ModeID: request.ModeID, DefinitionRevision: request.DefinitionRevision, Status: "queued", Build: request.Build, Region: request.Region, Capacity: request.Capacity, PlayerIDs: append([]string(nil), request.PlayerIDs...), Properties: request.Properties, CreatedAt: now, ExpiresAt: request.ExpiresAt}, nil
}

func (s Store) Get(ctx context.Context, ticketID, actorID string) (TicketRecord, error) {
	if s.Pool == nil {
		return TicketRecord{}, errors.New("matchmaking store is not configured")
	}
	var r TicketRecord
	var properties []byte
	err := s.Pool.QueryRow(ctx, `SELECT t.ticket_id,t.game_id,t.environment,t.mode_id,t.definition_revision,t.status,t.build,t.region,t.capacity,t.properties,t.created_at,t.expires_at FROM match.tickets t JOIN match.ticket_members m ON m.ticket_id=t.ticket_id WHERE t.ticket_id=$1 AND m.player_id=$2`, ticketID, actorID).Scan(&r.TicketID, &r.GameID, &r.Environment, &r.ModeID, &r.DefinitionRevision, &r.Status, &r.Build, &r.Region, &r.Capacity, &properties, &r.CreatedAt, &r.ExpiresAt)
	if err != nil {
		return TicketRecord{}, err
	}
	if err := json.Unmarshal(properties, &r.Properties); err != nil {
		return TicketRecord{}, err
	}
	return r, nil
}

func (s Store) Cancel(ctx context.Context, ticketID, actorID string) error {
	if s.Pool == nil {
		return errors.New("matchmaking store is not configured")
	}
	result, err := s.Pool.Exec(ctx, `UPDATE match.tickets SET status='cancelled' WHERE ticket_id=$1 AND status='queued' AND EXISTS (SELECT 1 FROM match.ticket_members WHERE ticket_id=$1 AND player_id=$2)`, ticketID, actorID)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func ticketID() (string, error) {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes[:]), nil
}
