import { describe, expect, test } from "vitest";

import type { ApiMe } from "../../src/spx/api/iam/schemas";
import { organizationFromPath, visibleManagementPages } from "../../src/spx/iam/navigation";

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
    expect(organizationFromPath("/account/security", "")).toBeUndefined();
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
