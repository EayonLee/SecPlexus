import type { ApiInput, ApiMapping } from "../api/ingest/schemas";

export function inputTrialKey(mapping: Pick<ApiMapping, "id" | "version"> | undefined, samples: string[]) {
  return JSON.stringify([mapping?.id, mapping?.version, samples]);
}

export function inputEndpoint(input: Pick<ApiInput, "protocol" | "id" | "syslog_port" | "kafka_topic">) {
  if (input.protocol === "http") return `/api/v1/ingest/events/${input.id}`;
  if (input.protocol === "syslog") return input.syslog_port ? `UDP/TCP :${input.syslog_port}` : null;
  return input.kafka_topic || null;
}
