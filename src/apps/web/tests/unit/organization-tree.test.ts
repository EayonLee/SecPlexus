import { describe, expect, test } from "vitest";

import type { ApiOrganization } from "../../src/spx/api/iam/schemas";
import { ancestorIDs, descendantIDs } from "../../src/spx/components/organization-tree";
import { settingsPages } from "../../src/spx/iam/navigation";

describe("organization tree helpers", () => {
  test("ancestorIDs returns the root-to-parent chain", () => {
    const items = fixture();
    expect(ancestorIDs(items, "c")).toEqual(["a", "b"]);
    expect(ancestorIDs(items, "a")).toEqual([]);
  });

  test("descendantIDs includes the node itself and every descendant", () => {
    const items = fixture();
    expect([...descendantIDs(items, "b")].sort()).toEqual(["b", "c"]);
    expect([...descendantIDs(items, "a")].sort()).toEqual(["a", "b", "c"]);
    expect([...descendantIDs(items, "c")].sort()).toEqual(["c"]);
  });

  test("descendantIDs on an unknown id still blocks itself", () => {
    expect([...descendantIDs(fixture(), "missing")]).toEqual(["missing"]);
  });
});

describe("management navigation order", () => {
  test("settings pages are a flat ordered list", () => {
    expect(settingsPages.map((page) => page.id)).toEqual(["roles", "organizations", "users", "audit"]);
  });

  test("no page exposes a group, since tabs were removed", () => {
    expect(settingsPages.every((page) => !("group" in page))).toBe(true);
  });
});

function fixture(): ApiOrganization[] {
  const base = { authz_epoch: "1", depth: 0, slug: "s" };

  return [
    { ...base, id: "a", name: "A", parent_id: null },
    { ...base, depth: 1, id: "b", name: "B", parent_id: "a" },
    { ...base, depth: 2, id: "c", name: "C", parent_id: "b" },
  ];
}
