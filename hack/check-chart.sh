#!/usr/bin/env bash
set -euo pipefail

helm_bin="${1:-helm}"
project_dir="$(cd -- "$(dirname -- "$0")/.." && pwd)"
chart_dir="$project_dir/charts/image-warmer"
schedule="${SCHEDULE:?Set SCHEDULE to a five-field cron expression for validation}"

"$helm_bin" lint "$chart_dir" --strict --set-string "operator.schedule=$schedule"
# Exercise optional resources as well as the default full-cluster policy.
"$helm_bin" template image-warmer "$chart_dir" \
  --namespace image-warmer-system --include-crds \
  --set-string "operator.schedule=$schedule" >/dev/null
"$helm_bin" template image-warmer "$chart_dir" \
  --namespace alternative-namespace \
  --set-string "operator.schedule=$schedule" \
  --set operator.metrics.enabled=true \
  --set policy.enabled=false \
  --set serviceAccount.create=false \
  --set-string serviceAccount.name=existing-controller >/dev/null


# Verify both default helper-image resolution and the deployment override.
assert_helper_image() {
  local expected="$1"
  shift
  local rendered
  rendered="$("$helm_bin" template image-warmer "$chart_dir" \
    --set-string "operator.schedule=$schedule" "$@")"
  if ! printf '%s\n' "$rendered" | grep -F -- "--warmup-helper-image=$expected" >/dev/null; then
    printf 'Incorrect helper image wiring, expected %s\n' "$expected" >&2
    exit 1
  fi
}
assert_helper_image example.com/operator:test \
  --set-string image.repository=example.com/operator --set-string image.tag=test
assert_helper_image example.com/helper:test --set-string operator.helperImage=example.com/helper:test
assert_helper_image "example.com/operator@sha256:$(printf '%064d' 0)" \
  --set-string image.repository=example.com/operator \
  --set-string "image.digest=sha256:$(printf '%064d' 0)"

# Exercise an aliased dependency as used by the GitOps wrapper. Helm propagates
# its reserved global object into subcharts even when it is empty.
wrapper_dir="$(mktemp -d)"
trap 'rm -rf -- "$wrapper_dir"' EXIT
mkdir -p "$wrapper_dir/charts"
cp -R -- "$chart_dir" "$wrapper_dir/charts/image-warmer"
cat > "$wrapper_dir/Chart.yaml" <<'YAML'
apiVersion: v2
name: wrapper-check
version: 0.1.0
dependencies:
  - name: image-warmer
    alias: kube-image-warmer
    version: "*"
YAML
"$helm_bin" template wrapper-check "$wrapper_dir" \
  --namespace image-warmer-system --include-crds \
  --set-string "kube-image-warmer.operator.schedule=$schedule" >/dev/null
"$helm_bin" template wrapper-check "$wrapper_dir" \
  --namespace image-warmer-system \
  --set-string "kube-image-warmer.operator.schedule=$schedule" \
  --set-string global.example=wrapper-check >/dev/null

expect_invalid() {
  if "$helm_bin" template image-warmer "$chart_dir" \
      --set-string "operator.schedule=$schedule" "$@" >/dev/null 2>&1; then
    printf 'Chart unexpectedly accepted invalid settings: %s\n' "$*" >&2
    exit 1
  fi
}
expect_invalid --set-string operator.schedule=
expect_invalid --set-string 'operator.schedule=* * * * * *'
expect_invalid --set policy.spec.maxConcurrentJobs=0
expect_invalid --set replicaCount=2 --set operator.leaderElection=false
expect_invalid --set registryCredentials.create=true
expect_invalid --set-string "operator.helperImage=invalid reference"
printf '%s\n' 'Chart render and invalid-value checks passed'
