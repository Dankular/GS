package matchmaking

import "testing"

func TestBatchIsDeterministicAndRequiresCompatibleTickets(t *testing.T) {
	policy := Policy{TeamSize: 1, Teams: 2, RatingWindow: 50, AllowedRegions: map[string]struct{}{"eu-west": {}}}
	tickets := []Ticket{
		{ID: "b", PlayerIDs: []string{"p2"}, ModeID: "arena", DefinitionRevision: 3, Build: "build-a", Region: "eu-west", Capacity: 2, Rating: 1020, QueuedAt: 2},
		{ID: "a", PlayerIDs: []string{"p1"}, ModeID: "arena", DefinitionRevision: 3, Build: "build-a", Region: "eu-west", Capacity: 2, Rating: 1000, QueuedAt: 1},
		{ID: "wrong-build", PlayerIDs: []string{"p3"}, ModeID: "arena", DefinitionRevision: 3, Build: "build-b", Region: "eu-west", Capacity: 2, Rating: 1000, QueuedAt: 0},
	}
	batch := Batch(tickets, policy)
	if len(batch) != 2 || batch[0].ID != "a" || batch[1].ID != "b" {
		t.Fatalf("unexpected batch: %#v", batch)
	}
}

func TestBatchRejectsIncompleteOrOutOfPolicyTickets(t *testing.T) {
	policy := Policy{TeamSize: 2, Teams: 2, RatingWindow: 100, AllowedRegions: map[string]struct{}{"eu-west": {}}}
	ticket := Ticket{ID: "solo", PlayerIDs: []string{"p"}, ModeID: "arena", DefinitionRevision: 1, Build: "b", Region: "eu-west", Capacity: 2}
	if got := Batch([]Ticket{ticket, ticket}, policy); got != nil {
		t.Fatalf("expected no batch: %#v", got)
	}
	if got := Batch(nil, Policy{}); got != nil {
		t.Fatal("invalid policy should not match")
	}
}
