import { request } from "../client";
import type { MappingBody } from "./schemas";
import { zMapping, zMappingBody, zMappingDetail, zSampleResult } from "./schemas";

// listMappings 读取解析映射列表；页面只需要名称与版本。
export const listMappings = () => request({ url: "/api/v1/ingest/mappings" }, zMapping.array());

// createMapping 新建解析映射；服务端会先编译，非法规则不会落库。
export async function createMapping(body: MappingBody) {
  return request({ method: "POST", url: "/api/v1/ingest/mappings", data: zMappingBody.parse(body) }, zMappingDetail, 201);
}

// updateMapping 换规则内容；服务端会把版本自增。
export async function updateMapping(id: string, body: MappingBody) {
  return request({ method: "PATCH", url: `/api/v1/ingest/mappings/${encodeURIComponent(id)}`, data: zMappingBody.parse(body) }, zMappingDetail);
}

// deleteMapping 删除映射；仍被接入点引用时服务端返回 409。
export async function deleteMapping(id: string): Promise<void> {
  await request({ method: "DELETE", url: `/api/v1/ingest/mappings/${encodeURIComponent(id)}` }, zMapping);
}

// validateMapping 用真实样本试跑规则，保存前就能看到命中率。
export async function validateMapping(definition: unknown, samples: string[]) {
  return request({ method: "POST", url: "/api/v1/ingest/mappings/validate", data: { definition, samples } }, zSampleResult);
}
