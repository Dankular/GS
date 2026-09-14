package commands

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
)

var (
	ErrInvalidEnvelope  = errors.New("invalid command envelope")
	ErrUnknownOperation = errors.New("unknown operation")
)

type Envelope struct {
	APIVersion string   `json:"apiVersion"`
	Kind       string   `json:"kind"`
	Metadata   Metadata `json:"metadata"`
	Actor      Actor    `json:"actor"`
	Spec       Spec     `json:"spec"`
}

type Metadata struct {
	RequestID          string `json:"requestId"`
	CorrelationID      string `json:"correlationId"`
	GameID             string `json:"gameId"`
	Environment        string `json:"environment"`
	DefinitionRevision int64  `json:"definitionRevision"`
}

type Actor struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

type Spec struct {
	Operation string         `json:"operation"`
	Arguments map[string]any `json:"arguments"`
}

type Result struct {
	RequestID     string         `json:"requestId"`
	CorrelationID string         `json:"correlationId"`
	Status        string         `json:"status"`
	Operation     string         `json:"operation"`
	Result        map[string]any `json:"result,omitempty"`
	Events        []string       `json:"events,omitempty"`
	Error         *CommandError  `json:"error,omitempty"`
}

type CommandError struct {
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	Retryable bool           `json:"retryable"`
	Details   map[string]any `json:"details,omitempty"`
}

var registry = map[string]struct{}{
	"profile.get": {}, "profile.patch_public_fields": {},
	"inventory.list": {}, "inventory.grant": {}, "inventory.consume": {}, "inventory.transfer": {},
	"wallet.get": {}, "wallet.credit": {}, "wallet.debit": {}, "wallet.transfer": {},
	"entitlement.list": {}, "entitlement.grant": {}, "entitlement.revoke": {},
	"progression.get": {}, "progression.add_xp": {}, "progression.complete_objective": {},
	"reward.preview": {}, "reward.claim": {}, "matchmaking.enqueue": {}, "matchmaking.status": {}, "matchmaking.cancel": {},
	"match.get": {}, "match.issue_join_claim": {}, "match.submit_result": {}, "match.abandon": {},
	"definition.validate": {}, "definition.diff": {}, "definition.publish": {}, "definition.activate": {}, "definition.rollback": {},
	"admin.player_snapshot": {}, "admin.execute_command": {}, "admin.audit_search": {}, "admin.player_restrict": {}, "admin.player_unrestrict": {},
}

var allowedArgumentKeys = map[string]map[string]struct{}{
	"profile.get": {}, "profile.patch_public_fields": {"fields": {}},
	"inventory.list": {"playerId": {}}, "inventory.grant": {"playerId": {}, "itemId": {}, "quantity": {}}, "inventory.consume": {"playerId": {}, "itemId": {}, "quantity": {}}, "inventory.transfer": {"playerId": {}, "targetPlayerId": {}, "itemId": {}, "quantity": {}},
	"wallet.get": {"playerId": {}, "currency": {}}, "wallet.credit": {"playerId": {}, "currency": {}, "amount": {}}, "wallet.debit": {"playerId": {}, "currency": {}, "amount": {}}, "wallet.transfer": {"playerId": {}, "targetPlayerId": {}, "currency": {}, "amount": {}},
	"entitlement.list": {"playerId": {}}, "entitlement.grant": {"playerId": {}, "entitlementId": {}}, "entitlement.revoke": {"playerId": {}, "entitlementId": {}},
	"progression.get": {"playerId": {}, "trackId": {}}, "progression.add_xp": {"playerId": {}, "trackId": {}, "amount": {}}, "progression.complete_objective": {"playerId": {}, "objectiveId": {}, "sourceId": {}},
	"reward.preview": {"playerId": {}, "rewardId": {}}, "reward.claim": {"playerId": {}, "rewardId": {}, "sourceId": {}},
	"matchmaking.enqueue": {"gameId": {}, "environment": {}, "modeId": {}, "definitionRevision": {}, "build": {}, "region": {}, "capacity": {}, "playerIds": {}, "properties": {}, "expiresAt": {}}, "matchmaking.status": {"ticketId": {}}, "matchmaking.cancel": {"ticketId": {}},
	"match.get": {"matchId": {}}, "match.issue_join_claim": {"matchId": {}}, "match.submit_result": {"matchId": {}, "sequence": {}, "payload": {}, "payloadDigest": {}}, "match.abandon": {"matchId": {}},
	"definition.validate": {"source": {}}, "definition.diff": {"source": {}, "gameId": {}, "revision": {}}, "definition.publish": {"source": {}, "reason": {}}, "definition.activate": {"gameId": {}, "environment": {}, "revision": {}, "reason": {}}, "definition.rollback": {"gameId": {}, "environment": {}, "revision": {}, "reason": {}},
	"admin.player_snapshot": {"playerId": {}}, "admin.execute_command": {"operation": {}, "targetPlayerId": {}, "arguments": {}, "reason": {}}, "admin.audit_search": {"action": {}, "limit": {}}, "admin.player_restrict": {"playerId": {}, "kind": {}, "reason": {}, "expiresAt": {}}, "admin.player_unrestrict": {"playerId": {}},
}

const (
	maxArgumentDepth      = 8
	maxArgumentStringSize = 4096
	maxArgumentCollection = 128
)

func (e Envelope) Validate() error {
	if e.APIVersion != "game.platform/v1alpha1" || e.Kind != "Command" {
		return fmt.Errorf("%w: apiVersion and kind are invalid", ErrInvalidEnvelope)
	}
	for name, value := range map[string]string{
		"requestId": e.Metadata.RequestID, "correlationId": e.Metadata.CorrelationID,
		"gameId": e.Metadata.GameID, "environment": e.Metadata.Environment,
		"actor.type": e.Actor.Type, "actor.id": e.Actor.ID, "operation": e.Spec.Operation,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%w: %s is required", ErrInvalidEnvelope, name)
		}
	}
	if e.Metadata.DefinitionRevision < 1 {
		return fmt.Errorf("%w: definitionRevision must be positive", ErrInvalidEnvelope)
	}
	if _, ok := registry[e.Spec.Operation]; !ok {
		return fmt.Errorf("%w: %s", ErrUnknownOperation, e.Spec.Operation)
	}
	if allowed, ok := allowedArgumentKeys[e.Spec.Operation]; ok {
		for key := range e.Spec.Arguments {
			if _, accepted := allowed[key]; !accepted {
				return fmt.Errorf("%w: unknown argument %q for %s", ErrInvalidEnvelope, key, e.Spec.Operation)
			}
		}
	}
	if len(e.Spec.Arguments) > 64 {
		return fmt.Errorf("%w: too many arguments", ErrInvalidEnvelope)
	}
	if err := validateArgumentValue(e.Spec.Arguments, 0, "arguments"); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidEnvelope, err)
	}
	return nil
}

func validateArgumentValue(value any, depth int, path string) error {
	if depth > maxArgumentDepth {
		return fmt.Errorf("%s exceeds maximum nesting depth", path)
	}
	switch typed := value.(type) {
	case nil, bool:
		return nil
	case string:
		if len(typed) > maxArgumentStringSize {
			return fmt.Errorf("%s exceeds maximum string length", path)
		}
	case json.Number:
		if len(typed.String()) > maxArgumentStringSize {
			return fmt.Errorf("%s exceeds maximum numeric length", path)
		}
		parsed, err := strconv.ParseFloat(typed.String(), 64)
		if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) {
			return fmt.Errorf("%s is not a finite number", path)
		}
	case float64:
		// Envelopes decoded from JSON use json.Number, but reject non-finite
		// values when callers construct an envelope directly.
		if math.IsNaN(typed) || math.IsInf(typed, 0) {
			return fmt.Errorf("%s is not a finite number", path)
		}
	case []any:
		if len(typed) > maxArgumentCollection {
			return fmt.Errorf("%s exceeds maximum collection size", path)
		}
		for index, item := range typed {
			if err := validateArgumentValue(item, depth+1, fmt.Sprintf("%s[%d]", path, index)); err != nil {
				return err
			}
		}
	case map[string]any:
		if len(typed) > maxArgumentCollection {
			return fmt.Errorf("%s exceeds maximum collection size", path)
		}
		for key, item := range typed {
			if len(key) > maxArgumentStringSize {
				return fmt.Errorf("%s contains an oversized key", path)
			}
			if err := validateArgumentValue(item, depth+1, path+"."+key); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("%s contains unsupported value type %T", path, value)
	}
	return nil
}

func DecodeStrict(data []byte) (Envelope, error) {
	var e Envelope
	d := json.NewDecoder(strings.NewReader(string(data)))
	d.DisallowUnknownFields()
	d.UseNumber()
	if err := d.Decode(&e); err != nil {
		return e, fmt.Errorf("%w: %v", ErrInvalidEnvelope, err)
	}
	var trailing any
	if err := d.Decode(&trailing); err != io.EOF {
		if err == nil {
			return e, fmt.Errorf("%w: trailing JSON data", ErrInvalidEnvelope)
		}
		return e, fmt.Errorf("%w: trailing JSON data: %v", ErrInvalidEnvelope, err)
	}
	return e, e.Validate()
}
