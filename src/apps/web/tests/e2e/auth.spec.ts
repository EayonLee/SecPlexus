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
async function recordAction(page: Page, row: ReturnType<Page["getByRole"]>, name: string) {
  const trigger = row.getByRole("button", { name: /的操作$/ });
  await trigger.click();
  await page.getByRole("menuitem", { name, exact: true }).click();
  return trigger;
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

  // 旧入口重定向到个人资料。没有角色的账号直接访问管理页仍被拒绝。
  const settings = await page.request.get("/settings", { maxRedirects: 0 });
  expect(settings.status()).toBe(308);
  expect(settings.headers().location).toBe("/settings/profile");
  expect((await page.request.get("/profile", { maxRedirects: 0 })).status()).toBe(404);
  await page.getByRole("button", { name: "账户菜单", exact: true }).click();
  const settingsEntry = page.getByRole("menuitem", { name: "系统设置", exact: true });
  await expect(settingsEntry).toHaveAttribute("href", "/settings/profile");
  await settingsEntry.press("Enter");
  await expect(page).toHaveURL(/\/settings\/profile$/);
  await page.goto("/settings");
  await expect(page).toHaveURL(/\/settings\/profile$/);
  const settingsNav = page.getByRole("navigation", { name: "系统设置导航" });
  await expect(settingsNav.getByRole("link", { name: "用户管理", exact: true })).toHaveCount(0);
  await page.goto("/settings/users");
  await expect(page.getByText("当前账号没有访问该页面的权限", { exact: true })).toBeVisible();

  await page.goto("/settings/profile");
  await page.getByRole("tab", { name: "密码与登录", exact: true }).click();
  await page.getByRole("button", { name: "更改密码", exact: true }).click();
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

  await page.goto("/settings/profile");
  await page.getByRole("button", { name: "个人资料操作", exact: true }).click();
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
  const pager = page.getByRole("navigation", { name: "分页导航", exact: true });
  await expect(pager.getByRole("button", { name: "首页", exact: true })).toBeDisabled();
  await expect(page.getByText("已选择 0 项", { exact: true })).toHaveCount(0);
  await page.getByRole("searchbox").press("Tab");
  await expect(page.getByRole("combobox", { name: "角色", exact: true })).toBeFocused();
  await page.getByRole("button", { name: "添加用户", exact: true }).click();
  const createSheet = page.getByRole("dialog");
  await expect(createSheet.getByLabel("用户名", { exact: true })).toBeFocused();
  const viewport = page.viewportSize();
  if (!viewport) throw new Error("Expected a fixed browser viewport");
  await expect.poll(async () => (await createSheet.boundingBox())?.width ?? 0).toBeCloseTo(512, 0);
  await page.keyboard.press("Escape");
  const addUser = page.getByRole("button", { name: "添加用户", exact: true });
  await expect(addUser).toBeFocused();
  await addUser.click();
  const username = `managed_${suffix}`;
  const password = randomBytes(16).toString("hex");
  await createSheet.getByLabel("用户名", { exact: true }).fill(username);
  await createSheet.getByLabel("显示名称", { exact: true }).fill("角色用户");
  await createSheet.getByLabel("联系邮箱", { exact: true }).fill(`${username}@example.com`);
  await createSheet.getByLabel("密码", { exact: true }).fill(password);
  await createSheet.getByLabel("确认密码", { exact: true }).fill(password);
  await createSheet.getByRole("button", { name: "创建新用户", exact: true }).click();
  await expect(createSheet).toHaveCount(0);

  // 列表分页按 10 条，新账号不一定落在第一页：用服务端搜索定位。
  await page.getByPlaceholder("搜索用户名或显示名", { exact: true }).fill(username);
  await page.getByRole("searchbox").press("Enter");
  const row = page.getByRole("row", { name: new RegExp(username) });
  await expect(row).toBeVisible();
  const status = page.getByRole("combobox", { name: "状态", exact: true });
  await status.focus();
  await status.press("Enter");
  const filteredUsers = page.waitForResponse(
    (response) =>
      new URL(response.url()).pathname === "/api/v1/iam/users" &&
      new URL(response.url()).searchParams.get("status") === "active",
  );
  await page.getByRole("option", { name: "启用", exact: true }).press("Enter");
  expect((await filteredUsers).status()).toBe(200);
  await row.getByRole("checkbox", { name: "选择 角色用户", exact: true }).check();
  await expect(page.getByText("已选择 1 项", { exact: true })).toBeVisible();
  const exported = page.waitForEvent("download");
  await page.getByRole("button", { name: "导出 CSV", exact: true }).click();
  expect((await exported).suggestedFilename()).toBe("secplexus-users.csv");
  await page.getByRole("tab", { name: "卡片视图", exact: true }).click();
  await expect(page.getByRole("table")).toHaveCount(0);
  await expect(page.getByRole("checkbox", { name: "选择 角色用户", exact: true })).toBeChecked();
  await page.getByRole("tab", { name: "列表视图", exact: true }).click();
  await expect(row).toBeVisible();
  await row.getByRole("checkbox", { name: "选择 角色用户", exact: true }).uncheck();
  await expect(page.getByText(/^已选择/)).toHaveCount(0);
  const opener = row.getByRole("button", { name: /的操作$/ });
  await opener.focus();
  await opener.press("Enter");
  await page.getByRole("menuitem", { name: "查看详情", exact: true }).press("Enter");
  const drawer = page.getByRole("dialog");
  await expect(drawer.getByText(`${username}@example.com`, { exact: true })).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(opener).toBeFocused();

  await recordAction(page, row, "编辑用户");
  await expect(drawer.getByLabel("显示名称", { exact: true })).toHaveValue("角色用户");
  await expect.poll(async () => (await drawer.boundingBox())?.width ?? 0).toBeCloseTo(Math.min(viewport.width, 512), 0);
  await drawer.getByLabel("显示名称", { exact: true }).fill("更新后的角色用户");
  const saveDetails = drawer.getByRole("button", { name: "保存", exact: true }).first();
  await saveDetails.click();
  await expect(saveDetails).toBeDisabled();
  await drawer.getByRole("button", { name: "停用", exact: true }).click();
  await expect(drawer.getByRole("button", { name: "启用", exact: true })).toBeEnabled();
  await drawer.getByRole("button", { name: "启用", exact: true }).click();
  await expect(drawer.getByRole("button", { name: "停用", exact: true })).toBeEnabled();
  await drawer.getByRole("checkbox", { name: roleName, exact: true }).check();
  const saveRoles = drawer.getByRole("button", { name: "保存", exact: true }).last();
  const roleSaved = page.waitForResponse(
    (response) => response.request().method() === "PUT" && response.url().endsWith("/roles"),
  );
  await saveRoles.click();
  expect((await roleSaved).ok()).toBe(true);
  const reset = randomBytes(16).toString("hex");
  await drawer.getByLabel("新密码", { exact: true }).fill(reset);
  await drawer.getByLabel("确认密码", { exact: true }).fill(reset);
  await drawer.getByRole("button", { name: "重置密码", exact: true }).click();
  await expect(drawer.getByLabel("新密码", { exact: true })).toHaveValue("");
  // 抽屉是模态的：打开时背后的表格对可访问性树不可见，因此先关闭再断言列表行。
  await drawerButton(drawer, "关闭").click();
  await expect(drawer).toHaveCount(0);
  await expect(row.getByText(roleName, { exact: true })).toBeVisible();
  await expect(row.getByText("更新后的角色用户", { exact: true })).toBeVisible();

  await page.goto("/settings/roles");
  await page.getByPlaceholder("搜索角色名称", { exact: true }).fill(roleName);
  await page.getByRole("searchbox").press("Enter");
  await page
    .getByRole("row", { name: new RegExp(roleName) })
    .getByRole("button", { name: /的操作$/ })
    .click();
  await expect(page.getByRole("menuitem", { name: "删除", exact: true })).toBeDisabled();

  const memberContext = await browser.newContext();
  const memberPage = await memberContext.newPage();
  await browserLogin(memberPage, username, reset);
  await memberPage.goto("/settings/profile");
  const memberNav = memberPage.getByRole("navigation", { name: "系统设置导航" });
  await expect(memberNav.getByRole("link", { name: "用户管理", exact: true })).toBeVisible();
  await expect(memberNav.getByRole("link", { name: "角色与权限", exact: true })).toHaveCount(0);
  await memberPage.goto("/settings/users");
  await expect(memberPage.getByRole("button", { name: "添加用户", exact: true })).toHaveCount(0);
  await memberPage.getByPlaceholder("搜索用户名或显示名", { exact: true }).fill(username);
  await memberPage.getByPlaceholder("搜索用户名或显示名", { exact: true }).press("Enter");
  await recordAction(memberPage, memberPage.getByRole("row", { name: new RegExp(username) }), "查看详情");
  await expect(
    memberPage
      .getByRole("dialog")
      .getByRole("definition")
      .filter({ hasText: /^更新后的角色用户$/ }),
  ).toBeVisible();
  await expect(memberPage.getByRole("dialog").getByRole("button", { name: "保存", exact: true })).toHaveCount(0);
  await memberPage.goto("/settings/roles");
  await expect(memberPage.getByText("当前账号没有访问该页面的权限", { exact: true })).toBeVisible();
  await memberContext.close();

  await page.goto("/settings/users");
  await page.getByPlaceholder("搜索用户名或显示名", { exact: true }).fill(username);
  await page.getByPlaceholder("搜索用户名或显示名", { exact: true }).press("Enter");
  await recordAction(page, row, "删除用户");
  const deleteUser = page.getByRole("alertdialog");
  await expect(deleteUser.getByRole("button", { name: "取消", exact: true })).toBeFocused();
  await expect(deleteUser.getByText(/更新后的角色用户/)).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(row.getByRole("button", { name: /的操作$/ })).toBeFocused();
  await recordAction(page, row, "删除用户");
  await deleteUser.getByRole("button", { name: "删除", exact: true }).click();
  await expect(row).toHaveCount(0);
  await expect(page.getByText("没有匹配的记录", { exact: true })).toBeVisible();
  await admin.dispose();
});

// 内置角色来自接口数据：第一行正常渲染，名称旁有「内置」徽标，只能只读查看。
test("内置超级管理员角色只读且不可删除", async ({ page }) => {
  await browserLogin(page, adminUsername, adminPassword);
  await page.goto("/settings/roles");

  const row = page.getByRole("row", { name: /超级管理员/ }).first();
  await expect(row).toBeVisible();
  await expect(page.getByRole("rowgroup", { name: "内置角色" }).getByRole("row", { name: /超级管理员/ })).toBeVisible();
  await row.getByRole("button", { name: /的操作$/ }).click();
  const remove = page.getByRole("menuitem", { name: "删除", exact: true });
  await expect(remove).toBeDisabled();
  await expect(remove).toHaveAttribute("title", "内置角色不可删除");

  await expect(page.getByRole("menuitem", { name: "编辑角色", exact: true })).toHaveCount(0);
  await expect(page.getByRole("menuitem", { name: "查看权限", exact: true })).toHaveCount(0);
  await page.getByRole("menuitem", { name: "查看详情", exact: true }).click();
  const drawer = page.getByRole("dialog");
  await expect(drawer.getByText("iam.users.read", { exact: true })).toBeVisible();
  await expect(drawer.getByRole("button", { name: "保存权限", exact: true })).toHaveCount(0);
  await expect(drawer.getByRole("button", { name: "保存角色信息", exact: true })).toHaveCount(0);
  await drawer.getByRole("button", { name: "关闭", exact: true }).click();
  await expect(drawer).toHaveCount(0);
});

test("角色管理：创建角色、勾选权限并可删除", async ({ page }) => {
  await browserLogin(page, adminUsername, adminPassword);
  await page.goto("/settings/roles");
  const name = `审计观察员 ${randomBytes(3).toString("hex")}`;

  const create = page.getByRole("button", { name: "创建角色", exact: true });
  await expect(create).toBeVisible();
  await expect(page.getByRole("button", { name: "导入 JSON", exact: true })).toHaveCount(0);
  await create.click();
  const drawer = page.getByRole("dialog");
  await drawer.getByLabel("角色名称", { exact: true }).fill(name);
  await drawer.getByLabel("说明", { exact: true }).fill("只读审计");
  await drawer.getByRole("checkbox", { name: "查看审计事件", exact: true }).check();
  await drawer.getByRole("button", { name: "创建角色", exact: true }).click();
  await expect(drawer).toHaveCount(0);

  const rolesFilter = page.getByRole("combobox", { name: "类型", exact: true });
  await rolesFilter.press("Enter");
  const filteredRoles = page.waitForResponse(
    (response) =>
      new URL(response.url()).pathname === "/api/v1/iam/roles" &&
      new URL(response.url()).searchParams.get("type") === "custom",
  );
  await page.getByRole("option", { name: "自定义角色", exact: true }).press("Enter");
  expect((await filteredRoles).status()).toBe(200);
  await expect(page.getByRole("rowgroup", { name: "内置角色" })).toHaveCount(0);
  await page.getByPlaceholder("搜索角色名称", { exact: true }).fill(name);
  await page.getByRole("searchbox").press("Enter");
  const row = page.getByRole("row", { name: new RegExp(name) });
  await expect(row).toBeVisible();
  const opener = await recordAction(page, row, "查看详情");
  await expect(drawer.getByText("只读审计", { exact: true })).toBeVisible();
  await drawer.getByRole("button", { name: "关闭", exact: true }).click();
  await expect(opener).toBeFocused();
  await recordAction(page, row, "编辑角色");
  await drawer.getByLabel("说明", { exact: true }).fill("审计和用户目录");
  const saveRole = drawer.getByRole("button", { name: "保存角色信息", exact: true });
  await saveRole.click();
  await expect(saveRole).toBeDisabled();
  await drawer.locator('[id="permission-iam.users.read"]').click();
  const savePermissions = drawer.getByRole("button", { name: "保存权限", exact: true });
  const grantsRequest = "**/api/v1/iam/roles/*/permissions";
  const release = Promise.withResolvers<void>();
  await page.route(grantsRequest, async (route) => {
    await release.promise;
    await route.continue();
  });
  const granted = page.waitForResponse(
    (response) => response.request().method() === "PUT" && response.url().endsWith("/permissions"),
  );
  await savePermissions.click();
  try {
    await expect(savePermissions).toBeDisabled();
  } finally {
    release.resolve();
  }
  expect((await granted).status()).toBe(204);
  await page.unroute(grantsRequest);
  await drawerButton(drawer, "关闭").click();
  await expect(row.getByText("审计和用户目录", { exact: true })).toBeVisible();
  await recordAction(page, row, "查看详情");
  await expect(drawer.getByText("iam.users.read", { exact: true })).toBeVisible();
  await drawer.getByRole("button", { name: "关闭", exact: true }).click();
  await recordAction(page, row, "删除");
  const confirm = page.getByRole("alertdialog");
  await expect(confirm.getByText(new RegExp(name))).toBeVisible();
  await confirm.getByRole("button", { name: "删除", exact: true }).click();
  await expect(page.getByRole("row", { name: new RegExp(name) })).toHaveCount(0);
});

test("配置管理的账号不能删除或停用，密码只能由配置更新", async ({ page }) => {
  await browserLogin(page, adminUsername, adminPassword);
  await page.goto("/settings/users");
  await page.getByPlaceholder("搜索用户名或显示名", { exact: true }).fill(adminUsername);
  await page.getByRole("searchbox").press("Enter");

  const row = page.getByRole("row", { name: new RegExp(adminUsername) });
  const opener = row.getByRole("button", { name: /的操作$/ });
  await opener.click();
  const remove = page.getByRole("menuitem", { name: "删除用户", exact: true });
  await expect(remove).toBeDisabled();
  await expect(remove).toHaveAttribute("title", "由配置管理");
  await page.getByRole("menuitem", { name: "编辑用户", exact: true }).click();
  const drawer = page.getByRole("dialog");
  const disable = drawer.getByRole("button", { name: "停用", exact: true });
  await expect(disable).toBeDisabled();
  await expect(disable).toHaveAttribute("title", "由配置管理");
  await expect(drawer.getByLabel("新密码", { exact: true })).toHaveCount(0);
  await expect(drawer.getByRole("textbox", { name: "显示名称", exact: true })).toHaveCount(0);
  await page.goto("/settings/profile");
  await page.getByRole("tab", { name: "密码与登录", exact: true }).click();
  const lock = page.getByRole("button", { name: "密码由系统配置管理", exact: true });
  await lock.focus();
  await expect(page.getByRole("tooltip")).toContainText("请在 IAM 配置中更新密码");
  await expect(page.getByRole("button", { name: "更改密码", exact: true })).toHaveCount(0);
});

test("审计页展示扁平事件列，不再有组织列与筛选", async ({ page }) => {
  await browserLogin(page, adminUsername, adminPassword);
  await page.goto("/settings/audit");

  await expect(page.getByRole("columnheader", { name: "时间", exact: true })).toBeVisible();
  await expect(page.getByRole("columnheader", { name: "操作者", exact: true })).toBeVisible();
  await expect(page.getByRole("columnheader", { name: "请求 ID", exact: true })).toBeVisible();
  await expect(page.getByRole("columnheader", { name: "组织", exact: true })).toHaveCount(0);
  await expect(page.getByText("组织", { exact: true })).toHaveCount(0);

  const outcome = page.getByRole("combobox", { name: "结果", exact: true });
  await outcome.click();
  await page.getByRole("option", { name: "成功", exact: true }).click();
  const filtered = page.waitForResponse(
    (response) =>
      response.url().includes("/audit") && new URL(response.url()).searchParams.get("outcome") === "success",
  );
  await page.getByRole("button", { name: "查询", exact: true }).click();
  expect((await filtered).ok()).toBeTruthy();
  await page.getByRole("button", { name: "重置", exact: true }).click();
  await expect(outcome).toHaveText("结果：全部");

  const dataRow = page
    .locator("tbody tr")
    .filter({ has: page.locator("td:nth-child(5):not(:empty)") })
    .first();
  await expect(dataRow).toBeVisible({ timeout: 20000 });
});

// 时区只影响显示：切换后同一记录的时间必须随之改变，且不依赖浏览器时区。
test("展示时区可切换并在刷新与清除本地缓存后保持", async ({ page, context }) => {
  await browserLogin(page, adminUsername, adminPassword);

  const opener = page.getByRole("button", { name: "更改时区", exact: true });
  const selector = page.getByRole("combobox", { name: "时区", exact: true });

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
    await page.goto("/settings/profile");
    await opener.click();
    await expect(selector).toBeFocused();
    await selector.fill(label);
    await expect(page.getByRole("option", { name: label, exact: true })).toBeVisible();
    await selector.press("ArrowDown");
    await selector.press("Enter");
    await expect(selector).toHaveValue(label);
    const save = page.getByRole("dialog").getByRole("button", { name: "保存", exact: true });
    await expect(save).toBeEnabled();
    const saved = page.waitForResponse(
      (response) => response.url().includes("/api/v1/iam/me") && response.request().method() === "PATCH",
    );
    await save.click();
    expect((await saved).status(), `PATCH /iam/me for ${label}`).toBe(200);
    await expect(page.getByRole("dialog")).toHaveCount(0);
    await expect(opener).toBeFocused();
    await expect(page.getByRole("definition").filter({ hasText: label })).toBeVisible();
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

test("管理列表保留加载、失败重试、服务端分页和筛选状态", async ({ page }) => {
  const admin = await adminClient();
  for (let index = 0; index < 13; index++) {
    await createRole(admin, `分页测试 ${index.toString().padStart(2, "0")}`, []);
  }
  await admin.dispose();
  await browserLogin(page, adminUsername, adminPassword);

  const rolesEndpoint = "**/api/v1/iam/roles?*";
  const failed = Promise.withResolvers<void>();
  await page.route(rolesEndpoint, async (route) => {
    await failed.promise;
    await route.fulfill({ status: 503 });
  });
  await page.goto("/settings/roles");
  const main = page.getByRole("main");
  await expect(main.getByRole("status")).toContainText("加载中");
  failed.resolve();
  const retry = main.getByRole("button", { name: "重试", exact: true });
  await expect(retry).toBeVisible();
  await page.unroute(rolesEndpoint);
  await retry.click();
  const next = main.getByRole("button", { name: "下一页", exact: true });
  await expect(next).toBeEnabled();
  const pageTwo = page.waitForResponse(
    (response) =>
      new URL(response.url()).pathname === "/api/v1/iam/roles" &&
      new URL(response.url()).searchParams.get("offset") === "10",
  );
  await next.click();
  expect((await pageTwo).ok()).toBe(true);

  const search = main.getByRole("searchbox");
  await search.fill("没有匹配的测试角色");
  await search.press("Enter");
  await expect(main.getByText("没有匹配的记录", { exact: true })).toBeVisible();
  await main.getByRole("button", { name: "清除筛选", exact: true }).click();
  await expect(search).toHaveValue("");
  await expect(main.getByRole("table").getByRole("row", { name: /超级管理员/ })).toBeVisible();
});

test("四个系统设置页在明暗主题和窄屏下保留操作入口", async ({ page }) => {
  await browserLogin(page, adminUsername, adminPassword);
  for (const width of [1440, 820, 390]) {
    await page.setViewportSize({ width, height: 844 });
    for (const theme of ["浅色", "深色"]) {
      await page.goto("/settings/profile");
      const appearance = page.getByRole("button", { name: "主题设置", exact: true });
      await appearance.click();
      await page.getByLabel(theme, { exact: true }).click();
      await page.keyboard.press("Escape");
      await expect(page.locator("html")).toHaveAttribute("data-theme-mode", theme === "深色" ? "dark" : "light");

      if (width >= 768) {
        await expect(page.getByRole("navigation", { name: "系统设置导航" })).toBeVisible();
      }
      const breadcrumb = page.getByRole("navigation", { name: "页面路径", exact: true });
      await expect(breadcrumb.getByText("系统设置", { exact: true })).toBeVisible();
      await expect(breadcrumb.getByRole("link", { name: "系统设置", exact: true })).toHaveCount(0);
      await expect(breadcrumb.locator('[aria-current="page"]')).toHaveText("个人资料");
      const zone = page.getByRole("button", { name: "更改时区", exact: true });
      await zone.click();
      const dialog = page.getByRole("dialog");
      await expect(dialog.getByRole("combobox", { name: "时区" })).toBeFocused();
      await page.keyboard.press("Escape");
      await expect(zone).toBeFocused();

      for (const [path, label, record] of [
        ["/settings/users", "用户管理", new RegExp(adminUsername)],
        ["/settings/roles", "角色与权限", /超级管理员/],
      ] as const) {
        await page.goto(path);
        await expect(breadcrumb.getByText("系统设置", { exact: true })).toBeVisible();
        await expect(breadcrumb.locator('[aria-current="page"]')).toHaveText(label);
        const main = page.getByRole("main");
        const row = main.getByRole("row", { name: record });
        const menu = row.getByRole("button", { name: /的操作$/ });
        await expect(menu).toBeInViewport();
        await expect(row.getByText(record).first()).toBeInViewport();
        await expect(main.getByRole("searchbox")).toBeInViewport();
        await expect.poll(() => main.evaluate((node) => node.scrollWidth <= node.clientWidth + 1)).toBe(true);
        await recordAction(page, row, "查看详情");
        await expect.poll(async () => (await dialog.boundingBox())?.width).toBeCloseTo(Math.min(width, 512), 0);
        await expect(dialog.getByRole("button", { name: "关闭", exact: true })).toBeInViewport();
        await page.keyboard.press("Escape");
        await expect(menu).toBeFocused();
      }

      await page.goto("/settings/audit");
      await expect(breadcrumb.getByText("系统设置", { exact: true })).toBeVisible();
      await expect(breadcrumb.locator('[aria-current="page"]')).toHaveText("审计日志");
      await expect(page.getByRole("heading", { name: "审计日志", exact: true })).toBeVisible();
      await expect(page.getByRole("columnheader", { name: "操作", exact: true })).toBeInViewport();
    }
  }
});
