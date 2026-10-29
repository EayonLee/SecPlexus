import { expect, test } from "vitest";

import {
  zCreateRoleBody,
  zPreviewPermissionBody,
  zSetMemberRolesBody,
  zSetRolePermissionsBody,
  zUpdateCurrentOrganizationBody,
  zUpdateOrganizationBody,
  zUpdateRoleBody,
} from "../../src/spx/api/iam/schemas";

const id = "09aa1d46-7c4b-493a-aef4-2e6e9b6c93ab";

test("create and update expose their actual required fields", () => {
  expect(zCreateRoleBody.safeParse({ name: "Review" }).success).toBe(false);
  expect(zCreateRoleBody.safeParse({ code: "review", name: "Review" }).success).toBe(true);
  expect(zUpdateRoleBody.safeParse({ name: "Review" }).success).toBe(false);
  expect(zUpdateRoleBody.safeParse({ name: "Review", status: "active", version: "1" }).success).toBe(true);
  expect(zUpdateRoleBody.safeParse({ name: "Review", status: "active", version: "1", code: "forbidden" }).success).toBe(
    false,
  );
  expect(zUpdateOrganizationBody.safeParse({ name: "Ops", authz_version: "1" }).success).toBe(false);
  expect(zUpdateCurrentOrganizationBody.safeParse({ name: "Ops", authz_version: "1" }).success).toBe(true);
  expect(
    zUpdateCurrentOrganizationBody.safeParse({ name: "Ops", authz_version: "1", status: "suspended" }).success,
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
  expect(zSetMemberRolesBody.safeParse({ role_ids, authz_version: "1" }).success).toBe(false);
  expect(zPreviewPermissionBody.safeParse({ membership_id: id, role_ids, authz_version: "1" }).success).toBe(false);
  expect(zSetRolePermissionsBody.safeParse({ permissions: [], version: "1" }).success).toBe(true);
  expect(zSetMemberRolesBody.safeParse({ role_ids: [], authz_version: "1" }).success).toBe(true);
});
