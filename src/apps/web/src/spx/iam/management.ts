"use client";
import { queryClient } from "./query";
import { refreshMe } from "./session";

/** managementChanged 刷新会话投影并统一失效所有可重取投影，包括未挂载的分页与关联目录。 */
export async function managementChanged() {
  await refreshMe();
  // IAM 变更会同时影响权限、用户、角色与审计，不能仅刷新当前表格页。
  await queryClient.invalidateQueries();
}
