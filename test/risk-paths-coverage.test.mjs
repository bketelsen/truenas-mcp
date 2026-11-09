// Guard (PR #52 cross-review, finding 1): every server file that registers a
// tool with DestructiveHint true must match a glob in .github/risk-paths.json
// at level L1, so that a change to a destructive tool gets level L1 and
// risk:high (ADR-0068, ADR-0078).
import assert from "node:assert/strict";
import { test } from "node:test";
import { readFileSync, readdirSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { levelOfFile, matchGlob, parseStrictJson } from "../scripts/lib/risk-paths.mjs";

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

// looserThanL1 returns the paths whose level under the list is not L1.
function looserThanL1(list, paths) {
  return paths.filter((path) => levelOfFile(list, path).level !== "L1");
}

test("every destructive tool file is a risk path", (t) => {
  const list = parseStrictJson(readFileSync(join(repo, ".github", "risk-paths.json"), "utf8"));
  const files = destructiveFiles(repo);
  t.diagnostic(`checked ${files.length} destructive tool files`);
  assert.ok(files.length > 0, "found no destructive tool file: the scan is broken");
  assert.deepEqual(uncovered(list, files), []);
});

test("every destructive tool file is at level L1", (t) => {
  const list = parseStrictJson(readFileSync(join(repo, ".github", "risk-paths.json"), "utf8"));
  const files = destructiveFiles(repo);
  t.diagnostic(`checked ${files.length} destructive tool files`);
  assert.ok(files.length > 0, "found no destructive tool file: the scan is broken");
  assert.deepEqual(looserThanL1(list, files), []);
});

test("the coverage check fails for a destructive file with no entry", () => {
  const list = { schema_version: 1, paths: [{ glob: "server/tools_dataset.go", why: "destructive operations: x" }] };
  assert.deepEqual(uncovered(list, ["server/tools_dataset.go", "server/tools_snapshot.go"]), ["server/tools_snapshot.go"]);
});

test("the level check fails for a destructive file at L2", () => {
  const list = {
    schema_version: 2,
    default_level: "L3",
    paths: [
      { glob: "server/tools_dataset.go", level: "L2", why: "destructive operations: x" },
      { glob: "server/tools_snapshot.go", level: "L1", why: "destructive operations: x" },
    ],
  };
  assert.deepEqual(looserThanL1(list, ["server/tools_dataset.go", "server/tools_snapshot.go"]), ["server/tools_dataset.go"]);
});
