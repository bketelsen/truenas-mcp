#!/usr/bin/env bash
set +x # Avoid leaking TRUENAS_API_KEY if the script is invoked with bash -x.
set -euo pipefail

usage() {
  cat <<'USAGE'
Safe first-contact runbook for truenas-mcp.

This script builds the server, checks that writes are disabled, and prints the
read-only command/configuration to use for first contact with TrueNAS SCALE.
It does not store or print your API key.
It does not contact TrueNAS unless you pass --run-server.

Required environment:
  TRUENAS_HOST       TrueNAS host, e.g. truenas.local
  TRUENAS_API_KEY    TrueNAS API key

Optional environment:
  TRUENAS_TLS_INSECURE=true   Allow self-signed TLS certificates

Required local tools:
  make, go, python3

Usage:
  scripts/first-contact.sh              # validate/build and print runbook
  scripts/first-contact.sh --run-server # exec the read-only MCP server
  scripts/first-contact.sh --help
USAGE
}

mode="plan"
case "${1:-}" in
  "") ;;
  --run-server) mode="run" ;;
  --help|-h) usage; exit 0 ;;
  *) echo "unknown argument: $1" >&2; usage >&2; exit 2 ;;
esac

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

fail() {
  echo "error: $*" >&2
  exit 1
}

[[ -n "${TRUENAS_HOST:-}" ]] || fail "TRUENAS_HOST is required"
[[ -n "${TRUENAS_API_KEY:-}" ]] || fail "TRUENAS_API_KEY is required"

if [[ -n "${TRUENAS_ENABLE_WRITES:-}" ]]; then
  fail "TRUENAS_ENABLE_WRITES is set. Unset it for first contact: unset TRUENAS_ENABLE_WRITES"
fi

if [[ "${TRUENAS_HOST}" == http://* || "${TRUENAS_HOST}" == https://* ]]; then
  fail "TRUENAS_HOST should be a host name/address only, not a URL"
fi

if [[ "${TRUENAS_TLS_INSECURE:-}" =~ ^(1|true|TRUE|yes|YES|on|ON)$ ]]; then
  tls_args=(--tls-insecure)
  tls_note="TLS verification will be skipped because TRUENAS_TLS_INSECURE is set."
else
  tls_args=()
  tls_note="TLS verification is enabled."
fi

echo "==> Building truenas-mcp"
make build

echo
echo "==> Safety checks"
echo "writes: disabled (TRUENAS_ENABLE_WRITES is unset)"
echo "$tls_note"
echo "api key: present, not printed"

declare -a cmd=("$repo_root/truenas-mcp" serve --host "$TRUENAS_HOST" --api-key '***')
if ((${#tls_args[@]})); then
  cmd+=("${tls_args[@]}")
fi

echo
echo "==> Read-only command shape"
printf '%q ' "${cmd[@]}"
echo

echo
echo "==> First prompts to use from your MCP client"
echo "- List the available TrueNAS tools."
echo "- Run truenas_health_report and summarize the result."
echo "- Run truenas_apps_update_report; do not update anything."
echo "- List recent TrueNAS jobs with truenas_jobs_list."

echo
echo "==> Claude MCP config template"
python3 - <<'PY'
import json, os
repo = os.getcwd()
args = ["serve", "--host", os.environ["TRUENAS_HOST"], "--api-key", "YOUR_API_KEY"]
if os.environ.get("TRUENAS_TLS_INSECURE", "").lower() in {"1", "true", "yes", "on"}:
    args.append("--tls-insecure")
print(json.dumps({
    "mcpServers": {
        "truenas": {
            "command": f"{repo}/truenas-mcp",
            "args": args,
        }
    }
}, indent=2))
PY

if [[ "$mode" == "run" ]]; then
  echo
  echo "==> Starting read-only MCP server over stdio"
  exec "$repo_root/truenas-mcp" serve --host "$TRUENAS_HOST" --api-key "$TRUENAS_API_KEY" "${tls_args[@]}"
fi

echo
echo "Run with --run-server when you are ready to start the read-only MCP server."
