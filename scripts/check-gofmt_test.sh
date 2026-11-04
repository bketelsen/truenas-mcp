#!/usr/bin/env bash
# Self-test for scripts/check-gofmt.sh: builds throwaway git repositories with
# generated fixtures (never tracked here, or the real check would read them)
# and checks that a clean file passes, and that an unformatted file, a parse
# error and an empty file list fail. The last two passed the old one-liner.
set -euo pipefail

CHECK_SCRIPT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/check-gofmt.sh"
# A mise shim cannot pick a Go outside this repository (the temp repositories
# have no go.mod), so resolve it to the real binary first.
GOFMT_PATH=${GOFMT:-gofmt}
GOFMT_PATH=$(command -v "$GOFMT_PATH")
if [[ $GOFMT_PATH == */mise/shims/* ]] && command -v mise >/dev/null 2>&1; then
  GOFMT_PATH=$(mise which gofmt)
fi
export GOFMT="$GOFMT_PATH"
tmp_root=$(mktemp -d)
trap 'rm -rf "$tmp_root"' EXIT
failures=0

run_check() {
  local repo=$1
  local output status
  output=$(cd "$repo" && GIT_CONFIG_GLOBAL=/dev/null "$CHECK_SCRIPT" 2>&1) && status=0 || status=$?
  printf '%s\n' "$status" "$output"
}

# 1. Well-formatted Go file.
repo="$tmp_root/formatted"
mkdir -p "$repo"
cat >"$repo/ok.go" <<'EOF'
package p

func ok() {}
EOF
(cd "$repo" && git init -q && git add .)
result=$(run_check "$repo")
status=${result%%$'\n'*}
if [[ $status == 0 ]]; then echo "PASS: well-formatted file"; else echo "FAIL: well-formatted file"; failures=$((failures + 1)); fi

# 2. Unformatted valid Go file.
repo="$tmp_root/unformatted"
mkdir -p "$repo"
cat >"$repo/bad.go" <<'EOF'
package p
func bad(){ }
EOF
(cd "$repo" && git init -q && git add .)
result=$(run_check "$repo")
status=${result%%$'\n'*}
output=${result#*$'\n'}
if [[ $status != 0 && $output == *bad.go* ]]; then echo "PASS: unformatted file"; else echo "FAIL: unformatted file"; failures=$((failures + 1)); fi

# 3. Parse-error Go file.
repo="$tmp_root/parse-error"
mkdir -p "$repo"
cat >"$repo/broken.go" <<'EOF'
package p
func {
EOF
(cd "$repo" && git init -q && git add .)
result=$(run_check "$repo")
status=${result%%$'\n'*}
if [[ $status != 0 ]]; then echo "PASS: parse error"; else echo "FAIL: parse error"; failures=$((failures + 1)); fi

# 4. Repository with no Go files.
repo="$tmp_root/no-go"
mkdir -p "$repo"
cat >"$repo/README" <<'EOF'
No Go sources here.
EOF
(cd "$repo" && git init -q && git add .)
result=$(run_check "$repo")
status=${result%%$'\n'*}
output=${result#*$'\n'}
if [[ $status != 0 && $output == *"check-gofmt: no tracked Go files"* ]]; then echo "PASS: no Go files"; else echo "FAIL: no Go files"; failures=$((failures + 1)); fi

# 5. Well-formatted file in a path with a space.
repo="$tmp_root/space"
mkdir -p "$repo/a b"
cat >"$repo/a b/ok.go" <<'EOF'
package p

func ok() {}
EOF
(cd "$repo" && git init -q && git add .)
result=$(run_check "$repo")
status=${result%%$'\n'*}
if [[ $status == 0 ]]; then echo "PASS: path with space"; else echo "FAIL: path with space"; failures=$((failures + 1)); fi

((failures == 0))
