import { z } from "zod";

import { pageSchema, request } from "../client";
import type { CreateInputBody, CreateInputResult, UpdateInputBody } from "./schemas";
import { zCreateInputBody, zCreateInputResult, zInput, zUpdateInputBody } from "./schemas";

// listInputPage 读取过滤后的接入点页，不获取整张表。
export function listInputPage(
  params: {
    limit: number;
    offset?: number;
    q?: string;
    protocol?: string;
    status?: string;
    rule?: string;
    name?: string;
    syslog_port?: number;
  },
  signal?: AbortSignal,
) {
  return request({ url: "/api/v1/ingest/inputs", params, signal }, pageSchema(zInput));
}

export const getInput = (id: string, signal?: AbortSignal) =>
  request({ url: `/api/v1/ingest/inputs/${encodeURIComponent(id)}`, signal }, zInput);

export const mappingInputCounts = (ids: string[], signal?: AbortSignal) =>
  request(
    {
      url: "/api/v1/ingest/inputs/counts-by-rule",
      params: { id: ids },
      paramsSerializer: { indexes: null },
      signal,
    },
    z.record(z.string(), z.int().nonnegative()),
  );

// createInput 新建接入点；响应里可能带一次性明文令牌。
export async function createInput(body: CreateInputBody): Promise<CreateInputResult> {
  return request(
    { method: "POST", url: "/api/v1/ingest/inputs", data: zCreateInputBody.parse(body) },
    zCreateInputResult,
    201,
  );
}

// updateInput 修改接入点；只提交要改的字段，其余保持原值。
export async function updateInput(id: string, body: UpdateInputBody): Promise<CreateInputResult> {
  return request(
    { method: "PATCH", url: `/api/v1/ingest/inputs/${encodeURIComponent(id)}`, data: zUpdateInputBody.parse(body) },
    zCreateInputResult,
  );
}

// deleteInput 删除接入点。
export async function deleteInput(id: string): Promise<void> {
  await request({ method: "DELETE", url: `/api/v1/ingest/inputs/${encodeURIComponent(id)}` }, z.undefined(), 204);
}
