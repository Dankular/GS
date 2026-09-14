package commands

import (
	"encoding/json"
	"errors"
	"fmt"
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
	if len(e.Spec.Arguments) > 64 {
		return fmt.Errorf("%w: too many arguments", ErrInvalidEnvelope)
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
	return e, e.Validate()
}
