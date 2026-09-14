package commands

import "time"

// OperationDefinition is the policy and execution contract for one command.
// Keeping this beside the closed registry prevents transport code and domain
// handlers from silently acquiring different authorization or reliability
// semantics.
type OperationDefinition struct {
	ActorType       string
	Scope           string
	InputSchema     string
	OutputSchema    string
	Isolation       string
	LockStrategy    string
	Idempotency     string
	RateLimitBucket string
	Event           string
	AuditPolicy     string
	MaxExecution    time.Duration
	DryRun          bool
}

var operationDefinitions = map[string]OperationDefinition{
	"profile.get":                    playerRead("profile-command", "profile.result.v1", "profile.read.v1", "profile fields"),
	"profile.patch_public_fields":    playerWrite("profile-command", "profile.result.v1", "profile.updated.v1", "profile fields"),
	"inventory.list":                 playerRead("inventory-read", "inventory.result.v1", "inventory.read.v1", "inventory state"),
	"inventory.grant":                playerWrite("inventory-mutation", "inventory.result.v1", "inventory.item_granted.v1", "inventory mutation"),
	"inventory.consume":              playerWrite("inventory-mutation", "inventory.result.v1", "inventory.item_consumed.v1", "inventory mutation"),
	"inventory.transfer":             playerWrite("inventory-mutation", "inventory.result.v1", "inventory.item_transferred.v1", "inventory mutation"),
	"wallet.get":                     playerRead("wallet-read", "wallet.result.v1", "wallet.read.v1", "wallet state"),
	"wallet.credit":                  playerWrite("wallet-mutation", "wallet.result.v1", "wallet.credited.v1", "wallet mutation"),
	"wallet.debit":                   playerWrite("wallet-mutation", "wallet.result.v1", "wallet.debited.v1", "wallet mutation"),
	"wallet.transfer":                playerWrite("wallet-mutation", "wallet.result.v1", "wallet.transferred.v1", "wallet mutation"),
	"entitlement.list":               playerRead("entitlement-read", "entitlement.result.v1", "entitlement.read.v1", "entitlement state"),
	"entitlement.grant":              playerWrite("entitlement-mutation", "entitlement.result.v1", "entitlement.granted.v1", "entitlement mutation"),
	"entitlement.revoke":             playerWrite("entitlement-mutation", "entitlement.result.v1", "entitlement.revoked.v1", "entitlement mutation"),
	"progression.get":                playerRead("progression-read", "progression.result.v1", "progression.read.v1", "progression state"),
	"progression.add_xp":             playerWrite("progression-mutation", "progression.result.v1", "progression.xp_added.v1", "progression mutation"),
	"progression.complete_objective": playerWrite("progression-mutation", "progression.result.v1", "progression.objective_completed.v1", "progression mutation"),
	"reward.preview":                 playerRead("reward-read", "reward.result.v1", "reward.previewed.v1", "reward state"),
	"reward.claim":                   playerWrite("reward-mutation", "reward.result.v1", "reward.claimed.v1", "reward mutation"),
	"matchmaking.enqueue":            playerWrite("matchmaking", "matchmaking.result.v1", "matchmaking.ticket_queued.v1", "matchmaking mutation"),
	"matchmaking.status":             playerRead("matchmaking-read", "matchmaking.result.v1", "matchmaking.ticket_read.v1", "matchmaking state"),
	"matchmaking.cancel":             playerWrite("matchmaking", "matchmaking.result.v1", "matchmaking.ticket_cancelled.v1", "matchmaking mutation"),
	"match.get":                      playerRead("match-read", "match.result.v1", "match.read.v1", "match state"),
	"match.issue_join_claim":         playerWrite("join-claim", "match.result.v1", "match.join_claim_issued.v1", "join claim issuance"),
	"match.submit_result":            serverWrite("match-result", "match.result.v1", "match.result_submitted.v1", "authoritative result"),
	"match.abandon":                  playerWrite("match-mutation", "match.result.v1", "match.abandoned.v1", "match mutation"),
	"definition.validate":            scopedDefinition("definition:validate", "definition-validate", "definition.result.v1", "definition.validated.v1", "none", true),
	"definition.diff":                scopedDefinition("definition:validate", "definition-validate", "definition.result.v1", "definition.diffed.v1", "none", true),
	"definition.publish":             scopedDefinition("definition:publish", "definition-publish", "definition.result.v1", "definition.published.v1", "definition publication", false),
	"definition.activate":            scopedDefinition("definition:activate", "definition-activate", "definition.result.v1", "definition.activated.v1", "definition activation", false),
	"definition.rollback":            scopedDefinition("definition:activate", "definition-activate", "definition.result.v1", "definition.rolled_back.v1", "definition activation", false),
	"admin.player_snapshot":          adminRead("admin-read", "admin.result.v1", "admin.snapshot_read.v1", "admin snapshot", false),
	"admin.execute_command":          adminWrite("admin-mutation", "admin.result.v1", "admin.command_executed.v1", "admin command", false),
	"admin.audit_search":             adminRead("admin-read", "admin.result.v1", "admin.audit_read.v1", "audit records", false),
	"admin.player_restrict":          adminWrite("admin-mutation", "admin.result.v1", "admin.player_restricted.v1", "restriction mutation", false),
	"admin.player_unrestrict":        adminWrite("admin-mutation", "admin.result.v1", "admin.player_unrestricted.v1", "restriction mutation", false),
}

func playerRead(bucket, output, event, audit string) OperationDefinition {
	return definition("player", "player:read", bucket, output, event, audit, "none", false)
}

func playerWrite(bucket, output, event, audit string) OperationDefinition {
	return definition("player", "player:write", bucket, output, event, audit, "ordered-resource-rows", false)
}

func serverWrite(bucket, output, event, audit string) OperationDefinition {
	return definition("server", "server:write", bucket, output, event, audit, "match-row-and-result-sequence", false)
}

func adminRead(bucket, output, event, audit string, dryRun bool) OperationDefinition {
	return definition("admin", "admin:read", bucket, output, event, audit, "none", dryRun)
}

func adminWrite(bucket, output, event, audit string, dryRun bool) OperationDefinition {
	return definition("admin", "admin:write", bucket, output, event, audit, "target-row-and-audit", dryRun)
}

func scopedDefinition(scope, bucket, output, event, audit string, dryRun bool) OperationDefinition {
	result := definition("admin", scope, bucket, output, event, audit, "none", dryRun)
	return result
}

func definition(actor, scope, bucket, output, event, audit, lockStrategy string, dryRun bool) OperationDefinition {
	return OperationDefinition{
		ActorType: actor, Scope: scope, InputSchema: "command-envelope.v1", OutputSchema: output,
		Isolation: "read-committed", LockStrategy: lockStrategy, Idempotency: "request-id", RateLimitBucket: bucket,
		Event: event, AuditPolicy: audit, MaxExecution: 5 * time.Second, DryRun: dryRun,
	}
}

// DefinitionFor returns the immutable command declaration for an operation.
func DefinitionFor(operation string) (OperationDefinition, bool) {
	definition, ok := operationDefinitions[operation]
	return definition, ok
}
