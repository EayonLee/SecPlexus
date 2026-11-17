import { http } from "../client";
import type { MappingBody } from "./schemas";
import { zMapping, zMappingBody, zMappingDetail, zSampleResult } from "./schemas";

// listMappingsTyped 读取解析映射列表；页面只需要名称与版本。
export async function listMappingsTyped() {
  const response = await http.get("/api/v1/ingest/mappings");

  return zMapping.array().parse(response.data.data);
}

// createMapping 新建解析映射；服务端会先编译，非法规则不会落库。
export async function createMapping(body: MappingBody) {
  const parsed = zMappingBody.parse(body);
  const response = await http.post("/api/v1/ingest/mappings", parsed);

  return zMappingDetail.parse(response.data.data);
}

// updateMapping 换规则内容；服务端会把版本自增。
export async function updateMapping(id: string, body: MappingBody) {
  const parsed = zMappingBody.parse(body);
  const response = await http.patch(`/api/v1/ingest/mappings/${id}`, parsed);

  return zMappingDetail.parse(response.data.data);
}

// deleteMapping 删除映射；仍被接入点引用时服务端返回 409。
export async function deleteMapping(id: string): Promise<void> {
  await http.delete(`/api/v1/ingest/mappings/${id}`);
}

// validateMapping 用真实样本试跑规则，保存前就能看到命中率。
export async function validateMapping(definition: unknown, samples: string[]) {
  const response = await http.post("/api/v1/ingest/mappings/validate", { definition, samples });

  return zSampleResult.parse(response.data.data);
}
