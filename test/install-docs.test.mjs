// Guard (PR #67 cross-review): `install SRC DIR/` does not create DIR, so an
// install command in the README or on a site page that copies into a
// directory must create that directory first, in the same code block, with
// `mkdir -p DIR`. The scan reports how many install commands it checked, and
// it fails when it finds none, so a change of the code-block syntax cannot
// make it pass on an empty list.
import assert from "node:assert/strict";
import { test } from "node:test";
import { readFileSync, readdirSync } from "node:fs";
import { dirname, join, relative, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const repo = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const fence = /^\s*(```|~~~)/;
// `install [options] SRC... DIR/`: the last argument ends with a slash.
const installToDir = /^\s*(?:sudo\s+)?install\s+(?:\S+\s+)*(\S+\/)\s*$/;

function markdownFiles(dir) {
  const out = [];
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const path = join(dir, entry.name);
    if (entry.isDirectory()) out.push(...markdownFiles(path));
    else if (entry.name.endsWith(".md")) out.push(path);
  }
  return out;
}

// installProblems returns the install commands whose destination directory
// no earlier `mkdir -p` in the same code block creates, and the count of the
// install commands it checked.
export function installProblems(text) {
  const problems = [];
  let checked = 0;
  let inBlock = false;
  let made = new Set();
  for (const [i, line] of text.split("\n").entries()) {
    if (fence.test(line)) {
      inBlock = !inBlock;
      made = new Set();
      continue;
    }
    if (!inBlock) continue;
    const mkdir = line.match(/^\s*(?:sudo\s+)?mkdir\s+-p\s+(.+?)\s*$/);
    if (mkdir) {
      for (const dir of mkdir[1].split(/\s+/)) made.add(dir.replace(/\/+$/, ""));
      continue;
    }
    const install = line.match(installToDir);
    if (install && !/\s-d\s/.test(line)) {
      checked++;
      const dir = install[1].replace(/\/+$/, "");
      if (!made.has(dir)) problems.push(`line ${i + 1}: ${line.trim()}`);
    }
  }
  return { problems, checked };
}

test("installProblems flags an install into a directory that the block does not create", () => {
  const bad = "```sh\ninstall -m 0755 truenas-mcp ~/.local/bin/\n```\n";
  assert.equal(installProblems(bad).problems.length, 1);
  const good = "```sh\nmkdir -p ~/.local/bin\ninstall -m 0755 truenas-mcp ~/.local/bin/\n```\n";
  assert.deepEqual(installProblems(good), { problems: [], checked: 1 });
  const otherBlock = "```sh\nmkdir -p ~/.local/bin\n```\n\n```sh\ninstall -m 0755 x ~/.local/bin/\n```\n";
  assert.equal(installProblems(otherBlock).problems.length, 1);
});

test("README and site pages create the directory before an install into it", (t) => {
  const files = [join(repo, "README.md"), ...markdownFiles(join(repo, "site", "content"))];
  let checked = 0;
  const problems = [];
  for (const file of files) {
    const result = installProblems(readFileSync(file, "utf8"));
    checked += result.checked;
    for (const p of result.problems) problems.push(`${relative(repo, file)} ${p}`);
  }
  t.diagnostic(`checked ${checked} install commands in ${files.length} files`);
  assert.ok(checked > 0, "no install command found: the scan is broken");
  assert.deepEqual(problems, []);
});
