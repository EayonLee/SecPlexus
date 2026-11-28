import { describe, expect, test } from "vitest";

import { permissions } from "../../src/auth/permissions.gen";
import { zCreateInputBody, zUpdateInputBody } from "../../src/spx/api/ingest/schemas";
import { ingestPages, settingsPages, visibleIngestPages } from "../../src/spx/iam/navigation";
import { inputEndpoint, inputTrialKey } from "../../src/spx/pages/ingest-model";
import { mixedDefinition, readEventValue } from "../../src/spx/pages/mixed-rule-model";
import { readRuleCatalog } from "../../src/spx/pages/rule-catalog";

// 导航是权限的第一道可见性：用户看不到自己无权访问的入口。
// 这里断言的是"配置与权限键的对应关系"，不是渲染结果。
describe("ingest navigation gating", () => {
  test("hides ingest entries without the read permissions", () => {
    const visible = visibleIngestPages({ permissions: [] });

    expect(visible.map((page) => page.id)).not.toContain("ingest");
    expect(visible.map((page) => page.id)).not.toContain("parsing-rules");
  });

  test("shows each entry only when its own permission is held", () => {
    const onlyInput = visibleIngestPages({ permissions: [permissions.IngestInputRead] });
    expect(onlyInput.map((page) => page.id)).toContain("ingest");
    expect(onlyInput.map((page) => page.id)).not.toContain("parsing-rules");

    const onlyMapping = visibleIngestPages({ permissions: [permissions.IngestParsingRuleRead] });
    expect(onlyMapping.map((page) => page.id)).toContain("parsing-rules");
    expect(onlyMapping.map((page) => page.id)).not.toContain("ingest");
  });

  test("binds each entry to a real permission key", () => {
    const ingest = ingestPages.find((page) => page.id === "ingest");
    const mappings = ingestPages.find((page) => page.id === "parsing-rules");

    expect(settingsPages.map((page) => page.path)).not.toContain("/settings/ingest");
    expect(ingest?.path).toBe("/ingest/inputs");
    expect(ingest?.action).toBe("ingest.input.read");
    expect(mappings?.action).toBe("ingest.parsing_rule.read");
  });
});

// 修改接入点的契约不接受 protocol：协议在创建后不可变。
// 前端在这里拒绝，比让服务端返回 400、再由界面翻译成人话要早一步。
describe("update input payload", () => {
  test("accepts only mutable fields", () => {
    expect(zUpdateInputBody.parse({ name: "edge" })).toEqual({ name: "edge" });
    expect(zUpdateInputBody.parse({ enabled: false })).toEqual({ enabled: false });
    expect(zUpdateInputBody.parse({ unbind_rule: true })).toEqual({ unbind_rule: true });
    expect(zUpdateInputBody.parse({ rotate_token: true })).toEqual({ rotate_token: true });
  });

  test("rejects protocol changes", () => {
    expect(() => zUpdateInputBody.parse({ protocol: "kafka" })).toThrow();
  });
});

test("requires connection settings for Kafka and Syslog inputs", () => {
  const name = "edge";
  expect(zCreateInputBody.safeParse({ name, protocol: "kafka", kafka_topic: "logs" }).success).toBe(false);
  expect(zCreateInputBody.safeParse({ name, protocol: "kafka", kafka_brokers: ["broker:9092"] }).success).toBe(false);
  expect(zCreateInputBody.safeParse({ name, protocol: "syslog" }).success).toBe(false);
  const kafka = { name, protocol: "kafka", kafka_brokers: ["broker:9092"], kafka_topic: "logs" };
  expect(zCreateInputBody.safeParse(kafka).success).toBe(false);
  expect(zCreateInputBody.safeParse({ ...kafka, kafka_start_position: "newest" }).success).toBe(false);
  expect(
    zCreateInputBody.safeParse({ ...kafka, kafka_start_position: "earliest" })
      .success,
  ).toBe(true);
});

describe("input list and preview", () => {
  test("requires a new sample run when the bound mapping version or sample changes", () => {
    const current = { id: "a", version: 2 };
    const saved = inputTrialKey(current, ["CEF"]);
    expect(inputTrialKey({ ...current }, ["CEF"])).toBe(saved);
    expect(inputTrialKey({ ...current, version: 3 }, ["CEF"])).not.toBe(saved);
    expect(inputTrialKey(current, ["updated"])).not.toBe(saved);
    expect(inputTrialKey(undefined, ["CEF"])).not.toBe(saved);
  });
});

test("catalog facets survive partial definitions and field previews follow actual OCSF paths", () => {
  const info = readRuleCatalog({
    vendor: " Acme ",
    product: "Edge",
    description: "Traffic",
    parse: { mode: "json" },
    field_map: { src: "src_endpoint.ip" },
  });
  expect(info).toMatchObject({ description: "Traffic", mode: "json" });
  expect(readRuleCatalog(null).description).toBe("");
  expect(readRuleCatalog({ parse: { mode: "legacy" } }).mode).toBeUndefined();
  expect(readEventValue({ src_endpoint: { ip: "192.0.2.5" } }, "src_endpoint.ip")).toBe("192.0.2.5");
  expect(readEventValue({ activity_id: 0 }, "activity_id")).toBe("0");
  expect(readEventValue({ value: "" }, "value")).toBe("");
  expect(readEventValue({ value: false }, "value")).toBe("false");
  expect(readEventValue({}, "constructor.name")).toBe("—");
});

test.each([{ match: undefined }, { match: null }, { match: [] }])("accepts unconditional API rules with $match", ({ match }) => {
  const definition = mixedDefinition.parse({
    contract_version: "v1",
    parse: { mode: "delimiter", delimiter: { separator: "|" } },
    branches: [{ id: "network", name: "Network", class_uid: 4001, field_map: { field_1: "src_endpoint.ip" }, match }],
  });
  expect(definition.parse.mode).toBe("delimiter");
  expect(definition.branches[0].class_uid).toBe(4001);
  expect(definition.branches[0].match).toEqual(match);
});

test("a Syslog input only exposes a copyable endpoint when it has a port", () => {
  const input = { id: "edge", protocol: "syslog", kafka_topic: null, syslog_port: null } as const;
  expect(inputEndpoint(input)).toBeNull();
  expect(inputEndpoint({ ...input, syslog_port: 5514 })).toBe("UDP/TCP :5514");
  expect(inputEndpoint({ ...input, protocol: "kafka" })).toBeNull();
});
