#!/usr/bin/env node
"use strict";

// Launcher: spawns the downloaded native skiletto binary with the caller's
// arguments and forwards its exit code.

const path = require("path");
const { spawnSync } = require("child_process");
const { resolveTarget } = require("../lib/platform");

const { binaryName } = resolveTarget(process.platform, process.arch);
const binary = path.join(__dirname, "..", "binary", binaryName);

const result = spawnSync(binary, process.argv.slice(2), { stdio: "inherit" });

if (result.error) {
  if (result.error.code === "ENOENT") {
    console.error(
      "skiletto: binary not found at " +
        binary +
        ". Reinstall the package to download it " +
        "(npm rebuild skiletto), or grab a build from " +
        "https://github.com/kumekay/skiletto/releases",
    );
    process.exit(1);
  }
  throw result.error;
}

process.exit(result.status === null ? 1 : result.status);
