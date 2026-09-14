package nakama_test

import (
	"os"
	"strings"
	"testing"
)

func TestRuntimeBridgeIsES5AndRegistersRPCs(t *testing.T) {
	data, err := os.ReadFile("runtime/index.js")
	if err != nil {
		t.Fatal(err)
	}
	source := string(data)
	for _, required := range []string{"function InitModule", "registerRpc(\"gameservice.health\"", "registerRpc(\"gameservice.profile\"", "registerRpc(\"gameservice.social\"", "nk.accountGetId", "nk.accountUpdateId", "nk.friendsList", "nk.friendsAdd", "nk.groupCreate", "nk.userGroupsList", "nk.notificationsList", "nk.channelMessageSend", "control-api:8080/health/live"} {
		if !strings.Contains(source, required) {
			t.Fatalf("runtime bridge missing %q", required)
		}
	}
	for _, forbidden := range []string{"require(", "process.", "fs.", "eval("} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("runtime bridge contains forbidden Node/unsafe API %q", forbidden)
		}
	}
}
