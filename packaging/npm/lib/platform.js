"use strict";

// Maps Node's process.platform / process.arch to the goreleaser release
// asset names (see .goreleaser.yaml, archive id "binary"):
//   skiletto_<os>_<arch>[.exe]

const OS_BY_PLATFORM = {
  linux: "linux",
  darwin: "darwin",
  win32: "windows",
};

const ARCH_BY_NODE_ARCH = {
  x64: "amd64",
  arm64: "arm64",
};

// resolveTarget returns the release asset name to download and the local
// binary file name for the given platform/arch, or throws a clear error
// for unsupported combinations.
function resolveTarget(platform, arch) {
  const os = OS_BY_PLATFORM[platform];
  const goarch = ARCH_BY_NODE_ARCH[arch];
  if (!os || !goarch) {
    const supported = [];
    for (const p of Object.keys(OS_BY_PLATFORM)) {
      for (const a of Object.keys(ARCH_BY_NODE_ARCH)) {
        supported.push(`${p}/${a}`);
      }
    }
    throw new Error(
      `skiletto: unsupported platform ${platform}/${arch}. ` +
        `Supported: ${supported.join(", ")}. ` +
        "Install manually from https://github.com/kumekay/skiletto/releases",
    );
  }
  const ext = os === "windows" ? ".exe" : "";
  return {
    assetName: `skiletto_${os}_${goarch}${ext}`,
    binaryName: `skiletto${ext}`,
  };
}

// downloadBaseURL returns the base URL that release assets are fetched from.
// SKILETTO_DOWNLOAD_BASE overrides it (used by tests and mirrors); otherwise
// the GitHub release for the given version tag (v<version>) is used.
function downloadBaseURL(version, env) {
  const e = env || process.env;
  if (e.SKILETTO_DOWNLOAD_BASE) {
    return e.SKILETTO_DOWNLOAD_BASE.replace(/\/+$/, "");
  }
  return `https://github.com/kumekay/skiletto/releases/download/v${version}`;
}

module.exports = {
  OS_BY_PLATFORM,
  ARCH_BY_NODE_ARCH,
  resolveTarget,
  downloadBaseURL,
};
