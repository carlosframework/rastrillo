#!/usr/bin/env bash
# Proves examples/helloworld ships and serves through the *real*
# carlosframework/platform binary — carlos ship, carlos promote,
# carlos add, carlos edge -dev — all against a local directory store
# and a local registry, no AWS/SSH required. This is what "deployed on
# the platform" means until this has real credentials for a live
# deployment: same release/registry/edge code, local bucket instead of
# S3, no TLS/ACME instead of a real domain.
#
# Usage: PLATFORM_REPO=/path/to/carlosframework/platform hack/local-deploy-demo.sh
set -euo pipefail

PLATFORM_REPO="${PLATFORM_REPO:?set PLATFORM_REPO to a checkout of carlosframework/platform}"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WORK="$(mktemp -d)"
trap 'kill "${BACKEND_PID:-0}" "${EDGE_PID:-0}" 2>/dev/null || true; rm -rf "$WORK"' EXIT

echo "==> building carlos (platform) and helloworld (rastrillo example)"
(cd "$PLATFORM_REPO" && go build -o "$WORK/carlos" .)
(cd "$ROOT/examples/helloworld" && go build -o "$WORK/helloworld" ./cmd/helloworld)

echo "==> generating a release signing key"
eval "$("$WORK/carlos" release-keygen)"
export CARLOS_RELEASE_KEY CARLOS_RELEASE_PUBKEY
export CARLOS_DEPLOYMENT_DIR="$WORK/bucket"
export CARLOS_DATA_DIR="$WORK"

VER="$(git -C "$ROOT" rev-parse --short HEAD 2>/dev/null || echo dev)"
echo "==> shipping + promoting helloworld $VER"
"$WORK/carlos" ship -app helloworld -version "$VER" -target "$(go env GOOS)-$(go env GOARCH)" "$WORK/helloworld"
"$WORK/carlos" promote -app helloworld "$VER" stable

echo "==> starting the backend"
"$WORK/helloworld" -addr 127.0.0.1:9101 >"$WORK/backend.log" 2>&1 &
BACKEND_PID=$!

echo "==> registering the route + starting the edge in dev mode"
"$WORK/carlos" add -registry "$WORK/registry.db" -app helloworld -kind instance -addr 127.0.0.1:9101 -channel stable helloworld.local
CARLOS_RELEASE_PUBKEY="$CARLOS_RELEASE_PUBKEY" "$WORK/carlos" edge -dev -http :9090 -registry "$WORK/registry.db" >"$WORK/edge.log" 2>&1 &
EDGE_PID=$!
sleep 2

echo "==> through the edge, by Host header:"
curl -sf -H "Host: helloworld.local" http://127.0.0.1:9090/
echo
curl -sf -H "Host: helloworld.local" http://127.0.0.1:9090/healthz
echo
echo "==> unknown host still 404s:"
curl -s -o /dev/null -w "%{http_code}\n" -H "Host: nope.local" http://127.0.0.1:9090/

echo "==> OK"
