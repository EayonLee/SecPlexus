import { readdirSync, readFileSync } from 'node:fs';
import { join } from 'node:path';

// 迁移边界检查：每个模块的迁移只能改动并引用自己 schema 内的对象。
// 背景：全平台共用一个 PostgreSQL，模块按 schema 隔离（iam.、ingestor.…）。
// 跨 schema 外键会把两个模块的迁移顺序、回滚顺序和表生命周期绑死，
// 因此在这里用机器检查替代人工约定；判断依据是 SQL 里的 REFERENCES 目标。

const moduleRoot = 'src/apps';

/** 解析单个模块目录内所有迁移文件的归属 schema。 */
export function moduleSchemas(root) {
  const schemas = new Map();
  for (const app of readdirSync(root, { withFileTypes: true })) {
    if (!app.isDirectory()) continue;
    let dir;
    try {
      dir = join(root, app.name, 'migrations');
    } catch {
      continue;
    }
    let files;
    try {
      files = readdirSync(dir).filter((name) => name.endsWith('.sql'));
    } catch {
      continue;
    }
    if (files.length === 0) continue;
    const schema = schemaUsedBy(app.name, files.map((name) => readFileSync(join(dir, name), 'utf8')));
    if (schema) schemas.set(app.name, { schema, dir, files });
  }
  return schemas;
}

/** 从建表/建 schema 语句推断模块归属 schema；一个模块只允许一个。 */
function schemaUsedBy(app, contents) {
  const schemas = new Set();
  for (const sql of contents) {
    for (const match of sql.matchAll(/(?:CREATE SCHEMA IF NOT EXISTS|CREATE SCHEMA)\s+([a-z_][a-z0-9_]*)/gi)) {
      schemas.add(match[1].toLowerCase());
    }
    for (const match of sql.matchAll(/(?:CREATE TABLE(?: IF NOT EXISTS)?)\s+([a-z_][a-z0-9_]*)\./gi)) {
      schemas.add(match[1].toLowerCase());
    }
  }
  if (schemas.size > 1) throw new Error(`module ${app} touches multiple schemas: ${[...schemas].join(', ')}`);
  return schemas.size === 1 ? [...schemas][0] : null;
}

/** 提取迁移 SQL 中全部外键引用目标（schema.table 形式）。 */
export function foreignKeyTargets(sql) {
  const targets = [];
  for (const match of sql.matchAll(/REFERENCES\s+(?:ONLY\s+)?([a-zA-Z_][a-zA-Z0-9_]*)\.([a-zA-Z_][a-zA-Z0-9_]*)/g)) {
    targets.push(`${match[1].toLowerCase()}.${match[2].toLowerCase()}`);
  }
  return targets;
}

/** 检查所有模块迁移；返回违规列表（模块、文件、目标 schema）。 */
export function checkMigrationBoundaries(root = moduleRoot) {
  const modules = moduleSchemas(root);
  const violations = [];
  for (const [app, { schema, dir, files }] of modules) {
    for (const name of files) {
      const sql = readFileSync(join(dir, name), 'utf8');
      for (const target of foreignKeyTargets(sql)) {
        if (target.startsWith(`${schema}.`)) continue;
        violations.push({ module: app, file: name, target });
      }
    }
  }
  return violations;
}

export function main() {
  const violations = checkMigrationBoundaries();
  if (violations.length > 0) {
    for (const { module, file, target } of violations) {
      console.error(`cross-schema reference: ${module}/migrations/${file} REFERENCES ${target}`);
    }
    console.error('Migrations may only reference objects in the owning module schema; store identifiers and resolve across modules through their APIs.');
    process.exitCode = 1;
    return;
  }
  console.log(`Migration boundaries valid (${checkMigrationBoundaries.name}).`);
}
