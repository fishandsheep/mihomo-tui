import { execFileSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";

import { parseVersionArg, platforms, repoRoot } from "./lib/platforms.mjs";

const version = parseVersionArg(process.argv.slice(2), process.env.MIHOMO_TUI_VERSION || "dev");
const outRoot = path.join(repoRoot, "dist", "npm");

fs.rmSync(outRoot, { recursive: true, force: true });
fs.mkdirSync(outRoot, { recursive: true });

for (const platform of platforms) {
  const targetDir = path.join(outRoot, platform.id);
  const targetPath = path.join(targetDir, platform.binaryName);

  fs.mkdirSync(targetDir, { recursive: true });

  execFileSync(
    "go",
    [
      "build",
      "-trimpath",
      "-ldflags",
      `-s -w -X main.version=${version}`,
      "-o",
      targetPath,
      "./cmd/tui"
    ],
    {
      cwd: repoRoot,
      env: {
        ...process.env,
        GOOS: platform.goos,
        GOARCH: platform.goarch,
        CGO_ENABLED: "0"
      },
      stdio: "inherit"
    }
  );

  if (platform.goos !== "windows") {
    fs.chmodSync(targetPath, 0o755);
  }
}
