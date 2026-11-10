import { describe, expect, test } from "vitest";

import type { ApiMe } from "../../src/spx/api/iam/schemas";
import { settingsPageFor, visibleSettingsPages } from "../../src/spx/iam/navigation";

const identity: ApiMe = {
  principal: {
    user_id: "u",
    session_id: "s",
    auth_epoch: "1",
    username: "member",
    display_name: "Member",
    configured: false,
    authenticated_at: "2026-10-28T00:00:00Z",
  },
  roles: [],
  permissions: ["iam.users.read"],
};

describe("settings area navigation", () => {
  test("every flat read permission exposes exactly its own settings page", () => {
    expect(visibleSettingsPages(identity).map((page) => page.id)).toEqual(["users"]);
    expect(visibleSettingsPages({ ...identity, permissions: ["iam.roles.read"] }).map((page) => page.id)).toEqual([
      "roles",
    ]);
    expect(visibleSettingsPages({ ...identity, permissions: ["iam.audit.read"] }).map((page) => page.id)).toEqual([
      "audit",
    ]);
    // 只有写入权限、没有读取权限时不显示任何管理页：菜单跟随可读能力。
    expect(visibleSettingsPages({ ...identity, permissions: ["iam.users.manage"] })).toEqual([]);
    expect(visibleSettingsPages({ ...identity, permissions: [] })).toEqual([]);
  });

  test("page resolution is flat: every settings page maps to one path", () => {
    expect(settingsPageFor("/settings/roles")?.id).toBe("roles");
    expect(settingsPageFor("/settings/users")?.id).toBe("users");
    expect(settingsPageFor("/settings/audit")?.id).toBe("audit");
    expect(settingsPageFor("/settings")).toBeUndefined();
    expect(settingsPageFor("/settings/platform/organizations")).toBeUndefined();
    expect(settingsPageFor("/settings/orgs/one/audit")).toBeUndefined();
  });

  test("audit is a single entry, not split by scope", () => {
    const ids = visibleSettingsPages({
      ...identity,
      permissions: ["iam.users.read", "iam.roles.read", "iam.audit.read"],
    }).map((page) => page.id);

    expect(ids.filter((id) => id === "audit")).toHaveLength(1);
    expect(new Set(ids).size).toBe(ids.length);
  });
});
