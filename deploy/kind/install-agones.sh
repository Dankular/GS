#!/bin/sh
set -eu
AGONES_VERSION="1.60.0"
KIND_NODE_IMAGE="kindest/node:v1.36.1"
command -v kind >/dev/null || { echo 'kind is required' >&2; exit 1; }
HELM_BIN="${HELM_BIN:-helm}"
if command -v helm3 >/dev/null 2>&1; then HELM_BIN=helm3; fi
command -v "$HELM_BIN" >/dev/null || { echo 'helm is required' >&2; exit 1; }
command -v kubectl >/dev/null || { echo 'kubectl is required' >&2; exit 1; }
command -v docker >/dev/null || { echo 'docker is required' >&2; exit 1; }
if ! kind get clusters | grep -qx gameservice; then
  kind create cluster --name gameservice --image "$KIND_NODE_IMAGE"
fi
"$HELM_BIN" repo add agones https://agones.dev/chart/stable
"$HELM_BIN" repo update
# Kind is a single-node validation cluster; keep Agones control-plane replicas
# at one so the test does not fail solely from local capacity. Production
# deployments retain the chart's HA defaults in the production values.
"$HELM_BIN" upgrade --install agones agones/agones --namespace agones-system --create-namespace --version "$AGONES_VERSION" --set agones.controller.replicas=1 --set agones.extensions.replicas=1 --set agones.allocator.replicas=1 --set agones.ping.replicas=1 --wait
docker build -f deploy/compose/simulator-server.Dockerfile -t gameservice-simulator:dev .
kind load docker-image gameservice-simulator:dev --name gameservice
kubectl apply -f deploy/kind/simulator-fleet.yaml
kubectl rollout status deployment/agones-controller -n agones-system --timeout=180s
kubectl wait --for=condition=Ready pod -l agones.dev/fleet-name=arena-deathmatch -n platform-gameservers-eu-west --timeout=180s
