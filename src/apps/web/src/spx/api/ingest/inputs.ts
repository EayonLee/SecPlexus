import { http } from "../client";
import type { CreateInputBody, CreateInputResult, UpdateInputBody } from "./schemas";
import { zCreateInputBody, zCreateInputResult, zInput, zUpdateInputBody } from "./schemas";

// listInputs 读取接入点列表。
export async function listInputs() {
  const response = await http.get("/api/v1/ingest/inputs");

  return response.data.data as unknown;
}

// createInput 新建接入点；返回体里可能带一次性明文令牌。
export async function createInput(body: CreateInputBody): Promise<CreateInputResult> {
  const parsed = zCreateInputBody.parse(body);
  const response = await http.post("/api/v1/ingest/inputs", parsed);

  return zCreateInputResult.parse(response.data.data);
}

// updateInput 修改接入点；只提交要改的字段，其余保持原值。
export async function updateInput(id: string, body: UpdateInputBody): Promise<void> {
  const parsed = zUpdateInputBody.parse(body);
  await http.patch(`/api/v1/ingest/inputs/${id}`, parsed);
}

// deleteInput 删除接入点。
export async function deleteInput(id: string): Promise<void> {
  await http.delete(`/api/v1/ingest/inputs/${id}`);
}

// listInputsTyped 用 zod 校验列表响应，避免把形状变化带进页面。
export async function listInputsTyped() {
  const response = await http.get("/api/v1/ingest/inputs");

  return zInput.array().parse(response.data.data);
}
