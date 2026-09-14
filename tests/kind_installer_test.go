package tests

import (
	"os"
	"strings"
	"testing"
)

func TestKindInstallerUsesReachableDockerHostForControlAPI(t *testing.T) {
	data, err := os.ReadFile("../deploy/kind/install-agones.sh")
	if err != nil {
		t.Fatal(err)
	}
	script := string(data)
	if !strings.Contains(script, "docker network inspect -f '{{range .IPAM.Config}}{{.Gateway}} {{end}}' kind") {
		t.Fatal("Kind installer must resolve the Docker bridge gateway for pod-to-Compose traffic")
	}
	if !strings.Contains(script, "http://${CONTROL_API_HOST}:8080") {
		t.Fatal("Kind installer must inject the reachable control API host")
	}
	if strings.Contains(script, "CONTROL_API_IP=") {
		t.Fatal("Kind installer must not inject an individual Compose container IP")
	}
}
