import { accessSync, constants } from "node:fs";
import { spawn } from "node:child_process";
import path from "node:path";
import { createRequire } from "node:module";

const require = createRequire(import.meta.url);

function detectLinuxLibc() {
  if (process.platform !== "linux") {
    return null;
  }

  const report = process.report?.getReport?.();
  if (report?.header?.glibcVersionRuntime) {
    return "gnu";
  }

  if (
    Array.isArray(report?.sharedObjects) &&
    report.sharedObjects.some((entry) => entry.toLowerCase().includes("musl"))
  ) {
    return "musl";
  }

  return "gnu";
}

function resolvePlatformPackageName() {
  switch (`${process.platform}:${process.arch}`) {
    case "darwin:arm64":
      return "@metacubex/mihomo-tui-darwin-arm64";
    case "darwin:x64":
      return "@metacubex/mihomo-tui-darwin-x64";
    case "linux:arm64":
      return detectLinuxLibc() === "gnu" ? "@metacubex/mihomo-tui-linux-arm64-gnu" : null;
    case "linux:x64":
      return detectLinuxLibc() === "gnu" ? "@metacubex/mihomo-tui-linux-x64-gnu" : null;
    case "win32:arm64":
      return "@metacubex/mihomo-tui-win32-arm64-msvc";
    case "win32:x64":
      return "@metacubex/mihomo-tui-win32-x64-msvc";
    default:
      return null;
  }
}

export function resolveBinaryPath() {
  const packageName = resolvePlatformPackageName();
  if (!packageName) {
    throw new Error(`mihomo-tui does not support ${process.platform} ${process.arch} on this runtime`);
  }

  let manifestPath;
  try {
    manifestPath = require.resolve(`${packageName}/package.json`);
  } catch {
    throw new Error(`mihomo-tui binary not found for ${process.platform} ${process.arch}; reinstall package to fetch platform binary`);
  }

  const binaryName = process.platform === "win32" ? "mihomo-tui.exe" : "mihomo-tui";
  const binaryPath = path.join(path.dirname(manifestPath), "bin", binaryName);
  try {
    accessSync(binaryPath, constants.F_OK);
  } catch {
    throw new Error(`mihomo-tui binary not found at ${binaryPath}`);
  }
  return binaryPath;
}

export async function run(args = []) {
  const binaryPath = resolveBinaryPath();

  await new Promise((resolve, reject) => {
    const child = spawn(binaryPath, args, {
      stdio: "inherit",
      env: process.env,
    });

    child.on("error", reject);
    child.on("exit", (code, signal) => {
      if (signal) {
        process.kill(process.pid, signal);
        return;
      }
      process.exitCode = code ?? 0;
      resolve();
    });
  });
}
