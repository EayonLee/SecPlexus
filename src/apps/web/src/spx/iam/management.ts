"use client";
import { queryClient } from "./query";
import { refreshIdentity, useIdentity } from "./session";

/** managementChanged 刷新身份并统一失效所有可重取投影，包括未挂载的分页与关联目录。 */
export async function managementChanged() {
  await refreshIdentity(useIdentity.getState().org_id);
  // IAM 变更会同时影响权限、成员、组织及审计，不能仅刷新当前表格页。
  await queryClient.invalidateQueries();
}
