import { type APIRequestContext, expect, type Page, request, test } from "@playwright/test";
import { z } from "zod";

import { type ApiRole, zApiMe } from "../../src/spx/api/iam/schemas";
import { execFileSync } from "node:child_process";
import { randomBytes } from "node:crypto";
import { resolve } from "node:path";

declare global {
  interface Window {
    streamAuthorized: boolean;
    streamRevoked: boolean;
  }
}

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

// 只修改本次测试库的当前浏览器会话，保持共享 API fixture 的登录有效性。
async function expireAuthentication(page: Page) {
  const response = await page.request.get("/api/v1/iam/me");
  expect(response.status()).toBe(200);
  const identity = zApiMe.parse((await response.json()).data);
  execFileSync(
    "go",
    [
      "run",
      "./tests/integration/database",
      "expire-auth",
      process.env.SPX_E2E_DATABASE ?? "",
      identity.principal.session_id,
    ],
    { cwd: resolve(process.cwd(), "../../.."), env: process.env, stdio: "pipe" },
  );
}

// 外部站点统计不属于身份链路，避免第三方网络阻塞浏览器启动。
test.beforeEach(async ({ context }) => {
  await context.route("https://www.googletagmanager.com/**", (route) => route.abort());
});

test.beforeAll(async () => {
  const api = await adminClient();
  await api.dispose();
});

test("新用户直接登录并可主动改密，组织权限贯穿 Web、Console 与两个 IAM 实例", async ({ page }) => {
  const admin = await adminClient();
  const me = (await (await admin.get("/api/v1/iam/me")).json()).data;
  const org = me.organizations[0].id;
  const username = `member_${randomBytes(4).toString("hex")}`;
  const initial = randomBytes(20).toString("hex");
  const password = randomBytes(20).toString("hex");
  const member = await mutate(
    admin,
    "post",
    `/api/v1/iam/orgs/${org}/members`,
    { username, display_name: "浏览器测试成员", password: initial },
    201,
  );
  const role = await mutate(
    admin,
    "post",
    `/api/v1/iam/orgs/${org}/roles`,
    { code: `reader_${randomBytes(4).toString("hex")}`, name: "组织访问" },
    201,
  );
  await mutate(admin, "put", `/api/v1/iam/orgs/${org}/roles/${role.id}/permissions`, {
    permissions: ["iam.organizations.read"],
    version: "1",
  });
  const organization = (await (await admin.get(`/api/v1/iam/orgs/${org}`)).json()).data;
  await mutate(admin, "put", `/api/v1/iam/orgs/${org}/members/${member.id}/roles`, {
    role_ids: [role.id],
    authz_version: organization.authz_version,
  });
  await browserLogin(page, username, initial);
  await expect(page.getByRole("heading", { name: "安全智驾工作台", exact: true })).toBeVisible();
  expect((await page.request.get(`/api/v1/console/orgs/${org}/context`)).status()).toBe(200);
  await page.goto("/settings/account");
  await expect(page).toHaveURL(/\/settings\/account$/);
  await page.getByLabel("当前密码", { exact: true }).fill(initial);
  await page.getByLabel("新密码", { exact: true }).fill(password);
  await page.getByLabel("确认新密码", { exact: true }).fill(password);
  await page.getByRole("button", { name: "修改密码并退出" }).click();
  await expect(page).toHaveURL(/\/login$/);
  await browserLogin(page, username, password);
  await expect(page.getByRole("combobox", { name: "当前组织", exact: true })).toHaveText(organization.name);
  await expect(page.getByRole("heading", { name: "安全智驾工作台", exact: true })).toBeVisible();
  expect((await page.request.get(`/api/v1/console/orgs/${org}/context`)).status()).toBe(200);
  const second = await page.request.get(`${process.env.SPX_E2E_CONSOLE_SECOND_URL}/api/v1/console/orgs/${org}/context`);
  expect(second.status()).toBe(200);
  await page.goto(`/management/platform/users`);
  await expect(page.getByText("当前账号没有访问该页面的权限", { exact: true })).toBeVisible();
  await page.goto(`/dashboard?org_id=${org}`);
  const context = (await (await page.request.get("/api/v1/iam/auth/context")).json()).data;
  const logout = await page.request.post("/api/v1/iam/auth/logout", {
    headers: { Origin: baseURL, "X-CSRF-Token": context.csrf_token },
  });
  expect(logout.status()).toBe(204);
  expect(
    (await page.request.get(`${process.env.SPX_E2E_CONSOLE_SECOND_URL}/api/v1/console/orgs/${org}/context`)).status(),
  ).toBe(401);
  await page.reload();
  await expect(page).toHaveURL(/\/login$/);
  await admin.dispose();
});

test("系统管理保留应用外壳，自定义角色可创建、配置权限和查看影响", async ({ page }) => {
  await browserLogin(page, adminUsername, adminPassword);
  await expect(page).toHaveURL(/\/dashboard$/);
  await expect(page.getByRole("combobox", { name: "当前组织", exact: true })).toBeVisible();
  const workspace = page.getByRole("navigation", { name: "主导航" });
  for (const label of ["平台成员", "组织管理", "角色与权限", "组织成员", "账号安全"])
    await expect(workspace.getByRole("link", { name: label, exact: true })).toHaveCount(0);
  await expect(workspace.getByRole("link", { name: "安全智驾工作台", exact: true })).toBeVisible();
  await page.getByRole("button", { name: "成员菜单", exact: true }).click();
  await page.getByRole("menuitem", { name: "系统管理", exact: true }).click();
  await expect(page.getByRole("navigation", { name: "管理导航" })).toBeVisible();
  await page.reload();
  await expect(page.getByRole("navigation", { name: "管理导航" })).toBeVisible();
  await page.getByRole("tab", { name: "角色与权限", exact: true }).click();
  await page.getByRole("button", { name: "创建角色", exact: true }).click();
  await page.getByPlaceholder("例如 security_auditor").fill(`audit_${randomBytes(4).toString("hex")}`);
  const roleName = `审计角色 ${randomBytes(3).toString("hex")}`;
  await page.getByRole("dialog").getByLabel("角色名称", { exact: true }).fill(roleName);
  await page.getByRole("button", { name: "确定", exact: true }).click();
  const row = page.getByRole("row").filter({ hasText: roleName });
  await expect(row).toBeVisible();
  await row.getByRole("button", { name: "查看", exact: true }).click();
  await page.getByRole("tab", { name: "权限", exact: true }).click();
  await page.getByText("查看组织审计", { exact: true }).click();
  await expect(page.getByRole("checkbox", { name: "查看组织审计", exact: true })).toBeChecked();
  await page.getByRole("button", { name: "预览变更影响", exact: true }).click();
  await expect(page.getByText(/影响 0 位成员/)).toBeVisible();
  await page.getByRole("button", { name: "保存权限", exact: true }).click();
  await expect(page.getByText("权限已保存，后续请求立即生效")).toBeVisible();
  await page.screenshot({ path: "test-results/management-role-editor.png", fullPage: true });
  await page.getByRole("dialog").getByRole("button", { name: "关闭", exact: true }).click();
  await page.getByRole("link", { name: "返回工作台", exact: true }).click();
  await expect(page.getByRole("navigation", { name: "主导航" }).getByRole("link")).toHaveCount(1);
});

test("跨标签退出清除身份，浏览器不持久化认证材料", async ({ page, context }) => {
  await browserLogin(page, adminUsername, adminPassword);
  await expect(page).toHaveURL(/\/dashboard$/);
  await expect(page.getByRole("combobox", { name: "当前组织", exact: true })).toBeVisible();
  const workspace = page.getByRole("navigation", { name: "主导航" });
  for (const label of ["平台成员", "组织管理", "角色与权限", "组织成员", "账号安全"])
    await expect(workspace.getByRole("link", { name: label, exact: true })).toHaveCount(0);
  await expect(workspace.getByRole("link", { name: "安全智驾工作台", exact: true })).toBeVisible();
  const other = await context.newPage();
  await other.goto("/dashboard");
  await other.getByRole("button", { name: "成员菜单", exact: true }).click();
  await expect(other.getByRole("menuitem", { name: "系统管理", exact: true })).toBeVisible();
  await other.keyboard.press("Escape");
  const stored = await page.evaluate(() =>
    Object.keys(localStorage).filter((key) => /token|session|userStatus|userRole/i.test(key)),
  );
  expect(stored).toEqual([]);
  await page.getByLabel("成员菜单").click();
  await page.getByRole("menuitem", { name: "退出登录", exact: true }).click();
  await expect(page).toHaveURL(/\/login$/);
  await expect(other).toHaveURL(/\/login$/);
  await other.close();
});

test("基础身份与自定义角色各自获得正确的管理导航", async ({ browser }) => {
  const admin = await adminClient();
  const me = (await (await admin.get("/api/v1/iam/me")).json()).data;
  const org = me.organizations[0].id;
  const roles: ApiRole[] = (await (await admin.get(`/api/v1/iam/orgs/${org}/roles?limit=100`)).json()).data.items;
  const orgAdmin = roles.find((role) => role.kind === "system" && role.code === "org_admin");
  if (!orgAdmin) throw new Error("Built-in organization administrator role is missing");
  for (const scenario of [
    {
      name: "普通成员",
      actions: [],
      role: undefined,
      visible: [],
      hidden: ["组织成员", "角色与权限", "组织审计", "平台成员"],
    },
    {
      name: "组织管理员",
      actions: [],
      role: orgAdmin.id,
      visible: ["组织成员", "角色与权限", "组织审计"],
      hidden: ["平台成员"],
    },
    {
      name: "自定义审计员",
      actions: ["iam.audit.read"],
      role: undefined,
      visible: ["组织审计"],
      hidden: ["组织成员", "角色与权限", "平台成员"],
    },
    {
      name: "自定义角色管理员",
      actions: ["iam.roles.read", "iam.roles.manage", "iam.roles.assign", "iam.members.read"],
      role: undefined,
      visible: ["组织成员", "角色与权限"],
      hidden: ["组织审计", "平台成员"],
    },
  ]) {
    const suffix = randomBytes(4).toString("hex");
    const username = `nav_${suffix}`;
    const initial = randomBytes(20).toString("hex");
    const password = randomBytes(20).toString("hex");
    const member = await mutate(
      admin,
      "post",
      `/api/v1/iam/orgs/${org}/members`,
      { username, display_name: scenario.name, password: initial },
      201,
    );
    let assignedRole = scenario.role;
    if (scenario.actions.length) {
      const created = await mutate(
        admin,
        "post",
        `/api/v1/iam/orgs/${org}/roles`,
        { code: `nav_${suffix}`, name: scenario.name },
        201,
      );
      await mutate(admin, "put", `/api/v1/iam/orgs/${org}/roles/${created.id}/permissions`, {
        permissions: scenario.actions,
        version: "1",
      });
      assignedRole = created.id;
    }
    if (assignedRole) {
      const organization = (await (await admin.get(`/api/v1/iam/orgs/${org}`)).json()).data;
      await mutate(admin, "put", `/api/v1/iam/orgs/${org}/members/${member.id}/roles`, {
        role_ids: [assignedRole],
        authz_version: organization.authz_version,
      });
    }
    const setup = await request.newContext({ baseURL });
    await mutate(setup, "post", "/api/v1/iam/auth/login", { username, password: initial }, 200);
    await mutate(setup, "post", "/api/v1/iam/me/password", { old_password: initial, new_password: password });
    await setup.dispose();
    const context = await browser.newContext();
    const page = await context.newPage();
    await browserLogin(page, username, password);
    await expect(page.getByRole("heading", { name: "安全智驾工作台", exact: true })).toBeVisible();
    if (!scenario.visible.length) {
      await page.getByRole("button", { name: "成员菜单", exact: true }).click();
      await expect(page.getByRole("menuitem", { name: "系统管理", exact: true })).toHaveCount(0);
      await page.keyboard.press("Escape");
      await page.goto("/management");
      await expect(page.getByText("当前账号没有访问该页面的权限", { exact: true })).toBeVisible();
      await page.goto(`/management/orgs/${org}/audit`);
      await expect(page.getByText("当前账号没有访问该页面的权限", { exact: true })).toBeVisible();
    } else {
      await page.getByRole("button", { name: "成员菜单", exact: true }).click();
      await page.getByRole("menuitem", { name: "系统管理", exact: true }).click();
      const sidebar = page.getByRole("navigation", { name: "管理导航" });
      for (const label of scenario.visible) {
        const section = label === "组织审计" ? "审计日志" : "成员与权限";
        await sidebar.getByRole("link", { name: section, exact: true }).click();
        await expect(page.getByRole("tab", { name: label, exact: true })).toBeVisible();
      }
      for (const label of scenario.hidden)
        await expect(page.getByRole("tab", { name: label, exact: true })).toHaveCount(0);
    }
    await context.close();
  }
  await admin.dispose();
});

test("长连接在组织权限撤销后停止访问", async ({ page }) => {
  const admin = await adminClient();
  const org = (await (await admin.get("/api/v1/iam/me")).json()).data.organizations[0].id;
  const suffix = randomBytes(4).toString("hex");
  const username = `stream_${suffix}`;
  const initial = randomBytes(20).toString("hex");
  const password = randomBytes(20).toString("hex");
  const member = await mutate(
    admin,
    "post",
    `/api/v1/iam/orgs/${org}/members`,
    { username, display_name: "长连接测试成员", password: initial },
    201,
  );
  const role = await mutate(
    admin,
    "post",
    `/api/v1/iam/orgs/${org}/roles`,
    { code: `stream_${suffix}`, name: "组织访问" },
    201,
  );
  await mutate(admin, "put", `/api/v1/iam/orgs/${org}/roles/${role.id}/permissions`, {
    permissions: ["iam.organizations.read"],
    version: "1",
  });
  const organization = (await (await admin.get(`/api/v1/iam/orgs/${org}`)).json()).data;
  await mutate(admin, "put", `/api/v1/iam/orgs/${org}/members/${member.id}/roles`, {
    role_ids: [role.id],
    authz_version: organization.authz_version,
  });
  const setup = await request.newContext({ baseURL });
  await mutate(setup, "post", "/api/v1/iam/auth/login", { username, password: initial }, 200);
  await mutate(setup, "post", "/api/v1/iam/me/password", { old_password: initial, new_password: password });
  await setup.dispose();
  await browserLogin(page, username, password);
  await expect(page.getByRole("heading", { name: "安全智驾工作台", exact: true })).toBeVisible();
  await page.evaluate((org) => {
    const stream = new EventSource(`/api/v1/console/orgs/${org}/events`);
    window.streamAuthorized = false;
    window.streamRevoked = false;
    stream.addEventListener("authorized", () => {
      window.streamAuthorized = true;
    });
    stream.addEventListener("access_revoked", () => {
      stream.close();
      window.streamRevoked = true;
    });
  }, org);
  await expect.poll(() => page.evaluate(() => window.streamAuthorized)).toBe(true);
  await mutate(admin, "put", `/api/v1/iam/orgs/${org}/roles/${role.id}/permissions`, { permissions: [], version: "2" });
  await expect.poll(() => page.evaluate(() => window.streamRevoked), { timeout: 31000 }).toBe(true);
  await page.goto(`/management/orgs/${org}/overview`);
  await expect(page.getByText("当前账号没有访问该页面的权限", { exact: true })).toBeVisible();
  await admin.dispose();
});

test("组织创建选择已有管理员，两个标签各自保留组织上下文", async ({ page, context }) => {
  const admin = await adminClient();
  const initial = (await (await admin.get("/api/v1/iam/me")).json()).data.organizations[0];
  const suffix = randomBytes(4).toString("hex");
  const name = `浏览器组织 ${suffix}`;
  await browserLogin(page, adminUsername, adminPassword);
  await expect(page).toHaveURL(/\/dashboard$/);
  await expect(page.getByRole("combobox", { name: "当前组织", exact: true })).toBeVisible();
  const workspace = page.getByRole("navigation", { name: "主导航" });
  for (const label of ["平台成员", "组织管理", "角色与权限", "组织成员", "账号安全"])
    await expect(workspace.getByRole("link", { name: label, exact: true })).toHaveCount(0);
  await expect(workspace.getByRole("link", { name: "安全智驾工作台", exact: true })).toBeVisible();
  await page.getByRole("button", { name: "成员菜单", exact: true }).click();
  await page.goto("/management/platform/organizations");
  await page.getByRole("button", { name: "创建组织", exact: true }).click();
  const dialog = page.getByRole("dialog");
  await dialog.getByLabel("组织名称", { exact: true }).fill(name);
  await dialog.getByPlaceholder("例如 security-team").fill(`team-${suffix}`);
  await dialog.getByRole("combobox", { name: "组织管理员", exact: true }).fill(adminUsername);
  await page
    .getByRole("option")
    .filter({ hasText: `(${adminUsername})` })
    .click();
  await dialog.getByRole("button", { name: "确定", exact: true }).click();
  await expect(page.getByRole("row").filter({ hasText: name })).toBeVisible();
  const created = (await (await admin.get("/api/v1/iam/me")).json()).data.organizations.find(
    (org: { name: string }) => org.name === name,
  );
  expect(created).toBeTruthy();
  const other = await context.newPage();
  await other.goto(`/dashboard?org_id=${initial.id}`);
  await expect(other.getByRole("heading", { name: "安全智驾工作台", exact: true })).toBeVisible();
  await page.goto(`/dashboard?org_id=${created.id}`);
  await expect(page.getByRole("heading", { name: "安全智驾工作台", exact: true })).toBeVisible();
  await expect(page.getByRole("combobox", { name: "当前组织", exact: true })).toHaveText(name);
  await expect(other.getByRole("combobox", { name: "当前组织", exact: true })).toHaveText(initial.name);
  expect((await other.request.get(`/api/v1/console/orgs/${initial.id}/context`)).status()).toBe(200);
  await other.reload();
  await expect(other).toHaveURL(new RegExp(`org_id=${initial.id}`));
  await expect(other.getByRole("heading", { name: "安全智驾工作台", exact: true })).toBeVisible();
  await other.close();
  await admin.dispose();
});

test("敏感操作在原弹框确认身份并完整保留草稿", async ({ page }) => {
  await page.emulateMedia({ reducedMotion: "reduce" });
  const admin = await adminClient();
  const me = (await (await admin.get("/api/v1/iam/me")).json()).data;
  await browserLogin(page, adminUsername, adminPassword);
  await page.goto("/management/platform/organizations");
  await page.getByRole("button", { name: "创建组织", exact: true }).click();
  const dialog = page.getByRole("dialog");
  const suffix = randomBytes(4).toString("hex");
  const name = `恢复草稿 ${suffix}`;
  const slug = `resume_${suffix}`;
  await dialog.getByLabel("组织名称", { exact: true }).fill(name);
  await dialog.getByPlaceholder("例如 security-team").fill(slug);
  await dialog.getByRole("combobox", { name: "组织管理员", exact: true }).fill(adminUsername);
  await page
    .getByRole("option")
    .filter({ hasText: `(${adminUsername})` })
    .click();
  await expireAuthentication(page);
  const rejected = page.waitForResponse(
    (response) => response.url().endsWith("/api/v1/iam/organizations") && response.request().method() === "POST",
  );
  await dialog.getByRole("button", { name: "确定", exact: true }).click();
  expect((await rejected).status()).toBe(403);
  await expect(dialog.getByRole("heading", { name: "确认当前密码", exact: true })).toBeVisible();
  await expect(dialog.getByLabel("当前密码", { exact: true })).toBeFocused();
  expect(
    await dialog.getByLabel("当前密码", { exact: true }).evaluate((element) => {
      const form = element.closest("form");
      return form ? getComputedStyle(form).animationName : undefined;
    }),
  ).toBe("none");
  await dialog.getByLabel("当前密码", { exact: true }).fill("wrong confirmation password");
  await dialog.getByRole("button", { name: "确认并继续", exact: true }).click();
  await expect(dialog.getByRole("alert")).toHaveText("当前密码不正确，请重新输入。");
  await dialog.getByRole("button", { name: "返回", exact: true }).click();
  await expect(dialog.getByLabel("组织名称", { exact: true })).toHaveValue(name);
  await expect(dialog.getByRole("combobox", { name: "组织管理员", exact: true })).toHaveValue(
    `${adminUsername} (${adminUsername})`,
  );
  await dialog.getByRole("button", { name: "确定", exact: true }).click();
  await dialog.getByLabel("当前密码", { exact: true }).fill(adminPassword);
  await dialog.getByRole("button", { name: "确认并继续", exact: true }).click();
  await expect(dialog.getByRole("heading", { name: "创建组织", exact: true })).toBeVisible();
  await expect(dialog.getByRole("button", { name: "确定", exact: true })).toBeFocused();
  await expect(dialog.getByLabel("组织名称", { exact: true })).toHaveValue(name);
  await expect(dialog.getByPlaceholder("例如 security-team")).toHaveValue(slug);
  await expect(dialog.getByRole("combobox", { name: "组织管理员", exact: true })).toHaveValue(
    `${adminUsername} (${adminUsername})`,
  );
  expect(
    (await (await admin.get("/api/v1/iam/organizations?limit=100")).json()).data.items.some(
      (org: { slug: string }) => org.slug === slug,
    ),
  ).toBe(false);
  await dialog.getByRole("button", { name: "确定", exact: true }).click();
  await expect(dialog).not.toBeVisible();
  const orgs = (await (await admin.get("/api/v1/iam/organizations?limit=100")).json()).data.items as { slug: string }[];
  expect(orgs.filter((org) => org.slug === slug)).toHaveLength(1);
  await page.goto(`/management/orgs/${me.organizations[0].id}/overview`);
  await expect(page.getByRole("tab", { name: "当前组织", exact: true })).toBeVisible();
  await admin.dispose();
});

test("设置使用独立的 Profile 和 Account 页面并保持单行菜单", async ({ page }) => {
  await browserLogin(page, adminUsername, adminPassword);
  await page.getByRole("button", { name: "成员菜单", exact: true }).click();
  await page.getByRole("menuitem", { name: "设置", exact: true }).click();
  await expect(page).toHaveURL(/\/settings\/profile$/);
  await expect(page.getByRole("heading", { name: "个人资料", exact: true })).toBeVisible();
  const navigation = page.getByRole("navigation", { name: "设置", exact: true });
  await expect(navigation.getByRole("link", { name: "登录设备", exact: true })).toHaveCount(0);
  await navigation.getByRole("link", { name: "账号", exact: true }).click();
  await expect(page).toHaveURL(/\/settings\/account$/);
  await expect(page.getByRole("heading", { name: "账号", exact: true })).toBeVisible();
  await expect(page.getByLabel("当前密码", { exact: true })).toHaveCount(0);
  await page.getByRole("button", { name: "密码由系统配置管理", exact: true }).focus();
  await expect(page.getByRole("tooltip")).toHaveText("请在 IAM 配置中更新密码");
  await page.getByRole("button", { name: "语言", exact: true }).click();
  await expect(page.getByRole("heading", { name: "Account", exact: true })).toBeVisible();
  await page.reload();
  await expect(page.getByRole("heading", { name: "Account", exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Member menu", exact: true }).click();
  for (const label of ["Settings", "System", "Sign out"]) {
    const item = page.getByRole("menuitem", { name: label, exact: true });
    await expect(item).toBeVisible();
    expect(await item.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  }
  await page.keyboard.press("Escape");
  await page.getByRole("button", { name: "Language", exact: true }).click();
  await expect(page.getByRole("heading", { name: "账号", exact: true })).toBeVisible();
  await page.getByRole("button", { name: "成员菜单", exact: true }).click();
  await page.getByRole("menuitem", { name: "系统管理", exact: true }).click();
  await page.getByRole("tab", { name: "平台成员", exact: true }).click();
  const configuredAccount = page.getByRole("row").filter({ hasText: adminUsername });
  await expect(configuredAccount.getByText("由配置管理", { exact: true })).toBeVisible();
  await expect(configuredAccount.getByRole("button", { name: "编辑", exact: true })).toHaveCount(0);
  await expect(configuredAccount.getByRole("button", { name: "重置密码", exact: true })).toHaveCount(0);
});

test("所有成员列表默认十条且修改每页数量后回到第一页", async ({ page }) => {
  const api = await adminClient();
  for (let index = 0; index < 11; index++) {
    await mutate(
      api,
      "post",
      "/api/v1/iam/users",
      {
        username: `paging_${randomBytes(5).toString("hex")}`,
        display_name: `Pagination member ${index}`,
        password: randomBytes(18).toString("hex"),
      },
      201,
    );
  }
  await browserLogin(page, adminUsername, adminPassword);
  const firstPage = page.waitForResponse(
    (response) => response.url().includes("/api/v1/iam/users?") && response.request().method() === "GET",
  );
  await page.goto("/management/platform/users");
  const response = await firstPage;
  expect(new URL(response.url()).searchParams.get("limit")).toBe("10");
  await expect(page.getByRole("table").locator("tbody tr")).toHaveCount(10);
  const next = page.waitForResponse(
    (response) =>
      response.url().includes("/api/v1/iam/users?") && new URL(response.url()).searchParams.get("offset") === "10",
  );
  await page.getByRole("button", { name: "下一页", exact: true }).click();
  await next;
  await page.getByRole("combobox", { name: "每页条数", exact: true }).click();
  const resized = page.waitForResponse(
    (response) =>
      response.url().includes("/api/v1/iam/users?") && new URL(response.url()).searchParams.get("limit") === "20",
  );
  await page.getByRole("option", { name: "20", exact: true }).click();
  const result = await resized;
  expect(new URL(result.url()).searchParams.get("offset")).toBe("0");
  const payload = await result.json();
  await expect(page.getByRole("table").locator("tbody tr")).toHaveCount(Math.min(20, payload.data.total));
  await api.dispose();
});

test("成员组合框通过服务器搜索和滚动加载后续候选", async ({ page }) => {
  const admin = await adminClient();
  const prefix = `picker_${randomBytes(4).toString("hex")}`;
  let disabled = "";
  for (let index = 0; index < 11; index++) {
    const username = `${prefix}_${String(index).padStart(2, "0")}`;
    const display_name = `候选成员 ${index}`;
    const created = await mutate(
      admin,
      "post",
      "/api/v1/iam/users",
      { username, display_name, password: randomBytes(16).toString("hex") },
      201,
    );
    if (index === 4) {
      disabled = username;
      await mutate(admin, "patch", `/api/v1/iam/users/${created.id}`, {
        display_name,
        status: "disabled",
        auth_version: "1",
      });
    }
  }
  await browserLogin(page, adminUsername, adminPassword);
  await page.goto("/management/platform/organizations");
  await page.getByRole("button", { name: "创建组织", exact: true }).click();
  const dialog = page.getByRole("dialog");
  const picker = dialog.getByRole("combobox", { name: "组织管理员", exact: true });
  await dialog.getByRole("button", { name: "确定", exact: true }).click();
  await expect(dialog.getByText("请输入 1–30 个字符，不能包含控制字符", { exact: true })).toBeVisible();
  await dialog.getByLabel("组织名称", { exact: true }).fill("校验与选择");
  await dialog.getByPlaceholder("例如 security-team").fill("Bad Key!");
  await dialog.getByRole("button", { name: "确定", exact: true }).click();
  await expect(
    dialog.getByText("使用 3–30 位小写字母、数字、_ 或 -，以字母开头、字母或数字结尾", { exact: true }),
  ).toBeVisible();
  await dialog.getByPlaceholder("例如 security-team").fill("picker_check");
  await dialog.getByRole("button", { name: "确定", exact: true }).click();
  await expect(picker).toBeFocused();
  await expect(picker).toHaveAttribute("aria-invalid", "true");
  const first = page.waitForResponse(
    (response) =>
      response.url().includes("/api/v1/iam/users?") && new URL(response.url()).searchParams.get("q") === prefix,
  );
  await picker.fill(prefix);
  expect(new URL((await first).url()).searchParams.get("limit")).toBe("10");
  await expect(page.getByRole("option")).toHaveCount(10);
  await picker.press("Escape");
  await expect(dialog).toBeVisible();
  await expect(picker).toHaveAttribute("aria-expanded", "false");
  await expect(dialog.getByLabel("组织名称", { exact: true })).toHaveValue("校验与选择");
  await picker.press("ArrowDown");
  await picker.fill(prefix);
  await expect(page.getByRole("option").filter({ hasText: prefix })).toHaveCount(10);
  await expect(page.getByRole("option").filter({ hasText: `(${disabled})` })).toBeDisabled();
  const next = page.waitForResponse(
    (response) =>
      response.url().includes("/api/v1/iam/users?") &&
      new URL(response.url()).searchParams.get("q") === prefix &&
      new URL(response.url()).searchParams.get("offset") === "10",
  );
  await page.getByRole("listbox").evaluate((element) => {
    element.scrollTop = element.scrollHeight;
  });
  await next;
  await expect(page.getByRole("option")).toHaveCount(11);
  await page
    .getByRole("option")
    .filter({ hasText: `(${prefix}_10)` })
    .click();
  await expect(picker).toHaveValue(`候选成员 10 (${prefix}_10)`);
  await dialog.getByRole("button", { name: "取消", exact: true }).click();
  await expect(dialog).not.toBeVisible();
  await admin.dispose();
});

test("身份确认保留原业务密码且不自动创建成员", async ({ page }) => {
  await browserLogin(page, adminUsername, adminPassword);
  await page.goto("/management/platform/users");
  await page.getByRole("button", { name: "添加成员", exact: true }).click();
  const dialog = page.getByRole("dialog");
  const username = `draft_${randomBytes(4).toString("hex")}`;
  const password = randomBytes(16).toString("hex");
  await dialog.getByLabel("用户名", { exact: true }).fill(username);
  await dialog.getByLabel("显示名称", { exact: true }).fill("密码草稿成员");
  await dialog.getByLabel("密码", { exact: true }).fill(password);
  await expireAuthentication(page);
  await dialog.getByRole("button", { name: "确定", exact: true }).click();
  await dialog.getByLabel("当前密码", { exact: true }).fill(adminPassword);
  await dialog.getByRole("button", { name: "确认并继续", exact: true }).click();
  await expect(dialog.getByRole("heading", { name: "添加平台成员", exact: true })).toBeVisible();
  const retained = (await dialog.getByLabel("密码", { exact: true }).inputValue()) === password;
  expect(retained).toBe(true);
  const api = await adminClient();
  expect((await (await api.get(`/api/v1/iam/users?q=${username}`)).json()).data.total).toBe(0);
  await dialog.getByRole("button", { name: "确定", exact: true }).click();
  await expect(dialog).not.toBeVisible();
  const member = await request.newContext({ baseURL });
  await mutate(member, "post", "/api/v1/iam/auth/login", { username, password }, 200);
  await member.dispose();
  await api.dispose();
});
