import { describe, expect, test } from "vitest";

import { permissions } from "../../src/auth/permissions.gen";
import { zUpdateInputBody } from "../../src/spx/api/ingest/schemas";
import { settingsPages, visibleSettingsPages } from "../../src/spx/iam/navigation";

// 导航是权限的第一道可见性：用户看不到自己无权访问的入口。
// 这里断言的是"配置与权限键的对应关系"，不是渲染结果。
describe("ingest navigation gating", () => {
  test("hides ingest entries without the read permissions", () => {
    const visible = visibleSettingsPages({ permissions: [] });

    expect(visible.map((page) => page.id)).not.toContain("ingest");
    expect(visible.map((page) => page.id)).not.toContain("mappings");
  });

  test("shows each entry only when its own permission is held", () => {
    const onlyInput = visibleSettingsPages({ permissions: [permissions.IngestInputRead] });
    expect(onlyInput.map((page) => page.id)).toContain("ingest");
    expect(onlyInput.map((page) => page.id)).not.toContain("mappings");

    const onlyMapping = visibleSettingsPages({ permissions: [permissions.IngestMappingRead] });
    expect(onlyMapping.map((page) => page.id)).toContain("mappings");
    expect(onlyMapping.map((page) => page.id)).not.toContain("ingest");
  });

  test("binds each entry to a real permission key", () => {
    const ingest = settingsPages.find((page) => page.id === "ingest");
    const mappings = settingsPages.find((page) => page.id === "mappings");

    expect(ingest?.action).toBe("ingest.input.read");
    expect(mappings?.action).toBe("ingest.mapping.read");
  });
});

// 修改接入点的契约不接受 protocol：协议在创建后不可变。
// 前端在这里拒绝，比让服务端返回 400、再由界面翻译成人话要早一步。
describe("update input payload", () => {
  test("accepts only mutable fields", () => {
    expect(zUpdateInputBody.parse({ name: "edge" })).toEqual({ name: "edge" });
    expect(zUpdateInputBody.parse({ enabled: false })).toEqual({ enabled: false });
    expect(zUpdateInputBody.parse({ mapping_id: null })).toEqual({ mapping_id: null });
  });

  test("rejects protocol changes", () => {
    expect(() => zUpdateInputBody.parse({ protocol: "kafka" })).toThrow();
  });
});
