import { describe, expect, test } from "vitest";

import type { ApiMe } from "../../src/spx/api/iam/schemas";
import { organizationFromPath, visibleManagementPages, visibleManagementSections } from "../../src/spx/iam/navigation";

const identity: ApiMe = {
  principal: {
    user_id: "u",
    session_id: "s",
    auth_version: "1",
    username: "member",
    display_name: "Member",
    is_super_admin: false,
    authenticated_at: "2026-10-28T00:00:00Z",
  },
  organizations: [],
  org_id: "one",
  capabilities: ["iam.members.read"],
  platform_capabilities: [],
};

describe("organization and management navigation", () => {
  test("an explicit organization in the URL owns the tab context", () => {
    expect(organizationFromPath("/management/orgs/two/members", "?org_id=one")).toBe("two");
    expect(organizationFromPath("/dashboard", "?org_id=one")).toBe("one");
    expect(organizationFromPath("/settings/account", "")).toBeUndefined();
  });
  test("organization actions do not grant platform navigation", () => {
    expect(visibleManagementPages(identity).map((page) => page.id)).toEqual(["members"]);
    expect(visibleManagementPages({ ...identity, org_id: undefined })).toEqual([]);
    expect(
      visibleManagementPages({ ...identity, capabilities: [], platform_capabilities: ["iam.platform.users.read"] }).map(
        (page) => page.id,
      ),
    ).toEqual(["users"]);
  });
});

test("management consolidates tasks without granting inaccessible tabs", () => {
  const admin = {
    ...identity,
    capabilities: ["iam.members.read", "iam.roles.read", "iam.organizations.read", "iam.audit.read"],
    platform_capabilities: [
      "iam.platform.users.read",
      "iam.platform.organizations.read",
      "iam.platform.audit.read",
      "iam.platform.security.read",
    ],
  };
  expect(visibleManagementSections(admin).map((section) => section.id)).toEqual(["people", "organization", "activity"]);
  expect(visibleManagementPages(admin).map((page) => page.id)).not.toContain("security");
  expect(visibleManagementSections(identity).map((section) => section.id)).toEqual(["people"]);
  expect(visibleManagementSections({ ...identity, org_id: undefined })).toEqual([]);
});
