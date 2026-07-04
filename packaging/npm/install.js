"use strict";

// postinstall script: downloads the skiletto release binary matching the
// current platform/arch into binary/ next to this file. Dependency-free
// (Node builtins only).

const fs = require("fs");
const path = require("path");
const http = require("http");
const https = require("https");
const { URL } = require("url");
const { resolveTarget, downloadBaseURL } = require("./lib/platform");

const MAX_REDIRECTS = 5;

// download fetches url into destPath, following up to MAX_REDIRECTS 3xx
// redirects (GitHub release downloads redirect to a CDN host). Rejects with
// a clear error on any non-200 response or transport failure.
function download(url, destPath, redirectsLeft) {
  if (redirectsLeft === undefined) redirectsLeft = MAX_REDIRECTS;
  return new Promise((resolve, reject) => {
    const u = new URL(url);
    const client = u.protocol === "http:" ? http : https;
    const req = client.get(u, (res) => {
      const status = res.statusCode || 0;
      if (status >= 300 && status < 400 && res.headers.location) {
        res.resume();
        if (redirectsLeft <= 0) {
          reject(new Error(`skiletto: too many redirects fetching ${url}`));
          return;
        }
        const next = new URL(res.headers.location, u).toString();
        resolve(download(next, destPath, redirectsLeft - 1));
        return;
      }
      if (status !== 200) {
        res.resume();
        reject(
          new Error(`skiletto: failed to download ${url} (HTTP ${status})`),
        );
        return;
      }
      const tmp = destPath + ".download";
      const file = fs.createWriteStream(tmp);
      res.pipe(file);
      file.on("finish", () => {
        file.close(() => {
          fs.renameSync(tmp, destPath);
          resolve();
        });
      });
      file.on("error", (err) => {
        fs.rmSync(tmp, { force: true });
        reject(err);
      });
    });
    req.on("error", (err) =>
      reject(new Error(`skiletto: download failed for ${url}: ${err.message}`)),
    );
  });
}

// install downloads the binary for the given options into binaryDir and
// makes it executable. Returns the path to the installed binary.
async function install(opts) {
  const options = opts || {};
  const platform = options.platform || process.platform;
  const arch = options.arch || process.arch;
  const version = options.version || require("./package.json").version;
  const binaryDir = options.binaryDir || path.join(__dirname, "binary");
  const env = options.env || process.env;

  const { assetName, binaryName } = resolveTarget(platform, arch);
  const base = downloadBaseURL(version, env);
  const url = `${base}/${assetName}`;
  const dest = path.join(binaryDir, binaryName);

  fs.mkdirSync(binaryDir, { recursive: true });
  await download(url, dest);
  if (platform !== "win32") {
    fs.chmodSync(dest, 0o755);
  }
  return dest;
}

module.exports = { install, download };

if (require.main === module) {
  install().catch((err) => {
    console.error(err.message || err);
    process.exit(1);
  });
}
