import { expect, test } from "vitest";

import cases from "../../../../../docs/plans/ingest-rule-validation/validation-cases.json";
import catalog from "../../../ingest/schema/events/catalog.json";
import { eventClassDetails } from "../../src/spx/api/ingest/event-schema";
import { isJSONLogSample } from "../../src/spx/api/ingest/rule-validation";
import { zMappingBody } from "../../src/spx/api/ingest/schemas";
import {
  duplicateMappingTargets,
  mappingPresets,
  missingMappingGroups,
  sharedMapping,
} from "../../src/spx/pages/rule-mapping-rows";

test.each([
  { targets: ["", ""], duplicates: [] },
  { targets: ["message", "action_id", "message"], duplicates: ["message"] },
  { targets: ["time", "severity_id", "severity_id", "time", "time"], duplicates: ["severity_id", "time"] },
])("finds duplicate standard targets in $targets", ({ targets, duplicates }) => {
  expect(duplicateMappingTargets(targets.map((target) => ({ target })))).toEqual(new Set(duplicates));
});

test.each([
  ['{\n  "source": "demo"\n}', true],
  ['{"nested":{"valid":[1,null]}}', true],
  ['{"source":}', false],
  ['{"a":1}\n{"b":2}', false],
  ['[{"source":"demo"}]', false],
  ['"demo"', false],
  ["null", false],
])("checks whether %s is one JSON object", (sample, valid) => {
  expect(isJSONLogSample(sample)).toBe(valid);
});

test.each(cases)("parses rule metadata $field with $code", ({ field, value, repeat, normalized, code }) => {
  const input = value.repeat(repeat ?? 1);
  const path = field === "description" ? ["definition", "description"] : [field];
  const payload = {
    name: "Example rule",
    vendor: "Test",
    product: "Edge",
    definition: field === "description" ? { description: input } : { description: "Network mapping" },
    ...(field === "description" ? {} : { [field]: input }),
  };
  const result = zMappingBody.safeParse(payload);
  if (code) {
    expect(result.error?.issues).toEqual(expect.arrayContaining([expect.objectContaining({ path, message: code })]));
    return;
  }
  expect(result.data).toMatchObject(
    field === "description" ? { definition: { description: normalized ?? input } } : { [field]: normalized ?? input },
  );
});

test("uses the generated class requirements and keeps conditions scoped to mapped objects", () => {
  const network = eventClassDetails.parse({ ...catalog, ...catalog.classes.find((item) => item.class_uid === 4001) });
  const http = eventClassDetails.parse({ ...catalog, ...catalog.classes.find((item) => item.class_uid === 4002) });
  const mapped = (target: string) => ({ id: target, source: "sample", target, fixed: false, conversion: "" });
  expect(missingMappingGroups([mapped("src_endpoint.port")], network.constraints)).toHaveLength(1);
  expect(missingMappingGroups([mapped("src_endpoint.ip")], network.constraints)).toEqual([]);
  expect(
    missingMappingGroups([mapped("http_request.url.hostname")], http.constraints).map(({ paths }) => paths),
  ).toContainEqual(["http_request.url.path", "http_request.url.url_string"]);
  expect(missingMappingGroups([mapped("http_request.url.path")], http.constraints)).toEqual([]);
  expect(
    missingMappingGroups([mapped("http_response.length")], http.constraints).map(({ paths }) => paths),
  ).toContainEqual(["http_response.code"]);
  expect(missingMappingGroups([mapped("http_response.code")], http.constraints)).toEqual([]);
});

test.each(catalog.classes.map(({ class_uid }) => class_uid))(
  "presets the core and removable fields for class %d",
  (uid) => {
    const detail = eventClassDetails.parse({ ...catalog, ...catalog.classes.find((item) => item.class_uid === uid) });
    const presets = mappingPresets(detail.fields, [], detail.constraints, detail.recommended_mapping_paths ?? []);
    for (const path of ["time", "activity_id", "severity_id"]) expect(presets.core.has(path)).toBe(true);
    expect(presets.core.has("metadata.uid")).toBe(false);
    expect(presets.recommended).toEqual(expect.arrayContaining(["message", "status_id"]));
    for (const group of detail.constraints.filter(({ when }) => !when)) {
      expect(group.mapping_paths.some((path) => presets.core.has(path))).toBe(true);
    }
  },
);

test("keeps executable file requirements conditional on the file object", () => {
  const detail = eventClassDetails.parse({ ...catalog, ...catalog.classes.find((item) => item.class_uid === 1007) });
  const configured = [{ id: "image", source: "image", target: "process.file.path", fixed: false, conversion: "" }];
  const idle = mappingPresets(detail.fields, [], detail.constraints, detail.recommended_mapping_paths ?? []);
  const active = mappingPresets(detail.fields, configured, detail.constraints, detail.recommended_mapping_paths ?? []);
  for (const path of ["process.file.name", "process.file.type_id"]) {
    expect(idle.core.has(path)).toBe(false);
    expect(active.core.has(path)).toBe(true);
  }
  expect(
    missingMappingGroups(configured, detail.constraints).filter(({ when }) => when === "process.file"),
  ).toHaveLength(2);
});

test("serializes class-specific defaults and keeps zero distinct from unset", () => {
  const network = eventClassDetails.parse({ ...catalog, ...catalog.classes.find((item) => item.class_uid === 4001) });
  const rows = [{ id: "result", source: "outcome", target: "status_id", fixed: false, conversion: "", fallback: "0" }];
  expect(sharedMapping(rows, network.fields)).toEqual({
    field_map: { outcome: "status_id" },
    defaults: { status_id: 0 },
  });
  expect(sharedMapping([{ ...rows[0], fallback: "-1" }], network.fields)).toBeUndefined();
});

test("accepts an extension HTTP status and rejects a value outside the catalog range", () => {
  const http = eventClassDetails.parse({ ...catalog, ...catalog.classes.find((item) => item.class_uid === 4002) });
  const row = { id: "code", source: "upstream", target: "http_response.code", fixed: false, conversion: "" };
  expect(sharedMapping([{ ...row, fallback: "499" }], http.fields)?.defaults).toEqual({ "http_response.code": 499 });
  expect(sharedMapping([{ ...row, fallback: "42" }], http.fields)).toBeUndefined();
});
