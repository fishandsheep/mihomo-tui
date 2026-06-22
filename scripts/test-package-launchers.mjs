import { execFileSync } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";

import { platforms, repoRoot } from "./lib/platforms.mjs";

function detectHostPlatform() {
  if (process.platform === "darwin" && process.arch === "arm64") {
    return "darwin-arm64";
  }
  if (process.platform === "darwin" && process.arch === "x64") {
    return "darwin-x64";
  }
  if (process.platform === "linux" && process.arch === "arm64") {
    return "linux-arm64-gnu";
  }
  if (process.platform === "linux" && process.arch === "x64") {
    return "linux-x64-gnu";
  }
  if (process.platform === "win32" && process.arch === "arm64") {
    return "win32-arm64-msvc";
  }
  if (process.platform === "win32" && process.arch === "x64") {
    return "win32-x64-msvc";
  }
  return null;
}

const hostPlatformId = detectHostPlatform();
if (!hostPlatformId) {
  console.log(`skip launcher smoke tests on unsupported host ${process.platform}/${process.arch}`);
  process.exit(0);
}

const hostPlatform = platforms.find((platform) => platform.id === hostPlatformId);
if (!hostPlatform) {
  throw new Error(`missing host platform config for ${hostPlatformId}`);
}

const hostBinaryPath = path.join(repoRoot, hostPlatform.packageDir, "bin", hostPlatform.binaryName);
if (!fs.existsSync(hostBinaryPath)) {
  throw new Error(`host binary missing: ${hostBinaryPath}. Run bun run build:binaries && bun run build:packages first.`);
}

const tempRoot = fs.mkdtempSync(path.join(os.tmpdir(), "mihomo-tui-launcher-"));
const stageRoot = path.join(tempRoot, "stage");
const installRoot = path.join(tempRoot, "install");
const stalePathDir = path.join(tempRoot, "stale-path");

fs.mkdirSync(stageRoot, { recursive: true });
fs.mkdirSync(installRoot, { recursive: true });
fs.mkdirSync(stalePathDir, { recursive: true });

const stagedPlatformDir = path.join(stageRoot, hostPlatform.id);
const stagedCliDir = path.join(stageRoot, "cli");
const stagedAliasDir = path.join(stageRoot, "mihomo-tui");

fs.cpSync(path.join(repoRoot, hostPlatform.packageDir), stagedPlatformDir, { recursive: true });
fs.cpSync(path.join(repoRoot, "packages", "cli"), stagedCliDir, { recursive: true });
fs.cpSync(path.join(repoRoot, "packages", "mihomo-tui"), stagedAliasDir, { recursive: true });
fs.rmSync(path.join(stagedPlatformDir, "node_modules"), { recursive: true, force: true });
fs.rmSync(path.join(stagedCliDir, "node_modules"), { recursive: true, force: true });
fs.rmSync(path.join(stagedAliasDir, "node_modules"), { recursive: true, force: true });

function writeJSON(filePath, value) {
  fs.writeFileSync(filePath, `${JSON.stringify(value, null, 2)}\n`);
}

const platformManifest = JSON.parse(fs.readFileSync(path.join(stagedPlatformDir, "package.json"), "utf8"));
const platformTarball = execFileSync("npm", ["pack"], {
  cwd: stagedPlatformDir,
  encoding: "utf8"
}).trim();

const cliManifestPath = path.join(stagedCliDir, "package.json");
const cliManifest = JSON.parse(fs.readFileSync(cliManifestPath, "utf8"));
cliManifest.optionalDependencies = {
  [platformManifest.name]: `file:${path.join(stagedPlatformDir, platformTarball)}`
};
writeJSON(cliManifestPath, cliManifest);
const cliTarball = execFileSync("npm", ["pack"], {
  cwd: stagedCliDir,
  encoding: "utf8"
}).trim();

const aliasManifestPath = path.join(stagedAliasDir, "package.json");
const aliasManifest = JSON.parse(fs.readFileSync(aliasManifestPath, "utf8"));
aliasManifest.dependencies = {
  "@qinshower/mihomo-tui": `file:${path.join(stagedCliDir, cliTarball)}`
};
writeJSON(aliasManifestPath, aliasManifest);
const aliasTarball = execFileSync("npm", ["pack"], {
  cwd: stagedAliasDir,
  encoding: "utf8"
}).trim();

execFileSync("npm", ["install", path.join(stagedAliasDir, aliasTarball)], {
  cwd: installRoot,
  stdio: "inherit"
});

const installedBin = path.join(installRoot, "node_modules", ".bin", process.platform === "win32" ? "mihomo-tui.cmd" : "mihomo-tui");
const launcherBin = path.join(installRoot, "node_modules", "@qinshower", "mihomo-tui", "bin.js");
const platformInstallDir = path.join(
  installRoot,
  "node_modules",
  "@qinshower",
  hostPlatform.packageName.split("/")[1]
);
if (!fs.existsSync(installedBin)) {
  throw new Error(`installed launcher missing: ${installedBin}`);
}

const versionOutput = execFileSync("node", [installedBin, "--version"], {
  cwd: installRoot,
  encoding: "utf8"
}).trim();

if (!versionOutput.startsWith("mihomo-tui ")) {
  throw new Error(`unexpected version output: ${versionOutput}`);
}

const bunOutput = execFileSync("bun", [installedBin, "--version"], {
  cwd: installRoot,
  encoding: "utf8"
}).trim();

if (bunOutput !== versionOutput) {
  throw new Error(`node/bun launcher output mismatch: ${versionOutput} vs ${bunOutput}`);
}

const staleName = process.platform === "win32" ? "mihomo-tui.cmd" : "mihomo-tui";
const stalePath = path.join(stalePathDir, staleName);
fs.writeFileSync(
  stalePath,
  process.platform === "win32"
    ? "@echo off\r\necho mihomo-tui 0.0.1-stale\r\n"
    : "#!/bin/sh\necho mihomo-tui 0.0.1-stale\n"
);
if (process.platform !== "win32") {
  fs.chmodSync(stalePath, 0o755);
}

fs.rmSync(path.join(platformInstallDir, "bin", hostPlatform.binaryName), { force: true });

let missingBinaryOutput = "";
try {
  execFileSync("node", [launcherBin, "--version"], {
    cwd: installRoot,
    env: {
      ...process.env,
      PATH: `${stalePathDir}${path.delimiter}${process.env.PATH || ""}`
    },
    encoding: "utf8",
    stdio: ["ignore", "pipe", "pipe"]
  });
  throw new Error("expected launcher to fail after removing platform package");
} catch (error) {
  missingBinaryOutput = `${error.stdout || ""}${error.stderr || ""}`;
}

if (missingBinaryOutput.includes("0.0.1-stale")) {
  throw new Error(`launcher fell back to stale PATH binary: ${missingBinaryOutput}`);
}
if (!missingBinaryOutput.includes("mihomo-tui binary not found")) {
  throw new Error(`unexpected missing-binary output: ${missingBinaryOutput}`);
}

console.log("launcher smoke tests passed");
