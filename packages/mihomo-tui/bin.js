#!/usr/bin/env node

const { run } = await import("@qinshower/mihomo-tui");

try {
  await run(process.argv.slice(2));
} catch (error) {
  console.error(error instanceof Error ? error.message : error);
  process.exit(1);
}
