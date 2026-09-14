#!/bin/sh
set -eu
AGONES_VERSION="1.60.0"
command -v kind >/dev/null || { echo 'kind is required' >&2; exit 1; }
command -v helm >/dev/null || { echo 'helm is required' >&2; exit 1; }
kind create cluster --name gameservice
helm repo add agones https://agones.dev/chart/stable
helm repo update
helm upgrade --install agones agones/agones --namespace agones-system --create-namespace --version "$AGONES_VERSION" --wait
