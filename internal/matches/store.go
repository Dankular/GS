package matches

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type RosterMember struct {
	PlayerID string `json:"playerId"`
	Slot     int    `json:"slot"`
	Team     string `json:"team,omitempty"`
}
type MatchSpec struct {
	MatchID            string
	GameID             string
	Environment        string
	ModeID             string
	DefinitionRevision int64
	Build              string
	AllocationID       string
	ServerAddress      string
	ServerPorts        map[string]int
}
type MatchRecord struct {
	MatchID            string         `json:"matchId"`
	GameID             string         `json:"gameId"`
	Environment        string         `json:"environment"`
	ModeID             string         `json:"modeId"`
	DefinitionRevision int64          `json:"definitionRevision"`
	State              State          `json:"state"`
	Build              string         `json:"build"`
	AllocationID       string         `json:"allocationId,omitempty"`
	ServerAddress      string         `json:"serverAddress,omitempty"`
	ServerPorts        map[string]int `json:"serverPorts,omitempty"`
	Roster             []RosterMember `json:"roster"`
}

type Store struct {
	Pool           *pgxpool.Pool
	JoinPrivateKey ed25519.PrivateKey
	Issuer         string
	Audience       string
	ClaimTTL       time.Duration
}

func (s Store) Create(ctx context.Context, spec MatchSpec, roster []RosterMember) (MatchRecord, error) {
	if s.Pool == nil {
		return MatchRecord{}, errors.New("match store is not configured")
	}
	if spec.MatchID == "" || spec.GameID == "" || spec.Environment == "" || spec.ModeID == "" || spec.DefinitionRevision < 1 || spec.Build == "" || len(roster) == 0 {
		return MatchRecord{}, errors.New("match creation data is incomplete")
	}
	if spec.AllocationID == "" {
		return MatchRecord{}, errors.New("allocation ID is required")
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return MatchRecord{}, err
	}
	defer tx.Rollback(ctx)
	ports, err := json.Marshal(spec.ServerPorts)
	if err != nil {
		return MatchRecord{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO match.matches(match_id,game_id,environment,mode_id,definition_revision,state,server_build,allocation_id,server_address,server_ports) VALUES($1,$2,$3,$4,$5,'Matched',$6,$7,$8,$9::jsonb)`, spec.MatchID, spec.GameID, spec.Environment, spec.ModeID, spec.DefinitionRevision, spec.Build, spec.AllocationID, spec.ServerAddress, ports); err != nil {
		return MatchRecord{}, err
	}
	for _, member := range roster {
		if member.PlayerID == "" || member.Slot < 0 {
			return MatchRecord{}, errors.New("invalid roster member")
		}
		if _, err := tx.Exec(ctx, `INSERT INTO match.roster_members(match_id,player_id,slot,team) VALUES($1,$2,$3,$4)`, spec.MatchID, member.PlayerID, member.Slot, member.Team); err != nil {
			return MatchRecord{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return MatchRecord{}, err
	}
	return MatchRecord{MatchID: spec.MatchID, GameID: spec.GameID, Environment: spec.Environment, ModeID: spec.ModeID, DefinitionRevision: spec.DefinitionRevision, State: Matched, Build: spec.Build, AllocationID: spec.AllocationID, ServerAddress: spec.ServerAddress, ServerPorts: clonePorts(spec.ServerPorts), Roster: append([]RosterMember(nil), roster...)}, nil
}

func (s Store) MarkAllocating(ctx context.Context, matchID string) error {
	result, err := s.Pool.Exec(ctx, `UPDATE match.matches SET state='Allocating',state_version=state_version+1,updated_at=now() WHERE match_id=$1 AND state='Matched'`, matchID)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (s Store) Get(ctx context.Context, matchID, playerID string) (MatchRecord, error) {
	if s.Pool == nil || strings.TrimSpace(matchID) == "" || strings.TrimSpace(playerID) == "" {
		return MatchRecord{}, errors.New("match lookup data is incomplete")
	}
	var record MatchRecord
	var ports []byte
	err := s.Pool.QueryRow(ctx, `SELECT m.match_id,m.game_id,m.environment,m.mode_id,m.definition_revision,m.state,m.server_build,COALESCE(m.allocation_id,''),COALESCE(m.server_address,''),COALESCE(m.server_ports,'{}'::jsonb) FROM match.matches m JOIN match.roster_members r ON r.match_id=m.match_id WHERE m.match_id=$1 AND r.player_id=$2`, matchID, playerID).Scan(&record.MatchID, &record.GameID, &record.Environment, &record.ModeID, &record.DefinitionRevision, &record.State, &record.Build, &record.AllocationID, &record.ServerAddress, &ports)
	if err != nil {
		return MatchRecord{}, err
	}
	if err := json.Unmarshal(ports, &record.ServerPorts); err != nil {
		return MatchRecord{}, err
	}
	rows, err := s.Pool.Query(ctx, `SELECT player_id,slot,COALESCE(team,'') FROM match.roster_members WHERE match_id=$1 ORDER BY slot`, matchID)
	if err != nil {
		return MatchRecord{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var member RosterMember
		if err := rows.Scan(&member.PlayerID, &member.Slot, &member.Team); err != nil {
			return MatchRecord{}, err
		}
		record.Roster = append(record.Roster, member)
	}
	return record, rows.Err()
}

func getTx(ctx context.Context, tx pgx.Tx, matchID, playerID string) (MatchRecord, error) {
	if strings.TrimSpace(matchID) == "" || strings.TrimSpace(playerID) == "" {
		return MatchRecord{}, errors.New("match lookup data is incomplete")
	}
	var record MatchRecord
	var ports []byte
	if err := tx.QueryRow(ctx, `SELECT m.match_id,m.game_id,m.environment,m.mode_id,m.definition_revision,m.state,m.server_build,COALESCE(m.allocation_id,''),COALESCE(m.server_address,''),COALESCE(m.server_ports,'{}'::jsonb) FROM match.matches m JOIN match.roster_members r ON r.match_id=m.match_id WHERE m.match_id=$1 AND r.player_id=$2`, matchID, playerID).Scan(&record.MatchID, &record.GameID, &record.Environment, &record.ModeID, &record.DefinitionRevision, &record.State, &record.Build, &record.AllocationID, &record.ServerAddress, &ports); err != nil {
		return MatchRecord{}, err
	}
	if err := json.Unmarshal(ports, &record.ServerPorts); err != nil {
		return MatchRecord{}, err
	}
	rows, err := tx.Query(ctx, `SELECT player_id,slot,COALESCE(team,'') FROM match.roster_members WHERE match_id=$1 ORDER BY slot`, matchID)
	if err != nil {
		return MatchRecord{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var member RosterMember
		if err := rows.Scan(&member.PlayerID, &member.Slot, &member.Team); err != nil {
			return MatchRecord{}, err
		}
		record.Roster = append(record.Roster, member)
	}
	return record, rows.Err()
}

func (s Store) GetTx(ctx context.Context, tx pgx.Tx, matchID, playerID string) (MatchRecord, error) {
	return getTx(ctx, tx, matchID, playerID)
}

func (s Store) MarkReady(ctx context.Context, matchID string) error {
	result, err := s.Pool.Exec(ctx, `UPDATE match.matches SET state='Ready',state_version=state_version+1,updated_at=now() WHERE match_id=$1 AND state='Allocating'`, matchID)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}
func (s Store) Start(ctx context.Context, matchID, playerID string) error {
	result, err := s.Pool.Exec(ctx, `UPDATE match.matches SET state='Running',state_version=state_version+1,updated_at=now() WHERE match_id=$1 AND state='Ready' AND EXISTS(SELECT 1 FROM match.roster_members WHERE match_id=$1 AND player_id=$2)`, matchID, playerID)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// StartRunning is idempotent for server lifecycle callbacks. The first valid
// join moves a match to Running; later joins observe the already-running state.
func (s Store) StartRunning(ctx context.Context, matchID string) error {
	result, err := s.Pool.Exec(ctx, `UPDATE match.matches SET state='Running',state_version=state_version+1,updated_at=now() WHERE match_id=$1 AND state IN ('Ready','Running')`, matchID)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}
func (s Store) Heartbeat(ctx context.Context, matchID string) error {
	result, err := s.Pool.Exec(ctx, `UPDATE match.matches SET updated_at=now() WHERE match_id=$1 AND state IN ('Ready','Running')`, matchID)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (s Store) IssueJoinClaim(ctx context.Context, matchID, playerID string, now time.Time) (string, error) {
	if len(s.JoinPrivateKey) != ed25519.PrivateKeySize {
		return "", errors.New("join claim signer is not configured")
	}
	return issueJoinClaim(ctx, s.Pool, s, matchID, playerID, now)
}

func (s Store) IssueJoinClaimTx(ctx context.Context, tx pgx.Tx, matchID, playerID string, now time.Time) (string, error) {
	if len(s.JoinPrivateKey) != ed25519.PrivateKeySize {
		return "", errors.New("join claim signer is not configured")
	}
	return issueJoinClaim(ctx, tx, s, matchID, playerID, now)
}

func issueJoinClaim(ctx context.Context, queryer interface {
	QueryRow(context.Context, string, ...any) pgx.Row
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}, s Store, matchID, playerID string, now time.Time) (string, error) {
	var allocationID, build, state string
	var slot int
	var team string
	if err := queryer.QueryRow(ctx, `SELECT m.allocation_id,m.server_build,m.state,r.slot,COALESCE(r.team,'') FROM match.matches m JOIN match.roster_members r ON r.match_id=m.match_id WHERE m.match_id=$1 AND r.player_id=$2`, matchID, playerID).Scan(&allocationID, &build, &state, &slot, &team); err != nil {
		return "", err
	}
	if state != "Ready" && state != "Running" {
		return "", fmt.Errorf("match is not joinable: %s", state)
	}
	ttl := s.ClaimTTL
	if ttl <= 0 {
		ttl = 2 * time.Minute
	}
	var jtiBytes [16]byte
	if _, err := rand.Read(jtiBytes[:]); err != nil {
		return "", err
	}
	jti := hex.EncodeToString(jtiBytes[:])
	claim := JoinClaim{Issuer: s.Issuer, Audience: s.Audience, Subject: playerID, MatchID: matchID, AllocationID: allocationID, ServerBuild: build, Team: team, Slot: slot, IssuedAt: now.Unix(), NotBefore: now.Unix() - 1, ExpiresAt: now.Add(ttl).Unix(), JTI: jti}
	token, err := SignClaim(claim, s.JoinPrivateKey)
	if err != nil {
		return "", err
	}
	if _, err := queryer.Exec(ctx, `INSERT INTO match.join_claims(jti,match_id,player_id,expires_at) VALUES($1,$2,$3,$4)`, jti, matchID, playerID, now.Add(ttl)); err != nil {
		return "", err
	}
	return token, nil
}

func (s Store) ConsumeClaim(ctx context.Context, jti, matchID, playerID string, now time.Time) error {
	result, err := s.Pool.Exec(ctx, `UPDATE match.join_claims SET consumed_at=$4 WHERE jti=$1 AND match_id=$2 AND player_id=$3 AND consumed_at IS NULL AND expires_at>$4`, jti, matchID, playerID, now)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func clonePorts(input map[string]int) map[string]int {
	if input == nil {
		return nil
	}
	output := make(map[string]int, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}
