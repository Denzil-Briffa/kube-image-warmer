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
printf '%s\n' 'Chart render and invalid-value checks passed'
