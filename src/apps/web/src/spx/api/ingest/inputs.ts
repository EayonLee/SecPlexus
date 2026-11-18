import { request } from "../client";
import type { CreateInputBody, CreateInputResult, UpdateInputBody } from "./schemas";
import { zCreateInputBody, zCreateInputResult, zInput, zUpdateInputBody } from "./schemas";

// listInputs 读取接入点列表。
export const listInputs = () => request({ url: "/api/v1/ingest/inputs" }, zInput.array());

// createInput 新建接入点；响应里可能带一次性明文令牌。
export async function createInput(body: CreateInputBody): Promise<CreateInputResult> {
  return request({ method: "POST", url: "/api/v1/ingest/inputs", data: zCreateInputBody.parse(body) }, zCreateInputResult, 201);
}

// updateInput 修改接入点；只提交要改的字段，其余保持原值。
export async function updateInput(id: string, body: UpdateInputBody): Promise<void> {
  await request({ method: "PATCH", url: `/api/v1/ingest/inputs/${encodeURIComponent(id)}`, data: zUpdateInputBody.parse(body) }, zInput);
}

// deleteInput 删除接入点。
export async function deleteInput(id: string): Promise<void> {
  await request({ method: "DELETE", url: `/api/v1/ingest/inputs/${encodeURIComponent(id)}` }, zInput);
}
