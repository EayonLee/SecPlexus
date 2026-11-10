import { expect, test } from "vitest";

import {
  zCreateRoleBody,
  zCreateUserBody,
  zEmail,
  zSetRolePermissionsBody,
  zSetUserRolesBody,
  zUpdateRoleBody,
  zUpdateUserBody,
} from "../../src/spx/api/iam/schemas";

const id = "09aa1d46-7c4b-493a-aef4-2e6e9b6c93ab";

test("create and update expose their actual required fields", () => {
  expect(zCreateRoleBody.safeParse({ name: "Review" }).success).toBe(false);
  expect(zCreateRoleBody.safeParse({ name: "Review", permissions: [] }).success).toBe(true);
  // 角色标识已移除：再提交 code 会因 strict 被拒绝。
  expect(zCreateRoleBody.safeParse({ name: "Review", code: "review", permissions: [] }).success).toBe(false);
  expect(zUpdateRoleBody.safeParse({ name: "Review" }).success).toBe(true);
  // 角色权限由独立端点维护，且并发版本已随扁平授权移除。
  expect(zUpdateRoleBody.safeParse({ name: "Review", version: "1" }).success).toBe(false);
  expect(zUpdateRoleBody.safeParse({ name: "Review", permissions: [] }).success).toBe(false);
  expect(zUpdateUserBody.safeParse({ display_name: "Ops", status: "active" }).success).toBe(true);
  expect(zUpdateUserBody.safeParse({ display_name: "Ops", status: "suspended" }).success).toBe(false);
  // 用户状态更新不再携带身份或组织并发版本。
  expect(zUpdateUserBody.safeParse({ display_name: "Ops", status: "active", auth_epoch: "1" }).success).toBe(false);
  expect(zUpdateUserBody.safeParse({ display_name: "Ops", status: "active", org_id: id }).success).toBe(false);
});

test("create user requires the account fields and keeps email optional", () => {
  const base = { username: "analyst", display_name: "Analyst", password: "123456789012" };
  expect(zCreateUserBody.safeParse(base).success).toBe(true);
  expect(zCreateUserBody.safeParse({ ...base, email: "" }).success).toBe(true);
  expect(zCreateUserBody.safeParse({ ...base, email: "analyst@example.com" }).success).toBe(true);
  expect(zCreateUserBody.safeParse({ ...base, email: "not-an-address" }).success).toBe(false);
  // 组织选择已删除：账号创建不再接受 org_id 或预置角色。
  expect(zCreateUserBody.safeParse({ ...base, org_id: id }).success).toBe(false);
  expect(zCreateUserBody.safeParse({ ...base, role_ids: [id] }).success).toBe(false);
  expect(zCreateUserBody.safeParse({ ...base, password: "12345678901" }).success).toBe(false);
  expect(zCreateUserBody.safeParse(base).success && zCreateUserBody.parse(base).password).toBe("123456789012");
});

test("role permission and user role sets reject duplicates and oversized submissions", () => {
  const permissions = ["iam.audit.read", "iam.audit.read"];
  expect(zSetRolePermissionsBody.safeParse({ permissions, version: "1" }).success).toBe(false);
  expect(zSetRolePermissionsBody.safeParse({ permissions }).success).toBe(false);
  expect(zSetRolePermissionsBody.safeParse({ permissions: [] }).success).toBe(true);
  const role_ids = Array.from(
    { length: 51 },
    (_, index) => `09aa1d46-7c4b-493a-aef4-${String(index).padStart(12, "0")}`,
  );
  expect(zSetUserRolesBody.safeParse({ role_ids, authz_epoch: "1" }).success).toBe(false);
  expect(zSetUserRolesBody.safeParse({ role_ids }).success).toBe(false);
  expect(zSetUserRolesBody.safeParse({ role_ids: [id, id] }).success).toBe(false);
  expect(zSetUserRolesBody.safeParse({ role_ids: [] }).success).toBe(true);
});

test("email rules accept the same local-part characters as the identity package", () => {
  for (const value of ["ana_lyst@example.com", "analyst+tag@example.com", "a.b@mail.example.co"]) {
    expect(zEmail.safeParse(value).success, value).toBe(true);
  }
  expect(zEmail.safeParse("a@example.com, b@example.com").success).toBe(false);
});
