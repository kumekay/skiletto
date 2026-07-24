"""Tests for build_wheel.py. Run with: python -m unittest discover packaging/pypi"""

import os
import tempfile
import unittest
import zipfile

from build_wheel import build, normalize_version


class NormalizeVersionTest(unittest.TestCase):
    def test_plain_release_passes_through(self):
        self.assertEqual(normalize_version("0.1.0"), "0.1.0")
        self.assertEqual(normalize_version("1.22.333"), "1.22.333")

    def test_rc_prerelease(self):
        self.assertEqual(normalize_version("0.1.0-rc1"), "0.1.0rc1")
        self.assertEqual(normalize_version("0.1.0-rc.1"), "0.1.0rc1")

    def test_beta_prerelease(self):
        self.assertEqual(normalize_version("0.1.0-beta1"), "0.1.0b1")
        self.assertEqual(normalize_version("0.1.0-beta.2"), "0.1.0b2")

    def test_alpha_prerelease(self):
        self.assertEqual(normalize_version("0.1.0-alpha1"), "0.1.0a1")
        self.assertEqual(normalize_version("0.1.0-alpha.3"), "0.1.0a3")

    def test_unmappable_prerelease_fails_loudly(self):
        with self.assertRaises(ValueError) as ctx:
            normalize_version("0.1.0-foo")
        self.assertIn("0.1.0-foo", str(ctx.exception))
        self.assertIn("PEP 440", str(ctx.exception))

    def test_invalid_release_part_fails_loudly(self):
        with self.assertRaises(ValueError):
            normalize_version("not-a-version")
        with self.assertRaises(ValueError):
            normalize_version("0.1.0-rc")  # prerelease without a number


class ConsoleScriptsTest(unittest.TestCase):
    """The wheel must expose both the full `skiletto` command and the short
    `tto` alias, so `uvx skiletto` and a global `tto` both work."""

    def _entry_points(self):
        with tempfile.TemporaryDirectory() as tmp:
            binary = os.path.join(tmp, "skiletto")
            with open(binary, "wb") as fh:
                fh.write(b"#!/bin/sh\necho fake\n")
            outdir = os.path.join(tmp, "wheelhouse")
            wheel = build("0.1.0", "linux", "amd64", binary, outdir)
            with zipfile.ZipFile(wheel) as zf:
                return zf.read("skiletto-0.1.0.dist-info/entry_points.txt").decode()

    def _console_script_lines(self):
        ep = self._entry_points()
        return [
            line.strip()
            for line in ep.splitlines()
            if line.strip() and not line.startswith("[")
        ]

    def test_exposes_skiletto_command(self):
        self.assertIn("skiletto = skiletto._launcher:main", self._console_script_lines())

    def test_exposes_tto_alias(self):
        self.assertIn("tto = skiletto._launcher:main", self._console_script_lines())


if __name__ == "__main__":
    unittest.main()
