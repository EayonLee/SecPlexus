import { type APIRequestContext, expect, request, test } from "@playwright/test";
import { z } from "zod";

import en from "../../messages/en.json";
import zh from "../../messages/zh-CN.json";
import { execFileSync } from "node:child_process";
import { randomBytes } from "node:crypto";

const environment = z
  .object({
    SPX_E2E_BASE_URL: z.url(),
    SPX_E2E_USERNAME: z.string().min(1),
    SPX_E2E_PASSWORD: z.string().min(1),
    SPX_E2E_KAFKA_BROKERS: z.string().min(1),
    SPX_E2E_INPUT_TOPIC: z.string().min(1),
    SPX_E2E_EVENT_TOPIC: z.string().min(1),
    SPX_E2E_CLICKHOUSE_URL: z.url().optional(),
  })
  .parse(process.env);
const baseURL = environment.SPX_E2E_BASE_URL;
// 用例在 web 目录下运行，go run 需要仓库根目录；由 harness 传入。
const repositoryRoot = process.env.SPX_E2E_ROOT ?? process.cwd();

async function csrf(api: APIRequestContext) {
  const response = await api.get("/api/v1/iam/auth/context");
  expect(response.status()).toBe(200);

  return (await response.json()).data.csrf_token as string;
}

async function login(username: string, password: string) {
  const api = await request.newContext({ baseURL });
  const response = await api.post("/api/v1/iam/auth/login", {
    data: { username, password },
    headers: { Origin: baseURL, "X-CSRF-Token": await csrf(api) },
  });
  expect(response.status()).toBe(200);

  return api;
}

// readEvent 从事件 topic 读一条消息：受理成功不等于产出了正确的事件。
function readTopic(topic: string, input = "", count = 1): unknown {
  const output = execFileSync(
    "go",
    [
      "run",
      "./tests/integration/kafka-read",
      "-brokers",
      environment.SPX_E2E_KAFKA_BROKERS,
      "-topic",
      topic,
      "-timeout",
      "30s",
      "-input",
      input,
      "-count",
      String(count),
    ],
    { cwd: repositoryRoot, encoding: "utf8", timeout: 120000 },
  );

  return JSON.parse(output);
}

function readEvent() {
  return z
    .object({
      metadata: z.object({
        uid: z.string(),
        version: z.string(),
        spx: z.object({
          input_id: z.string(),
          vendor: z.string(),
          product: z.string(),
          "parse.outcome": z.string(),
          parsing_rule_id: z.uuid(),
          parsing_rule_version: z.int().positive(),
        }),
      }),
      class_uid: z.number(),
      activity_id: z.number(),
      type_uid: z.number(),
      src_endpoint: z.object({ ip: z.string() }),
    })
    .parse(readTopic(environment.SPX_E2E_EVENT_TOPIC));
}

// 一个流程验证 Web 同源管理、IAM 权限、Ingest 推送和 Kafka 产出。
test("解析规则与数据接入可在页面上完成创建，并真实接收设备推送", async ({ page }) => {
  test.setTimeout(90_000);
  const admin = await login(environment.SPX_E2E_USERNAME, environment.SPX_E2E_PASSWORD);
  const device = await request.newContext({ baseURL });
  await page.context().addCookies((await admin.storageState()).cookies);
  const suffix = randomBytes(4).toString("hex");
  const mappingName = `e2e-mapping-${suffix}`;
  const inputName = `e2e-input-${suffix}`;
  const sample = '{"src":"192.0.2.7","host":"  e2e-host  "}';
  const sheet = page.getByRole("main");

  try {
    const catalogPath = "/api/v1/ingest/event-schema";
    expect((await device.get(catalogPath)).status()).toBe(401);
    const catalog = await admin.get(catalogPath);
    expect(catalog.status()).toBe(200);
    expect(await catalog.json()).toMatchObject({
      data: {
        ocsf_version: "1.10.0",
        contract_version: "v1",
        classes: expect.arrayContaining([
          expect.objectContaining({ class_uid: 4001, name: expect.objectContaining({ "zh-CN": "网络活动" }) }),
        ]),
      },
    });
    const classResponse = await admin.get(`${catalogPath}/classes/4001`);
    expect(classResponse.status()).toBe(200);
    expect(await classResponse.json()).toMatchObject({
      data: {
        class_uid: 4001,
        fields: expect.arrayContaining([
          expect.objectContaining({
            path: "src_endpoint.ip",
            logical_type: "ip_t",
            name: expect.objectContaining({ "zh-CN": "源 IP" }),
          }),
          expect.objectContaining({
            path: "activity_id",
            enum: expect.arrayContaining([
              expect.objectContaining({ value: 6, name: expect.objectContaining({ en: "Traffic" }) }),
            ]),
          }),
          expect.objectContaining({
            path: "activity_name",
            condition: expect.objectContaining({ en: expect.stringContaining("99") }),
          }),
        ]),
      },
    });
    expect((await admin.get(`${catalogPath}/classes/6003`)).status()).toBe(200);
    expect((await admin.get(`${catalogPath}/classes/3007`)).status()).toBe(200);
    const allFields = (await (await admin.get(`${catalogPath}/fields`)).json()).data.fields;
    const destinations = allFields.filter((field: { path: string }) => field.path === "dst_endpoint.ip");
    expect(destinations).toHaveLength(1);
    expect(destinations[0].usages).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ class_uid: 3002 }),
        expect.objectContaining({ class_uid: 4001 }),
      ]),
    );
    expect(
      (
        await device.post("/api/v1/ingest/parsing-rules/extract", {
          data: { parse: { mode: "json" }, samples: [sample] },
          headers: { Origin: baseURL, "X-CSRF-Token": "unauthenticated" },
        })
      ).status(),
    ).toBe(403);
    await page.goto("/ingest/parsing-rules");
    await expect(page).toHaveURL(/\/ingest\/parsing-rules$/);
    const group = page.getByRole("group", { name: "数据接入", exact: true });
    await expect(group.getByRole("link", { name: "解析规则" })).toHaveAttribute("aria-current", "page");
    await expect(group.getByRole("link", { name: "数据接入" })).toHaveAttribute("href", "/ingest/inputs");
    await expect(page.getByRole("heading", { name: "解析规则", exact: true })).toBeVisible();
    await expect(page.getByRole("link", { name: "E2E sample parser", exact: true })).toBeVisible();
    const desktop = page.viewportSize();
    if (!desktop) throw new Error("需要浏览器视口验证行操作");
    await page.setViewportSize({ width: 390, height: 844 });
    const rule = page
      .getByRole("row")
      .filter({ has: page.getByRole("link", { name: "E2E sample parser", exact: true }) });
    const rowAction = rule.getByRole("button");
    await expect(rowAction).toBeInViewport({ ratio: 1 });
    await rowAction.click();
    await expect(page.getByRole("menuitem", { name: "查看详情", exact: true })).toBeVisible();
    await expect(page.getByRole("menuitem", { name: "导出规则", exact: true })).toHaveCount(0);
    await page.keyboard.press("Escape");
    await page.setViewportSize(desktop);
    expect((await admin.get(catalogPath, { params: { contract_version: "1.0.0" } })).status()).toBe(404);
    await page.getByRole("link", { name: "新建规则", exact: true }).click();
    await expect(sheet.getByRole("heading", { name: "规则分支" })).toHaveCount(0);
    await expect(sheet.getByRole("link", { name: "导入旧版规则" })).toHaveCount(0);
    await expect(sheet.getByText("报文分帧", { exact: true })).toHaveCount(0);
    await sheet.getByLabel("名称", { exact: true }).fill(mappingName);
    await sheet.getByLabel("数据源厂商", { exact: true }).fill("E2E Vendor");
    await sheet.getByLabel("数据源产品", { exact: true }).fill("E2E Edge");
    await sheet.getByLabel("规则描述", { exact: true }).fill("Network sample mapping");
    await sheet.getByRole("combobox", { name: "事件类型", exact: true }).click();
    await expect(page.getByRole("option", { name: "基础事件", exact: true })).toHaveCount(0);
    await expect(page.getByRole("option", { name: "多种事件类型", exact: true })).toHaveCount(0);
    await page.getByRole("option", { name: "网络活动", exact: true }).click();
    const progress = sheet.getByRole("list", { name: "解析规则", exact: true });
    const extractStep = progress.getByRole("button", { name: "提取字段", exact: true });
    await extractStep.click({ position: { x: 5, y: 5 } });
    await expect(sheet.getByRole("textbox", { name: "日志样本", exact: true })).toBeFocused();

    const parserType = sheet.getByRole("combobox", { name: "解析类型", exact: true });
    await parserType.click();
    await page.getByRole("option", { name: "分隔符", exact: true }).click();
    await sheet.getByLabel("分隔符", { exact: true }).fill("|");
    await sheet.getByRole("textbox", { name: "日志样本", exact: true }).fill("192.0.2.7|test-host");
    await extractStep.click({ position: { x: 5, y: 5 } });
    await expect(sheet.getByRole("cell", { name: "field_1", exact: true })).toBeVisible();
    await expect(sheet.getByRole("cell", { name: "field_2", exact: true })).toBeVisible();

    await progress.getByRole("button", { name: "基础配置", exact: true }).click();
    await parserType.click();
    await page.getByRole("option", { name: "JSON", exact: true }).click();
    await sheet.getByRole("textbox", { name: "日志样本", exact: true }).fill(sample);
    const sampleEditor = sheet.getByRole("region", { name: "日志样本", exact: true });
    const addSample = sampleEditor.getByRole("button", { name: "添加样本", exact: true });
    const sampleCount = sampleEditor.getByRole("status");
    await expect(sampleCount).toHaveText("1/20");
    await addSample.click();
    await expect(sampleCount).toHaveText("2/20");
    const firstSample = sampleEditor.getByRole("textbox", { name: "样本 1", exact: true });
    const secondSample = sampleEditor.getByRole("textbox", { name: "样本 2", exact: true });
    await expect(firstSample).toHaveValue(sample);
    await expect(secondSample).toBeFocused();
    const initialHeight = await secondSample.evaluate((element) => element.clientHeight);
    await secondSample.fill("line 1\nline 2\nline 3\nline 4\nline 5");
    await expect(secondSample).toHaveJSProperty("clientHeight", initialHeight);
    expect(await secondSample.evaluate((element) => element.scrollHeight)).toBeGreaterThan(initialHeight);
    const addPosition = await addSample.boundingBox();
    const samplePosition = await secondSample.boundingBox();
    expect(addPosition?.y).toBeGreaterThan((samplePosition?.y ?? 0) + (samplePosition?.height ?? 0));
    await secondSample.blur();
    await expect(secondSample).toHaveAttribute("aria-invalid", "true");
    await expect(firstSample).not.toHaveAttribute("aria-invalid", "true");
    await extractStep.click();
    await expect(secondSample).toBeFocused();
    await expect(progress.locator('[aria-current="step"]')).toContainText("基础配置");
    await parserType.click();
    await page.getByRole("option", { name: "键值对", exact: true }).click();
    await expect(secondSample).toHaveAttribute("aria-invalid", "false");
    await firstSample.fill("src=192.0.2.7 host=test-host");
    await addSample.click();
    const thirdSample = sampleEditor.getByRole("textbox", { name: "样本 3", exact: true });
    await thirdSample.fill("missing separators");
    await secondSample.clear();
    await extractStep.click();
    await expect(thirdSample).toHaveAttribute("aria-invalid", "true");
    await expect(thirdSample).toBeFocused();
    await expect(secondSample).toHaveAttribute("aria-invalid", "false");
    await expect(firstSample).toHaveAttribute("aria-invalid", "false");
    await sampleEditor.getByRole("button", { name: "移除样本 3", exact: true }).click();
    await parserType.click();
    await page.getByRole("option", { name: "JSON", exact: true }).click();
    await firstSample.fill(sample);
    await sampleEditor.getByRole("button", { name: "移除样本 2", exact: true }).click();
    await expect(sampleCount).toHaveText("1/20");
    await expect(sampleEditor.getByRole("textbox", { name: "日志样本", exact: true })).toHaveValue(sample);
    await sheet.getByRole("button", { name: "提取字段", exact: true }).filter({ visible: true }).last().click();
    await expect(sheet.getByRole("cell", { name: "src", exact: true })).toBeVisible();
    await expect(sheet.getByRole("textbox", { name: "日志样本", exact: true })).toHaveJSProperty("readOnly", true);
    await sheet
      .getByRole("row")
      .filter({ has: page.getByRole("cell", { name: "host", exact: true }) })
      .getByRole("button", { name: "裁剪", exact: true })
      .click();
    const processor = page.getByRole("dialog", { name: "裁剪" });
    await processor.getByLabel("派生字段名", { exact: true }).fill("normalized_host");
    await processor.getByRole("button", { name: "应用处理", exact: true }).click();
    await expect(sheet.getByRole("cell", { name: "normalized_host", exact: true })).toBeVisible();
    await expect(sheet.getByRole("button", { name: "添加映射", exact: true })).toHaveCount(0);
    await sheet.getByRole("button", { name: "字段映射", exact: true }).last().click();
    const sourceIP = sheet.getByRole("combobox", { name: "源字段 · 源 IP", exact: true });
    const sourceHost = sheet.getByRole("combobox", { name: "源字段 · 源主机名", exact: true });
    await sheet.getByRole("button", { name: "验证输出", exact: true }).click();
    await expect(sourceIP).toHaveAttribute("aria-invalid", "true");
    await sourceIP.fill("src");
    await expect(page.getByRole("option", { name: "src string", exact: true })).toBeVisible();
    await sourceIP.press("ArrowDown");
    await sourceIP.press("Enter");
    await expect(sourceIP).toHaveValue("src");
    await sourceHost.fill("src");
    await expect(sourceHost).toHaveAttribute("aria-invalid", "true");
    await sourceHost.fill("normalized_host");
    await sourceHost.press("ArrowDown");
    await sourceHost.press("Enter");
    await expect(sourceHost).not.toHaveAttribute("aria-invalid", "true");
    await sheet
      .locator('[data-mapping-path="connection_info.protocol_name"]')
      .getByRole("button", { name: /移除映射/ })
      .click();
    await sheet.getByRole("button", { name: "添加字段映射", exact: true }).click();
    const optionalTarget = sheet.getByRole("combobox", { name: "搜索标准字段", exact: true });
    await optionalTarget.fill("connection_info.protocol_name");
    await page.getByRole("option", { name: /connection_info\.protocol_name/ }).click();
    const protocolSource = sheet.getByRole("combobox", { name: /^源字段 · .*协议/ });
    await protocolSource.fill("protocol");
    await protocolSource.press("Tab");
    await expect(protocolSource).toHaveValue("protocol");
    await expect(sheet.getByRole("button", { name: "事件分类", exact: true })).toHaveCount(0);
    await sheet.getByRole("button", { name: "验证输出", exact: true }).click();
    await expect(sheet.getByText("解析成功", { exact: true })).toBeVisible();
    await sheet.getByRole("button", { name: "新建", exact: true }).click();
    await expect(sheet.getByRole("link", { name: mappingName, exact: true })).toBeVisible();

    await sheet.getByRole("searchbox").fill("E2E Edge");
    const savedRule = sheet
      .getByRole("row")
      .filter({ has: page.getByRole("link", { name: mappingName, exact: true }) });
    await expect(savedRule.getByRole("cell", { name: "E2E Vendor", exact: true })).toBeVisible();
    await expect(savedRule.getByRole("cell", { name: "E2E Edge", exact: true })).toBeVisible();
    await savedRule.getByRole("link", { name: mappingName, exact: true }).click();
    await expect(sheet.getByLabel("名称", { exact: true })).not.toBeEditable();
    await expect(sheet.getByLabel("规则描述", { exact: true })).toHaveValue("Network sample mapping");
    await sheet.getByRole("list", { name: "解析规则", exact: true }).getByRole("button", { name: "提取字段" }).click();
    await expect(sheet.getByRole("region", { name: "提取字段", exact: true }).getByRole("table")).toBeVisible();
    await sheet.getByRole("list", { name: "解析规则", exact: true }).getByRole("button", { name: "字段映射" }).click();
    await expect(sheet.getByRole("combobox", { name: "源字段 · 源主机名", exact: true })).toBeDisabled();
    await sheet.getByRole("list", { name: "解析规则", exact: true }).getByRole("button", { name: "预览保存" }).click();
    await expect(sheet.getByRole("region", { name: "事件预览", exact: true })).toBeVisible();
    await expect(sheet.getByRole("button", { name: "保存", exact: true })).toHaveCount(0);
    await sheet.getByRole("button", { name: "编辑", exact: true }).click();
    const description = "Rule updated from the browser";
    await sheet.getByLabel("规则描述", { exact: true }).fill(description);
    await sheet.getByRole("textbox", { name: "日志样本", exact: true }).fill(sample);
    await sheet.getByRole("button", { name: "提取字段", exact: true }).last().click();
    await sheet.getByRole("button", { name: "字段映射", exact: true }).last().click();
    await expect(sheet.getByRole("combobox", { name: "源字段 · 源主机名", exact: true })).toHaveValue(
      "normalized_host",
    );
    await sheet.getByRole("button", { name: "验证输出", exact: true }).click();
    await expect(sheet.getByText("解析成功", { exact: true })).toBeVisible();
    await sheet.getByRole("button", { name: "保存", exact: true }).click();
    await expect(savedRule.getByText(description)).toBeVisible();
    await savedRule.getByRole("link", { name: mappingName, exact: true }).click();
    await expect(sheet.getByLabel("规则描述", { exact: true })).toHaveValue(description);
    await expect(sheet.locator("time[datetime]")).toBeVisible();
    await sheet.getByRole("button", { name: "返回列表", exact: true }).click();
    const summaryPage = await admin.get("/api/v1/ingest/parsing-rules", {
      params: { limit: 1, q: ` ${mappingName.toUpperCase()} `, origin: "custom", parse_type: "json" },
    });
    expect(summaryPage.status()).toBe(200);
    const summary = (await summaryPage.json()).data.items[0];
    expect(summary).toMatchObject({ name: mappingName, parse_type: "json", version: 2 });
    expect(summary).not.toHaveProperty("definition");
    expect(summary).not.toHaveProperty("output_classes");
    const ruleDetail = await admin.get(`/api/v1/ingest/parsing-rules/${summary.id}`);
    expect(ruleDetail.status()).toBe(200);
    expect((await ruleDetail.json()).data.definition.description).toBe(description);
    expect((await device.get(`/api/v1/ingest/parsing-rules/${summary.id}`)).status()).toBe(401);
    expect((await admin.get("/api/v1/ingest/parsing-rules?limit=101")).status()).toBe(400);
    const defaultRules = await admin.get("/api/v1/ingest/parsing-rules");
    expect((await defaultRules.json()).data).toMatchObject({ limit: 20, offset: 0 });
    const matchingClasses = await admin.get("/api/v1/ingest/parsing-rules", { params: { class_uid: 4001, q: description } });
    expect((await matchingClasses.json()).data).toMatchObject({
      limit: 20,
      items: [{ id: summary.id, class_uids: [4001] }],
    });
    const otherClasses = await admin.get("/api/v1/ingest/parsing-rules", { params: { class_uid: 4002, q: description } });
    expect((await otherClasses.json()).data.items).toEqual([]);
    expect((await admin.get(`/api/v1/ingest/parsing-rules/${summary.id}/preview`)).status()).toBe(404);
    const eventTypeTrigger = page.getByRole("combobox", { name: "事件类型", exact: true });
    const eventTypeFilter = page.getByRole("combobox", { name: "搜索事件类型", exact: true });
    await expect(eventTypeTrigger).toContainText("全部");
    await eventTypeTrigger.click();
    await expect(eventTypeFilter).toHaveValue("");
    await eventTypeFilter.pressSequentially("4001");
    await expect(page.getByRole("option", { name: "HTTP 活动", exact: true })).toHaveCount(0);
    await expect(page.getByRole("option", { name: "网络活动", exact: true })).toBeVisible();
    await eventTypeFilter.press("ArrowDown");
    await eventTypeFilter.press("Enter");
    await expect(eventTypeFilter).toBeHidden();
    await expect(eventTypeTrigger).toContainText("网络活动");
    await eventTypeTrigger.click();
    await expect(eventTypeFilter).toBeFocused();
    await expect(eventTypeFilter).toHaveValue("");
    await page.keyboard.press("Escape");
    await expect(eventTypeFilter).toBeHidden();
    await expect(eventTypeTrigger).toContainText("网络活动");
    await expect(eventTypeTrigger).toBeFocused();
    await expect(page).toHaveURL(/class_uid=4001/);
    await expect(savedRule).toBeVisible();
    await expect(page.getByRole("columnheader", { name: "版本", exact: true })).toHaveCount(0);
    await expect(page.getByRole("columnheader", { name: "数据接入", exact: true })).toBeVisible();
    await page.goto("/ingest/inputs");
    await expect(page.getByRole("combobox", { name: "解析规则", exact: true })).toContainText("全部");
    await page.getByRole("link", { name: "新建接入点", exact: true }).click();
    await sheet.getByLabel("名称", { exact: true }).fill(inputName);
    await sheet.getByLabel("数据源产品", { exact: true }).fill("Actual Edge");
    const picker = sheet.getByRole("combobox", { name: "解析规则" });
    await expect(picker).toBeEnabled();
    await picker.press("ArrowDown");
    await expect(page.getByRole("listbox")).toBeVisible();
    await picker.press("ControlOrMeta+A");
    await picker.pressSequentially("E2E Edge");
    await expect(page.getByRole("option", { name: new RegExp(mappingName) })).toBeVisible();
    await picker.press("ArrowDown");
    await picker.press("Enter");
    await expect(picker).toHaveValue(new RegExp(mappingName));
    await picker.press("ControlOrMeta+A");
    await picker.pressSequentially("no-match-" + suffix);
    await expect(page.getByText("没有匹配的规则")).toBeVisible();
    await expect(page.getByRole("option")).toHaveCount(0);
    await page.keyboard.press("Escape");
    await expect(picker).toBeFocused();
    await expect(picker).toHaveValue(new RegExp(mappingName));
    await sheet.getByRole("button", { name: "下一步" }).click();
    await sheet.getByRole("button", { name: "下一步" }).click();
    await expect(sheet.getByRole("button", { name: "下一步" })).toBeDisabled();
    await sheet.getByRole("textbox", { name: "日志样本", exact: true }).fill(JSON.stringify(JSON.parse(sample), null, 2));
    await sheet.getByRole("button", { name: "验证样本" }).click();
    await expect(sheet.getByRole("region", { name: "事件预览" })).toContainText("192.0.2.7");
    await sheet.getByRole("button", { name: "下一步" }).click();
    await expect(sheet.getByRole("definition").filter({ hasText: "E2E Vendor" })).toBeVisible();
    await expect(sheet.getByRole("definition").filter({ hasText: "Actual Edge" })).toBeVisible();
    await sheet.getByRole("button", { name: "创建并启用" }).click();
    const credential = sheet.getByRole("region", { name: "设备令牌" });
    await expect(credential.getByText("请立即复制该令牌。它只显示一次，之后无法找回。")).toBeVisible();
    const token = (await credential.locator("code").textContent())?.trim();
    await credential.getByRole("button", { name: "完成" }).click();
    await page.reload();
    await expect(credential).toHaveCount(0);
    if (!token) throw new Error("设备令牌未出现");
    const listed = await admin.get("/api/v1/ingest/inputs");
    const input = z
      .object({
        data: z.object({ limit: z.literal(20), items: z.array(z.object({ name: z.string(), id: z.string() })) }),
      })
      .parse(await listed.json())
      .data.items.find((row) => row.name === inputName);
    if (!input) throw new Error("接入点未出现");
    for (const q of [inputName, "E2E Vendor", "Actual Edge"]) {
      const filtered = await admin.get("/api/v1/ingest/inputs", { params: { q, rule: summary.id } });
      expect((await filtered.json()).data).toMatchObject({ limit: 20, items: [{ id: input.id }] });
    }
    const singleInput = await admin.get(`/api/v1/ingest/inputs/${input.id}`);
    expect(singleInput.status()).toBe(200);
    expect((await singleInput.json()).data).toMatchObject({ id: input.id, parsing_rule_id: summary.id });
    const staleBinding = await admin.patch(`/api/v1/ingest/inputs/${input.id}`, {
      data: { parsing_rule_id: summary.id, expected_rule_version: 2147483647 },
      headers: { Origin: baseURL, "X-CSRF-Token": await csrf(admin) },
    });
    expect(staleBinding.status()).toBe(409);
    expect((await device.get(`/api/v1/ingest/inputs/${input.id}`)).status()).toBe(401);

    const push = async (value: string) =>
      (
        await device.post(`/api/v1/ingest/events/${input.id}`, {
          data: sample,
          headers: { Authorization: `Bearer ${value}`, "Content-Type": "application/json" },
        })
      ).status();
    await expect.poll(() => push(token), { timeout: 35000, intervals: [500] }).toBe(202);
    const event = readEvent();
    expect(event.metadata.spx.input_id).toBe(input.id);
    expect(event.metadata.spx.parsing_rule_id).toBe(summary.id);
    expect(event.metadata.spx.parsing_rule_version).toBe(summary.version);
    expect(event.metadata.spx.vendor).toBe("E2E Vendor");
    expect(event.metadata.spx.product).toBe("Actual Edge");
    expect(event.metadata.spx["parse.outcome"]).toBe("ok");
    expect(event.metadata.uid).toBeTruthy();
    expect(event.metadata.version).toBe("1.10.0");
    expect(event.type_uid).toBe(event.class_uid * 100 + event.activity_id);
    expect(event.src_endpoint.ip).toBe("192.0.2.7");
    const invalid = await device.post(`/api/v1/ingest/events/${input.id}`, {
      data: '{"src":"invalid-ip"}',
      headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/json" },
    });
    expect(invalid.status()).toBe(202);
    const records = z
      .array(z.object({ class_uid: z.number() }).passthrough())
      .parse(readTopic(environment.SPX_E2E_EVENT_TOPIC, input.id, 2));
    const diagnostic = records.find((record) => record.class_uid === 0);
    expect(JSON.stringify(diagnostic)).not.toContain("invalid-ip");
    const failure = z
      .object({
        class_uid: z.literal(0),
        metadata: z.object({
          spx: z.object({
            input_id: z.string(),
            "parse.outcome": z.literal("partial"),
            "parse.error": z.object({ reason: z.literal("contract_violation") }),
          }),
        }),
      })
      .parse(diagnostic);
    expect(failure.metadata.spx.input_id).toBe(input.id);
    await expect
      .poll(async () => {
        const response = await admin.get("/api/v1/iam/audit-events");
        if (!response.ok()) return false;
        const payload = z
          .object({
            data: z.object({
              items: z.array(
                z.object({
                  target_id: z.string().nullish(),
                  action: z.string(),
                  outcome: z.string(),
                }),
              ),
            }),
          })
          .safeParse(await response.json());
        return (
          payload.success &&
          payload.data.data.items.some(
            (item) => item.target_id === input.id && item.action === "ingest.input.write" && item.outcome === "success",
          )
        );
      })
      .toBe(true);

    await expect(page).toHaveURL(new RegExp(`${input.id}$`));
    await expect(page.getByTestId("received-count")).toHaveText(/[1-9]/, { timeout: 15000 });
    await group.getByRole("link", { name: "解析规则" }).click();
    const row = page.getByRole("row").filter({ has: page.getByRole("link", { name: mappingName, exact: true }) });
    const countsPath = `/api/v1/ingest/inputs/counts-by-rule?id=${summary.id}`;
    expect((await device.get(countsPath)).status()).toBe(401);
    const counts = await admin.get(countsPath);
    expect(counts.status()).toBe(200);
    expect((await counts.json()).data).toMatchObject({ [summary.id]: 1 });
    await row.getByRole("link", { name: new RegExp(mappingName + " 的 1 个数据接入") }).click();
    await expect(page).toHaveURL(/\/ingest\/inputs\?rule=/);
    await expect(page.getByRole("cell", { name: inputName, exact: true })).toBeVisible();
    const ruleTrigger = page.getByRole("combobox", { name: "解析规则", exact: true });
    const ruleFilter = page.getByRole("combobox", { name: "搜索解析规则", exact: true });
    await expect(ruleTrigger).toContainText(mappingName);
    await ruleTrigger.click();
    await expect(ruleFilter).toHaveValue("");
    await ruleFilter.pressSequentially("E2E sample parser");
    await expect(page.getByRole("option", { name: "E2E sample parser", exact: true })).toBeVisible();
    await page
      .getByRole("option", { name: /^E2E sample parser/ })
      .first()
      .click();
    await expect(page.getByRole("cell", { name: inputName, exact: true })).toHaveCount(0);
    await ruleTrigger.click();
    await expect(ruleFilter).toHaveValue("");
    await ruleFilter.pressSequentially(mappingName);
    await page.getByRole("option", { name: new RegExp(mappingName) }).click();
    await expect(page.getByRole("cell", { name: inputName, exact: true })).toBeVisible();
    const selectedInput = page.getByRole("checkbox", { name: `选择 ${inputName}`, exact: true });
    for (const [action, state] of [
      ["停用", "已停用"],
      ["启用", "已启用"],
    ] as const) {
      await selectedInput.check();
      await page
        .getByText("已选 1 项", { exact: true })
        .locator("..")
        .getByRole("button", { name: action, exact: true })
        .click();
      await page.getByRole("alertdialog").getByRole("button", { name: action, exact: true }).click();
      await expect(
        page.getByRole("row").filter({ has: selectedInput }).getByText(state, { exact: true }),
      ).toBeVisible();
    }
    const actions = page.getByRole("button", { name: `${inputName} 的操作` });
    await actions.click();
    await page.getByRole("menuitem", { name: "编辑配置" }).click();
    await expect(page.getByLabel("名称", { exact: true })).toHaveValue(inputName);
    await page.getByLabel("名称", { exact: true }).fill("Discarded draft");
    await page.keyboard.press("Escape");
    await expect(page.getByRole("dialog", { name: "编辑配置" })).toHaveCount(0);
    await page.getByRole("button", { name: "编辑配置", exact: true }).click();
    await expect(page.getByLabel("名称", { exact: true })).toHaveValue(inputName);
    const concurrent = await admin.patch(`/api/v1/ingest/inputs/${input.id}`, {
      data: { product: "Updated Product" },
      headers: { Origin: baseURL, "X-CSRF-Token": await csrf(admin) },
    });
    expect(concurrent.status()).toBe(200);
    await expect(
      page.getByRole("definition", { includeHidden: true }).filter({ hasText: "Updated Product" }),
    ).toHaveCount(1);
    await page.getByLabel("数据源厂商", { exact: true }).fill("Updated Vendor");
    await page.getByRole("button", { name: "保存", exact: true }).click();
    await expect(page.getByRole("definition").filter({ hasText: "Updated Vendor" })).toBeVisible();
    await expect(page.getByRole("definition").filter({ hasText: "Updated Product" })).toBeVisible();
    await page.getByRole("button", { name: "返回列表", exact: true }).click();
    await expect(page).toHaveURL(/\/ingest\/inputs\?rule=/);
    await actions.click();
    await page.getByRole("menuitem", { name: "轮换令牌" }).click();
    await page.getByRole("alertdialog").getByRole("button", { name: "轮换令牌" }).click();
    await expect(page.getByRole("dialog").locator("code")).toBeVisible();
    const rotated = (await page.getByRole("dialog").locator("code").textContent())?.trim();
    if (!rotated) throw new Error("新设备令牌未出现");
    expect(rotated !== token).toBe(true);
    await page.getByRole("dialog").getByRole("button", { name: "完成" }).click();
    await expect.poll(() => push(rotated), { timeout: 35000, intervals: [500] }).toBe(202);
    expect(await push(token)).toBe(401);

    await actions.click();
    await page.getByRole("menuitem", { name: "删除", exact: true }).click();
    await page.getByRole("alertdialog").getByRole("button", { name: "删除", exact: true }).click();
    await expect(page.getByRole("alertdialog")).toHaveCount(0);
    await expect(page.getByRole("cell", { name: inputName, exact: true })).toHaveCount(0);

    await page.goto("/ingest/parsing-rules");
    await expect(page).toHaveURL(/\/ingest\/parsing-rules$/);
    await page.getByRole("button", { name: `${mappingName} 的操作` }).click();
    await page.getByRole("menuitem", { name: "删除", exact: true }).click();
    await page.getByRole("alertdialog").getByRole("button", { name: "删除", exact: true }).click();
    await expect(page.getByRole("alertdialog")).toHaveCount(0);
    await expect(page.getByRole("cell", { name: mappingName, exact: true })).toHaveCount(0);
  } finally {
    await device.dispose();
    await admin.dispose();
  }
});

test("业务表格的表头与数据列统一左对齐", async ({ page }) => {
  const admin = await login(environment.SPX_E2E_USERNAME, environment.SPX_E2E_PASSWORD);
  try {
    await page.context().addCookies((await admin.storageState()).cookies);
    for (const path of [
      "/dashboard",
      "/settings/users",
      "/settings/roles",
      "/settings/audit",
      "/ingest/inputs",
      "/ingest/parsing-rules",
    ]) {
      await page.goto(path);
      const main = page.getByRole("main");
      await expect(main.getByRole("columnheader").first()).toBeVisible();
      const wrong = await main
        .locator("table :is(th, td)")
        .evaluateAll((cells) =>
          cells
            .filter((cell) => cell.getClientRects().length && getComputedStyle(cell).textAlign !== "left")
            .map((cell) => cell.textContent?.trim()),
        );
      expect(wrong, path).toEqual([]);
    }
  } finally {
    await admin.dispose();
  }
});

test("日志样本字段类型与处理结果", async ({ page }) => {
  const admin = await login(environment.SPX_E2E_USERNAME, environment.SPX_E2E_PASSWORD);
  try {
    await page.context().addCookies((await admin.storageState()).cookies);
    await page.goto("/ingest/parsing-rules/new");
    const main = page.getByRole("main");
    await main.getByRole("textbox", { name: "名称", exact: true }).fill("Extraction preview");
    await main.getByLabel("数据源厂商", { exact: true }).fill("Test");
    await main.getByLabel("数据源产品", { exact: true }).fill("Edge");
    await main.getByLabel("规则描述", { exact: true }).fill("Extraction preview");
    await main.getByRole("combobox", { name: "事件类型", exact: true }).click();
    await page.getByRole("option", { name: "网络活动", exact: true }).click();
    await main
      .getByRole("textbox", { name: "日志样本", exact: true })
      .fill('{"name":" padded ","count":12,"enabled":true,"list":[1],"object":{"item":1},"empty":null}');
    await main.getByRole("button", { name: "提取字段", exact: true }).last().click();
    await expect(main.getByRole("region", { name: "日志样本", exact: true })).toBeVisible();
    await expect(main.getByRole("button", { name: "重新提取", exact: true })).toHaveCount(0);
    await expect(main.getByRole("columnheader", { name: "操作", exact: true })).toHaveCSS("text-align", "left");
    const typeBadges = main.locator("[data-value-type]");
    await expect(typeBadges).toHaveCount(6);
    const numberRow = main.getByRole("row").filter({ has: page.getByRole("cell", { name: "count", exact: true }) });
    await expect(numberRow.getByRole("button", { name: "裁剪", exact: true })).toBeDisabled();
    await expect(numberRow.getByRole("button", { name: "解码", exact: true })).toBeDisabled();
    for (const theme of ["light", "dark"]) {
      const colors = await typeBadges.evaluateAll((badges) =>
        badges.map((badge) => {
          const style = getComputedStyle(badge);
          return {
            color: style.color,
            size: style.fontSize,
            cellSize: getComputedStyle(badge.closest("td")!).fontSize,
          };
        }),
      );
      expect(new Set(colors.map(({ color }) => color)).size, theme).toBe(5);
      expect(colors.every(({ size, cellSize }) => size === cellSize)).toBe(true);
      if (theme === "light") {
        await page.getByRole("button", { name: "切换到深色模式" }).click();
        await expect(page.locator("html")).toHaveClass(/dark/);
      }
    }
    await page.setViewportSize({ width: 390, height: 844 });
    await expect(main.locator('[data-value-type="string"]')).toBeVisible();
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390);
    await page.setViewportSize({ width: 1440, height: 960 });
    const nameRow = main.getByRole("row").filter({ has: page.getByRole("cell", { name: "name", exact: true }) });
    await nameRow.getByRole("button", { name: "裁剪", exact: true }).click();
    const panel = page.getByRole("dialog", { name: "裁剪" });
    await panel.getByLabel("派生字段名").fill("clean_name");
    await page.route("**/api/v1/ingest/parsing-rules/extract", (route) => route.abort(), { times: 1 });
    await panel.getByRole("button", { name: "预览结果", exact: true }).click();
    await expect(panel.getByRole("alert")).toBeVisible();
    await panel.getByRole("button", { name: "预览结果", exact: true }).click();
    await expect(panel.getByRole("textbox", { name: "处理结果", exact: true })).toHaveValue("padded");
    await expect(main.getByRole("cell", { name: "clean_name", exact: true })).toHaveCount(0);
    await panel.getByRole("button", { name: "应用处理", exact: true }).click();
    await expect(main.getByRole("cell", { name: "clean_name", exact: true })).toBeVisible();
    await main.getByRole("button", { name: /clean_name/ }).click();
    await expect(main.getByRole("cell", { name: "clean_name", exact: true })).toHaveCount(0);
    await expect(main.getByRole("cell", { name: "name", exact: true })).toBeVisible();
  } finally {
    await admin.dispose();
  }
});

test("解析规则基础字段校验和重名恢复", async ({ page }) => {
  const admin = await login(environment.SPX_E2E_USERNAME, environment.SPX_E2E_PASSWORD);
  const name = `e2e-valid-${randomBytes(4).toString("hex")}`;
  const path = "/api/v1/ingest/parsing-rules";
  const sample = '{"src":"192.0.2.18"}';
  const definition = {
    contract_version: "v1",
    parse: { mode: "json" },
    description: "Traffic mapping",
    branches: [{ id: "network", name: "Network", class_uid: 4001, field_map: { src: "src_endpoint.ip" } }],
  };
  try {
    const headers = { Origin: baseURL, "X-CSRF-Token": await csrf(admin) };
    const invalid = await admin.post(path, { headers, data: { name, vendor: "厂".repeat(121), definition } });
    expect(invalid.status()).toBe(400);
    expect(await invalid.json()).toMatchObject({
      code: "invalid_request",
      data: null,
      field_errors: [{ field: "vendor", code: "too_long", limit: 120 }],
    });

    await page.context().addCookies((await admin.storageState()).cookies);
    await page.setViewportSize({ width: 390, height: 844 });
    await page.goto("/ingest/parsing-rules/new");
    const main = page.getByRole("main");
    const ruleName = main.getByRole("textbox", { name: "名称", exact: true });
    const vendor = main.getByRole("textbox", { name: "数据源厂商", exact: true });
    for (const field of [
      ruleName,
      vendor,
      main.getByLabel("数据源产品", { exact: true }),
      main.getByLabel("规则描述", { exact: true }),
    ]) {
      await field.click();
      await field.press("Tab");
      await expect(field).toHaveAttribute("aria-invalid", "false");
      await expect(field).not.toHaveAttribute("aria-describedby", /error/);
    }
    await main.getByRole("list", { name: "解析规则", exact: true }).getByRole("button", { name: "提取字段" }).click();
    await expect(ruleName).toBeFocused();
    await expect(ruleName).toHaveAttribute("aria-invalid", "true");
    await main.getByRole("combobox", { name: "事件类型", exact: true }).click();
    for (const [category, eventType] of [
      ["系统活动", "文件系统活动"],
      ["安全发现", "漏洞发现"],
      ["身份与访问管理", "认证活动"],
      ["网络类", "网络活动"],
      ["应用活动", "API 活动"],
    ]) {
      await expect(
        page.getByRole("group", { name: category, exact: true }).getByRole("option", { name: eventType, exact: true }),
      ).toBeVisible();
    }
    await page.getByRole("option", { name: "网络活动", exact: true }).click();
    await ruleName.fill("E2E sample parser");
    await vendor.fill("𠮷".repeat(121));
    await main.getByRole("textbox", { name: "日志样本", exact: true }).fill(sample);
    await main.getByRole("button", { name: "提取字段", exact: true }).last().click();
    await expect(vendor).toBeFocused();
    await expect(vendor).toHaveAttribute("aria-invalid", "true");
    await expect(vendor).toHaveAccessibleDescription(/120/);
    await vendor.fill("Example");
    await main.getByLabel("数据源产品", { exact: true }).fill("Edge");
    await main.getByLabel("规则描述", { exact: true }).fill("Traffic mapping");
    await expect(vendor).toHaveAttribute("aria-invalid", "false");

    await main.getByRole("button", { name: "提取字段", exact: true }).last().click();
    await main.getByRole("button", { name: "字段映射", exact: true }).last().click();
    const sourceIP = main.getByRole("combobox", { name: "源字段 · 源 IP", exact: true });
    await sourceIP.fill("src");
    await sourceIP.press("Tab");
    await main.getByRole("button", { name: "验证输出", exact: true }).click();
    await main.getByRole("button", { name: "新建", exact: true }).click();
    await expect(ruleName).toBeFocused();
    await expect(ruleName).toHaveAccessibleDescription(/已存在/);
    await expect(main.getByRole("textbox", { name: "日志样本", exact: true })).toHaveValue(sample);

    await ruleName.fill(name);
    await main.getByRole("button", { name: "预览保存", exact: true }).click();
    await main.getByRole("button", { name: "新建", exact: true }).click();
    await expect(main.getByRole("link", { name, exact: true })).toBeVisible();

    const list = await admin.get(path, { params: { limit: 1, q: name } });
    expect(list.status()).toBe(200);
    const rule = (await list.json()).data.items[0];
    const stale = await admin.patch(`${path}/${rule.id}`, {
      headers,
      data: { name, vendor: "Example", product: "Edge", definition, expected_version: rule.version + 1 },
    });
    expect(stale.status()).toBe(409);
    expect(await stale.json()).toMatchObject({
      code: "conflict",
      field_errors: [{ field: "expected_version", code: "revision_conflict" }],
    });
  } finally {
    await admin.dispose();
  }
});

test("分隔符样本有第二条记录时留在原步骤", async ({ page }) => {
  const admin = await login(environment.SPX_E2E_USERNAME, environment.SPX_E2E_PASSWORD);
  const path = "/api/v1/ingest/parsing-rules";
  const headers = { Origin: baseURL, "X-CSRF-Token": await csrf(admin) };
  let ruleID: string | undefined;
  try {
    await page.context().addCookies((await admin.storageState()).cookies);
    await page.goto("/ingest/parsing-rules/new");
    const main = page.getByRole("main");
    await main.getByLabel("名称", { exact: true }).fill("分隔符样本校验");
    await main.getByLabel("数据源厂商", { exact: true }).fill("Example");
    await main.getByLabel("数据源产品", { exact: true }).fill("Edge");
    await main.getByLabel("规则描述", { exact: true }).fill("Traffic mapping");
    await main.getByRole("combobox", { name: "事件类型", exact: true }).click();
    await page.getByRole("option", { name: "网络活动", exact: true }).click();
    await main.getByRole("combobox", { name: "解析类型", exact: true }).click();
    await page.getByRole("option", { name: "分隔符", exact: true }).click();
    await main.getByLabel("分隔符", { exact: true }).fill("|");
    const sample = main.getByRole("textbox", { name: "日志样本", exact: true });
    await sample.fill("one|two\nthree|four");
    await main.getByRole("button", { name: "提取字段", exact: true }).last().click();
    await expect(sample).toHaveAttribute("aria-invalid", "true");
    await expect(sample).toBeFocused();
    await sample.fill('one|"two\nthree"');
    await main.getByRole("button", { name: "提取字段", exact: true }).last().click();
    await expect(main.getByRole("region", { name: "提取字段", exact: true }).getByRole("row").filter({ hasText: "field_2" }))
      .toContainText(/two\s+three/);

    const definition = {
      contract_version: "v1",
      description: "Delimiter rule created through the API",
      parse: { mode: "delimiter", delimiter: { separator: "|" } },
      branches: [{ id: "network", name: "Network", class_uid: 4001, field_map: { field_1: "src_endpoint.ip" } }],
    };
    const created = await admin.post(path, {
      headers,
      data: {
        name: `Delimiter API rule ${randomBytes(4).toString("hex")}`,
        vendor: "Example",
        product: "Edge",
        definition,
        samples: ["192.0.2.1|traffic"],
      },
    });
    expect(created.status()).toBe(201);
    const stored = (await created.json()).data;
    ruleID = z.uuid().parse(stored.id);
    await page.goto(`/ingest/parsing-rules/${ruleID}`);
    await expect(main.getByRole("combobox", { name: "解析类型", exact: true })).toContainText("分隔符");
    await expect(main.getByLabel("规则描述", { exact: true })).toHaveValue(definition.description);
    await expect(main.getByLabel("分隔符", { exact: true })).toHaveValue("|");
    await main.getByRole("button", { name: "编辑", exact: true }).click();
    const updatedDescription = "Delimiter rule with an updated description";
    await main.getByLabel("规则描述", { exact: true }).fill(updatedDescription);
    await main.getByRole("button", { name: "保存", exact: true }).click();
    await main.getByRole("link", { name: z.string().parse(stored.name), exact: true }).click();
    await expect(main.getByLabel("规则描述", { exact: true })).toHaveValue(updatedDescription);
    await expect(main.locator("time[datetime]")).toHaveAttribute("datetime", z.string().parse(stored.verification.verified_at));
  } finally {
    if (ruleID) await admin.delete(`${path}/${ruleID}`, { headers });
    await admin.dispose();
  }
});

test("认证活动按标准字段配置，样本缺失时使用枚举默认值", async ({ page }) => {
  const admin = await login(environment.SPX_E2E_USERNAME, environment.SPX_E2E_PASSWORD);
  const name = `e2e-auth-${randomBytes(4).toString("hex")}`;
  try {
    await page.context().addCookies((await admin.storageState()).cookies);
    await page.goto("/ingest/parsing-rules/new");
    const main = page.getByRole("main");
    await main.getByRole("textbox", { name: "名称", exact: true }).fill(name);
    await main.getByLabel("数据源厂商", { exact: true }).fill("Test");
    await main.getByLabel("数据源产品", { exact: true }).fill("Edge");
    await main.getByLabel("规则描述", { exact: true }).fill("Authentication mapping");
    await main.getByRole("combobox", { name: "事件类型", exact: true }).fill("登录事件");
    await page.getByRole("option", { name: "认证活动", exact: true }).click();
    await main.getByRole("textbox", { name: "日志样本", exact: true }).fill('{"account":"alice%2Dlog","app":"sshd"}');
    await main.getByRole("button", { name: "提取字段", exact: true }).last().click();
    const account = main.getByRole("row").filter({ has: page.getByRole("cell", { name: "account", exact: true }) });
    await account.getByRole("button", { name: "解码", exact: true }).click();
    const processor = page.getByRole("dialog", { name: "解码" });
    await processor.getByRole("combobox", { name: "处理方式" }).click();
    await page.getByRole("option", { name: "URI 解码", exact: true }).click();
    await processor.getByLabel("派生字段名", { exact: true }).fill("decoded_account");
    await processor.getByRole("button", { name: "预览结果", exact: true }).click();
    await expect(processor.getByRole("textbox", { name: "处理结果", exact: true })).toHaveValue("alice-log");
    await processor.getByRole("button", { name: "应用处理", exact: true }).click();
    await expect(main.getByRole("row").filter({ hasText: "decoded_account" })).toContainText("alice-log");

    await main.getByRole("button", { name: "添加字段", exact: true }).click();
    const supplement = page.getByRole("dialog", { name: "添加字段" });
    await supplement.getByLabel("源字段", { exact: true }).fill("outcome");
    await supplement.getByLabel("样本值", { exact: true }).fill("1");
    await supplement.getByRole("button", { name: "添加一行" }).click();
    await supplement.getByLabel("源字段", { exact: true }).last().fill("tenant");
    await supplement.getByLabel("样本值", { exact: true }).last().fill("sample-only-tenant");
    await supplement.getByRole("button", { name: "应用", exact: true }).click();
    await expect(main.getByRole("row").filter({ hasText: "outcome" })).toContainText("number");

    await main.getByRole("button", { name: "字段映射", exact: true }).last().click();
    await expect(main.getByText("账号名称", { exact: true })).toBeVisible();
    const mappedField = (path: string) => main.locator(`[data-mapping-path="${path}"]`);
    await expect(mappedField("user.name").getByText("string", { exact: true })).toBeVisible();
    await expect(mappedField("message").getByText("string", { exact: true })).toBeVisible();
    await mappedField("message")
      .getByRole("button", { name: /移除映射/ })
      .click();
    await expect(mappedField("message")).toHaveCount(0);
    await main.getByRole("button", { name: "验证输出", exact: true }).click();
    await expect(main.getByRole("region", { name: "事件预览", exact: true })).toHaveCount(0);
    await expect(mappedField("user.name").getByRole("combobox")).toBeFocused();
    await expect(main.getByText("目的域名", { exact: true })).toHaveCount(0);
    await expect(main.getByText("服务名称", { exact: true })).toBeVisible();
    const sourceUser = main.getByRole("combobox", { name: "源字段 · 账号名称" });
    await sourceUser.fill("decoded_account");
    await page.getByRole("option", { name: "decoded_account string", exact: true }).click();
    const sourceService = main.getByRole("combobox", { name: "源字段 · 服务名称" });
    await sourceService.fill("outcome");
    await page.getByRole("option", { name: "outcome number", exact: true }).click();
    await expect(sourceService).toHaveAccessibleDescription(/样本类型与标准字段不兼容/);
    await sourceService.fill("decoded_account");
    await page.getByRole("option", { name: "decoded_account string", exact: true }).click();
    await expect(sourceUser).toHaveValue("decoded_account");
    await expect(sourceService).toHaveValue("decoded_account");
    await expect(sourceUser).toHaveAttribute("aria-invalid", "true");
    await expect(mappedField("user.name").getByRole("button", { name: /移除映射/ })).toHaveCount(0);
    await sourceService.fill("app");
    await page.getByRole("option", { name: "app string", exact: true }).click();
    await mappedField("status_id")
      .getByRole("button", { name: /移除映射/ })
      .click();
    await expect(mappedField("status_id")).toHaveCount(0);
    await main.getByRole("button", { name: "添加字段映射", exact: true }).click();
    const statusTarget = main.getByRole("combobox", { name: "搜索标准字段", exact: true });
    await statusTarget.fill("status_id");
    await page.getByRole("option", { name: /status_id/ }).click();
    const mapRow = main.getByRole("combobox", { name: "源字段 · 操作结果" });
    await mapRow.fill("outcome");
    await page.getByRole("option", { name: "outcome number", exact: true }).click();
    await main.getByRole("combobox", { name: "默认值 · 操作结果", exact: true }).click();
    await page.getByRole("option", { name: "失败", exact: true }).click();
    await main.getByRole("button", { name: "验证输出", exact: true }).click();
    await expect(main.getByRole("row").filter({ hasText: "操作结果" })).toContainText("默认值");
    await expect(main.getByRole("cell", { name: "账号名称 user.name", exact: true })).toBeVisible();
    await expect(main.getByRole("cell", { name: "失败 默认值", exact: true })).toBeVisible();
    await expect(main.getByText("解析成功", { exact: true })).toBeVisible();
    await main.getByRole("tab", { name: "OCSF JSON", exact: true }).click();
    await expect(main.getByRole("tabpanel")).toContainText('"status_id": 2');
    await main.getByRole("button", { name: "新建", exact: true }).click();
    await expect(main.getByRole("link", { name, exact: true })).toBeVisible();

    const listed = await admin.get("/api/v1/ingest/parsing-rules", { params: { q: name } });
    const id = (await listed.json()).data.items[0].id;
    const saved = (await (await admin.get(`/api/v1/ingest/parsing-rules/${id}`)).json()).data.definition;
    expect(saved.branches[0]).toMatchObject({ class_uid: 3002, defaults: { status_id: 2 } });
    expect(JSON.stringify(saved)).not.toContain("sample-only-tenant");
  } finally {
    await admin.dispose();
  }
});

test("HTTP 映射预览区分非法状态码与缺失时的默认值", async ({ page }) => {
  const admin = await login(environment.SPX_E2E_USERNAME, environment.SPX_E2E_PASSWORD);
  try {
    await page.context().addCookies((await admin.storageState()).cookies);
    await page.goto("/ingest/parsing-rules/new");
    const main = page.getByRole("main");
    await main.getByRole("textbox", { name: "名称", exact: true }).fill("HTTP preview");
    await main.getByLabel("数据源厂商", { exact: true }).fill("Test");
    await main.getByLabel("数据源产品", { exact: true }).fill("Proxy");
    await main.getByLabel("规则描述", { exact: true }).fill("HTTP response examples");
    await main.getByRole("combobox", { name: "事件类型", exact: true }).click();
    await page.getByRole("option", { name: "HTTP 活动", exact: true }).click();
    await main.getByRole("button", { name: "添加样本", exact: true }).click();
    await main.getByRole("textbox", { name: "样本 1", exact: true }).fill('{"code":42,"path":"/first"}');
    await main.getByRole("textbox", { name: "样本 2", exact: true }).fill('{"path":"/second"}');
    await main.getByRole("button", { name: "提取字段", exact: true }).last().click();
    await main.getByRole("button", { name: "字段映射", exact: true }).last().click();
    await expect(main.getByText(/至少配置一项/)).toHaveCount(0);
    const receiptTime = main.getByRole("combobox", { name: "默认值 · 事件发生时间", exact: true });
    await receiptTime.click();
    await page.getByRole("option", { name: "接收时间", exact: true }).click();

    await main.getByRole("button", { name: "添加字段映射", exact: true }).click();
    const pending = main.getByRole("group", { name: "添加字段映射", exact: true });
    await expect(pending.getByRole("combobox", { name: "搜索标准字段" })).toBeFocused();
    await pending.getByRole("button", { name: "删除", exact: true }).click();

    for (const [field, path] of [
      ["请求路径", "path"],
      ["HTTP 状态码", "code"],
    ] as const) {
      await main.getByRole("combobox", { name: `源字段 · ${field}` }).fill(path);
      await page.getByRole("option", { name: new RegExp(`^${path} (number|string)$`) }).click();
    }
    const method = main.getByRole("combobox", { name: "默认值 · HTTP 方法", exact: true });
    await method.press("ArrowDown");
    await expect(page.getByRole("option", { name: "GET", exact: true })).toBeVisible();
    await expect(page.getByRole("option", { name: "LOCK", exact: true })).toHaveCount(0);
    await method.fill("LOCK");
    await page.getByRole("option", { name: "LOCK", exact: true }).click();
    await expect(method).toHaveValue("LOCK");

    const status = main.getByRole("combobox", { name: "默认值 · HTTP 状态码", exact: true });
    await status.fill("404");
    await page.getByRole("option", { name: "404 Not Found", exact: true }).click();
    await status.fill("499");
    await expect(page.getByRole("option", { name: "499", exact: true })).toBeVisible();
    await status.press("Tab");
    await expect(status).toHaveValue("499");
    await main.getByRole("button", { name: "验证输出", exact: true }).click();

    const result = main.getByRole("region", { name: "事件预览", exact: true });
    const statusRow = result.getByRole("row").filter({ hasText: "HTTP 状态码" });
    await expect(statusRow.getByRole("button", { name: "返回修改", exact: true })).toBeVisible();
    await expect(statusRow).not.toContainText("499");
    await main.getByRole("combobox", { name: "当前样本", exact: true }).click();
    await page.getByRole("option", { name: /^样本 2/ }).click();
    await expect(statusRow).toContainText("499");
    await expect(statusRow).toContainText("默认值");

    await page.setViewportSize({ width: 1024, height: 768 });
    await expect(result.getByRole("table")).toBeVisible();
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(1024);
    await page.setViewportSize({ width: 390, height: 844 });
    await expect(result.getByRole("tab", { name: "OCSF JSON" })).toBeVisible();
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390);
    await result.getByRole("combobox", { name: "当前样本", exact: true }).click();
    await page.getByRole("option", { name: /^样本 1/ }).click();
    await statusRow.getByRole("button", { name: "返回修改", exact: true }).click();
    await expect(main.getByRole("combobox", { name: "源字段 · HTTP 状态码", exact: true })).toBeFocused();
  } finally {
    await admin.dispose();
  }
});

test("Ingest 的只读会话可以查看接入和规则预览但不能写入", async ({ page }) => {
  const admin = await login(environment.SPX_E2E_USERNAME, environment.SPX_E2E_PASSWORD);
  const username = `ingest_reader_${randomBytes(6).toString("hex")}`;
  const password = randomBytes(24).toString("hex");
  try {
    const user = await admin.post("/api/v1/iam/users", {
      data: { username, display_name: "数据接入观察员", password },
      headers: { Origin: baseURL, "X-CSRF-Token": await csrf(admin) },
    });
    expect(user.status()).toBe(201);
    const reader = await login(username, password);
    try {
      await page.goto("/ingest/inputs");
      await expect(page).toHaveURL(/\/login\?next=/);
      await page.getByRole("textbox", { name: "用户名", exact: true }).fill(username);
      await page.getByLabel("密码", { exact: true }).fill(password);
      await page.getByRole("button", { name: "登录", exact: true }).click();
      await expect(page.getByRole("table")).toBeVisible();
      await expect(page.getByRole("link", { name: "新建接入点", exact: true })).toHaveCount(0);
      await expect(page).toHaveURL(/\/ingest\/inputs$/);
      await page.goto("/ingest/inputs/new");
      await expect(page.getByRole("button", { name: "下一步" })).toHaveCount(0);
      await expect(page.getByText("无权访问", { exact: true })).toBeVisible();
      const rules = await reader.get("/api/v1/ingest/parsing-rules", {
        params: { q: "E2E sample parser", class_uid: 3002 },
      });
      expect(rules.status()).toBe(200);
      const [{ id: builtinId, class_uids }] = (await rules.json()).data.items;
      expect(class_uids).toEqual(expect.arrayContaining([3002, 4001]));
      const saved = await reader.get(`/api/v1/ingest/parsing-rules/${builtinId}`);
      expect(saved.status()).toBe(200);
      expect((await saved.json()).data.definition).not.toHaveProperty("samples");
      await page.goto(`/ingest/parsing-rules/${builtinId}?edit=1`);
      const progress = page.getByRole("list", { name: "解析规则", exact: true });
      await expect(page.getByLabel("名称", { exact: true })).not.toBeEditable();
      await progress.getByRole("button", { name: "预览保存" }).click();
      await expect(page.getByRole("region", { name: "事件预览" })).toBeVisible();
      await expect(page.getByRole("button", { name: "保存", exact: true })).toHaveCount(0);
      const draft = await reader.post("/api/v1/ingest/parsing-rules/extract", {
        data: { parse: { mode: "json" }, samples: ['{"value":"sample"}'] },
        headers: { Origin: baseURL, "X-CSRF-Token": await csrf(reader) },
      });
      expect(draft.status()).toBe(403);
      const rejected = await reader.post("/api/v1/ingest/inputs", {
        data: { name: username, protocol: "http" },
        headers: { Origin: baseURL, "X-CSRF-Token": await csrf(reader) },
      });
      expect(rejected.status()).toBe(403);
    } finally {
      await reader.dispose();
    }
  } finally {
    await admin.dispose();
  }
});

test("Kafka 向导验证连接信息并保存入站配置", async ({ page }) => {
  const admin = await login(environment.SPX_E2E_USERNAME, environment.SPX_E2E_PASSWORD);
  try {
    await page.context().addCookies((await admin.storageState()).cookies);
    await page.goto("/ingest/inputs");
    const sheet = page.getByRole("main");
    await page.getByRole("link", { name: "新建接入点", exact: true }).click();
    await sheet.getByRole("button", { name: "下一步" }).click();
    const name = sheet.getByLabel("名称", { exact: true });
    await expect(name).toBeFocused();
    await expect(name).toHaveAttribute("aria-invalid", "true");
    const inputName = `kafka-input-${randomBytes(4).toString("hex")}`;
    const inputTopic = environment.SPX_E2E_INPUT_TOPIC;
    const publishSample = () => execFileSync(
      "go",
      ["run", "./tests/integration/kafka-send", "-brokers", environment.SPX_E2E_KAFKA_BROKERS, "-topic", inputTopic],
      { cwd: repositoryRoot, timeout: 120000 },
    );
    publishSample();
    await name.fill(inputName);
    await sheet.getByRole("button", { name: "下一步" }).click();
    await expect(sheet.getByRole("radio", { name: "HTTP", exact: true })).toBeChecked();
    await sheet.getByRole("radio", { name: "Kafka", exact: true }).click();
    const brokers = sheet.getByLabel("引导服务器");
    const topic = sheet.getByLabel("主题", { exact: true });
    await sheet.getByRole("button", { name: "下一步" }).click();
    await expect(brokers).toBeFocused();
    for (const field of [brokers, topic]) await expect(field).toHaveAttribute("aria-invalid", "true");
    await brokers.fill(environment.SPX_E2E_KAFKA_BROKERS);
    await topic.fill(inputTopic);
    const start = sheet.getByRole("combobox", { name: "首次消费位置" });
    await sheet.getByRole("button", { name: "下一步" }).click();
    await expect(start).toBeFocused();
    await expect(start).toHaveAttribute("aria-invalid", "true");
    await start.press("Enter");
    await page.getByRole("option", { name: "从接入创建时间开始" }).click();

    await sheet.getByRole("button", { name: "下一步" }).click();
    await sheet.getByRole("button", { name: "下一步" }).click();
    await sheet.getByRole("button", { name: "创建并启用" }).click();
    await expect(sheet.getByRole("heading", { name: inputName, exact: true })).toBeVisible();
    await expect(sheet.getByText(inputTopic, { exact: true })).toBeVisible();
    await expect(sheet.getByText("从接入创建时间开始", { exact: true })).toBeVisible();
    publishSample();
    await expect(sheet.getByTestId("received-count")).toHaveText("1", { timeout: 35000 });
  } finally {
    await admin.dispose();
  }
});

test("控制台布局随偏好切换并在不同路由和刷新后生效", async ({ page }) => {
  const admin = await login(environment.SPX_E2E_USERNAME, environment.SPX_E2E_PASSWORD);
  try {
    await page.context().addCookies((await admin.storageState()).cookies);
    await page.setViewportSize({ width: 1440, height: 900 });
    const title = page.getByRole("main").getByRole("heading", { level: 1 });
    const typography = () =>
      title.evaluate((element) => {
        const style = getComputedStyle(element);
        return [style.fontFamily, style.fontSize, style.fontWeight, style.lineHeight, style.letterSpacing];
      });
    await page.goto("/dashboard");
    await expect(title).toBeVisible();
    const reference = await typography();
    for (const path of [
      "/settings/profile",
      "/settings/users",
      "/settings/roles",
      "/settings/audit",
      "/ingest/parsing-rules",
      "/ingest/parsing-rules/new",
      "/ingest/inputs/new",
      "/ingest/inputs",
    ]) {
      await page.goto(path);
      await expect(title).toBeVisible();
      expect(await typography(), path).toEqual(reference);
    }
    const content = page.getByRole("table");
    await expect(content).toBeVisible();
    const width = () => content.evaluate((node) => node.getBoundingClientRect().width);
    await page.getByRole("button", { name: "主题设置" }).click();
    await page.getByLabel("全宽布局", { exact: true }).click();
    const full = await width();
    await page.getByLabel("居中布局", { exact: true }).click();
    await expect.poll(width).toBeLessThan(full - 40);
    await page.keyboard.press("Escape");
    await page.goto("/settings/users");
    await page.reload();
    await expect(page.locator("html")).toHaveAttribute("data-content-layout", "centered");
    const centered = await width();
    await page.getByRole("button", { name: "主题设置" }).click();
    await page.getByLabel("全宽布局", { exact: true }).click();
    await expect.poll(width).toBeGreaterThan(centered + 40);
    await page.keyboard.press("Escape");
    await page.goto("/ingest/inputs/new?via=syslog");
    const progress = page.getByRole("list", { name: "新建接入点" });
    const source = page.getByRole("region", { name: "日志来源", exact: true });
    const summary = page.getByRole("region", { name: "日志解析", exact: true });
    for (const w of [2560, 1440, 390]) {
      await page.setViewportSize({ width: w, height: 900 });
      const left = await source.boundingBox(),
        right = await summary.boundingBox(),
        outer = await progress.boundingBox();
      if (!left || !right || !outer) throw new Error("向导缺少可见分区");
      if (w === 390) expect(right.y).toBeGreaterThan(left.y + left.height);
      else {
        expect(Math.abs(left.y - right.y)).toBeLessThan(2);
        expect(Math.abs(right.x + right.width - outer.x - outer.width)).toBeLessThan(2);
      }
      expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(w);
      await page.screenshot({ path: test.info().outputPath(`wizard-${w}.png`) });
    }
    await page.getByRole("textbox", { name: "名称", exact: true }).fill("layout-verification");
    await page.getByRole("button", { name: "下一步" }).click();
    await expect(page.getByRole("radio", { name: "Syslog", exact: true })).toBeChecked();
  } finally {
    await admin.dispose();
  }
});

test("控制台共享入口在语言切换与导航后保留翻译和浮层", async ({ page }) => {
  const admin = await login(environment.SPX_E2E_USERNAME, environment.SPX_E2E_PASSWORD);
  try {
    await page
      .context()
      .addCookies([
        ...(await admin.storageState()).cookies,
        { name: "spx-locale", value: "zh-CN", url: baseURL, httpOnly: true, sameSite: "Lax" },
      ]);
    const i18nErrors: string[] = [];
    page.on("console", (message) => {
      if (/MISSING_MESSAGE|MISSING_FORMAT|FORMATTING_ERROR|INVALID_MESSAGE/.test(message.text())) {
        i18nErrors.push(message.text());
      }
    });
    const main = page.getByRole("main");
    const html = page.locator("html");
    const namespaces = Object.keys(en).join("|");
    const untranslated = new RegExp(`\\b(?:${namespaces})\\.[a-z][\\w.]*\\b`);
    await page.goto("/dashboard");

    for (const [locale, messages] of [
      ["zh-CN", zh],
      ["en", en],
    ] as const) {
      await expect(html).toHaveAttribute("lang", locale);
      await expect(main.getByRole("heading", { name: messages.Dashboard.title })).toBeVisible();
      const inputLink = page.getByRole("link", { name: messages.Navigation.inputs, exact: true });
      await inputLink.click();
      await expect(main.getByRole("heading", { name: messages.Navigation.inputs })).toBeVisible();
      const filter = main.getByRole("combobox", { name: messages.Ingest.protocol, exact: true });
      await expect(filter).toContainText(messages.Ingest.protocol);
      await expect(filter).toContainText(messages.Common.allOptions);
      await filter.press("ArrowDown");
      await expect(page.getByRole("option", { name: messages.Ingest.syslog, exact: true })).toBeVisible();
      await page.keyboard.press("Escape");
      await expect(filter).toBeFocused();
      await expect(main).not.toContainText(untranslated);

      await page.getByRole("link", { name: messages.Navigation.mappings, exact: true }).click();
      await expect(main.getByRole("heading", { name: messages.Navigation.mappings })).toBeVisible();
      await expect(main.getByRole("combobox", { name: messages.Mappings.origin })).toContainText(
        messages.Common.allOptions,
      );
      await expect(main).not.toContainText(untranslated);

      await page.getByRole("button", { name: messages.Navigation.accountMenu }).click();
      await page.getByRole("menuitem", { name: messages.Navigation.settings }).click();
      await page.getByRole("button", { name: messages.Account.changeTimeZone }).click();
      const dialog = page.getByRole("dialog", { name: messages.Account.changeTimeZone });
      await expect(dialog.getByRole("button", { name: messages.Common.close, exact: true })).toBeVisible();
      await expect(dialog).not.toContainText(untranslated);
      await page.keyboard.press("Escape");
      for (const name of [messages.Navigation.users, messages.Navigation.roles, messages.Navigation.audit]) {
        await page.getByRole("link", { name, exact: true }).click();
        await expect(main.getByRole("heading", { name, exact: true })).toBeVisible();
        await expect(main.getByRole("table")).toBeVisible();
        await expect(main).not.toContainText(untranslated);
        if (name === messages.Navigation.roles) {
          await main.getByRole("tab", { name: messages.Management.permissionCatalog, exact: true }).click();
          await expect(main.getByRole("tabpanel")).toContainText(messages.PermissionDescriptions.auditRead);
        }
      }
      if (locale === "zh-CN") {
        await page.getByRole("button", { name: messages.Common.language }).click();
        await expect(html).toHaveAttribute("lang", "en");
        await page.goto("/dashboard");
      }
    }
    expect(i18nErrors).toEqual([]);
  } finally {
    await admin.dispose();
  }
});

test("同一接入点归一化网络、认证和异常事件", async ({ page }) => {
  const admin = await login(environment.SPX_E2E_USERNAME, environment.SPX_E2E_PASSWORD);
  const device = await request.newContext({ baseURL });
  const inputName = `e2e-fortigate-${randomBytes(4).toString("hex")}`;
  try {
    await page.context().addCookies((await admin.storageState()).cookies);
    await page.goto("/ingest/parsing-rules");
    await page.getByRole("link", { name: "E2E sample parser", exact: true }).click();
    const main = page.getByRole("main");
    const exampleLog = JSON.stringify({
      kind: "traffic", source: "192.0.2.1", alternate_source: "192.0.2.3", destination: "198.51.100.1",
    });
    const signInLog = JSON.stringify({ kind: "sign-in", account: "example", destination: "192.0.2.2", result: 1 });
    await main.getByRole("button", { name: "提取字段", exact: true }).last().click();
    await expect(main.getByRole("heading", { name: "日志分流" })).toBeVisible();
    await main.getByRole("button", { name: "字段映射", exact: true }).last().click();
    await expect(main.getByRole("heading", { name: "日志分流" })).toHaveCount(0);
    await main.getByRole("button", { name: "预览保存", exact: true }).last().click();
    await expect(main.getByRole("region", { name: "事件预览" })).toBeVisible();
    await main.screenshot({ path: test.info().outputPath("mixed-rule-desktop.png") });
    await page.getByRole("button", { name: "切换到深色模式" }).click();
    await expect(page.locator("html")).toHaveClass(/dark/);
    await main.screenshot({ path: test.info().outputPath("mixed-rule-dark.png") });
    await page.getByRole("button", { name: "切换到浅色模式" }).click();
    await page.setViewportSize({ width: 412, height: 900 });
    await expect(main.getByRole("heading", { name: "E2E sample parser", exact: true })).toBeVisible();
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(412);
    await main.screenshot({ path: test.info().outputPath("mixed-rule-narrow.png") });
    await page.setViewportSize({ width: 1440, height: 1000 });
    const customName = `${inputName}-rule`;
    await main.getByRole("button", { name: "复制规则", exact: true }).click();
    await main.getByRole("textbox", { name: "名称", exact: true }).fill(customName);
    await main.getByRole("textbox", { name: "日志样本", exact: true }).fill(exampleLog);
    await main.getByRole("button", { name: "添加样本" }).click();
    await main.getByRole("textbox", { name: "样本 2", exact: true }).fill(signInLog);
    await main.getByRole("button", { name: "提取字段", exact: true }).last().click();
    await main.getByRole("textbox", { name: "分支名称", exact: true }).fill("Connection closed");
    await main.getByRole("button", { name: "字段映射", exact: true }).last().click();
    await main.getByRole("button", { name: "验证输出", exact: true }).click();
    await expect(main.getByRole("region", { name: "事件预览" })).toContainText("Connection closed");
    await main.getByRole("button", { name: "新建", exact: true }).click();
    await main.getByRole("link", { name: customName, exact: true }).click();
    await expect(main.getByRole("heading", { name: customName, exact: true })).toBeVisible();
    await expect(page).toHaveURL(/\/ingest\/parsing-rules\/[0-9a-f-]{36}/);
    const ruleID = page.url().split("?")[0].split("/").at(-1);
    const detailReply = await admin.get(`/api/v1/ingest/parsing-rules/${ruleID}`);
    expect(detailReply.status()).toBe(200);
    const detail = (await detailReply.json()).data;
    await main.getByRole("button", { name: "编辑", exact: true }).click();
    await main.getByRole("textbox", { name: "日志样本", exact: true }).fill(exampleLog);
    await main.getByRole("button", { name: "提取字段", exact: true }).last().click();
    await expect(main.getByRole("textbox", { name: "分支名称", exact: true })).toHaveValue("Connection closed");
    await main.getByRole("textbox", { name: "分支名称", exact: true }).fill("Closed firewall connection");
    await main.getByRole("button", { name: "字段映射", exact: true }).last().click();
    const sourceIP = main.getByRole("combobox", { name: "源字段 · 源 IP", exact: true });
    await sourceIP.fill("alternate_source");
    await page.getByRole("option", { name: "alternate_source string", exact: true }).click();
    const previewResponse = page.waitForResponse((response) => new URL(response.url()).pathname === "/api/v1/ingest/parsing-rules/validate");
    await main.getByRole("button", { name: "验证输出", exact: true }).click();
    expect((await (await previewResponse).json()).data.required_branches).toEqual([detail.definition.branches[0].id]);
    await expect(main.getByRole("button", { name: "保存", exact: true })).toBeEnabled();
    await main.getByRole("button", { name: "保存", exact: true }).click();
    await expect(main.getByRole("link", { name: customName, exact: true })).toBeVisible();
    const staleUpdate = await admin.patch(`/api/v1/ingest/parsing-rules/${ruleID}`, {
      data: {
        name: detail.name,
        vendor: detail.vendor,
        product: detail.product,
        definition: detail.definition,
        expected_version: detail.version,
      },
      headers: { Origin: baseURL, "X-CSRF-Token": await csrf(admin) },
    });
    expect(staleUpdate.status()).toBe(409);
    await page.goto(`/ingest/parsing-rules/${ruleID}`);
    await main.getByRole("button", { name: "修订历史", exact: true }).click();
    const revisions = page.getByRole("dialog", { name: "修订历史", exact: true });
    await revisions.getByRole("button", { name: /^修订 1/ }).click();
    await expect(revisions.getByText("branches", { exact: true })).toBeVisible();
    await revisions.getByRole("button", { name: "恢复此修订", exact: true }).click();
    await page.getByRole("alertdialog").getByRole("button", { name: "恢复此修订", exact: true }).click();
    await expect.poll(async () => {
      const response = await admin.get(`/api/v1/ingest/parsing-rules/${ruleID}`);
      return (await response.json()).data;
    }).toMatchObject({ version: 3, definition: { branches: [{ name: "Connection closed" }, { name: "Sign in" }] } });
    await page.goto("/ingest/inputs/new");
    await main.getByLabel("名称", { exact: true }).fill(inputName);
    const picker = main.getByRole("combobox", { name: "解析规则" });
    await picker.fill("E2E sample parser");
    await page.getByRole("option", { name: /^E2E sample parser Example/ }).click();
    await main.getByRole("button", { name: "下一步" }).click();
    await main.getByRole("button", { name: "下一步" }).click();
    const samples = [
      exampleLog,
      signInLog,
      JSON.stringify({ kind: "sign-in", account: "example", destination: "192.0.2.2", result: 2 }),
      JSON.stringify({ kind: "sign-in", destination: "192.0.2.2", result: 2 }),
      JSON.stringify({ kind: "other" }),
    ].join("\n");
    await main.getByRole("textbox", { name: "日志样本", exact: true }).fill(exampleLog);
    await main.getByRole("button", { name: "添加样本", exact: true }).click();
    await main.getByRole("textbox", { name: "样本 2", exact: true }).fill(signInLog);
    await main.getByRole("button", { name: "验证样本", exact: true }).click();
    await expect(main.getByRole("region", { name: "事件预览" })).toBeVisible();
    await main.getByRole("button", { name: "下一步" }).click();
    await main.getByRole("button", { name: "创建并启用" }).click();
    const credential = main.getByRole("region", { name: "设备令牌" });
    const token = (await credential.locator("code").textContent())?.trim();
    if (!token) throw new Error("接入点令牌未出现");
    const {
      data: { items },
    } = await (await admin.get("/api/v1/ingest/inputs", { params: { limit: 1, name: inputName } })).json();
    const inputId = z.uuid().parse(items[0]?.id);
    await expect
      .poll(
        async () => {
          const result = await device.post(`/api/v1/ingest/events/${inputId}`, {
            data: samples,
            headers: { Authorization: `Bearer ${token}`, "Content-Type": "text/plain" },
          });
          return result.status();
        },
        { timeout: 35_000 },
      )
      .toBe(202);
    const events = z
      .array(
        z.object({
          class_uid: z.int(),
          status_id: z.int().optional(),
          metadata: z.object({ spx: z.object({ "parse.outcome": z.string() }) }),
        }),
      )
      .parse(readTopic(environment.SPX_E2E_EVENT_TOPIC, inputId, 5));
    expect(events.map((event) => [event.class_uid, event.status_id, event.metadata.spx["parse.outcome"]])).toEqual(
      expect.arrayContaining([
        [4001, undefined, "ok"],
        [3002, 1, "ok"],
        [3002, 2, "ok"],
        [0, 2, "partial"],
        [0, undefined, "error"],
      ]),
    );
    await expect(main.getByTestId("received-count")).toHaveText("5");
    if (environment.SPX_E2E_CLICKHOUSE_URL) {
      const endpoint = new URL(environment.SPX_E2E_CLICKHOUSE_URL);
      endpoint.searchParams.set(
        "query",
        "SELECT class_uid, parse_outcome FROM spx.events FINAL WHERE input_id={input:UUID} ORDER BY class_uid, parse_outcome FORMAT JSONEachRow",
      );
      endpoint.searchParams.set("param_input", inputId);
      await expect
        .poll(
          async () => {
            const response = await fetch(endpoint, { method: "POST" });
            if (!response.ok) throw new Error(`Event query returned ${response.status}`);
            return (await response.text())
              .trim()
              .split("\n")
              .filter(Boolean)
              .map((line) => JSON.parse(line));
          },
          { timeout: 30_000 },
        )
        .toEqual([
          { class_uid: 0, parse_outcome: "error" },
          { class_uid: 0, parse_outcome: "partial" },
          { class_uid: 3002, parse_outcome: "ok" },
          { class_uid: 3002, parse_outcome: "ok" },
          { class_uid: 4001, parse_outcome: "ok" },
        ]);
    }
  } finally {
    await device.dispose();
    await admin.dispose();
  }
});
