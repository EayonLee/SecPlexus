import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { copyFileSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import test from "node:test";

for (const collision of ["permission", "result"]) {
  test(`catalog generator rejects ${collision} identifier collisions without changing exports`, (t) => {
    const root = mkdtempSync(path.join(tmpdir(), "spx-catalog-generator-"));
    t.after(() => rmSync(root, { recursive: true, force: true }));
    mkdirSync(path.join(root, "scripts"));
    for (const script of ["generate-catalogs.mjs", "catalog-files.mjs"]) {
      copyFileSync(new URL(script, import.meta.url), path.join(root, "scripts", script));
    }
    const codes = [{ name: "OK", code: "ok", status: 200, message: "成功", module: "common" }];
    const permissions = [{ key: "iam.foo.bar_baz", scope: "organization", delegable: true }];
    const codesPath = path.join(root, "src/packages/go/result/error-codes.json");
    const permissionsPath = path.join(root, "src/apps/iam/internal/authorization/permissions.json");
    for (const [filename, entries] of [
      [codesPath, codes],
      [permissionsPath, permissions],
    ]) {
      mkdirSync(path.dirname(filename), { recursive: true });
      writeFileSync(filename, JSON.stringify(entries));
    }
    const generate = (...args) =>
      spawnSync(process.execPath, ["scripts/generate-catalogs.mjs", ...args], {
        cwd: root,
        encoding: "utf8",
      });
    const initial = generate();
    assert.equal(initial.status, 0, initial.stderr);
    const manifest = "src/packages/generated-catalogs.json";
    const exports = JSON.parse(readFileSync(path.join(root, manifest), "utf8"));
    const before = [manifest, ...exports].map((filename) => [filename, readFileSync(path.join(root, filename))]);
    if (collision === "permission") {
      permissions.push({ key: "iam.foo_bar.baz", scope: "organization", delegable: true });
      writeFileSync(permissionsPath, JSON.stringify(permissions));
    } else {
      codes.push({ name: "OK", code: "other", status: 400, message: "失败", module: "common" });
      writeFileSync(codesPath, JSON.stringify(codes));
    }
    for (const args of [[], ["--check"]]) {
      const rejected = generate(...args);
      assert.notEqual(rejected.status, 0, "colliding identifiers were accepted");
      assert.match(
        rejected.stderr,
        collision === "permission" ? /Duplicate permission export name/ : /Duplicate result export name/,
      );
      for (const [filename, content] of before) {
        assert.deepEqual(readFileSync(path.join(root, filename)), content, `${filename} changed after rejection`);
      }
    }
  });
}
