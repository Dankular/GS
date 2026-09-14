package compiler

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"time"

	"gopkg.in/yaml.v3"
)

var digestPattern = regexp.MustCompile(`^sha256:[0-9a-fA-F]{64}$`)

type Definition struct {
	APIVersion string   `yaml:"apiVersion" json:"apiVersion"`
	Kind       string   `yaml:"kind" json:"kind"`
	Metadata   Metadata `yaml:"metadata" json:"metadata"`
	Spec       Spec     `yaml:"spec" json:"spec"`
}
type Metadata struct {
	GameID   string            `yaml:"gameId" json:"gameId"`
	Revision int64             `yaml:"revision" json:"revision"`
	Labels   map[string]string `yaml:"labels,omitempty" json:"labels,omitempty"`
}
type Spec struct {
	Catalog     Catalog     `yaml:"catalog" json:"catalog"`
	Progression any         `yaml:"progression,omitempty" json:"progression,omitempty"`
	Rewards     []Reward    `yaml:"rewards,omitempty" json:"rewards,omitempty"`
	MatchModes  []MatchMode `yaml:"matchModes,omitempty" json:"matchModes,omitempty"`
}
type Catalog struct {
	Currencies []Currency `yaml:"currencies,omitempty" json:"currencies,omitempty"`
	Items      []Item     `yaml:"items,omitempty" json:"items,omitempty"`
}
type Currency struct {
	ID         string `yaml:"id" json:"id"`
	Precision  int64  `yaml:"precision" json:"precision"`
	MinBalance int64  `yaml:"minBalance" json:"minBalance"`
	MaxBalance int64  `yaml:"maxBalance" json:"maxBalance"`
}
type Item struct {
	ID         string   `yaml:"id" json:"id"`
	StackLimit int64    `yaml:"stackLimit" json:"stackLimit"`
	Tags       []string `yaml:"tags,omitempty" json:"tags,omitempty"`
	Tradeable  bool     `yaml:"tradeable" json:"tradeable"`
}
type Reward struct {
	ID            string  `yaml:"id" json:"id"`
	OncePerPlayer bool    `yaml:"oncePerPlayer" json:"oncePerPlayer"`
	Grants        []Grant `yaml:"grants,omitempty" json:"grants,omitempty"`
}
type Grant struct {
	Item     string `yaml:"item,omitempty" json:"item,omitempty"`
	Quantity int64  `yaml:"quantity,omitempty" json:"quantity,omitempty"`
	Currency string `yaml:"currency,omitempty" json:"currency,omitempty"`
	Amount   int64  `yaml:"amount,omitempty" json:"amount,omitempty"`
}
type MatchMode struct {
	ID            string       `yaml:"id" json:"id"`
	MinPlayers    int64        `yaml:"minPlayers" json:"minPlayers"`
	MaxPlayers    int64        `yaml:"maxPlayers" json:"maxPlayers"`
	TeamSize      int64        `yaml:"teamSize" json:"teamSize"`
	TicketTimeout string       `yaml:"ticketTimeout" json:"ticketTimeout"`
	Regions       []string     `yaml:"regions" json:"regions"`
	FleetRef      string       `yaml:"fleetRef" json:"fleetRef"`
	ServerBuild   string       `yaml:"serverBuild" json:"serverBuild"`
	ResultPolicy  ResultPolicy `yaml:"resultPolicy,omitempty" json:"resultPolicy,omitempty"`
	Rating        RatingPolicy `yaml:"rating,omitempty" json:"rating,omitempty"`
}

type ResultPolicy struct {
	Schema      string `yaml:"schema" json:"schema"`
	MaxDuration string `yaml:"maxDuration" json:"maxDuration"`
}

type RatingPolicy struct {
	LeaderboardID string `yaml:"leaderboardId" json:"leaderboardId"`
	Strategy      string `yaml:"strategy" json:"strategy"`
}

type Report struct {
	Definition Definition `json:"definition"`
	Canonical  []byte     `json:"-"`
	Digest     string     `json:"digest"`
	Errors     []string   `json:"errors,omitempty"`
}

func Compile(r io.Reader) (Report, error) {
	var d Definition
	dec := yaml.NewDecoder(r)
	dec.KnownFields(true)
	if err := dec.Decode(&d); err != nil {
		return Report{}, fmt.Errorf("decode definition: %w", err)
	}
	if err := validate(d); err != nil {
		return Report{}, err
	}
	canonical, err := json.Marshal(d)
	if err != nil {
		return Report{}, fmt.Errorf("canonicalize definition: %w", err)
	}
	h := sha256.Sum256(canonical)
	return Report{Definition: d, Canonical: canonical, Digest: "sha256:" + hex.EncodeToString(h[:])}, nil
}

func validate(d Definition) error {
	if d.APIVersion != "game.platform/v1alpha1" || d.Kind != "GameDefinition" {
		return errors.New("definition apiVersion/kind is invalid")
	}
	if d.Metadata.GameID == "" || d.Metadata.Revision < 1 {
		return errors.New("metadata gameId and positive revision are required")
	}
	seen := map[string]bool{}
	for _, c := range d.Spec.Catalog.Currencies {
		if c.ID == "" || seen[c.ID] {
			return fmt.Errorf("invalid or duplicate currency %q", c.ID)
		}
		seen[c.ID] = true
		if c.Precision < 0 || c.Precision > 18 || c.MinBalance < 0 || c.MaxBalance < c.MinBalance {
			return fmt.Errorf("invalid currency %q bounds", c.ID)
		}
	}
	seen = map[string]bool{}
	for _, i := range d.Spec.Catalog.Items {
		if i.ID == "" || seen[i.ID] {
			return fmt.Errorf("invalid or duplicate item %q", i.ID)
		}
		seen[i.ID] = true
		if i.StackLimit < 1 {
			return fmt.Errorf("item %q stackLimit must be positive", i.ID)
		}
	}
	seen = map[string]bool{}
	for _, r := range d.Spec.Rewards {
		if r.ID == "" || seen[r.ID] {
			return fmt.Errorf("invalid or duplicate reward %q", r.ID)
		}
		seen[r.ID] = true
		for _, g := range r.Grants {
			if (g.Item == "") == (g.Currency == "") || g.Quantity < 0 || g.Amount < 0 {
				return fmt.Errorf("invalid grant in reward %q", r.ID)
			}
		}
	}
	seen = map[string]bool{}
	for _, m := range d.Spec.MatchModes {
		if m.ID == "" || seen[m.ID] {
			return fmt.Errorf("invalid or duplicate match mode %q", m.ID)
		}
		seen[m.ID] = true
		if m.MinPlayers < 1 || m.MaxPlayers < m.MinPlayers || m.TeamSize < 1 || m.FleetRef == "" || !digestPattern.MatchString(m.ServerBuild) {
			return fmt.Errorf("invalid match mode %q", m.ID)
		}
		if len(m.Regions) == 0 {
			return fmt.Errorf("match mode %q requires a region", m.ID)
		}
		if m.ResultPolicy.MaxDuration != "" {
			if m.ResultPolicy.Schema == "" {
				return fmt.Errorf("match mode %q result policy schema is required", m.ID)
			}
			if duration, err := time.ParseDuration(m.ResultPolicy.MaxDuration); err != nil || duration <= 0 {
				return fmt.Errorf("match mode %q has invalid result maxDuration", m.ID)
			}
		}
		if (m.Rating.LeaderboardID == "") != (m.Rating.Strategy == "") {
			return fmt.Errorf("match mode %q rating leaderboardId and strategy must be provided together", m.ID)
		}
		if m.Rating.LeaderboardID != "" && m.Rating.Strategy != "authoritative" {
			return fmt.Errorf("match mode %q has unsupported rating strategy", m.ID)
		}
	}
	return nil
}

func Canonical(report Report) string { return string(report.Canonical) }

func Diff(a, b Report) string {
	if a.Digest == b.Digest {
		return "no changes"
	}
	return fmt.Sprintf("definition digest %s -> %s", a.Digest, b.Digest)
}

func SortedIDs(d Definition) []string {
	ids := []string{}
	for _, c := range d.Spec.Catalog.Currencies {
		ids = append(ids, c.ID)
	}
	for _, i := range d.Spec.Catalog.Items {
		ids = append(ids, i.ID)
	}
	sort.Strings(ids)
	return ids
}
