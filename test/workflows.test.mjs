// Guard (issue #64): frostyard/core ADR-0021 asks for least-privilege
// workflows. Every workflow in .github/workflows/ sets `permissions: {}` at
// the top and grants permissions per job, and every external `uses:` is
// pinned to a full 40-character commit SHA with a trailing comment: a version
// line (`# vX.Y.Z`) for an action, any comment for a reusable workflow.
import assert from "node:assert/strict";
import { test } from "node:test";
import { readFileSync, readdirSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const repo = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const workflowDir = join(repo, ".github", "workflows");
const jobKey = /^ {2}([A-Za-z0-9_.-]+):\s*(?:#.*)?$/;
const jobPermissions = /^ {4}permissions:/;
const usesLine = /^\s*(?:-\s+)?uses:\s*(\S+)\s*(?:#\s*(.*))?$/;
const sha40 = /^[0-9a-f]{40}$/;
const versionComment = /^v\d+(?:\.\d+)*(?:\s|$)/;

// topLevelPermissions returns the value of the `permissions:` key at column 0:
// its inline value as a string, its indented child lines as an array, or null
// when the workflow has no top-level `permissions:` key.
function topLevelPermissions(text) {
  const lines = text.split("\n");
  for (let i = 0; i < lines.length; i++) {
    if (!lines[i].startsWith("permissions:")) continue;
    const inline = lines[i].slice("permissions:".length).trim();
    if (inline !== "") return inline;
    const block = [];
    for (let j = i + 1; j < lines.length; j++) {
      const line = lines[j];
      if (line.trim() === "") continue;
      if (!line.startsWith(" ")) break;
      block.push(line);
    }
    return block;
  }
  return null;
}

// parseJobs returns the jobs of the top-level `jobs:` block as { id,
// hasPermissions } entries, in file order.
function parseJobs(text) {
  const jobs = [];
  let inJobs = false;
  let current = null;
  for (const line of text.split("\n")) {
    if (!inJobs) {
      if (line.startsWith("jobs:")) inJobs = true;
      continue;
    }
    if (line.trim() !== "" && !line.startsWith(" ")) break;
    const job = jobKey.exec(line);
    if (job) {
      current = { id: job[1], hasPermissions: false };
      jobs.push(current);
      continue;
    }
    if (current !== null && jobPermissions.test(line)) current.hasPermissions = true;
  }
  return jobs;
}

// jobsWithoutPermissions returns the ids of the jobs that have no
// `permissions:` key at 4 spaces.
function jobsWithoutPermissions(text) {
  return parseJobs(text)
    .filter((job) => !job.hasPermissions)
    .map((job) => job.id);
}

// unpinnedUses returns each `uses:` line that is not a local `./` or a
// `docker://` ref pinned to a 40-character commit SHA with a valid trailing
// comment, as { line, ref, reason }.
function unpinnedUses(text) {
  const offending = [];
  text.split("\n").forEach((line, index) => {
    const match = usesLine.exec(line);
    if (match === null) return;
    const ref = match[1];
    const comment = (match[2] ?? "").trim();
    const reason = (why) => offending.push({ line: index + 1, ref, reason: why });
    if (ref.startsWith("./") || ref.startsWith("docker://")) return;
    const at = ref.lastIndexOf("@");
    if (at < 0) return reason("no commit SHA");
    if (!sha40.test(ref.slice(at + 1))) return reason("not a 40-character commit SHA");
    if (comment === "") return reason("no trailing comment");
    if (!ref.includes("/.github/workflows/") && !versionComment.test(comment)) {
      return reason("the comment is not a version");
    }
  });
  return offending;
}

// workflowFiles returns the names of the YAML files in .github/workflows/.
function workflowFiles() {
  return readdirSync(workflowDir)
    .filter((name) => name.endsWith(".yml") || name.endsWith(".yaml"))
    .sort();
}

// read returns the text of 1 workflow file.
function read(name) {
  return readFileSync(join(workflowDir, name), "utf8");
}

// describePermissions makes a topLevelPermissions value readable in a
// failure message.
function describePermissions(value) {
  if (value === null) return "missing";
  if (Array.isArray(value)) return value.map((line) => line.trim()).join(", ");
  return value;
}

test("every workflow sets permissions: {} at the top", (t) => {
  const files = workflowFiles();
  t.diagnostic(`checked ${files.length} workflow files`);
  assert.ok(files.length > 0, "found no workflow file: the scan is broken");
  const offending = files
    .map((name) => ({ file: name, permissions: describePermissions(topLevelPermissions(read(name))) }))
    .filter((entry) => entry.permissions !== "{}");
  assert.deepEqual(offending, [], "a workflow does not set permissions: {} at the top");
});

test("every job grants its own permissions", (t) => {
  const files = workflowFiles();
  t.diagnostic(`checked ${files.length} workflow files`);
  assert.ok(files.length > 0, "found no workflow file: the scan is broken");
  const offending = [];
  for (const name of files) {
    const text = read(name);
    assert.ok(parseJobs(text).length > 0, `${name}: found no job: the scan is broken`);
    for (const id of jobsWithoutPermissions(text)) offending.push({ file: name, job: id });
  }
  assert.deepEqual(offending, [], "a job has no permissions: key");
});

test("every external uses: is pinned to a SHA with a comment", (t) => {
  const files = workflowFiles();
  t.diagnostic(`checked ${files.length} workflow files`);
  assert.ok(files.length > 0, "found no workflow file: the scan is broken");
  const offending = [];
  for (const name of files) {
    for (const entry of unpinnedUses(read(name))) offending.push({ file: name, ...entry });
  }
  assert.deepEqual(offending, [], "a uses: line is not pinned to a SHA with a comment");
});

test("the top-level check accepts permissions: {}", () => {
  assert.equal(topLevelPermissions("on: push\npermissions: {}\njobs:\n  a:\n    permissions:\n      contents: read\n"), "{}");
});

test("the top-level check rejects permissions: write-all", () => {
  assert.notEqual(topLevelPermissions("permissions: write-all\n"), "{}");
});

test("the top-level check rejects read-all", () => {
  assert.notEqual(topLevelPermissions("permissions: read-all\n"), "{}");
});

test("the top-level check rejects a block with contents: write", () => {
  assert.notEqual(topLevelPermissions("permissions:\n  contents: write\n"), "{}");
});

test("the top-level check rejects a workflow with no top-level permissions", () => {
  assert.equal(topLevelPermissions("jobs:\n  a:\n    permissions:\n      contents: read\n"), null);
});

test("a job-level permissions block is not the top-level one", () => {
  const text = "permissions: {}\njobs:\n  release:\n    permissions:\n      contents: write\n";
  assert.equal(topLevelPermissions(text), "{}");
});

test("a job with no permissions is reported", () => {
  const text = "permissions: {}\njobs:\n  build:\n    permissions:\n      contents: read\n  deploy:\n    runs-on: ubuntu-latest\n";
  assert.deepEqual(jobsWithoutPermissions(text), ["deploy"]);
});

test("an action pinned to a tag is reported", () => {
  assert.equal(unpinnedUses("      - uses: actions/checkout@v4\n").length, 1);
});

test("an action pinned to a short SHA is reported", () => {
  assert.equal(unpinnedUses("      - uses: actions/checkout@3d3c42e # v7.0.1\n").length, 1);
});

test("a SHA with no comment is reported", () => {
  assert.equal(unpinnedUses("      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1\n").length, 1);
});

test("a SHA with a branch comment is reported", () => {
  const line = "      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # main\n";
  assert.equal(unpinnedUses(line).length, 1);
});

test("a SHA with a version comment is accepted", () => {
  const line = "      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1\n";
  assert.deepEqual(unpinnedUses(line), []);
});

test("a reusable workflow SHA with any comment is accepted", () => {
  const sha = "c996ea3fb54bcf1a3c621113522213702a7673e2";
  const line = `    uses: frostyard/core/.github/workflows/pr-triage.yml@${sha} # core#225 (frostyard/core main, 2026-11-09)\n`;
  assert.deepEqual(unpinnedUses(line), []);
});

test("a local and a docker ref are not checked", () => {
  assert.deepEqual(unpinnedUses("      - uses: ./.github/actions/build\n      - uses: docker://alpine:3\n"), []);
});
