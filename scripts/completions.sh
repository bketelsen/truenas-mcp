#!/bin/sh
# Generate shell completion scripts for packaging. GoReleaser runs this as a
# before hook; the nfpm packages install the results system-wide.
set -e
rm -rf completions
mkdir completions
go build -o build/truenas-mcp .
for sh in bash zsh fish; do
  ./build/truenas-mcp completion "$sh" >"completions/truenas-mcp.$sh"
done
