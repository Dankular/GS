//go:build chaos

package chaos

import (
	"context"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

var allowedServices = map[string]bool{
	"control-api": true, "outbox-worker": true, "leaderboard-worker": true,
	"matchmaking-worker": true, "simulator-server": true,
}

func TestRestartServiceRecovers(t *testing.T) {
	if os.Getenv("GAMESERVICE_CHAOS_ENABLE") != "1" {
		t.Skip("set GAMESERVICE_CHAOS_ENABLE=1 to run destructive Docker chaos against an explicitly selected service")
	}
	service := os.Getenv("GAMESERVICE_CHAOS_SERVICE")
	if !allowedServices[service] {
		t.Fatalf("GAMESERVICE_CHAOS_SERVICE must be an application service, got %q", service)
	}
	composeFile := os.Getenv("GAMESERVICE_CHAOS_COMPOSE_FILE")
	if composeFile == "" {
		composeFile = "deploy/compose/compose.yaml"
	}
	composeDir := os.Getenv("GAMESERVICE_CHAOS_DIR")
	if composeDir == "" {
		composeDir = "."
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	runDocker(t, ctx, composeDir, composeFile, "kill", service)
	runDocker(t, ctx, composeDir, composeFile, "up", "-d", service)

	if service != "control-api" {
		waitForRunningService(t, ctx, composeDir, composeFile, service)
		return
	}
	healthURL := strings.TrimRight(os.Getenv("GAMESERVICE_CHAOS_HEALTH_URL"), "/")
	if healthURL == "" {
		healthURL = "http://127.0.0.1:8080/health/ready"
	}
	client := &http.Client{Timeout: 5 * time.Second}
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
		if err == nil {
			response, requestErr := client.Do(request)
			if requestErr == nil {
				response.Body.Close()
				if response.StatusCode == http.StatusOK {
					return
				}
			}
		}
		time.Sleep(2 * time.Second)
	}
	t.Fatalf("service %q did not recover at %s", service, healthURL)
}

func waitForRunningService(t *testing.T, ctx context.Context, dir, file, service string) {
	t.Helper()
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		commandArgs := []string{"compose", "-f", file, "ps", "--status", "running", "--services"}
		command := exec.CommandContext(ctx, "docker", commandArgs...)
		command.Dir = dir
		if output, err := command.Output(); err == nil {
			for _, running := range strings.Fields(string(output)) {
				if running == service {
					return
				}
			}
		}
		time.Sleep(2 * time.Second)
	}
	t.Fatalf("service %q did not return to running state", service)
}

func runDocker(t *testing.T, ctx context.Context, dir, file string, args ...string) {
	t.Helper()
	commandArgs := append([]string{"compose", "-f", file}, args...)
	command := exec.CommandContext(ctx, "docker", commandArgs...)
	command.Dir = dir
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("docker compose %s failed: %v (%s)", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
}
