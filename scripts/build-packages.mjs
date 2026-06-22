import fs from "node:fs";
import path from "node:path";

import { platforms, repoRoot } from "./lib/platforms.mjs";

for (const platform of platforms) {
  const sourcePath = path.join(repoRoot, "dist", "npm", platform.id, platform.binaryName);
  const binDir = path.join(repoRoot, platform.packageDir, "bin");
  const targetPath = path.join(binDir, platform.binaryName);

  if (!fs.existsSync(sourcePath)) {
    throw new Error(`missing built binary: ${sourcePath}`);
  }

  fs.mkdirSync(binDir, { recursive: true });
  for (const entry of fs.readdirSync(binDir)) {
    if (entry === ".gitkeep") {
      continue;
    }
    fs.rmSync(path.join(binDir, entry), { recursive: true, force: true });
  }
  fs.copyFileSync(sourcePath, targetPath);

  if (platform.goos !== "windows") {
    fs.chmodSync(targetPath, 0o755);
  }
}
