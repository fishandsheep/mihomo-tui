import path from "node:path";
import { fileURLToPath } from "node:url";

export const repoRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../..");

export const platforms = [
  {
    id: "darwin-arm64",
    packageDir: "packages/darwin-arm64",
    packageName: "@metacubex/mihomo-tui-darwin-arm64",
    goos: "darwin",
    goarch: "arm64",
    os: "darwin",
    arch: "arm64",
    binaryName: "mihomo-tui",
    archiveName: "mihomo-tui-darwin-arm64.tar.gz"
  },
  {
    id: "darwin-x64",
    packageDir: "packages/darwin-x64",
    packageName: "@metacubex/mihomo-tui-darwin-x64",
    goos: "darwin",
    goarch: "amd64",
    os: "darwin",
    arch: "x64",
    binaryName: "mihomo-tui",
    archiveName: "mihomo-tui-darwin-x64.tar.gz"
  },
  {
    id: "linux-arm64-gnu",
    packageDir: "packages/linux-arm64-gnu",
    packageName: "@metacubex/mihomo-tui-linux-arm64-gnu",
    goos: "linux",
    goarch: "arm64",
    os: "linux",
    arch: "arm64",
    binaryName: "mihomo-tui",
    archiveName: "mihomo-tui-linux-arm64-gnu.tar.gz"
  },
  {
    id: "linux-x64-gnu",
    packageDir: "packages/linux-x64-gnu",
    packageName: "@metacubex/mihomo-tui-linux-x64-gnu",
    goos: "linux",
    goarch: "amd64",
    os: "linux",
    arch: "x64",
    binaryName: "mihomo-tui",
    archiveName: "mihomo-tui-linux-x64-gnu.tar.gz"
  },
  {
    id: "win32-arm64-msvc",
    packageDir: "packages/win32-arm64-msvc",
    packageName: "@metacubex/mihomo-tui-win32-arm64-msvc",
    goos: "windows",
    goarch: "arm64",
    os: "win32",
    arch: "arm64",
    binaryName: "mihomo-tui.exe",
    archiveName: "mihomo-tui-win32-arm64-msvc.zip"
  },
  {
    id: "win32-x64-msvc",
    packageDir: "packages/win32-x64-msvc",
    packageName: "@metacubex/mihomo-tui-win32-x64-msvc",
    goos: "windows",
    goarch: "amd64",
    os: "win32",
    arch: "x64",
    binaryName: "mihomo-tui.exe",
    archiveName: "mihomo-tui-win32-x64-msvc.zip"
  }
];

export function normalizeVersion(rawVersion) {
  return rawVersion.replace(/^v/, "");
}

export function parseVersionArg(argv, fallback) {
  for (let i = 0; i < argv.length; i += 1) {
    const arg = argv[i];
    if (arg === "--version" && argv[i + 1]) {
      return normalizeVersion(argv[i + 1]);
    }
    if (arg.startsWith("--version=")) {
      return normalizeVersion(arg.slice("--version=".length));
    }
  }
  return normalizeVersion(fallback);
}
