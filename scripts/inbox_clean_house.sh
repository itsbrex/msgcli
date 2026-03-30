#!/usr/bin/env bash
set -euo pipefail

# Conservative inbox cleanup + triage report for msgcli.
# Dry-run by default; only moves mail when --apply is set.

ACCOUNT=""
LIMIT=300
APPLY=0
AUDIT_ROOT="docs/inbox-audit"
TARGET_ACCOUNT_ALIAS=""
TARGET_ACCOUNT_EMAIL=""
TARGET_ACCOUNT_VALID=""

usage() {
  cat <<'EOF'
Usage:
  scripts/inbox_clean_house.sh [--account <alias>] [--limit <n>] [--apply] [--audit-root <dir>]

Behavior:
  - Always writes JSON audit artifacts to <audit-root>/<timestamp>/
  - Builds:
      - candidates_safe_auto_archive.json
      - candidates_review_first.json
      - actionable_reply_queue_sample.json
  - Dry-run by default (no mail moved)
  - With --apply, moves only strict safe candidates to Archive
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --account)
      ACCOUNT="${2:-}"
      shift 2
      ;;
    --limit)
      LIMIT="${2:-300}"
      shift 2
      ;;
    --apply)
      APPLY=1
      shift
      ;;
    --audit-root)
      AUDIT_ROOT="${2:-docs/inbox-audit}"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown arg: $1" >&2
      usage
      exit 1
      ;;
  esac
done

if ! command -v msgcli >/dev/null 2>&1; then
  echo "msgcli not found on PATH" >&2
  exit 1
fi
if ! command -v jq >/dev/null 2>&1; then
  echo "jq not found on PATH" >&2
  exit 1
fi

if [[ "${APPLY}" -eq 1 && -z "${ACCOUNT}" ]]; then
  echo "Error: --account is required when --apply is set." >&2
  exit 1
fi

if [[ -n "${ACCOUNT}" ]]; then
  if ! target_status_json="$(msgcli auth status -a "${ACCOUNT}" -o json)"; then
    echo "Error: unable to resolve account '${ACCOUNT}' via msgcli auth status." >&2
    exit 1
  fi

  target_account_json="$(jq -c 'first((.accounts // [])[]?) // empty' <<<"${target_status_json}")"
  if [[ -z "${target_account_json}" ]]; then
    echo "Error: could not resolve account '${ACCOUNT}' to a canonical account." >&2
    exit 1
  fi

  TARGET_ACCOUNT_ALIAS="$(jq -r '.alias // ""' <<<"${target_account_json}")"
  TARGET_ACCOUNT_EMAIL="$(jq -r '.email // ""' <<<"${target_account_json}")"
  TARGET_ACCOUNT_VALID="$(jq -r '.valid // ""' <<<"${target_account_json}")"

  if [[ "${APPLY}" -eq 1 && "${TARGET_ACCOUNT_VALID}" != "true" ]]; then
    echo "Error: target account '${ACCOUNT}' is not valid for apply mode." >&2
    exit 1
  fi

  if [[ -n "${TARGET_ACCOUNT_ALIAS}" ]]; then
    ACCOUNT="${TARGET_ACCOUNT_ALIAS}"
  elif [[ -n "${TARGET_ACCOUNT_EMAIL}" ]]; then
    ACCOUNT="${TARGET_ACCOUNT_EMAIL}"
  else
    echo "Error: resolved account missing alias/email; cannot scope mailbox operations safely." >&2
    exit 1
  fi

  echo "Target account (canonical): alias='${TARGET_ACCOUNT_ALIAS:-n/a}', email='${TARGET_ACCOUNT_EMAIL:-n/a}', valid='${TARGET_ACCOUNT_VALID:-unknown}'"
fi

timestamp="$(date +'%Y-%m-%d_%H-%M-%S')"
audit_dir="${AUDIT_ROOT}/${timestamp}"
mkdir -p "${audit_dir}/logs"

account_flag=()
if [[ -n "${ACCOUNT}" ]]; then
  account_flag=(-a "${ACCOUNT}")
fi

msgcli auth list "${account_flag[@]}" -o json > "${audit_dir}/auth_list.json"
msgcli auth status "${account_flag[@]}" -o json > "${audit_dir}/auth_status.json"
msgcli mail folders "${account_flag[@]}" -o json > "${audit_dir}/folders_before.json"
msgcli mail list "${account_flag[@]}" -o json --folder inbox --limit "${LIMIT}" > "${audit_dir}/inbox_sample_before.json"
msgcli mail list "${account_flag[@]}" -o json --folder inbox --limit "${LIMIT}" --query "isRead:false" > "${audit_dir}/inbox_unread_query_sample.json"

# Strict safe set: automated marketing/alert senders only.
jq '[.[] |
    {id,
     subject,
     from:(.from.emailAddress.address//""),
     fromName:(.from.emailAddress.name//""),
     receivedDateTime,
     isRead,
     hasAttachments}
  ] |
  map(select(.from|ascii_downcase|test("^(no-reply@alerts\\.costar\\.com|no-reply@c\\.costarmail\\.com|salesforge@mail\\.beehiiv\\.com|webinars@e\\.zoom\\.us|noreply-marketplace@zoom\\.us)$"))) |
  map(. + {bucket:"safe_auto_archive", reason:"Automated newsletter/alert sender", confidence:0.97})' \
  "${audit_dir}/inbox_sample_before.json" > "${audit_dir}/candidates_safe_auto_archive.json"

# Potential clutter that may still matter. Do not auto-move.
jq '[.[] |
    {id,
     subject,
     from:(.from.emailAddress.address//""),
     fromName:(.from.emailAddress.name//""),
     receivedDateTime,
     isRead,
     hasAttachments}
  ] |
  map(select(.from|ascii_downcase|test("^(daily@insights\\.rensystems\\.com|communications@cresa\\.com|postmaster@cresa\\.com)$"))) |
  map(. + {bucket:"review_first", reason:"High-volume notifications; may include useful context", confidence:0.65})' \
  "${audit_dir}/inbox_sample_before.json" > "${audit_dir}/candidates_review_first.json"

# Action queue sample from unread inbox items, excluding obvious automated senders.
jq '[.[] |
    select(.isRead==false) |
    {id,
     subject,
     from:(.from.emailAddress.address//""),
     fromName:(.from.emailAddress.name//""),
     receivedDateTime,
     hasAttachments,
     bodyPreview}
  ] |
  map(select((.from|ascii_downcase|test("^(no-reply|noreply|postmaster|communications@cresa\\.com|daily@insights\\.rensystems\\.com|salesforce@cresa\\.com|microsoftsecurity|microsoft|zoom|beehiiv|alerts\\.costar|costarmail|mailer-daemon|notification|notifications@)"))|not)) |
  map(. + {priority:(if (.subject|test("\\b(urgent|asap|today|deadline|confirm|approval|invoice|rfp|lease|proposal|next steps|action required)\\b";"i")) then "high" else "normal" end)})' \
  "${audit_dir}/inbox_sample_before.json" > "${audit_dir}/actionable_reply_queue_sample.json"

safe_count="$(jq 'length' "${audit_dir}/candidates_safe_auto_archive.json")"
review_count="$(jq 'length' "${audit_dir}/candidates_review_first.json")"
reply_count="$(jq 'length' "${audit_dir}/actionable_reply_queue_sample.json")"
high_priority_count="$(jq '[.[]|select(.priority=="high")]|length' "${audit_dir}/actionable_reply_queue_sample.json")"

if [[ "${APPLY}" -eq 1 ]]; then
  jq -r '.[].id' "${audit_dir}/candidates_safe_auto_archive.json" > "${audit_dir}/logs/safe_ids_to_move.txt"
  : > "${audit_dir}/logs/move_results.ndjson"
  while IFS= read -r id; do
    [[ -z "${id}" ]] && continue
    ts="$(date -u +'%Y-%m-%dT%H:%M:%SZ')"
    is_read="$(jq -r --arg id "${id}" 'first(.[] | select(.id==$id) | .isRead) // false' "${audit_dir}/candidates_safe_auto_archive.json")"
    if [[ "${is_read}" != "true" ]]; then
      echo "Skipping unread safe-auto candidate id=${id} (isRead=${is_read})" >&2
      jq -nc --arg ts "${ts}" --arg id "${id}" --arg status "skipped_unread" --arg reason "isRead guard blocked auto-archive" \
        '{timestamp:$ts,id:$id,status:$status,reason:$reason}' >> "${audit_dir}/logs/move_results.ndjson"
      continue
    fi
    if out="$(msgcli mail move "${account_flag[@]}" "${id}" --folder archive -o json 2>&1)"; then
      jq -nc --arg ts "${ts}" --arg id "${id}" --arg status "ok" --arg output "${out}" \
        '{timestamp:$ts,id:$id,status:$status,raw_output:$output}' >> "${audit_dir}/logs/move_results.ndjson"
    else
      code="$?"
      jq -nc --arg ts "${ts}" --arg id "${id}" --arg status "error" --arg output "${out}" --argjson code "${code}" \
        '{timestamp:$ts,id:$id,status:$status,exit_code:$code,raw_output:$output}' >> "${audit_dir}/logs/move_results.ndjson"
    fi
  done < "${audit_dir}/logs/safe_ids_to_move.txt"
  jq -s '.' "${audit_dir}/logs/move_results.ndjson" > "${audit_dir}/move_results.json"
else
  echo '[]' > "${audit_dir}/move_results.json"
fi

msgcli mail folders "${account_flag[@]}" -o json > "${audit_dir}/folders_after.json"
msgcli mail list "${account_flag[@]}" -o json --folder inbox --limit "${LIMIT}" > "${audit_dir}/inbox_sample_after.json"

jq -n \
  --slurpfile before "${audit_dir}/folders_before.json" \
  --slurpfile after "${audit_dir}/folders_after.json" \
  --slurpfile moved "${audit_dir}/move_results.json" \
  --argjson safe_count "${safe_count}" \
  --argjson review_count "${review_count}" \
  --argjson reply_count "${reply_count}" \
  --argjson high_priority_count "${high_priority_count}" \
  --argjson applied "${APPLY}" '
  def stat($arr; $name):
    (first(($arr[0][]? | select(.displayName==$name) | {unreadItemCount,totalItemCount}))? // {unreadItemCount:null,totalItemCount:null});
  {
    timestamp: (now|todate),
    apply_mode: ($applied==1),
    candidate_counts: {
      safe_auto_archive: $safe_count,
      review_first: $review_count,
      actionable_reply_queue: $reply_count,
      actionable_high_priority: $high_priority_count
    },
    moved_ok: ($moved[0] | map(select(.status=="ok")) | length),
    moved_error: ($moved[0] | map(select(.status=="error")) | length),
    inbox_before: stat($before;"Inbox"),
    inbox_after: stat($after;"Inbox"),
    archive_before: stat($before;"Archive"),
    archive_after: stat($after;"Archive")
  }' > "${audit_dir}/summary.json"

if [[ ! -s "${audit_dir}/summary.json" ]]; then
  echo "Error: summary materialization failed: ${audit_dir}/summary.json is empty." >&2
  exit 1
fi

# Refresh run/file indexes so triage UI auto-discovers this run.
if [[ -x "scripts/update_inbox_audit_indexes.sh" ]]; then
  scripts/update_inbox_audit_indexes.sh "${AUDIT_ROOT}" >/dev/null
fi

echo "Audit directory: ${audit_dir}"
cat "${audit_dir}/summary.json"
