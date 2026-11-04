#!/usr/bin/env bash
# Fail unless every tracked Go file is gofmt -s clean. Used by make
# verify-static and the CI Verify job. Unlike the old one-liner
# `test -z "$(gofmt -s -l $(git ls-files '*.go'))"`, this fails closed: a gofmt
# error (for example a parse error) fails, and an empty file list fails instead
# of letting gofmt read stdin. Self-test: scripts/check-gofmt_test.sh.
set -euo pipefail

GOFMT="${GOFMT:-gofmt}"

git_list_file=$(mktemp)
gofmt_stdout=$(mktemp)
gofmt_stderr=$(mktemp)
trap 'rm -f "$git_list_file" "$gofmt_stdout" "$gofmt_stderr"' EXIT

if ! git ls-files -z -- '*.go' >"$git_list_file"; then
  echo "check-gofmt: git ls-files failed" >&2
  exit 1
fi

mapfile -d '' -t files <"$git_list_file"
if ((${#files[@]} == 0)); then
  echo "check-gofmt: no tracked Go files" >&2
  exit 1
fi

status=0
"$GOFMT" -s -l -- "${files[@]}" >"$gofmt_stdout" 2>"$gofmt_stderr" || status=$?

if ((status != 0)); then
  cat "$gofmt_stderr" >&2
  echo "check-gofmt: gofmt failed (exit $status)" >&2
  exit 1
fi

if [[ -s "$gofmt_stdout" ]]; then
  echo "check-gofmt: these files need gofmt -s:" >&2
  cat "$gofmt_stdout" >&2
  exit 1
fi

echo "check-gofmt: ${#files[@]} files formatted"
