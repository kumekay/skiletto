"use strict";

const test = require("node:test");
const assert = require("node:assert");
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");
const http = require("node:http");
const { install } = require("../install");

// Serves a fake release layout: a redirect from the "release download" path
// to a CDN-style path that returns the binary bytes — mirroring how GitHub
// releases 302 to their storage host.
function startServer(bytes) {
  const server = http.createServer((req, res) => {
    if (req.url === "/rel/skiletto_linux_amd64") {
      res.writeHead(302, { location: "/cdn/blob" });
      res.end();
      return;
    }
    if (req.url === "/cdn/blob") {
      res.writeHead(200, { "content-type": "application/octet-stream" });
      res.end(bytes);
      return;
    }
    res.writeHead(404);
    res.end("not found");
  });
  return new Promise((resolve) => {
    server.listen(0, "127.0.0.1", () => resolve(server));
  });
}

test("install downloads the binary, following redirects, and marks it executable", async () => {
  const payload = Buffer.from("#!/bin/sh\necho fake-skiletto\n");
  const server = await startServer(payload);
  const port = server.address().port;
  const binaryDir = fs.mkdtempSync(path.join(os.tmpdir(), "skiletto-npm-"));
  try {
    const dest = await install({
      platform: "linux",
      arch: "x64",
      version: "9.9.9",
      binaryDir,
      env: { SKILETTO_DOWNLOAD_BASE: `http://127.0.0.1:${port}/rel` },
    });
    assert.strictEqual(dest, path.join(binaryDir, "skiletto"));
    assert.deepStrictEqual(fs.readFileSync(dest), payload);
    const mode = fs.statSync(dest).mode & 0o777;
    assert.strictEqual(mode & 0o100, 0o100, "binary should be user-executable");
  } finally {
    server.close();
    fs.rmSync(binaryDir, { recursive: true, force: true });
  }
});

test("install rejects with a clear error when the asset is missing", async () => {
  const server = await startServer(Buffer.from("x"));
  const port = server.address().port;
  const binaryDir = fs.mkdtempSync(path.join(os.tmpdir(), "skiletto-npm-"));
  try {
    await assert.rejects(
      install({
        platform: "darwin",
        arch: "arm64",
        version: "9.9.9",
        binaryDir,
        env: { SKILETTO_DOWNLOAD_BASE: `http://127.0.0.1:${port}/rel` },
      }),
      /HTTP 404/,
    );
  } finally {
    server.close();
    fs.rmSync(binaryDir, { recursive: true, force: true });
  }
});
