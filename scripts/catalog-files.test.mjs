import assert from "node:assert/strict";
import { mkdtempSync, mkdirSync, writeFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import test from "node:test";
import { extraGeneratedFiles } from "./catalog-files.mjs";

test("generated inventory rejects unlisted files while leaving handwritten inputs outside ownership", () => {
  const root = mkdtempSync(path.join(tmpdir(), "spx-catalog-inventory-"));
  try {
    const generated = [
      "src/apps/web/src/auth/extra.gen.ts",
      "src/packages/go/result/extra.gen.go",
      "src/packages/go/iamclient/extra.gen.go",
    ];
    const handwritten = [
      "src/apps/web/src/spx/api/iam/schemas.ts",
      "src/apps/iam/internal/model/http.go",
      "src/packages/go/iamclient/types.go",
    ];
    for (const filename of [...generated, ...handwritten]) {
      mkdirSync(path.dirname(path.join(root, filename)), { recursive: true });
      writeFileSync(path.join(root, filename), "test");
    }
    assert.deepEqual(extraGeneratedFiles(new Map(), [], root).sort(), generated.sort());
    assert.deepEqual(extraGeneratedFiles(new Map(generated.map((name) => [name, "test"])), [], root), []);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});
