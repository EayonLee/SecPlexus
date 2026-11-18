import { type APIRequestContext, expect, request, test } from "@playwright/test";
import { execFileSync } from "node:child_process";
import { randomBytes } from "node:crypto";
import { z } from "zod";

const environment = z
  .object({
    SPX_E2E_BASE_URL: z.url(),
    SPX_E2E_USERNAME: z.string().min(1),
    SPX_E2E_PASSWORD: z.string().min(1),
    SPX_E2E_KAFKA_BROKERS: z.string().min(1),
    SPX_E2E_EVENT_TOPIC: z.string().min(1),
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

async function adminClient() {
  const api = await request.newContext({ baseURL });
  const response = await api.post("/api/v1/iam/auth/login", {
    data: { username: environment.SPX_E2E_USERNAME, password: environment.SPX_E2E_PASSWORD },
    headers: { Origin: baseURL, "X-CSRF-Token": await csrf(api) },
  });
  expect(response.status()).toBe(200);

  return api;
}

// readEvent 从事件 topic 读一条消息：受理成功不等于产出了正确的事件。
function readEvent(): Record<string, any> {
  const output = execFileSync(
    "go",
    [
      "run",
      "./tests/integration/kafka-read",
      "-brokers",
      environment.SPX_E2E_KAFKA_BROKERS,
      "-topic",
      environment.SPX_E2E_EVENT_TOPIC,
      "-timeout",
      "30s",
    ],
    { cwd: repositoryRoot, encoding: "utf8", timeout: 120000 },
  );

  return JSON.parse(output);
}

async function mutate(api: APIRequestContext, method: "post" | "delete", path: string, data?: unknown, expected = 204) {
  const response = await api[method](path, { data, headers: { Origin: baseURL, "X-CSRF-Token": await csrf(api) } });
  expect(response.status(), `${method} ${path}: ${await response.text()}`).toBe(expected);

  return expected === 204 ? undefined : (await response.json()).data;
}

// 页面管理能力与推送链路一起验证：浏览器 → Web 同源反代 → ingest → Kafka。
// 只测页面会漏掉"代理没配好"，只测 API 会漏掉"页面调不通"，所以一条用例走全程。
test("解析映射与数据接入可在页面上完成创建，并真实接收设备推送", async ({ page }) => {
  const admin = await adminClient();
  // 接口登录拿到的会话要交给浏览器，否则访问设置页会跳登录。
  await page.context().addCookies((await admin.storageState()).cookies);
  const suffix = randomBytes(4).toString("hex");
  const mappingName = `e2e-mapping-${suffix}`;
  const inputName = `e2e-input-${suffix}`;

  try {
    // 映射：页面写规则 → 保存 → 列表可见。
    await page.goto("/settings/mappings");
    await expect(page.getByRole("heading", { name: "解析映射" })).toBeVisible();
    await page.getByPlaceholder("例如：cisco-asa").fill(mappingName);
    await page.getByRole("button", { name: "新建" }).click();
    await expect(page.getByRole("cell", { name: mappingName })).toBeVisible();

    // 接入点：页面输入名称 → 新建 → 一次性令牌可见。
    await page.goto("/settings/ingest");
    await expect(page.getByRole("heading", { name: "数据接入" })).toBeVisible();
    await page.getByPlaceholder("例如：边界防火墙").fill(inputName);
    await page.getByRole("button", { name: "新建" }).click();

    // 令牌只显示一次：页面必须给出完整值，否则设备无法接入。
    await expect(page.getByText("请立即复制该令牌。它只显示一次，之后无法找回。")).toBeVisible();
    const token = (await page.locator("code").first().innerText()).trim();
    expect(token.length).toBeGreaterThanOrEqual(32);

    await expect(page.getByRole("cell", { name: inputName })).toBeVisible();

    // 页面建出的映射要能被接入点引用：拿到它的 id 后建一条绑定映射的接入点。
    const mappings = (await admin.get("/api/v1/ingest/mappings")).ok()
      ? (await (await admin.get("/api/v1/ingest/mappings")).json()).data
      : [];
    const mapping = (mappings as { id: string; name: string }[]).find((item) => item.name === mappingName);
    expect(mapping, "页面创建的映射没有出现在列表里").toBeTruthy();

    const bound = (await mutate(admin, "post", "/api/v1/ingest/inputs", {
      name: `${inputName}-bound`,
      protocol: "http",
      mapping_id: mapping!.id,
    }, 201)) as { input: { id: string }; token: string };
    expect(bound.token).toBeTruthy();

    // 新建的接入点要等注册表刷新后才可接收推送；轮询到 202 为止，
    // 超过刷新周期仍 401 才是真的鉴权问题。
    const device = await request.newContext({ baseURL });
    try {
      await expect
        .poll(
          async () => {
            const attempt = await device.post(`/api/v1/ingest/events/${bound.input.id}`, {
              data: { src: "10.0.0.7", host: "e2e-host" },
              headers: { Authorization: `Bearer ${bound.token}`, "Content-Type": "application/json" },
            });

            return attempt.status();
          },
          { timeout: 20000, intervals: [500] },
        )
        .toBe(202);

      // 事件必须真的到达总线，并且带上我们承诺的形状。
      const event = readEvent();
      const metadata = event.metadata as Record<string, any>;
      expect(metadata?.spx?.input_id).toBe(bound.input.id);
      expect(metadata?.spx?.["parse.outcome"]).toBe("ok");
      expect(metadata?.uid).toBeTruthy();
      expect(metadata?.version).toBeTruthy();
      expect(typeof event.class_uid).toBe("number");
      expect(event.type_uid).toBe(event.class_uid * 100 + event.activity_id);
      expect(event.src_endpoint?.ip).toBe("10.0.0.7");
    } finally {
      await device.dispose();
    }
  } finally {
    await admin.dispose();
  }
});
