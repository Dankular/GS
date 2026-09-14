package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	agonessdk "agones.dev/agones/sdks/go"
	"github.com/Dankular/GameService/internal/simulator"
)

func main() {
	key, err := base64.RawStdEncoding.DecodeString(os.Getenv("SERVER_CLAIM_PUBLIC_KEY"))
	if err != nil || len(key) != ed25519.PublicKeySize {
		slog.Error("SERVER_CLAIM_PUBLIC_KEY must be base64 Ed25519 public key")
		os.Exit(1)
	}
	roster := map[string]int{}
	for _, value := range strings.Split(os.Getenv("MATCH_ROSTER"), ",") {
		fields := strings.SplitN(value, ":", 2)
		if len(fields) == 2 {
			slot, parseErr := strconv.Atoi(fields[1])
			if parseErr == nil {
				roster[fields[0]] = slot
			}
		}
	}
	var lifecycle simulator.Lifecycle
	var readyReporter simulator.Lifecycle
	var sdk *agonessdk.SDK
	if os.Getenv("AGONES_ENABLED") == "true" {
		var sdkErr error
		sdk, sdkErr = agonessdk.NewSDK()
		if sdkErr != nil {
			slog.Error("Agones SDK initialization failed", "error", sdkErr)
			os.Exit(1)
		}
		lifecycle = agonesLifecycle{sdk: sdk}
	}
	dynamic := os.Getenv("AGONES_DYNAMIC_ASSIGNMENT") == "true"
	assignedMatchID := os.Getenv("MATCH_ID")
	assignedServerToken := os.Getenv("SERVER_RESULT_TOKEN")
	if os.Getenv("CONTROL_API_URL") != "" {
		readyReporter = simulator.HTTPReadyLifecycle{ControlURL: os.Getenv("CONTROL_API_URL"), TokenSource: func() string { return assignedServerToken }, MatchID: func() string { return assignedMatchID }}
	}
	server, err := simulator.New(simulator.Config{MatchID: os.Getenv("MATCH_ID"), AllocationID: os.Getenv("ALLOCATION_ID"), Build: os.Getenv("SERVER_BUILD"), Roster: roster, PublicKey: ed25519.PublicKey(key), Lifecycle: lifecycle, ResultSink: &simulator.HTTPResultSink{ControlURL: os.Getenv("CONTROL_API_URL"), Token: os.Getenv("SERVER_RESULT_TOKEN")}, DynamicAssignment: dynamic})
	if err != nil {
		slog.Error("simulator configuration failed", "error", err)
		os.Exit(1)
	}
	if err := server.Bootstrap(); err != nil {
		slog.Error("simulator bootstrap failed", "error", err)
		os.Exit(1)
	}
	if dynamic {
		go watchAssignment(sdk, server, readyReporter, &assignedMatchID, &assignedServerToken)
	}
	addr := os.Getenv("SIMULATOR_ADDR")
	if addr == "" {
		addr = ":7000"
	}
	slog.Info("simulator listening", "addr", addr)
	if err := http.ListenAndServe(addr, server.Handler()); err != nil {
		slog.Error("simulator stopped", "error", err)
		os.Exit(1)
	}
}

func watchAssignment(sdk *agonessdk.SDK, server *simulator.Server, readyReporter simulator.Lifecycle, assignedMatchID, assignedServerToken *string) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for range ticker.C {
		gameServer, err := sdk.GameServer()
		if err != nil || gameServer == nil || gameServer.GetObjectMeta() == nil {
			continue
		}
		annotations := gameServer.GetObjectMeta().GetAnnotations()
		matchID := annotations["gameservice.io/match-id"]
		allocationID := annotations["gameservice.io/allocation-id"]
		build := annotations["gameservice.io/server-build"]
		roster := parseRoster(annotations["gameservice.io/match-roster"])
		serverToken := annotations["gameservice.io/server-token"]
		if err := server.AssignWithServerToken(matchID, allocationID, build, roster, serverToken); err == nil {
			*assignedMatchID = matchID
			*assignedServerToken = serverToken
			if readyReporter != nil {
				if err := readyReporter.Ready(); err != nil {
					slog.Error("control plane ready report failed", "error", err)
					continue
				}
			}
			return
		}
	}
}

func parseRoster(value string) map[string]int {
	roster := map[string]int{}
	for _, item := range strings.Split(value, ",") {
		fields := strings.SplitN(item, ":", 2)
		if len(fields) != 2 || strings.TrimSpace(fields[0]) == "" {
			continue
		}
		slot, err := strconv.Atoi(fields[1])
		if err == nil && slot >= 0 {
			roster[fields[0]] = slot
		}
	}
	return roster
}

type agonesLifecycle struct {
	sdk interface {
		Ready() error
		Health() error
		Shutdown() error
	}
}

func (a agonesLifecycle) Ready() error    { return a.sdk.Ready() }
func (a agonesLifecycle) Health() error   { return a.sdk.Health() }
func (a agonesLifecycle) Shutdown() error { return a.sdk.Shutdown() }
