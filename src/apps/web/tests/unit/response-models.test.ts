import { expect, test } from "vitest";

import { pageSchema } from "../../src/spx/api/client";
import { zApiPrincipal, zApiRole } from "../../src/spx/api/iam/schemas";

test("date-time validation accepts RFC 3339 offsets returned by PostgreSQL", () => {
  const principal = {
    user_id: "user",
    session_id: "session",
    username: "member",
    display_name: "Member",
    is_super_admin: false,
    auth_version: "1",
    authenticated_at: "2026-10-29T08:30:00+08:00",
  };
  expect(zApiPrincipal.safeParse(principal).success).toBe(true);
  expect(zApiPrincipal.safeParse({ ...principal, authenticated_at: "2026-10-29T08:30:00" }).success).toBe(false);
});

test("role response validation preserves numeric counts and string versions", () => {
  const roles = pageSchema(zApiRole);
  expect(() =>
    roles.parse({
      items: [
        {
          id: "role",
          code: "member",
          name: "Member",
          description: "",
          kind: "system",
          status: "active",
          version: "1",
          permissions: [],
          member_count: 0,
          allowed_actions: ["iam.roles.read"],
        },
      ],
      total: 1,
      limit: 20,
      offset: 0,
    }),
  ).not.toThrow();
});
