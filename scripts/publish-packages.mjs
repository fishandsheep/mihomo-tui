import { execFileSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";

import { normalizeVersion, platforms, repoRoot } from "./lib/platforms.mjs";

function parseArgs(argv) {
  const options = {
    dryRun: false,
    tolerateRepublish: false,
    tag: undefined,
    version: undefined
  };

  for (let i = 0; i < argv.length; i += 1) {
    const arg = argv[i];
    if (arg === "--dry-run") {
      options.dryRun = true;
      continue;
    }
    if (arg === "--tolerate-republish") {
      options.tolerateRepublish = true;
      continue;
    }
    if (arg === "--tag" && argv[i + 1]) {
      options.tag = argv[i + 1];
      i += 1;
      continue;
    }
    if (arg.startsWith("--tag=")) {
      options.tag = arg.slice("--tag=".length);
      continue;
    }
    if (arg === "--version" && argv[i + 1]) {
      options.version = normalizeVersion(argv[i + 1]);
      i += 1;
      continue;
    }
    if (arg.startsWith("--version=")) {
      options.version = normalizeVersion(arg.slice("--version=".length));
    }
  }

  return options;
}

function deriveDistTag(version) {
  const normalized = normalizeVersion(version);
  const prerelease = normalized.split("-", 2)[1];
  if (!prerelease) {
    return "latest";
  }
  return prerelease.split(".", 1)[0];
}

function readVersion() {
  const rootPackage = JSON.parse(
    fs.readFileSync(path.join(repoRoot, "package.json"), "utf8")
  );
  return normalizeVersion(rootPackage.version);
}

const options = parseArgs(process.argv.slice(2));
const version = options.version || process.env.MIHOMO_TUI_VERSION || readVersion();
const tag = options.tag || deriveDistTag(version);

const packageDirs = [
  ...platforms.map((platform) => platform.packageDir),
  "packages/cli",
  "packages/mihomo-tui"
];

for (const packageDir of packageDirs) {
  const packagePath = path.join(repoRoot, packageDir);
  const command = ["publish", "--tag", tag];

  if (options.dryRun) {
    command.push("--dry-run");
  }
  if (options.tolerateRepublish) {
    command.push("--tolerate-republish");
  }

  execFileSync("bun", command, {
    cwd: packagePath,
    stdio: "inherit",
    env: process.env
  });
}
