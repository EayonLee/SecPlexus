import { existsSync, readdirSync } from "node:fs";
import path from "node:path";

// Only the small business-catalog exports belong to this generator.
export function extraGeneratedFiles(outputs, previous, root = ".") {
  const actual = new Set(previous.filter((filename) => existsSync(path.join(root, filename))));
  for (const directory of ["src/packages/go/result", "src/packages/go/iamclient", "src/apps/web/src/auth"]) {
    const absolute = path.join(root, directory);
    if (!existsSync(absolute)) continue;
    for (const entry of readdirSync(absolute, { withFileTypes: true })) {
      if (entry.isFile() && entry.name.includes(".gen.")) actual.add(`${directory}/${entry.name}`);
    }
  }
  return [...actual].filter((filename) => !outputs.has(filename));
}
