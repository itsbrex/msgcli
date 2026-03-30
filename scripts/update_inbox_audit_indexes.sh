#!/usr/bin/env bash
set -euo pipefail

AUDIT_ROOT="${1:-docs/inbox-audit}"

if [[ ! -d "${AUDIT_ROOT}" ]]; then
  echo "Audit root not found: ${AUDIT_ROOT}" >&2
  exit 1
fi

runs=()
for d in "${AUDIT_ROOT}"/*/; do
  [[ -d "${d}" ]] || continue
  run="${d%/}"
  run_name="${run##*/}"
  runs+=("${run_name}")

  find "${run}" -type f -name '*.json' ! -name 'index.json' \
    | sed "s#^${run}/##" \
    | sort \
    | jq -R -s 'split("\n") | map(select(length>0))' > "${run}/index.json"
done

printf '%s\n' "${runs[@]}" \
  | sort \
  | jq -R -s 'split("\n") | map(select(length>0))' > "${AUDIT_ROOT}/runs_index.json"

echo "Updated ${AUDIT_ROOT}/runs_index.json and per-run index.json files."
