import { z } from "zod";

// zMapping 描述解析映射；definition 是规则的 JSON 形态，页面只展示版本与名称。
export const zMapping = z.object({
  id: z.string(),
  name: z.string(),
  version: z.number(),
  created_at: z.string(),
  updated_at: z.string(),
});

export type ApiMapping = z.infer<typeof zMapping>;

// zMappingDetail 在列表字段之外带上 definition：编辑页需要完整规则。
// definition 用 unknown 承接，页面按 JSON 文本编辑，形状由服务端编译校验。
export const zMappingDetail = zMapping.extend({
  definition: z.unknown(),
});

export type ApiMappingDetail = z.infer<typeof zMappingDetail>;

// zMappingBody 是创建或修改映射的请求体。
export const zMappingBody = z
  .object({
    name: z.string().min(1).max(100),
    definition: z.record(z.string(), z.unknown()),
  })
  .strict();

export type MappingBody = z.infer<typeof zMappingBody>;

// zInput 描述接入点。字段与 ingest 管理 API 的响应一一对应。
export const zInput = z.object({
  id: z.string(),
  name: z.string(),
  protocol: z.enum(["http", "syslog", "kafka"]),
  mapping_id: z.string().nullable(),
  syslog_port: z.number().nullable(),
  kafka_brokers: z.array(z.string()).nullable(),
  kafka_topic: z.string().nullable(),
  enabled: z.boolean(),
  created_at: z.string(),
  updated_at: z.string(),
});

export type ApiInput = z.infer<typeof zInput>;

// zCreateInputBody 是新建接入点的请求体。
export const zCreateInputBody = z.object({
  name: z.string().min(1).max(100),
  protocol: z.enum(["http", "syslog", "kafka"]),
  syslog_port: z.number().int().min(1024).max(65535).optional(),
  kafka_topic: z.string().min(1).max(249).optional(),
  kafka_brokers: z.array(z.string().min(1)).optional(),
});

export type CreateInputBody = z.infer<typeof zCreateInputBody>;

// zUpdateInputBody 是修改接入点的请求体；协议不可变，因此不在字段里。
export const zUpdateInputBody = z
  .object({
    name: z.string().min(1).max(100).optional(),
    enabled: z.boolean().optional(),
    mapping_id: z.string().nullable().optional(),
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
  compile_error: z.string().optional(),
  failures: z.array(z.object({ index: z.number(), sample: z.string(), reason: z.string() })).optional(),
});

export type SampleResult = z.infer<typeof zSampleResult>;

