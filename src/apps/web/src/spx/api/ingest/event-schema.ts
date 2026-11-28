import { z } from "zod";

import { request } from "../client";

const text = z.object({ en: z.string(), "zh-CN": z.string() });
const version = {
  contract_version: z.string(),
  ocsf_version: z.string(),
  catalog_revision: z.string(),
};
const eventClass = z.object({
  class_uid: z.int(),
  name: text,
  description: text,
  category: text,
  category_uid: z.int(),
  aliases: z.object({ en: z.string().array(), "zh-CN": z.string().array() }).optional(),
});
export type EventClassSummary = z.infer<typeof eventClass>;
const field = z.object({
  path: z.string(),
  name: text,
  description: text,
  logical_type: z.string(),
  json_type: z.string(),
  array: z.boolean(),
  requirement: z.enum(["required", "recommended", "optional"]),
  condition: text.optional(),
  owner: z.enum(["mapping", "platform", "derived"]),
  unit: z.string().optional(),
  object_fields: z.array(z.string()).optional(),
  required_output: z.array(z.string()).optional(),
  enum: z.array(z.object({ value: z.int(), name: text, description: text.partial().optional() })).optional(),
  integer_range: z.object({ min: z.int(), max: z.int() }).optional(),
  value_suggestions: z.array(z.object({ value: z.int(), label: z.string() })).optional(),
  preferred_enum_values: z.int().array().optional(),
});

export type EventField = z.infer<typeof field>;

export const eventClassDetails = eventClass.extend({
  ...version,
  fields: field.array(),
  recommended_mapping_paths: z.string().array().optional(),
  constraints: z
    .array(
      z.object({
        paths: z.string().array(),
        mode: z.enum(["at_least_one", "just_one"]),
        when: z.string().optional(),
        mapping_paths: z.string().array(),
      }),
    )
    .default([]),
});

export type EventClassDetails = z.infer<typeof eventClassDetails>;

export function listEventClasses(signal: AbortSignal, contractVersion?: string) {
  return request(
    { url: "/api/v1/ingest/event-schema", signal, params: { contract_version: contractVersion } },
    z.object({ ...version, classes: eventClass.array() }),
  );
}

export function getEventClass(uid: number, signal: AbortSignal, contractVersion?: string) {
  return request(
    { url: `/api/v1/ingest/event-schema/classes/${uid}`, signal, params: { contract_version: contractVersion } },
    eventClassDetails,
  );
}

const catalogField = field.extend({
  usages: z.array(z.object({ class_uid: z.int(), field })),
});

export type CatalogField = z.infer<typeof catalogField>;

export function listEventFields(signal: AbortSignal) {
  return request(
    { url: "/api/v1/ingest/event-schema/fields", signal },
    z.object({ ...version, fields: catalogField.array() }),
  );
}
