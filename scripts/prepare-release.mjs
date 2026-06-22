import fs from "node:fs";
import path from "node:path";

import { normalizeVersion, platforms, repoRoot } from "./lib/platforms.mjs";

const rawVersion = process.argv[2] || process.env.MIHOMO_TUI_VERSION;
if (!rawVersion) {
  throw new Error("release version required: bun run prepare:release <version>");
}

const version = normalizeVersion(rawVersion);

function updateJSON(filePath, mutate) {
  const current = JSON.parse(fs.readFileSync(filePath, "utf8"));
  mutate(current);
  fs.writeFileSync(filePath, `${JSON.stringify(current, null, 2)}\n`);
}

updateJSON(path.join(repoRoot, "package.json"), (pkg) => {
  pkg.version = version;
});

updateJSON(path.join(repoRoot, "packages", "mihomo-tui", "package.json"), (pkg) => {
  pkg.version = version;
  pkg.dependencies["@metacubex/mihomo-tui"] = version;
});

updateJSON(path.join(repoRoot, "packages", "cli", "package.json"), (pkg) => {
  pkg.version = version;
  for (const platform of platforms) {
    pkg.optionalDependencies[platform.packageName] = version;
  }
});

for (const platform of platforms) {
  updateJSON(path.join(repoRoot, platform.packageDir, "package.json"), (pkg) => {
    pkg.version = version;
  });
}
