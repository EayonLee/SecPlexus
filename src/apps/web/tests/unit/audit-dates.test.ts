import { expect, test } from "vitest";

import { auditDateBounds } from "../../src/spx/api/iam/audit-dates";

test("audit dates bound the viewed day across daylight saving", () => {
  expect(auditDateBounds("2026-03-08", "2026-03-08", "America/New_York")).toEqual({
    from: "2026-03-08T05:00:00.000Z",
    to: "2026-03-09T04:00:00.000Z",
  });
  expect(auditDateBounds("2026-03-08", undefined, "Asia/Shanghai")).toEqual({
    from: "2026-03-07T16:00:00.000Z",
    to: undefined,
  });
});
