package leaderboards

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/Dankular/GameService/internal/compiler"
	"github.com/Dankular/GameService/internal/nakama"
	"github.com/Dankular/GameService/internal/outbox"
	"github.com/jackc/pgx/v5/pgxpool"
	"time"
)

type ResultPlayer struct {
	PlayerID string `json:"playerId"`
	Score    int64  `json:"score"`
	Subscore int64  `json:"subscore,omitempty"`
}
type ResultPayload struct {
	Players []ResultPlayer `json:"players"`
}
type Publisher struct {
	Pool   *pgxpool.Pool
	Nakama nakama.Client
}

func DecodeResultPayload(payload []byte) (ResultPayload, error) {
	var result ResultPayload
	if err := json.Unmarshal(payload, &result); err != nil || len(result.Players) == 0 {
		return ResultPayload{}, errors.New("match result has no leaderboard players")
	}
	for _, player := range result.Players {
		if player.PlayerID == "" {
			return ResultPayload{}, errors.New("match result contains an empty player ID")
		}
	}
	return result, nil
}

func (p Publisher) Publish(ctx context.Context, event outbox.Event) error {
	if event.EventType != "match.result.accepted.v1" {
		return nil
	}
	var envelope struct {
		Sequence int64 `json:"sequence"`
	}
	if err := json.Unmarshal(event.Payload, &envelope); err != nil || envelope.Sequence < 1 {
		return errors.New("invalid match result event")
	}
	var gameID, modeID string
	var payload, canonical []byte
	if err := p.Pool.QueryRow(ctx, `SELECT m.game_id,m.mode_id,r.payload,d.canonical FROM match.matches m JOIN match.results r ON r.match_id=m.match_id JOIN platform.definition_revisions d ON d.game_id=m.game_id AND d.revision=m.definition_revision WHERE m.match_id=$1 AND r.result_sequence=$2`, event.AggregateID, envelope.Sequence).Scan(&gameID, &modeID, &payload, &canonical); err != nil {
		return fmt.Errorf("load match result: %w", err)
	}
	var definition compiler.Definition
	if err := json.Unmarshal(canonical, &definition); err != nil {
		return fmt.Errorf("decode match definition: %w", err)
	}
	rating := configuredRating(definition, modeID)
	if rating.LeaderboardID == "" && rating.Tournament == nil {
		return nil
	}
	result, err := DecodeResultPayload(payload)
	if err != nil {
		return err
	}
	for _, player := range result.Players {
		record := nakama.LeaderboardRecord{UserID: player.PlayerID, Score: player.Score, Subscore: player.Subscore}
		if rating.LeaderboardID != "" {
			if err := p.Nakama.WriteLeaderboardRecord(ctx, rating.LeaderboardID, record); err != nil {
				return err
			}
		}
		if rating.Tournament != nil {
			duration, err := time.ParseDuration(rating.Tournament.Duration)
			if err != nil {
				return fmt.Errorf("parse tournament duration: %w", err)
			}
			if err := p.Nakama.WriteTournamentRecord(ctx, nakama.TournamentConfig{
				ID: rating.Tournament.ID, DurationSeconds: int64(duration / time.Second),
				ResetSchedule: rating.Tournament.ResetSchedule, JoinRequired: rating.Tournament.JoinRequired,
				MaxScoreAttempts: rating.Tournament.MaxScoreAttempts,
			}, record); err != nil {
				return err
			}
		}
	}
	return nil
}

func configuredLeaderboard(definition compiler.Definition, modeID string) string {
	return configuredRating(definition, modeID).LeaderboardID
}

func configuredRating(definition compiler.Definition, modeID string) compiler.RatingPolicy {
	for _, mode := range definition.Spec.MatchModes {
		if mode.ID == modeID {
			return mode.Rating
		}
	}
	return compiler.RatingPolicy{}
}
