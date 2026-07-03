"""Console entry point: exec the bundled skiletto binary."""

import os
import sys


def _binary_path() -> str:
    name = "skiletto.exe" if os.name == "nt" else "skiletto"
    return os.path.join(os.path.dirname(__file__), "data", name)


def main() -> int:
    binary = _binary_path()
    if not os.path.exists(binary):
        sys.stderr.write(
            "skiletto: bundled binary not found at %s. "
            "Reinstall the package, or download a build from "
            "https://github.com/kumekay/skiletto/releases\n" % binary
        )
        return 1

    # Ensure the extracted binary is executable (pip does not always preserve
    # the executable bit for package data files).
    if os.name != "nt" and not os.access(binary, os.X_OK):
        try:
            os.chmod(binary, 0o755)
        except OSError:
            pass

    args = [binary, *sys.argv[1:]]
    if os.name == "nt":
        import subprocess

        return subprocess.run(args).returncode

    os.execv(binary, args)
    return 1  # unreachable on success


if __name__ == "__main__":
    raise SystemExit(main())
