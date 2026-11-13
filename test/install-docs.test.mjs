// Guard (PR #67 cross-review): `install SRC DIR/` does not create DIR, so an
// install command in the README or on a site page that copies into a
// directory must create that directory first, in the same code block, with
// `mkdir -p DIR` or `install -d DIR`. Every line whose command is `install`
// counts and is checked; a line it cannot parse fails. The scan reports how
// many install commands it checked, and it fails when it finds none, so a
// change of the code-block syntax cannot make it pass on an empty list.
import assert from "node:assert/strict";
import { test } from "node:test";
import { readFileSync, readdirSync } from "node:fs";
import { dirname, join, relative, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const repo = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const fence = /^\s*(```|~~~)/;
// Any line whose command word is `install` (after an optional `sudo`) is an
// install command; its arguments are parsed separately, so a comment, quotes
// or `$HOME` cannot make the scan skip the line.
const installLine = /^\s*(?:sudo\s+)?install(?:\s|$)/;
const mkdirLine = /^\s*(?:sudo\s+)?mkdir(?:\s|$)/;

// words splits a shell line into words: it drops a trailing `# comment`, and
// removes single and double quotes. It returns null for an unbalanced quote.
function words(line) {
  const out = [];
  let cur = "";
  let quote = "";
  let inWord = false;
  for (const ch of line) {
    if (quote) {
      if (ch === quote) quote = "";
      else cur += ch;
    } else if (ch === "'" || ch === '"') {
      quote = ch;
      inWord = true;
    } else if (/\s/.test(ch)) {
      if (inWord) out.push(cur);
      cur = "";
      inWord = false;
    } else if (ch === "#" && !inWord) {
      break;
    } else {
      cur += ch;
      inWord = true;
    }
  }
  if (quote) return null;
  if (inWord) out.push(cur);
  return out;
}

// dirKey normalizes a directory: `$HOME` and `${HOME}` become `~`, and
// trailing slashes go.
function dirKey(dir) {
  return dir.replace(/^\$\{?HOME\}?(?=\/|$)/, "~").replace(/\/+$/, "") || "/";
}

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
// no earlier `mkdir -p` or `install -d` in the same code block creates, and
// the count of the install commands it checked. An install line that it
// cannot parse is a problem too, never a skip.
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
    const where = `line ${i + 1}: ${line.trim()}`;
    if (mkdirLine.test(line)) {
      const w = words(line);
      if (w && w.includes("-p")) {
        for (const dir of w.filter((x) => !x.startsWith("-") && x !== "sudo" && x !== "mkdir")) made.add(dirKey(dir));
      }
      continue;
    }
    if (!installLine.test(line)) continue;
    checked++;
    const w = words(line);
    if (!w) {
      problems.push(`${where} (cannot parse)`);
      continue;
    }
    const args = w.slice(w.indexOf("install") + 1);
    if (args.includes("-d")) {
      for (const dir of args.filter((x) => !x.startsWith("-") && !/^[0-7]{3,4}$/.test(x))) made.add(dirKey(dir));
      continue;
    }
    const dest = args[args.length - 1];
    if (!dest || dest.startsWith("-")) {
      problems.push(`${where} (no destination)`);
      continue;
    }
    const dir = dest.endsWith("/") ? dirKey(dest) : dirKey(dest.slice(0, Math.max(dest.lastIndexOf("/"), 0)) || ".");
    if (dir !== "." && !made.has(dir)) problems.push(where);
  }
  return { problems, checked };
}

test("installProblems flags an install into a directory that the block does not create", () => {
  const bad = "```sh\ninstall -m 0755 truenas-mcp ~/.local/bin/\n```\n";
  assert.deepEqual(installProblems(bad).checked, 1);
  assert.equal(installProblems(bad).problems.length, 1);
  const good = "```sh\nmkdir -p ~/.local/bin\ninstall -m 0755 truenas-mcp ~/.local/bin/\n```\n";
  assert.deepEqual(installProblems(good), { problems: [], checked: 1 });
  const otherBlock = "```sh\nmkdir -p ~/.local/bin\n```\n\n```sh\ninstall -m 0755 x ~/.local/bin/\n```\n";
  assert.equal(installProblems(otherBlock).problems.length, 1);
});

test("installProblems counts and checks an install line with a comment, quotes, $HOME or a file destination", () => {
  // Each bad case sits beside a good, counted command, so a skipped line
  // shows as a missing problem, not only as a lower count.
  const good = "mkdir -p /opt/a\ninstall -m 0755 x /opt/a/\n";
  const cases = [
    "install -m 0755 truenas-mcp ~/.local/bin/ # copy the binary",
    'install -m 0755 truenas-mcp "$HOME/.local/bin/"',
    "install -m 0755 truenas-mcp '${HOME}/.local/bin/'",
    "install -m 0755 truenas-mcp ~/.local/bin/truenas-mcp",
    "sudo install -m 0755 truenas-mcp /usr/local/libexec/",
  ];
  for (const bad of cases) {
    const r = installProblems("```sh\n" + good + bad + "\n```\n");
    assert.equal(r.checked, 2, bad);
    assert.equal(r.problems.length, 1, bad);
  }
  const unparsable = installProblems('```sh\ninstall -m 0755 x "~/.local/bin/\n```\n');
  assert.deepEqual(unparsable.problems.length, 1);
  // [block, install commands in it]: `install -d` counts as an install command.
  const fixed = [
    ['mkdir -p "$HOME/.local/bin"\ninstall -m 0755 truenas-mcp ~/.local/bin/ # copy', 1],
    ["install -d ~/.local/bin\ninstall -m 0755 truenas-mcp ~/.local/bin/truenas-mcp", 2],
  ];
  for (const [ok, checked] of fixed) {
    assert.deepEqual(installProblems("```sh\n" + ok + "\n```\n"), { problems: [], checked }, ok);
  }
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
