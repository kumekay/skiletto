"use strict";

const test = require("node:test");
const assert = require("node:assert");
const path = require("node:path");
const pkg = require("../package.json");

// The package name stays `skiletto` (what `npx skiletto` installs), but the
// short `tto` alias must be exposed as a command too, so both `skiletto` and
// `tto` work after a global install.
test("exposes both the skiletto command and the tto alias", () => {
  assert.strictEqual(pkg.name, "skiletto");
  assert.strictEqual(pkg.bin.skiletto, "bin/skiletto.js");
  assert.strictEqual(pkg.bin.tto, "bin/skiletto.js");
});

test("declared bin launchers exist on disk", () => {
  for (const launcher of Object.values(pkg.bin)) {
    const abs = path.join(__dirname, "..", launcher);
    assert.ok(require("node:fs").existsSync(abs), `missing ${launcher}`);
  }
});
