package compiler

import (
	"strings"
	"testing"
)

const valid = `apiVersion: game.platform/v1alpha1
kind: GameDefinition
metadata: {gameId: arena, revision: 1}
spec:
  catalog:
    currencies: [{id: coins, precision: 0, minBalance: 0, maxBalance: 1000}]
    items: [{id: sword, stackLimit: 1}]
  matchModes: [{id: deathmatch, minPlayers: 2, maxPlayers: 8, teamSize: 1, regions: [eu-west], fleetRef: deathmatch-v1, serverBuild: sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef}]
`

func TestCompileIsDeterministic(t *testing.T) {
	a, err := Compile(strings.NewReader(valid))
	if err != nil {
		t.Fatal(err)
	}
	b, err := Compile(strings.NewReader(valid))
	if err != nil {
		t.Fatal(err)
	}
	if a.Digest != b.Digest || len(a.Canonical) == 0 {
		t.Fatal("digest is not deterministic")
	}
}
func TestCompileRejectsUnknownFields(t *testing.T) {
	_, err := Compile(strings.NewReader(valid + "unknown: true\n"))
	if err == nil {
		t.Fatal("expected unknown field rejection")
	}
}
func TestCompileRejectsBadBuildDigest(t *testing.T) {
	_, err := Compile(strings.NewReader(strings.Replace(valid, "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", "latest", 1)))
	if err == nil {
		t.Fatal("expected digest validation")
	}
}
