// Guard (PR #52 cross-review, finding 1): every server file that registers a
// tool with DestructiveHint true must match a glob in .github/risk-paths.json,
// so that a change to a destructive tool gets the risk:high label.
import assert from "node:assert/strict";
import { test } from "node:test";
import { readFileSync, readdirSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { matchGlob, parseStrictJson } from "../scripts/lib/risk-paths.mjs";

const repo = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const destructiveHint = /DestructiveHint:\s*new\(true\)/;

// destructiveFiles returns the repository paths of the non-test Go files in
// server/ that declare at least 1 tool with DestructiveHint true.
function destructiveFiles(root) {
  return readdirSync(join(root, "server"))
    .filter((name) => name.endsWith(".go") && !name.endsWith("_test.go"))
    .filter((name) => destructiveHint.test(readFileSync(join(root, "server", name), "utf8")))
    .map((name) => `server/${name}`)
    .sort();
}

// uncovered returns the paths that no glob of the risk-path list matches.
function uncovered(list, paths) {
  return paths.filter((path) => !list.paths.some((entry) => matchGlob(entry.glob, path)));
}

test("every destructive tool file is a risk path", (t) => {
  const list = parseStrictJson(readFileSync(join(repo, ".github", "risk-paths.json"), "utf8"));
  const files = destructiveFiles(repo);
  t.diagnostic(`checked ${files.length} destructive tool files`);
  assert.ok(files.length > 0, "found no destructive tool file: the scan is broken");
  assert.deepEqual(uncovered(list, files), []);
});

test("the coverage check fails for a destructive file with no entry", () => {
  const list = { schema_version: 1, paths: [{ glob: "server/tools_dataset.go", why: "destructive operations: x" }] };
  assert.deepEqual(uncovered(list, ["server/tools_dataset.go", "server/tools_snapshot.go"]), ["server/tools_snapshot.go"]);
});
