#!/usr/bin/env bash
set -euo pipefail

ensure_label() {
  local labels
  labels=$(gh label list --search "$1" --json name --jq '.[].name')
  if ! grep -qxF "$1" <<< "$labels"; then
    gh label create "$1" --color "$2" --description "$3"
  fi
}

confirm_issue() {
  local url="$1" label="$2"
  if ! gh issue view "$url" --json labels --jq '.labels[].name' | grep -qxF "$label"; then
    echo "::error::issue does not confirm $label: $url" >&2
    return 1
  fi
  if ! gh issue view "$url" --json assignees --jq '.assignees[].login' | grep -qxF "$INCIDENT_ASSIGNEE"; then
    echo "::error::issue does not confirm $INCIDENT_ASSIGNEE: $url" >&2
    return 1
  fi
}

file_once() {
  local title="$1" label="$2" body="$3" existing listed_title listed_url created
  existing=$(gh issue list --state all --search "in:title \"$title\"" --limit 100 --json title,url --jq '.[] | [.title, .url] | @tsv')
  while IFS=$'\t' read -r listed_title listed_url; do
    [ "$listed_title" = "$title" ] || continue
    confirm_issue "$listed_url" "$label"
    echo "::notice::issue already exists: $title"
    return
  done <<< "$existing"
  created=$(gh issue create --title "$title" --label "$label" --assignee "$INCIDENT_ASSIGNEE" --body "$body")
  confirm_issue "$created" "$label"
  echo "$created"
}

if [ "${ESCAPE:-}" = 1 ]; then
  ensure_label sev/P1 B60205 'Stop dispatch and investigate immediately'
  HALT="curl -X PUT -b \"\$COOKIE\" -H \"Content-Type: application/json\" -d '{\"note\":\"suspected gVisor escape-class advisory, pool not yet patched\"}' https://<host>/admin/dispatch/halt"
  while IFS=$'\t' read -r id severity summary source; do
    [ -n "$id" ] || continue
    TITLE="gVisor security advisory $id"
    BODY=$(printf "Severity: provisional P1, pending escape-class triage.\nTrigger: %s [%s] %s\nSource: %s\nAutomatic action: no dispatch halt; operator must halt now.\nScene: %s/actions/runs/%s and the control-plane incident halt state.\n\nPatch the affected pool within 24h or disable SelfHostedProvider. To stop dispatching as an operator:\n\n\`\`\`\n%s\n\`\`\`\n\nThe halt preserves the scene and must be lifted manually after remediation.\n" "$id" "$severity" "$summary" "$source" "https://github.com/$GITHUB_REPOSITORY" "$GITHUB_RUN_ID" "$HALT")
    file_once "$TITLE" sev/P1 "$BODY"
  done < advisories.tsv
  while IFS=$'\t' read -r tag source; do
    [ -n "$tag" ] || continue
    TITLE="gVisor release $tag security review"
    BODY=$(printf "Severity: provisional P1, pending escape-class triage.\nTrigger: release notes mention escape or CVE.\nSource: %s\nAutomatic action: no dispatch halt; operator must halt now.\nScene: %s/actions/runs/%s and the control-plane incident halt state.\n\nPatch the affected pool within 24h or disable SelfHostedProvider. To stop dispatching as an operator:\n\n\`\`\`\n%s\n\`\`\`\n" "$source" "https://github.com/$GITHUB_REPOSITORY" "$GITHUB_RUN_ID" "$HALT")
    file_once "$TITLE" sev/P1 "$BODY"
  done < relnotes.tsv
fi

if [ -n "${FAIL:-}" ]; then
  ensure_label sev/P3 0969DA 'Baseline drift or warning requiring review'
  TITLE="gVisor baseline has drifted (SEC-002 P-04)"
  BODY=$(printf 'Severity: P3.\nTrigger: %s\nAutomatic action: no dispatch halt.\nScene: %s/actions/runs/%s and infra/nodes/gvisor-baseline.txt.\n\nUpdate the baseline and rebuild the pool.\n' "$FAIL" "https://github.com/$GITHUB_REPOSITORY" "$GITHUB_RUN_ID")
  existing=$(gh issue list --state open --search "in:title \"$TITLE\"" --json title --jq '.[].title')
  if grep -qxF "$TITLE" <<< "$existing"; then
    echo "::notice::baseline drift issue already open"
  else
    gh issue create --title "$TITLE" --label sev/P3 --assignee "$INCIDENT_ASSIGNEE" --body "$BODY"
  fi
fi
