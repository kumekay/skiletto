"use strict";

const test = require("node:test");
const assert = require("node:assert");
const { resolveTarget, downloadBaseURL } = require("../lib/platform");

test("resolveTarget maps supported platform/arch to asset and binary names", () => {
  assert.deepStrictEqual(resolveTarget("linux", "x64"), {
    assetName: "skiletto_linux_amd64",
    binaryName: "skiletto",
  });
  assert.deepStrictEqual(resolveTarget("linux", "arm64"), {
    assetName: "skiletto_linux_arm64",
    binaryName: "skiletto",
  });
  assert.deepStrictEqual(resolveTarget("darwin", "x64"), {
    assetName: "skiletto_darwin_amd64",
    binaryName: "skiletto",
  });
  assert.deepStrictEqual(resolveTarget("darwin", "arm64"), {
    assetName: "skiletto_darwin_arm64",
    binaryName: "skiletto",
  });
  assert.deepStrictEqual(resolveTarget("win32", "x64"), {
    assetName: "skiletto_windows_amd64.exe",
    binaryName: "skiletto.exe",
  });
  assert.deepStrictEqual(resolveTarget("win32", "arm64"), {
    assetName: "skiletto_windows_arm64.exe",
    binaryName: "skiletto.exe",
  });
});

test("resolveTarget throws a clear error for unsupported platform", () => {
  assert.throws(() => resolveTarget("freebsd", "x64"), /unsupported platform freebsd\/x64/);
});

test("resolveTarget throws a clear error for unsupported arch", () => {
  assert.throws(() => resolveTarget("linux", "ia32"), /unsupported platform linux\/ia32/);
});

test("downloadBaseURL defaults to the GitHub release for the version tag", () => {
  assert.strictEqual(
    downloadBaseURL("1.2.3", {}),
    "https://github.com/kumekay/skiletto/releases/download/v1.2.3",
  );
});

test("downloadBaseURL honors SKILETTO_DOWNLOAD_BASE and trims trailing slashes", () => {
  assert.strictEqual(
    downloadBaseURL("1.2.3", { SKILETTO_DOWNLOAD_BASE: "http://127.0.0.1:9999/rel/" }),
    "http://127.0.0.1:9999/rel",
  );
});
