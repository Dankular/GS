#!/bin/sh
set -eu
AGONES_VERSION="1.60.0"
KIND_NODE_IMAGE="kindest/node@sha256:3489c7674813ba5d8b1a9977baea8a6e553784dab7b84759d1014dbd78f7ebd5"
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
"$HELM_BIN" upgrade --install agones agones/agones --namespace agones-system --create-namespace --version "$AGONES_VERSION" \
  --set agones.controller.replicas=1 \
  --set agones.extensions.replicas=1 \
  --set agones.allocator.replicas=1 \
  --set agones.ping.replicas=1 \
  --set agones.allocator.service.serviceType=NodePort \
  --set agones.ping.http.serviceType=NodePort \
  --set agones.ping.udp.serviceType=NodePort \
  --wait
NODE_IP="$(docker inspect -f '{{(index .NetworkSettings.Networks "kind").IPAddress}}' gameservice-control-plane)"
CONTROL_API_CONTAINER="${CONTROL_API_CONTAINER:-compose-control-api-1}"
docker network connect kind "$CONTROL_API_CONTAINER" 2>/dev/null || true
CONTROL_API_IP="$(docker inspect -f '{{(index .NetworkSettings.Networks "kind").IPAddress}}' "$CONTROL_API_CONTAINER")"
# The chart's development certificate does not include the Kind node IP, while
# the Docker Compose worker reaches the NodePort through that IP. Re-issue only
# the dev allocator server certificate with the exact SAN used by the worker;
# client authentication continues to use Agones' separate allocator-client-ca.
CERT_DIR="$(mktemp -d)"
trap 'rm -rf "$CERT_DIR"' EXIT
openssl req -x509 -newkey rsa:2048 -nodes -subj /CN=gameservice-agones-dev-ca \
  -keyout "$CERT_DIR/ca.key" -out "$CERT_DIR/ca.crt" -days 3650 >/dev/null 2>&1
openssl req -newkey rsa:2048 -nodes -subj /CN=agones-allocator \
  -keyout "$CERT_DIR/server.key" -out "$CERT_DIR/server.csr" >/dev/null 2>&1
cat > "$CERT_DIR/server.ext" <<EOF
subjectAltName=IP:${NODE_IP},DNS:agones-allocator.agones-system.svc,DNS:agones-allocator.agones-system.svc.cluster.local
extendedKeyUsage=serverAuth
EOF
openssl x509 -req -in "$CERT_DIR/server.csr" -CA "$CERT_DIR/ca.crt" -CAkey "$CERT_DIR/ca.key" \
  -CAcreateserial -out "$CERT_DIR/server.crt" -days 365 -sha256 -extfile "$CERT_DIR/server.ext" >/dev/null 2>&1
kubectl create secret tls allocator-tls -n agones-system --cert "$CERT_DIR/server.crt" --key "$CERT_DIR/server.key" --dry-run=client -o yaml | kubectl apply -f -
kubectl create secret generic allocator-tls-ca -n agones-system --from-file=tls-ca.crt="$CERT_DIR/ca.crt" --dry-run=client -o yaml | kubectl apply -f -
kubectl rollout restart deployment/agones-allocator -n agones-system
kubectl rollout status deployment/agones-allocator -n agones-system --timeout=180s
ALLOCATOR_DIR="${AGONES_ALLOCATOR_SECRET_DIR:-/var/lib/gameservice/allocator}"
mkdir -p "$ALLOCATOR_DIR"
kubectl get secret allocator-client.default -n default -o yaml | awk '$1=="tls.crt:"{print $2}' | base64 -d > "$ALLOCATOR_DIR/client.crt"
kubectl get secret allocator-client.default -n default -o yaml | awk '$1=="tls.key:"{print $2}' | base64 -d > "$ALLOCATOR_DIR/client.key"
kubectl get secret allocator-tls-ca -n agones-system -o yaml | awk '$1=="tls-ca.crt:"{print $2}' | base64 -d > "$ALLOCATOR_DIR/ca.pem"
chown -R 65532:65532 "$ALLOCATOR_DIR"
chmod 700 "$ALLOCATOR_DIR"
chmod 600 "$ALLOCATOR_DIR"/*
if docker inspect compose-matchmaking-worker-1 >/dev/null 2>&1; then
  docker restart compose-matchmaking-worker-1 >/dev/null
fi
docker build -f deploy/compose/simulator-server.Dockerfile -t gameservice-simulator:dev .
kind load docker-image gameservice-simulator:dev --name gameservice
if [ -z "${SERVER_CLAIM_PUBLIC_KEY:-}" ] && [ -f .env ]; then
  SERVER_CLAIM_PUBLIC_KEY="$(awk -F= '$1 == "SERVER_CLAIM_PUBLIC_KEY" { print $2; exit }' .env)"
fi
PUBLIC_KEY="${SERVER_CLAIM_PUBLIC_KEY:-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA}"
kubectl create secret generic gameservice-server-claims --namespace platform-gameservers-eu-west \
  --from-literal="public-key=$PUBLIC_KEY" --dry-run=client -o yaml | kubectl apply -f -
sed "s|__CONTROL_API_URL__|http://${CONTROL_API_IP}:8080|g" deploy/kind/simulator-fleet.yaml | kubectl apply -f -
kubectl rollout status deployment/agones-controller -n agones-system --timeout=180s
kubectl wait --for=jsonpath='{.status.state}'=Ready gameserver -l agones.dev/fleet=arena-deathmatch -n platform-gameservers-eu-west --timeout=180s
