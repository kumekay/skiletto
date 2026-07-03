#!/usr/bin/env python3
"""Build a platform-specific skiletto wheel that bundles a prebuilt binary.

The wheel is a pure-launcher package (a console entry point that execs the
native binary) tagged for a single platform, so pip picks the right one.
Dependency-free: assembles the wheel with the standard library only.

Release-time usage (one call per goreleaser binary), e.g. from dist/:

    python packaging/pypi/build_wheel.py \
        --version 0.1.0 \
        --os linux --arch amd64 \
        --binary dist/skiletto_linux_amd64_v1/skiletto \
        --outdir wheelhouse

Produces wheelhouse/skiletto-0.1.0-py3-none-manylinux2014_x86_64.whl
"""

import argparse
import base64
import hashlib
import os
import sys
import zipfile

# goreleaser os/arch -> PyPI platform tag.
PLATFORM_TAGS = {
    ("linux", "amd64"): "manylinux2014_x86_64",
    ("linux", "arm64"): "manylinux2014_aarch64",
    ("darwin", "amd64"): "macosx_10_12_x86_64",
    ("darwin", "arm64"): "macosx_11_0_arm64",
    ("windows", "amd64"): "win_amd64",
    ("windows", "arm64"): "win_arm64",
}

SUMMARY = "Package manager for agent skills"
LAUNCHER_DIR = os.path.join(os.path.dirname(os.path.abspath(__file__)), "skiletto")


def _record_line(arcname: str, data: bytes) -> str:
    digest = hashlib.sha256(data).digest()
    b64 = base64.urlsafe_b64encode(digest).rstrip(b"=").decode("ascii")
    return "%s,sha256=%s,%d" % (arcname, b64, len(data))


def build(version: str, goos: str, goarch: str, binary: str, outdir: str) -> str:
    tag = PLATFORM_TAGS.get((goos, goarch))
    if tag is None:
        raise SystemExit("unsupported os/arch: %s/%s" % (goos, goarch))

    binary_name = "skiletto.exe" if goos == "windows" else "skiletto"
    full_tag = "py3-none-%s" % tag
    distinfo = "skiletto-%s.dist-info" % version

    metadata = (
        "Metadata-Version: 2.1\n"
        "Name: skiletto\n"
        "Version: %s\n"
        "Summary: %s\n"
        "Home-page: https://github.com/kumekay/skiletto\n"
        "License: MIT\n"
        "Requires-Python: >=3.8\n"
        "\n"
        "%s. This wheel bundles the platform-native skiletto binary.\n"
    ) % (version, SUMMARY, SUMMARY)

    wheel_meta = (
        "Wheel-Version: 1.0\n"
        "Generator: skiletto-build_wheel\n"
        "Root-Is-Purelib: false\n"
        "Tag: %s\n"
    ) % full_tag

    entry_points = "[console_scripts]\nskiletto = skiletto._launcher:main\n"

    # (arcname, bytes, unix_mode)
    members = []
    for fname in ("__init__.py", "_launcher.py"):
        with open(os.path.join(LAUNCHER_DIR, fname), "rb") as fh:
            members.append(("skiletto/%s" % fname, fh.read(), 0o644))
    with open(binary, "rb") as fh:
        members.append(("skiletto/data/%s" % binary_name, fh.read(), 0o755))
    members.append(("%s/METADATA" % distinfo, metadata.encode(), 0o644))
    members.append(("%s/WHEEL" % distinfo, wheel_meta.encode(), 0o644))
    members.append(("%s/entry_points.txt" % distinfo, entry_points.encode(), 0o644))

    record_lines = [_record_line(name, data) for name, data, _ in members]
    record_lines.append("%s/RECORD,," % distinfo)
    record = ("\n".join(record_lines) + "\n").encode()
    members.append(("%s/RECORD" % distinfo, record, 0o644))

    os.makedirs(outdir, exist_ok=True)
    wheel_path = os.path.join(outdir, "skiletto-%s-%s.whl" % (version, full_tag))
    with zipfile.ZipFile(wheel_path, "w", zipfile.ZIP_DEFLATED) as zf:
        for name, data, mode in members:
            info = zipfile.ZipInfo(name)
            info.external_attr = (mode & 0xFFFF) << 16
            info.compress_type = zipfile.ZIP_DEFLATED
            zf.writestr(info, data)
    return wheel_path


def main(argv=None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--version", required=True)
    parser.add_argument("--os", dest="goos", required=True)
    parser.add_argument("--arch", dest="goarch", required=True)
    parser.add_argument("--binary", required=True, help="path to the prebuilt binary")
    parser.add_argument("--outdir", default="wheelhouse")
    args = parser.parse_args(argv)
    path = build(args.version, args.goos, args.goarch, args.binary, args.outdir)
    print(path)
    return 0


if __name__ == "__main__":
    sys.exit(main())
