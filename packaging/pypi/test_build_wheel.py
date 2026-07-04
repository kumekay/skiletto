"""Tests for build_wheel.py. Run with: python -m unittest discover packaging/pypi"""

import unittest

from build_wheel import normalize_version


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


if __name__ == "__main__":
    unittest.main()
