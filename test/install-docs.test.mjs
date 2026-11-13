// Guard (PR #67 cross-review): `install SRC DIR/` does not create DIR, so an
// install command in the README or on a site page that copies into a
// directory must create that directory first, in the same code block, with
// `mkdir -p DIR` or `install -d DIR`. Every line that holds the word
// `install` or `mkdir` (outside a comment, and not a package manager's
// `TOOL install`) counts and must be 1 simple command in a small grammar
// (unquoted words, no separators, `sudo` the only prefix); any other line
// fails. The scan reports how
// many install commands it checked, and it fails when it finds none, so a
// change of the code-block syntax cannot make it pass on an empty list.
import assert from "node:assert/strict";
import { test } from "node:test";
import { readFileSync, readdirSync } from "node:fs";
import { dirname, join, relative, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const repo = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const fence = /^\s*(```|~~~)/;
// A candidate is any line that holds the word `install` or `mkdir` outside a
// trailing comment, wherever it stands, so no prefix (`sudo -n`, `command`,
// `FOO=bar`) can hide one. A candidate must be 1 simple command in the
// grammar below, or the scan reports it.
const candidate = /(?:^|[^A-Za-z0-9_-])(?:install|mkdir)(?:[^A-Za-z0-9_-]|$)/;
// Package managers whose `install` subcommand is not install(1): a line
// `TOOL install ...` (or `sudo TOOL install ...`) is not an install command.
const packageTools = new Set(["make", "apt", "apt-get", "dnf", "yum", "zypper", "apk", "brew", "go", "pip", "pipx", "npm", "mise", "flatpak", "rpm-ostree"]);
// The supported grammar: unquoted words, no separators, pipes, quotes,
// substitutions or redirections, and an optional trailing `# comment`.
const plainWord = /^[A-Za-z0-9_.\/~$+=,:@%{}*-]+$/;

// simpleCommand returns the words of 1 simple command, without a trailing
// comment, or null when the line is outside the supported grammar.
function simpleCommand(line) {
  const body = line.replace(/\s+#.*$/, "").trim();
  if (body === "") return null;
  const out = body.split(/\s+/);
  return out.every((w) => plainWord.test(w)) ? out : null;
}

// dirKey normalizes an unquoted directory: `$HOME` and `${HOME}` expand to
// `~`, and trailing slashes go.
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
// no earlier `mkdir -p DIR` or `install -d DIR` in the same code block
// creates, and the count of the install and mkdir commands it checked. A line
// outside the supported grammar is a problem, never a skip:
//   mkdir -p DIR...
//   install [-m MODE] SRC... DIR/   or   install [-m MODE] SRC DIR/FILE
//   install -d [-m MODE] DIR...
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
    const code = line.replace(/(?:^|\s)#.*$/, "");
    if (!candidate.test(code)) continue;
    const where = `line ${i + 1}: ${line.trim()}`;
    let w = simpleCommand(line);
    // `sudo` with no option of its own is the only allowed prefix.
    if (w && w[0] === "sudo" && w.length > 1 && !w[1].startsWith("-")) w = w.slice(1);
    if (w && w[1] === "install" && packageTools.has(w[0])) continue;
    checked++;
    if (!w || (w[0] !== "install" && w[0] !== "mkdir")) {
      problems.push(`${where} (unsupported syntax or prefix)`);
      continue;
    }
    let args = w.slice(1);
    if (w[0] === "mkdir") {
      if (args[0] !== "-p" || args.length < 2 || args.slice(1).some((a) => a.startsWith("-"))) {
        problems.push(`${where} (unsupported mkdir form)`);
        continue;
      }
      for (const dir of args.slice(1)) made.add(dirKey(dir));
      continue;
    }
    let createDirs = false;
    const operands = [];
    for (let k = 0; k < args.length; k++) {
      if (args[k] === "-m" && k + 1 < args.length) k++;
      else if (args[k] === "-d") createDirs = true;
      else if (args[k].startsWith("-")) operands.push(null);
      else operands.push(args[k]);
    }
    if (operands.includes(null) || operands.length === 0 || (!createDirs && operands.length < 2)) {
      problems.push(`${where} (unsupported install form)`);
      continue;
    }
    if (createDirs) {
      for (const dir of operands) made.add(dirKey(dir));
      continue;
    }
    const dest = operands[operands.length - 1];
    const slash = dest.lastIndexOf("/");
    const dir = dest.endsWith("/") ? dirKey(dest) : slash > 0 ? dirKey(dest.slice(0, slash)) : slash === 0 ? "/" : ".";
    if (dir !== "." && !made.has(dir)) problems.push(where);
  }
  return { problems, checked };
}

test("installProblems flags an install into a directory that the block does not create", () => {
  const bad = "```sh\ninstall -m 0755 truenas-mcp ~/.local/bin/\n```\n";
  assert.deepEqual(installProblems(bad).checked, 1);
  assert.equal(installProblems(bad).problems.length, 1);
  const good = "```sh\nmkdir -p ~/.local/bin\ninstall -m 0755 truenas-mcp ~/.local/bin/\n```\n";
  assert.deepEqual(installProblems(good), { problems: [], checked: 2 });
  const otherBlock = "```sh\nmkdir -p ~/.local/bin\n```\n\n```sh\ninstall -m 0755 x ~/.local/bin/\n```\n";
  assert.equal(installProblems(otherBlock).problems.length, 1);
});

test("installProblems checks every install line and refuses what it cannot read", () => {
  // Each case sits beside a good block that counts 2 commands, so a skipped
  // line shows as a missing problem, not only as a lower count.
  const good = "mkdir -p /opt/a\ninstall -m 0755 x /opt/a/\n";
  const bad = [
    "install -m 0755 truenas-mcp ~/.local/bin/ # copy the binary",
    "install -m 0755 truenas-mcp ~/.local/bin/truenas-mcp",
    "sudo install -m 0755 truenas-mcp /usr/local/libexec/",
    'install -m 0755 truenas-mcp "$HOME/.local/bin/"',
    'mkdir -p "~/.local/bin"',
    "mkdir -p '${HOME}/.local/bin'",
    "mkdir -p /tmp/work && echo ~/.local/bin",
    "mkdir -p -m 0755 /tmp/work",
    "echo ok; install -m 0755 x ~/.local/bin/",
    "install -m 0755 x $(dirname ~/.local/bin/x)/",
    "install --mode=0755 x ~/.local/bin/",
    "sudo -n install -m 0755 truenas-mcp ~/.local/bin/",
    "command install -m 0755 truenas-mcp ~/.local/bin/",
    "FOO=bar install -m 0755 truenas-mcp ~/.local/bin/",
    "env mkdir -p ~/.local/bin",
    "xargs install -m 0755 -t ~/.local/bin/",
  ];
  for (const line of bad) {
    const r = installProblems("```sh\n" + good + line + "\n```\n");
    assert.equal(r.checked, 3, line);
    assert.equal(r.problems.length, 1, line);
  }
  // A quoted or compound mkdir does not count as made: the install after it
  // fails too.
  for (const mk of ['mkdir -p "~/.local/bin"', "mkdir -p /tmp/work && echo ~/.local/bin"]) {
    const r = installProblems("```sh\n" + mk + "\ninstall -m 0755 x ~/.local/bin/\n```\n");
    assert.equal(r.problems.length, 2, mk);
  }
  // [block, commands in it]: these pass.
  const fixed = [
    ["mkdir -p $HOME/.local/bin\ninstall -m 0755 truenas-mcp ~/.local/bin/ # copy", 2],
    ["install -d -m 0755 ~/.local/bin\ninstall -m 0755 truenas-mcp ~/.local/bin/truenas-mcp", 2],
    ["make install   # installs into $GOPATH/bin\nsudo apt install ./x.deb\n# install the binary", 0],
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
  t.diagnostic(`checked ${checked} install and mkdir commands in ${files.length} files`);
  assert.ok(checked > 0, "no install or mkdir command found: the scan is broken");
  assert.deepEqual(problems, []);
});
