import { z } from "zod";

import type { EventField } from "@/spx/api/ingest/event-schema";

export const parserModes = ["json", "cef", "kv", "regex", "delimiter"] as const;
const text = z.string().trim().catch("");
const metadata = z.object({
  description: text,
  parse: z
    .object({ mode: z.enum(parserModes).optional().catch(undefined) })
    .optional()
    .catch(undefined),
});

export function readRuleCatalog(definition: unknown) {
  const item = metadata.safeParse(definition).data;
  return {
    description: item?.description ?? "",
    mode: item?.parse?.mode,
  };
}

export function mappingFieldType(field: EventField) {
  const kind = field.array ? "array" : field.json_type === "integer" ? "number" : field.json_type;
  const type =
    field.logical_type === "ip_t" ? "IP" : field.logical_type === "timestamp_t" ? "timestamp" : field.json_type;
  return { kind, label: field.array ? `${type}[]` : type };
}
