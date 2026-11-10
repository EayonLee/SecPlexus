import { expect, test } from "vitest";

import { pageSchema } from "../../src/spx/api/client";
import { zApiAuditEvent, zApiPrincipal, zApiRole } from "../../src/spx/api/iam/schemas";

test("date-time validation accepts RFC 3339 offsets returned by PostgreSQL", () => {
  const principal = {
    user_id: "user",
    session_id: "session",
    username: "member",
    display_name: "Member",
    configured: false,
    auth_epoch: "1",
    authenticated_at: "2026-10-29T08:30:00+08:00",
  };
  expect(zApiPrincipal.safeParse(principal).success).toBe(true);
  expect(zApiPrincipal.safeParse({ ...principal, authenticated_at: "2026-10-29T08:30:00" }).success).toBe(false);
});

test("role response validation preserves the builtin flag and user count", () => {
  const roles = pageSchema(zApiRole);
  const role = {
    id: "role",
    name: "Member",
    description: "",
    builtin: false,
    permissions: ["iam.users.read"],
    user_count: 3,
  };
  expect(roles.parse({ items: [role], total: 1, limit: 20, offset: 0 }).items[0].user_count).toBe(3);
  // 内置标记是接口数据，不能缺省：缺省会让前端无法决定只读与不可删除。
  const { builtin: _builtin, ...missingBuiltin } = role;
  expect(zApiRole.safeParse(missingBuiltin).success).toBe(false);
  // 已删除的并发版本与派生操作集合不再进入模型。
  const parsed = zApiRole.parse({ ...role, version: "1", allowed_actions: ["iam.roles.read"] });
  expect("version" in parsed).toBe(false);
  expect("allowed_actions" in parsed).toBe(false);
  const { user_count: _count, ...missingCount } = role;
  expect(zApiRole.safeParse(missingCount).success).toBe(false);
});

test("audit events keep occurred_at and expose no organization column", () => {
  const event = {
    id: "event",
    occurred_at: "2026-10-29T08:30:00+08:00",
    actor_type: "user",
    actor_id: "user",
    actor_name: "Member",
    action: "iam.user.created",
    target_type: "user",
    target_id: "target",
    outcome: "success",
    request_id: "request",
    metadata: {},
  };
  const parsed = zApiAuditEvent.parse({ ...event, org_id: "org" });
  expect("org_id" in parsed).toBe(false);
  const { occurred_at: _time, ...missingTime } = event;
  expect(zApiAuditEvent.safeParse(missingTime).success).toBe(false);
});
