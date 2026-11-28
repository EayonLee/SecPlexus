import { z } from "zod";

import { ruleMetadata } from "./rule-validation";

// zMapping 描述解析映射；definition 是规则的 JSON 形态，页面只展示版本与名称。
const sourceIdentity = { vendor: z.string().trim().max(120), product: z.string().trim().max(120) };

export const zMapping = z.object({
  vendor: z.string(),
  product: z.string(),
  id: z.string(),
  name: z.string(),
  version: z.number(),
  builtin: z.boolean(),
  class_uids: z.array(z.int()).optional(),
  created_at: z.string(),
  updated_at: z.string(),
});

export type ApiMapping = z.infer<typeof zMapping>;

export const zMappingSummary = zMapping.extend({
  description: z.string().optional(),
  parse_type: z.string().optional(),
});

export type ApiMappingSummary = z.infer<typeof zMappingSummary>;

// zMappingDetail 在列表字段之外带上 definition：编辑页需要完整规则。
// definition 用 unknown 承接，页面按 JSON 文本编辑，形状由服务端编译校验。
export const zRuleVerification = z.object({
  definition_digest: z.string().regex(/^[0-9a-f]{64}$/),
  verified_at: z.string(),
  sample_count: z.int().nonnegative(),
  passed: z.int().nonnegative(),
  covered_branches: z.array(z.string()),
});

export const zMappingDetail = zMapping.extend({
  definition: z.unknown(),
  verification: zRuleVerification.optional(),
});

export type ApiMappingDetail = z.infer<typeof zMappingDetail>;

// zMappingBody 是创建或修改映射的请求体。
export const zMappingBody = z
  .object({
    name: ruleMetadata.shape.name,
    vendor: ruleMetadata.shape.vendor,
    product: ruleMetadata.shape.product,
    definition: z.object({ description: ruleMetadata.shape.description }).catchall(z.unknown()),
    samples: z.array(z.string()).max(20).optional(),
    expected_version: z.int().positive().optional(),
  })
  .strict();

export type MappingBody = z.infer<typeof zMappingBody>;

// zInput 描述接入点。字段与 ingest 管理 API 的响应一一对应。
export const zInput = z.object({
  ...sourceIdentity,
  id: z.string(),
  name: z.string(),
  protocol: z.enum(["http", "syslog", "kafka"]),
  parsing_rule_id: z.string().nullable(),
  syslog_port: z.number().nullable(),
  kafka_brokers: z.array(z.string()).nullable(),
  kafka_topic: z.string().nullable(),
  kafka_credential: z.string().optional(),
  kafka_start_position: z.enum(["earliest", "since_created"]).optional(),
  enabled: z.boolean(),
  revision: z.int().nonnegative().optional(),
  credential_generation: z.int().nonnegative().optional(),
  activation: z.object({
    state: z.enum(["applied", "applying", "failed", "unknown"]),
    reason: z.enum(["rule_invalid", "configuration_invalid", "credential_invalid", "kafka_position_unavailable"]).optional(),
  }).optional(),
  observations_ready: z.boolean().optional(),
  traffic_status: z.enum(["receiving", "unknown", "waiting", "quiet", "disabled"]).optional(),
  stats: z
    .object({
      received: z.number(),
      parse_failed: z.number(),
      last_received: z.string(),
      confirmed: z.number().optional(),
      last_confirmed: z.string().optional(),
      parsed: z.number().optional(),
      partial: z.number().optional(),
      unclassified: z.number().optional(),
    })
    .optional(),
  created_at: z.string(),
  updated_at: z.string(),
});

export type ApiInput = z.infer<typeof zInput>;

// zCreateInputBody 是新建接入点的请求体。
const inputFields = {
  vendor: sourceIdentity.vendor.default(""),
  product: sourceIdentity.product.default(""),
  name: z.string().trim().min(1).max(100),
  parsing_rule_id: z.string().optional(),
  expected_rule_version: z.int().positive().optional(),
};
export const zCreateInputBody = z.discriminatedUnion("protocol", [
  z.object({ ...inputFields, protocol: z.literal("http") }),
  z.object({ ...inputFields, protocol: z.literal("syslog"), syslog_port: z.int().min(1024).max(65535) }),
  z.object({
    ...inputFields,
    protocol: z.literal("kafka"),
    kafka_brokers: z.array(z.string().trim().min(1)).min(1),
    kafka_topic: z.string().trim().min(1).max(249),
    kafka_credential: z.string().regex(/^[a-z][a-z0-9_-]{0,63}$/).optional(),
    kafka_start_position: z.enum(["earliest", "since_created"]),
  }),
]);

export type CreateInputBody = z.infer<typeof zCreateInputBody>;

// zUpdateInputBody 是修改接入点的请求体；协议不可变，因此不在字段里。
export const zUpdateInputBody = z
  .object({
    name: z.string().min(1).max(100).optional(),
    vendor: sourceIdentity.vendor.optional(),
    product: sourceIdentity.product.optional(),
    enabled: z.boolean().optional(),
    parsing_rule_id: z.string().optional(),
    expected_rule_version: z.int().positive().optional(),
    unbind_rule: z.boolean().optional(),
    rotate_token: z.boolean().optional(),
  })
  .strict();

export type UpdateInputBody = z.infer<typeof zUpdateInputBody>;

// zCreateInputResult 带回一次性明文令牌：服务端只存摘要，之后无从取回。
export const zCreateInputResult = z.object({
  input: zInput,
  token: z.string().optional(),
});

export type CreateInputResult = z.infer<typeof zCreateInputResult>;

// zSampleResult 是样本验证结果：编译是否通过、命中数与失败明细。
export const zSampleResult = z.object({
  compiled: z.boolean(),
  total: z.number(),
  matched: z.number(),
  required_branches: z.array(z.string()),
  compile_error: z.string().optional(),
  compile_issue: z.object({ path: z.string().optional(), code: z.string() }).optional(),
  failures: z.array(z.object({ index: z.number(), reason: z.string() })).optional(),
  previews: z
    .array(
      z.object({
        index: z.number(),
        fields: z.record(z.string(), z.unknown()),
        event: z.object({ unmapped: z.record(z.string(), z.unknown()).optional() }).catchall(z.unknown()),
        partition_key: z.string(),
        branch_id: z.string().optional(),
        status: z.enum(["ok", "partial", "error"]),
        candidate_class_uid: z.int().optional(),
        applied_defaults: z.string().array().optional(),
        issues: z.array(z.object({ path: z.string().optional(), code: z.string() })).optional(),
      }),
    )
    .optional(),
});

export type SampleResult = z.infer<typeof zSampleResult>;
