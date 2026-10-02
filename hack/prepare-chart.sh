#!/usr/bin/env bash
set -euo pipefail

project_dir="$(cd -- "$(dirname -- "$0")/.." && pwd)"
chart_dir="$project_dir/charts/image-warmer"
mkdir -p "$chart_dir/crds" "$chart_dir/files"

# CRDs and permissions come from controller-gen. Do not hand-edit chart copies.
shopt -s nullglob
crds=("$project_dir"/config/crd/bases/*.yaml)
if (( ${#crds[@]} == 0 )); then
  printf '%s\n' 'No generated CRDs found. Run make manifests first.' >&2
  exit 1
fi
for old_crd in "$chart_dir"/crds/*.yaml; do
  rm -f -- "$old_crd"
done
cp -- "${crds[@]}" "$chart_dir/crds/"
for role in manager-role leader-election-role metrics-auth-role; do
  source_file="$project_dir/config/rbac/role.yaml"
  if [[ "$role" != manager-role ]]; then
    source_file="$project_dir/config/rbac/${role//-/_}.yaml"
  fi
  cp -- "$source_file" "$chart_dir/files/$role.yaml"
done
