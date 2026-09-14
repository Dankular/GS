package matchmaking

import "sort"

// Ticket is the immutable matchmaking snapshot stored when a request enters
// the queue. Matching never reads mutable player state during a batch.
type Ticket struct {
	ID                 string
	GameID             string
	Environment        string
	PlayerIDs          []string
	ModeID             string
	DefinitionRevision int64
	Build              string
	Region             string
	Capacity           int
	Properties         map[string]any
	Rating             int64
	QueuedAt           int64
}

type Policy struct {
	TeamSize       int
	Teams          int
	RatingWindow   int64
	AllowedRegions map[string]struct{}
}

func (p Policy) validate() bool { return p.TeamSize > 0 && p.Teams > 0 && p.RatingWindow >= 0 }

// Batch returns a stable, deterministic group of tickets or nil when no
// complete group is currently compatible. It does not mutate its inputs.
func Batch(tickets []Ticket, policy Policy) []Ticket {
	if !policy.validate() || len(tickets) < policy.Teams {
		return nil
	}
	ordered := append([]Ticket(nil), tickets...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].QueuedAt != ordered[j].QueuedAt {
			return ordered[i].QueuedAt < ordered[j].QueuedAt
		}
		return ordered[i].ID < ordered[j].ID
	})
	for i, seed := range ordered {
		if !compatibleRegion(seed, policy) || len(seed.PlayerIDs) != policy.TeamSize {
			continue
		}
		batch := []Ticket{seed}
		for _, candidate := range ordered[i+1:] {
			if len(batch) == policy.Teams {
				break
			}
			if compatible(seed, candidate, policy) {
				batch = append(batch, candidate)
			}
		}
		if len(batch) == policy.Teams {
			return batch
		}
	}
	return nil
}

func compatible(a, b Ticket, p Policy) bool {
	return a.ModeID == b.ModeID && a.DefinitionRevision == b.DefinitionRevision && a.Build == b.Build && a.Capacity == b.Capacity && compatibleRegion(a, p) && compatibleRegion(b, p) && abs(a.Rating-b.Rating) <= p.RatingWindow && len(b.PlayerIDs) == p.TeamSize
}

func compatibleRegion(t Ticket, p Policy) bool {
	if len(p.AllowedRegions) == 0 {
		return t.Region != ""
	}
	_, ok := p.AllowedRegions[t.Region]
	return ok
}

func abs(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}
