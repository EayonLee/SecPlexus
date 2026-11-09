import { describe, expect, test } from "vitest";

import type { ApiMe } from "../../src/spx/api/iam/schemas";
import { organizationFromPath, settingsPageFor, visibleSettingsPages } from "../../src/spx/iam/navigation";

const identity: ApiMe = {
  principal: {
    user_id: "u",
    session_id: "s",
    auth_epoch: "1",
    username: "member",
    display_name: "Member",
    is_super_admin: false,
    authenticated_at: "2026-10-28T00:00:00Z",
  },
  organizations: [],
  org_id: "one",
  capabilities: ["iam.users.read"],
  platform_capabilities: [],
};

describe("settings area navigation", () => {
  test("an explicit organization in the settings URL wins over the query fallback", () => {
    expect(organizationFromPath("/settings/orgs/two/users", "?org_id=one")).toBe("two");
    expect(organizationFromPath("/dashboard", "?org_id=one")).toBe("one");
    expect(organizationFromPath("/settings", "")).toBeUndefined();
  });

  test("organization actions do not grant platform pages", () => {
    // 组织成员与平台成员已合并为「用户管理」一页，组织级权限即可见。
    expect(visibleSettingsPages(identity).map((page) => page.id)).toEqual(["users"]);
    expect(visibleSettingsPages({ ...identity, org_id: undefined })).toEqual([]);
    expect(visibleSettingsPages({ ...identity, capabilities: [], platform_capabilities: ["iam.platform.users.read"] })).toEqual([]);
  });

  test("page resolution is flat: every settings page maps to one path", () => {
    expect(settingsPageFor("/settings/orgs/one/roles", "one")?.id).toBe("roles");
    expect(settingsPageFor("/settings/orgs/one/audit", "one")?.id).toBe("audit");
    expect(settingsPageFor("/settings/platform/organizations", "one")?.id).toBe("organizations");
    expect(settingsPageFor("/settings", "one")).toBeUndefined();
    expect(settingsPageFor("/management/orgs/one/roles", "one")).toBeUndefined();
  });

  test("audit is a single entry, not split by scope", () => {
    const ids = visibleSettingsPages({
      ...identity,
      capabilities: ["iam.audit.read"],
      platform_capabilities: ["iam.platform.audit.read"],
    }).map((page) => page.id);

    expect(ids.filter((id) => id === "audit")).toHaveLength(1);
  });
});
