import { type APIRequestContext, expect, type Page, request, test } from "@playwright/test";
import { z } from "zod";

import { zApiMe } from "../../src/spx/api/iam/schemas";
import { randomBytes } from "node:crypto";

const environment = z
  .object({
    SPX_E2E_BASE_URL: z.url(),
    SPX_E2E_USERNAME: z.string().min(1),
    SPX_E2E_PASSWORD: z.string().min(1),
  })
  .parse(process.env);
const baseURL = environment.SPX_E2E_BASE_URL;
const adminUsername = environment.SPX_E2E_USERNAME;
const adminPassword = environment.SPX_E2E_PASSWORD;

async function csrf(api: APIRequestContext) {
  const response = await api.get("/api/v1/iam/auth/context");
  expect(response.status()).toBe(200);
  return (await response.json()).data.csrf_token as string;
}
async function mutate(
  api: APIRequestContext,
  method: "post" | "put" | "patch" | "delete",
  path: string,
  data?: unknown,
  expected = 204,
) {
  const response = await api[method](path, { data, headers: { Origin: baseURL, "X-CSRF-Token": await csrf(api) } });
  expect(response.status()).toBe(expected);
  return expected === 204 ? undefined : (await response.json()).data;
}
let administratorState: Awaited<ReturnType<APIRequestContext["storageState"]>> | undefined;
async function adminClient() {
  if (administratorState) return request.newContext({ baseURL, storageState: administratorState });
  const api = await request.newContext({ baseURL });
  await mutate(api, "post", "/api/v1/iam/auth/login", { username: adminUsername, password: adminPassword }, 200);
  administratorState = await api.storageState();
  return api;
}

// 扁平授权只有一套权限：角色全局存在，用户直接持有角色。
async function createRole(api: APIRequestContext, name: string, permissions: string[]) {
  const created = (await mutate(api, "post", "/api/v1/iam/roles", { name, permissions }, 201)) as { id: string };

  return created.id;
}
async function createUser(api: APIRequestContext, username: string, displayName: string, password: string) {
  const created = (await mutate(
    api,
    "post",
    "/api/v1/iam/users",
    { username, display_name: displayName, password },
    201,
  )) as { id: string };

  return created.id;
}
// 抽屉顶部图标按钮与底部文字按钮的可访问名相同，按页脚容器定位底部按钮。
function drawerButton(drawer: ReturnType<Page["getByRole"]>, name: string) {
  return drawer.locator('[data-slot="sheet-footer"]').getByRole("button", { name, exact: true });
}
async function browserLogin(page: Page, username: string, password: string) {
  await page
    .context()
    .addCookies([{ name: "spx-locale", value: "zh-CN", url: baseURL, httpOnly: true, sameSite: "Lax" }]);
  await page.goto("/login");
  await page.getByRole("textbox", { name: "用户名", exact: true }).fill(username);
  await page.getByLabel("密码", { exact: true }).fill(password);
  await page.getByRole("button", { name: "登录", exact: true }).click();
  await expect(page).toHaveURL(/\/dashboard$/);
}

// 外部站点统计不属于身份链路，避免第三方网络阻塞浏览器启动。
test.beforeEach(async ({ context }) => {
  await context.route("https://www.googletagmanager.com/**", (route) => route.abort());
});

test.beforeAll(async () => {
  const api = await adminClient();
  await api.dispose();
});

test("身份接口是扁平的：principal、roles、permissions 且不含组织字段", async () => {
  const api = await adminClient();
  const raw = (await (await api.get("/api/v1/iam/me")).json()).data;
  const identity = zApiMe.parse(raw);

  expect(identity.principal.configured).toBe(true);
  expect(identity.permissions).toEqual(
    expect.arrayContaining([
      "iam.users.read",
      "iam.users.manage",
      "iam.roles.read",
      "iam.roles.manage",
      "iam.audit.read",
    ]),
  );
  expect(identity.roles.some((role) => role.name === "超级管理员")).toBe(true);
  expect(raw).not.toHaveProperty("organizations");
  expect(raw).not.toHaveProperty("org_id");
  expect(raw).not.toHaveProperty("platform_capabilities");
  expect(identity.principal).not.toHaveProperty("is_super_admin");
  await api.dispose();
});

test("写请求必须携带 CSRF 令牌", async () => {
  const api = await request.newContext({ baseURL });
  await mutate(api, "post", "/api/v1/iam/auth/login", { username: adminUsername, password: adminPassword }, 200);
  const username = `csrf_${randomBytes(4).toString("hex")}`;
  const body = { username, display_name: "CSRF user", password: randomBytes(16).toString("hex") };

  const rejected = await api.post("/api/v1/iam/users", { data: body, headers: { Origin: baseURL } });
  expect(rejected.status()).toBe(403);

  const accepted = await api.post("/api/v1/iam/users", {
    data: body,
    headers: { Origin: baseURL, "X-CSRF-Token": await csrf(api) },
  });
  expect(accepted.status()).toBe(201);
  const created = (await accepted.json()).data.id as string;
  await mutate(api, "delete", `/api/v1/iam/users/${created}`);
  await api.dispose();
});

test("新用户登录、改密并在退出后使所有标签页失效", async ({ page, context }) => {
  const admin = await adminClient();
  const username = `member_${randomBytes(4).toString("hex")}`;
  const initial = randomBytes(20).toString("hex");
  const password = randomBytes(20).toString("hex");
  await createUser(admin, username, "浏览器测试用户", initial);
  await admin.dispose();

  await browserLogin(page, username, initial);
  await expect(page.getByRole("heading", { name: "安全运营工作台", exact: true })).toBeVisible();

  // 没有角色的账号只有个人资料：管理页不出现在设置导航里，直接访问也会被拒绝。
  await page.goto("/settings");
  const settingsNav = page.getByRole("navigation", { name: "设置导航" });
  await expect(settingsNav.getByRole("link", { name: "用户管理", exact: true })).toHaveCount(0);
  await page.goto("/settings/users");
  await expect(page.getByText("当前账号没有访问该页面的权限", { exact: true })).toBeVisible();

  await page.goto("/settings");
  await page.getByLabel("当前密码", { exact: true }).fill(initial);
  await page.getByLabel("新密码", { exact: true }).fill(password);
  await page.getByLabel("确认新密码", { exact: true }).fill(password);
  await page.getByRole("button", { name: "修改密码并退出", exact: true }).click();
  await expect(page).toHaveURL(/\/login$/);

  await browserLogin(page, username, password);

  const other = await context.newPage();
  await other.goto("/dashboard");
  await expect(other.getByRole("heading", { name: "安全运营工作台", exact: true })).toBeVisible();
  const stored = await page.evaluate(() =>
    Object.keys(localStorage).filter((key) => /token|session|userStatus|userRole/i.test(key)),
  );
  expect(stored).toEqual([]);

  await page.getByLabel("账户菜单", { exact: true }).click();
  await page.getByRole("menuitem", { name: "退出登录", exact: true }).click();
  await expect(page).toHaveURL(/\/login$/);
  await expect(other).toHaveURL(/\/login$/);
  await other.close();
});

// 创建用户走一次真实表单：账号、邮箱、两次密码；角色在管理抽屉里勾选保存。
test("用户管理：创建账号、分配角色并验证权限边界", async ({ page, browser }) => {
  const admin = await adminClient();
  const suffix = randomBytes(4).toString("hex");
  const roleName = `只读用户 ${suffix}`;
  await createRole(admin, roleName, ["iam.users.read"]);

  await browserLogin(page, adminUsername, adminPassword);
  await page.goto("/settings/users");
  await page.getByRole("button", { name: "添加用户", exact: true }).click();
  const createSheet = page.getByRole("dialog");
  const username = `managed_${suffix}`;
  const password = randomBytes(16).toString("hex");
  await createSheet.getByLabel("用户名", { exact: true }).fill(username);
  await createSheet.getByLabel("显示名称", { exact: true }).fill("角色用户");
  await createSheet.getByLabel("联系邮箱", { exact: true }).fill(`${username}@example.com`);
  await createSheet.getByLabel("密码", { exact: true }).fill(password);
  await createSheet.getByLabel("确认密码", { exact: true }).fill(password);
  await createSheet.getByRole("button", { name: "确定", exact: true }).click();
  await expect(createSheet).toHaveCount(0);

  // 列表分页按 10 条，新账号不一定落在第一页：用服务端搜索定位。
  await page.getByPlaceholder("搜索用户名或显示名", { exact: true }).fill(username);
  await page.getByRole("button", { name: "查询", exact: true }).click();
  const row = page.getByRole("row", { name: new RegExp(username) });
  await expect(row).toBeVisible();
  await row.getByRole("button", { name: "管理", exact: true }).click();

  const drawer = page.getByRole("dialog");
  await expect(drawer.getByText("角色用户", { exact: true })).toBeVisible();
  await drawer.getByRole("checkbox", { name: roleName, exact: true }).check();
  await drawer.getByRole("button", { name: "保存", exact: true }).click();
  // 抽屉是模态的：打开时背后的表格对可访问性树不可见，因此先关闭再断言列表行。
  await drawerButton(drawer, "关闭").click();
  await expect(drawer).toHaveCount(0);
  await expect(row.getByText(roleName, { exact: true })).toBeVisible();

  const memberContext = await browser.newContext();
  const memberPage = await memberContext.newPage();
  await browserLogin(memberPage, username, password);
  await memberPage.goto("/settings");
  const memberNav = memberPage.getByRole("navigation", { name: "设置导航" });
  await expect(memberNav.getByRole("link", { name: "用户管理", exact: true })).toBeVisible();
  await expect(memberNav.getByRole("link", { name: "角色与权限", exact: true })).toHaveCount(0);
  await memberPage.goto("/settings/roles");
  await expect(memberPage.getByText("当前账号没有访问该页面的权限", { exact: true })).toBeVisible();
  await memberContext.close();
  await admin.dispose();
});

// 内置角色来自接口数据：第一行正常渲染，名称旁有「内置」徽标，只能只读查看。
test("内置超级管理员角色只读且不可删除", async ({ page }) => {
  await browserLogin(page, adminUsername, adminPassword);
  await page.goto("/settings/roles");

  const row = page.getByRole("row", { name: /超级管理员/ }).first();
  await expect(row).toBeVisible();
  await expect(row.getByText("内置", { exact: true })).toBeVisible();
  const remove = row.getByRole("button", { name: "删除", exact: true });
  await expect(remove).toBeDisabled();
  await expect(remove).toHaveAttribute("title", "内置角色不可删除");

  await row.getByRole("button", { name: "编辑", exact: true }).click();
  const drawer = page.getByRole("dialog");
  await expect(drawer.getByText("iam.users.read", { exact: true })).toBeVisible();
  await expect(drawer.getByRole("button", { name: "保存权限", exact: true })).toHaveCount(0);
  await expect(drawer.getByRole("button", { name: "保存角色信息", exact: true })).toHaveCount(0);
  await drawerButton(drawer, "关闭").click();
  await expect(drawer).toHaveCount(0);
});

test("角色管理：创建角色、勾选权限并可删除", async ({ page }) => {
  await browserLogin(page, adminUsername, adminPassword);
  await page.goto("/settings/roles");
  const name = `审计观察员 ${randomBytes(3).toString("hex")}`;

  await page.getByRole("button", { name: "创建角色", exact: true }).click();
  const drawer = page.getByRole("dialog");
  await drawer.getByLabel("角色名称", { exact: true }).fill(name);
  await drawer.getByLabel("说明", { exact: true }).fill("只读审计");
  // 权限键是契约标识，不依赖权限描述文案。
  await drawer.locator('[id="permission-iam.audit.read"]').click();
  await drawer.getByRole("button", { name: "创建角色", exact: true }).click();
  await expect(drawer).toHaveCount(0);

  await page.getByPlaceholder("搜索角色名称", { exact: true }).fill(name);
  await page.getByRole("button", { name: "查询", exact: true }).click();
  const row = page.getByRole("row", { name: new RegExp(name) });
  await expect(row).toBeVisible();
  await row.getByRole("button", { name: "删除", exact: true }).click();
  const confirm = page.getByRole("dialog");
  await confirm.getByRole("button", { name: "确定", exact: true }).click();
  await expect(page.getByRole("row", { name: new RegExp(name) })).toHaveCount(0);
});

test("配置管理的账号不能删除或停用，密码只能由配置更新", async ({ page }) => {
  await browserLogin(page, adminUsername, adminPassword);
  await page.goto("/settings/users");
  await page.getByPlaceholder("搜索用户名或显示名", { exact: true }).fill(adminUsername);
  await page.getByRole("button", { name: "查询", exact: true }).click();

  const row = page.getByRole("row", { name: new RegExp(adminUsername) });
  await row.getByRole("button", { name: "管理", exact: true }).click();
  const drawer = page.getByRole("dialog");
  const disable = drawer.getByRole("button", { name: "停用", exact: true });
  await expect(disable).toBeDisabled();
  await expect(disable).toHaveAttribute("title", "由配置管理");
  const remove = drawer.getByRole("button", { name: "删除用户", exact: true });
  await expect(remove).toBeDisabled();
  await expect(remove).toHaveAttribute("title", "由配置管理");
  await expect(drawer.getByLabel("新密码", { exact: true })).toHaveCount(0);
});

test("审计页展示扁平事件列，不再有组织列与筛选", async ({ page }) => {
  await browserLogin(page, adminUsername, adminPassword);
  await page.goto("/settings/audit");

  await expect(page.getByRole("columnheader", { name: "时间", exact: true })).toBeVisible();
  await expect(page.getByRole("columnheader", { name: "操作者", exact: true })).toBeVisible();
  await expect(page.getByRole("columnheader", { name: "请求 ID", exact: true })).toBeVisible();
  await expect(page.getByRole("columnheader", { name: "组织", exact: true })).toHaveCount(0);
  await expect(page.getByText("组织", { exact: true })).toHaveCount(0);

  const dataRow = page
    .locator("tbody tr")
    .filter({ has: page.locator("td:nth-child(5):not(:empty)") })
    .first();
  await expect(dataRow).toBeVisible({ timeout: 20000 });
});

// 时区只影响显示：切换后同一记录的时间必须随之改变，且不依赖浏览器时区。
test("展示时区可切换并在刷新与清除本地缓存后保持", async ({ page, context }) => {
  await browserLogin(page, adminUsername, adminPassword);

  await page.goto("/settings");
  const selector = page.getByRole("combobox", { name: "时区", exact: true });
  await expect(selector).toBeVisible();

  // 时区只影响显示：同一条审计记录在不同时区下渲染的时间必须不同。
  // 切换时区本身会写入新审计，所以必须按 request_id 锁定同一条记录再比较。
  const auditRows = async () => {
    await page.goto("/settings/audit");
    const dataRow = page
      .locator("tbody tr")
      .filter({ has: page.locator("td:nth-child(5):not(:empty)") })
      .first();
    await expect(dataRow).toBeVisible({ timeout: 20000 });
    return page.locator("tbody tr").evaluateAll((rows) =>
      rows
        .map((row) => {
          const cells = row.querySelectorAll("td");
          return { time: (cells[0]?.textContent ?? "").trim(), requestID: (cells[4]?.textContent ?? "").trim() };
        })
        .filter((row) => row.requestID !== ""),
    );
  };

  const setZone = async (label: string) => {
    await page.goto("/settings");
    await selector.click();
    // 时区候选按输入过滤后截断渲染，必须先输入才能在列表中命中靠后的时区。
    if (label !== "UTC" && label !== "跟随浏览器") await selector.fill(label);
    const saved = page.waitForResponse(
      (response) => response.url().includes("/api/v1/iam/me") && response.request().method() === "PATCH",
    );
    await page.locator('[data-slot="combobox-item"]').filter({ hasText: label }).first().click();
    expect((await saved).status(), `PATCH /iam/me for ${label}`).toBe(200);
    await expect(page.getByRole("combobox", { name: "时区", exact: true })).toHaveValue(label);
  };

  const browserZone = await page.evaluate(() => Intl.DateTimeFormat().resolvedOptions().timeZone);
  expect(browserZone).not.toBe("UTC");

  await setZone("UTC");
  const utcRows = await auditRows();
  expect(utcRows.length).toBeGreaterThan(0);
  const anchor = utcRows[0];
  expect(anchor.time).not.toBe("");

  await setZone("Asia/Shanghai");

  // 偏好镜像 cookie 由身份刷新同步，等待它与已保存偏好一致后再断言渲染。
  await expect
    .poll(() => page.evaluate(() => document.cookie.includes("spx-timezone-preference=Asia%2FShanghai")), {
      timeout: 20000,
    })
    .toBe(true);
  const savedZone = await page.evaluate(async () => ({
    timezone: (await fetch("/api/v1/iam/me", { credentials: "include" }).then((response) => response.json())).data
      ?.timezone,
    cookie: document.cookie,
  }));
  expect(savedZone.timezone, "saved preference after choosing Asia/Shanghai").toBe("Asia/Shanghai");

  const shanghaiAnchor = (await auditRows()).find((row) => row.requestID === anchor.requestID);
  expect(shanghaiAnchor, `record ${anchor.requestID} missing after zone change`).toBeDefined();
  expect(shanghaiAnchor?.time, `rendered with cookie ${savedZone.cookie}`).not.toBe(anchor.time);

  await page.reload();
  const reloaded = (await auditRows()).find((row) => row.requestID === anchor.requestID);
  expect(reloaded?.time).toBe(shanghaiAnchor?.time);

  // 偏好真源是数据库：清掉本地镜像 cookie 后首屏仍按已保存时区渲染。
  await context.clearCookies({ name: "spx-timezone-preference" });
  await page.reload();
  const restored = (await auditRows()).find((row) => row.requestID === anchor.requestID);
  expect(restored?.time).toBe(shanghaiAnchor?.time);
});
