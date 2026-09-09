// Package version holds build metadata for truenas-mcp.
//
// The values are overwritten at link time with -X flags. The Makefile sets them
// from git describe for local builds and GoReleaser sets them for releases:
//
//	-X truenas-mcp/version.Version=v0.2.0 -X truenas-mcp/version.Commit=abc1234 ...
//
// A plain `go build` leaves the defaults in place, which is how a development
// binary identifies itself.
package version

var (
	// Version is the semantic version of the build (a git tag for releases).
	Version = "dev"
	// Commit is the short git commit hash the binary was built from.
	Commit = "none"
	// Date is the UTC build (or commit) timestamp in RFC 3339 format.
	Date = "unknown"
	// BuiltBy names the tool that produced the binary: make, goreleaser, or local.
	BuiltBy = "local"
)
