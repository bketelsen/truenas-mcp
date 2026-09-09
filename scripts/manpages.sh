#!/bin/sh
# Generate the gzipped man page for packaging. GoReleaser runs this as a
# before hook; the nfpm packages install the result under man1.
set -e
rm -rf manpages
mkdir manpages
go run . man | gzip -c -9 >manpages/truenas-mcp.1.gz
