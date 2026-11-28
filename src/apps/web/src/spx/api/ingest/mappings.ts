import { z } from "zod";

import { pageSchema, request } from "../client";
import type { MappingBody } from "./schemas";
import { zMappingBody, zMappingDetail, zMappingSummary, zRuleVerification, zSampleResult } from "./schemas";

export const mappingKey = ["ingest", "mappings"] as const;

export function listMappingPage(
  params: {
    limit: number;
    offset?: number;
    q?: string;
    parse_type?: string;
    origin?: string;
    class_uid?: number;
    id?: string[];
  },
  signal?: AbortSignal,
) {
  return request(
    { url: "/api/v1/ingest/parsing-rules", params, paramsSerializer: { indexes: null }, signal },
    pageSchema(zMappingSummary),
  );
}

export const getMapping = (id: string, signal?: AbortSignal) =>
  request({ url: `/api/v1/ingest/parsing-rules/${encodeURIComponent(id)}`, signal }, zMappingDetail);

// createMapping 新建解析映射；服务端会先编译，非法规则不会落库。
export async function createMapping(body: MappingBody, signal?: AbortSignal) {
  return request(
    { method: "POST", url: "/api/v1/ingest/parsing-rules", data: zMappingBody.parse(body), signal },
    zMappingDetail,
    201,
  );
}

// updateMapping 换规则内容；服务端会把版本自增。
export async function updateMapping(id: string, body: MappingBody, signal?: AbortSignal) {
  return request(
    { method: "PATCH", url: `/api/v1/ingest/parsing-rules/${encodeURIComponent(id)}`, data: zMappingBody.parse(body), signal },
    zMappingDetail,
  );
}

// deleteMapping 删除映射；仍被接入点引用时服务端返回 409。
export async function deleteMapping(id: string): Promise<void> {
  await request({ method: "DELETE", url: `/api/v1/ingest/parsing-rules/${encodeURIComponent(id)}` }, z.undefined(), 204);
}

// validateMapping 用真实样本试跑规则，保存前就能看到命中率。
export async function validateMapping(
  definition: unknown,
  samples: string[],
  source?: { vendor: string; product: string; parsing_rule_id?: string; expected_version?: number },
  signal?: AbortSignal,
) {
  return request(
    { method: "POST", url: "/api/v1/ingest/parsing-rules/validate", data: { definition, samples, ...source }, signal },
    zSampleResult,
  );
}

const zExtractionValueType = z.enum(["string", "number", "boolean", "object", "array", "null"]);
export type ExtractionValueType = z.infer<typeof zExtractionValueType>;

const zExtractionResult = z.object({
  compiled: z.boolean(),
  compile_error: z.string().optional(),
  previews: z
    .array(
      z.object({
        index: z.number(),
        fields: z.record(z.string(), z.unknown()).nullable(),
        field_types: z.record(z.string(), zExtractionValueType).optional(),
        reason: z.string().optional(),
        truncated: z.boolean().optional(),
        issues: z.array(z.object({ path: z.string().optional(), code: z.string() })).optional(),
      }),
    )
    .optional(),
});

export function extractMapping(
  settings: { parse: unknown; processors?: unknown },
  samples: string[],
  signal: AbortSignal,
) {
  return request(
    { method: "POST", url: "/api/v1/ingest/parsing-rules/extract", data: { ...settings, samples }, signal },
    zExtractionResult,
  );
}

const zRevision = z.object({
  parsing_rule_id: z.uuid(),
  version: z.int().positive(),
  name: z.string(),
  vendor: z.string(),
  product: z.string(),
  recorded_at: z.string(),
  author_id: z.uuid().optional(),
  verification: zRuleVerification.optional(),
  definition: z.record(z.string(), z.unknown()).optional(),
});

export function listMappingRevisions(id: string, before: number | undefined, signal: AbortSignal) {
  return request(
    { url: `/api/v1/ingest/parsing-rules/${encodeURIComponent(id)}/revisions`, params: { before_version: before }, signal },
    z.object({ items: z.array(zRevision), next_before_version: z.int().positive().optional() }),
  );
}

export function getMappingRevision(id: string, version: number, signal: AbortSignal) {
  return request(
    { url: `/api/v1/ingest/parsing-rules/${encodeURIComponent(id)}/revisions/${version}`, signal },
    zRevision,
  );
}

export function restoreMappingRevision(id: string, version: number, expectedVersion: number) {
  return request(
    {
      method: "POST",
      url: `/api/v1/ingest/parsing-rules/${encodeURIComponent(id)}/revisions/${version}/restore`,
      data: { expected_version: expectedVersion },
    },
    zMappingDetail,
  );
}
