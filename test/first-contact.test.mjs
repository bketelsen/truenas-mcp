// The guided configuration must launch the binary produced by `make build`.
// Run only the planning path, with a stubbed make and a dummy API key.
import assert from "node:assert/strict";
import { test } from "node:test";
import { copyFileSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";

const repo = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const makefile = readFileSync(join(repo, "Makefile"), "utf8");
const binary = makefile.match(/^BINARY\s*:?=\s*(\S+)\s*$/m)?.[1];
const output = makefile.match(/^build:\n\t\$\(GO\) build [^\n]* -o (\S+) \.\s*$/m)?.[1];
assert.ok(binary && output, "cannot read the Makefile build output; update the test for the new recipe");
const buildOutput = output.replaceAll("$(BINARY)", binary);
assert.ok(!buildOutput.includes("$"), "unresolved build output variable");

for (const tls of ["", "true"]) {
  test(`first contact configuration uses the build output without leaking the key (TLS insecure=${tls || "unset"})`, (t) => {
    // A space in the clone path also checks the script's path quoting.
    const root = mkdtempSync(join(tmpdir(), "first-contact clone-"));
    t.after(() => rmSync(root, { recursive: true, force: true }));
    mkdirSync(join(root, "scripts"));
    mkdirSync(join(root, "stub-bin"));
    copyFileSync(join(repo, "scripts", "first-contact.sh"), join(root, "scripts", "first-contact.sh"));
    const log = join(root, "make.log");
    writeFileSync(join(root, "stub-bin", "make"), '#!/bin/sh\nprintf \'%s\\n\' "$*" >> "$FIRST_CONTACT_MAKE_LOG"\n', { mode: 0o755 });
    const key = "dummy-first-contact-key-not-for-output";
    const env = {
      ...process.env,
      PATH: `${join(root, "stub-bin")}:${process.env.PATH}`,
      FIRST_CONTACT_MAKE_LOG: log,
      TRUENAS_HOST: "nas.example",
      TRUENAS_API_KEY: key,
      TRUENAS_TLS_INSECURE: tls,
    };
    delete env.TRUENAS_ENABLE_WRITES;
    const result = spawnSync("bash", [join(root, "scripts", "first-contact.sh")], {
      cwd: tmpdir(), env, encoding: "utf8", timeout: 10000,
    });
    const allOutput = `${result.stdout || ""}\n${result.stderr || ""}`;
    assert.ok(!allOutput.includes(key), "the script printed the dummy API key");
    assert.equal(result.status, 0, allOutput);
    assert.equal(readFileSync(log, "utf8"), "build\n", "only the stubbed make build must run");
    const marker = "==> Claude MCP config template\n";
    const start = result.stdout.indexOf(marker);
    assert.notEqual(start, -1, "missing configuration template");
    const json = result.stdout.slice(start + marker.length).split("\nRun with --run-server")[0].trim();
    const config = JSON.parse(json);
    assert.equal(config.mcpServers.truenas.command, join(root, buildOutput));
    assert.deepEqual(config.mcpServers.truenas.args, [
      "serve", "--host", "nas.example", "--api-key", "YOUR_API_KEY",
      ...(tls ? ["--tls-insecure"] : []),
    ]);
    assert.ok(allOutput.includes("writes: disabled"));
    assert.ok(allOutput.includes("api key: present, not printed"));
  });
}
