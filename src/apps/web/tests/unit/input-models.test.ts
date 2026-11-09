import { expect, test } from "vitest";

import {
  zCreateOrganizationBody,
  zCreateRoleBody,
  zPreviewPermissionBody,
  zSetOrganizationUserRolesBody,
  zSetRolePermissionsBody,
  zUpdateOrganizationBody,
  zUpdateRoleBody,
} from "../../src/spx/api/iam/schemas";

const id = "09aa1d46-7c4b-493a-aef4-2e6e9b6c93ab";

test("create and update expose their actual required fields", () => {
  expect(zCreateRoleBody.safeParse({ name: "Review" }).success).toBe(false);
  expect(zCreateRoleBody.safeParse({ name: "Review", permissions: [] }).success).toBe(true);
  // 角色标识已移除：再提交 code 会因 strict 被拒绝。
  expect(zCreateRoleBody.safeParse({ name: "Review", code: "review", permissions: [] }).success).toBe(false);
  expect(zUpdateRoleBody.safeParse({ name: "Review" }).success).toBe(false);
  expect(zUpdateRoleBody.safeParse({ name: "Review", version: "1" }).success).toBe(true);
  expect(zUpdateRoleBody.safeParse({ name: "Review", version: "1", code: "forbidden" }).success).toBe(false);
  expect(zUpdateOrganizationBody.safeParse({ name: "Ops", authz_epoch: "1" }).success).toBe(true);
  expect(
    zUpdateOrganizationBody.safeParse({ name: "Ops", authz_epoch: "1", status: "suspended" }).success,
  ).toBe(false);
});

test("preview and save reject duplicate permissions and oversized role sets", () => {
  const permissions = ["iam.audit.read", "iam.audit.read"];
  expect(zSetRolePermissionsBody.safeParse({ permissions, version: "1" }).success).toBe(false);
  expect(zPreviewPermissionBody.safeParse({ role_id: id, permissions, version: "1" }).success).toBe(false);
  const role_ids = Array.from(
    { length: 51 },
    (_, index) => `09aa1d46-7c4b-493a-aef4-${String(index).padStart(12, "0")}`,
  );
  expect(zSetOrganizationUserRolesBody.safeParse({ role_ids, authz_epoch: "1" }).success).toBe(false);
  expect(zPreviewPermissionBody.safeParse({ membership_id: id, role_ids, authz_epoch: "1" }).success).toBe(false);
  expect(zSetRolePermissionsBody.safeParse({ permissions: [], version: "1" }).success).toBe(true);
  expect(zSetOrganizationUserRolesBody.safeParse({ role_ids: [], authz_epoch: "1" }).success).toBe(true);
});

test("organization names and identifiers have separate Unicode-aware rules", () => {
  const body = { name: "  安全运营中心  ", slug: "soc_team-01", admin_user_id: id };
  expect(zCreateOrganizationBody.parse(body).name).toBe("安全运营中心");
  expect(zCreateOrganizationBody.safeParse({ ...body, name: "😀".repeat(30) }).success).toBe(true);
  for (const name of ["中".repeat(31), "Ops\nTeam", "Ops\u0085Team", " "]) {
    expect(zCreateOrganizationBody.safeParse({ ...body, name }).success).toBe(false);
    expect(zUpdateOrganizationBody.safeParse({ name, authz_epoch: "1" }).success).toBe(false);
  }
  for (const slug of ["a".repeat(31), "1soc", "soc_", "SOC", "ab"]) {
    expect(zCreateOrganizationBody.safeParse({ ...body, slug }).success).toBe(false);
  }
});
