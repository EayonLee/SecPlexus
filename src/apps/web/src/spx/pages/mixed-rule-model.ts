import { z } from "zod";

import { parserModes } from "./rule-catalog";

const condition = z.union([
  z.object({ field: z.string(), equals: z.string() }),
  z.object({ field: z.string(), in: z.array(z.string()) }),
]);

const fieldMappings = z.object({
  field_map: z.record(z.string(), z.string()).default({}),
  constants: z.record(z.string(), z.unknown()).optional(),
  defaults: z.record(z.string(), z.unknown()).optional(),
  value_map: z.record(z.string(), z.record(z.string(), z.int())).optional(),
  time: z
    .object({ field: z.string().optional(), formats: z.array(z.string()).optional(), location: z.string().optional() })
    .optional(),
  severity: z
    .object({
      field: z.string().optional(),
      values: z.record(z.string(), z.int()).optional(),
      default: z.int().optional(),
    })
    .optional(),
});

const branch = fieldMappings.extend({
  id: z.string(),
  name: z.string(),
  class_uid: z.int(),
  // 保留 API 的条件写法，避免仅改名称或描述时改变已验证定义。
  match: z.array(condition).nullish(),
});

export const fieldOperations = [
  "trim",
  "trim_prefix",
  "trim_suffix",
  "decode_base64",
  "decode_uri",
  "decode_hex",
  "json",
] as const;

const processor = z.object({
  operation: z.enum(fieldOperations),
  source: z.string(),
  target: z.string(),
  path: z.string().optional(),
  value: z.string().optional(),
});

export type FieldProcessor = z.infer<typeof processor>;

export const mixedDefinition = z.object({
  contract_version: z.literal("v1"),
  catalog_revision: z.string().optional(),
  description: z.string().optional(),
  parse: z.looseObject({
    mode: z.enum(parserModes),
    pattern: z.string().optional(),
    kv: z
      .object({ delimiter: z.string().optional(), entry: z.string().optional(), quote: z.string().optional() })
      .optional(),
    delimiter: z.object({ separator: z.string().optional(), columns: z.array(z.string()).optional() }).optional(),
  }),
  encoding: z.string().optional(),
  branches: z.array(branch).default([]),
  shared: fieldMappings.optional(),
  processors: z.array(processor).optional(),
});

export type MixedDefinition = z.infer<typeof mixedDefinition>;
export type RuleBranch = z.infer<typeof branch>;

export function newBranch(id: string): RuleBranch {
  return { id, name: "", class_uid: 0, match: [], field_map: {} };
}

export function readEventValue(event: Record<string, unknown>, path: string) {
  let value: unknown = event;
  for (const part of path.split(".")) {
    value =
      typeof value === "object" && value !== null ? Object.getOwnPropertyDescriptor(value, part)?.value : undefined;
  }
  return value === undefined ? "—" : typeof value === "string" ? value : JSON.stringify(value);
}

export function splitRuleSamples(raw: string) {
  return raw.split(/\r?\n/).filter((sample) => sample.trim());
}
