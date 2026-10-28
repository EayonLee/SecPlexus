import { type APIRequestContext, expect, type Page, request, test } from "@playwright/test";

import type { ApiRole } from "../../src/spx/api/iam/types.gen";
import { randomBytes } from "node:crypto";

declare global {
  interface Window {
    streamAuthorized: boolean;
    streamRevoked: boolean;
  }
}

const baseURL = process.env.SPX_E2E_BASE_URL!;
const initialPassword = process.env.SPX_E2E_INITIAL_PASSWORD!;
const adminPassword = process.env.SPX_E2E_PASSWORD!;
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
async function adminClient() {
  const api = await request.newContext({ baseURL });
  await mutate(api, "post", "/api/v1/iam/auth/login", { username: "admin", password: adminPassword }, 200);
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
}

// 外部站点统计不属于身份链路，避免第三方网络阻塞浏览器启动。
test.beforeEach(async ({ context }) => {
  await context.route("https://www.googletagmanager.com/**", (route) => route.abort());
});

test.beforeAll(async () => {
  const api = await request.newContext({ baseURL });
  const existing = await api.post("/api/v1/iam/auth/login", {
    data: { username: "admin", password: adminPassword },
    headers: { Origin: baseURL, "X-CSRF-Token": await csrf(api) },
  });
  if (existing.status() === 401) {
    await mutate(api, "post", "/api/v1/iam/auth/login", { username: "admin", password: initialPassword }, 200);
    await mutate(api, "post", "/api/v1/iam/me/password", {
      old_password: initialPassword,
      new_password: adminPassword,
    });
  } else expect(existing.status()).toBe(200);
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
  await page.goto("/account/security");
  await expect(page).toHaveURL(/\/account\/security$/);
  await page.getByLabel("当前密码", { exact: true }).fill(initial);
  await page.getByLabel("新密码", { exact: true }).fill(password);
  await page.getByLabel("确认新密码", { exact: true }).fill(password);
  await page.getByRole("button", { name: "修改密码并退出" }).click();
  await expect(page).toHaveURL(/\/login$/);
  await browserLogin(page, username, password);
  await expect(page.getByLabel("当前组织")).toHaveValue(org);
  await expect(page.getByRole("heading", { name: "安全智驾工作台", exact: true })).toBeVisible();
  expect((await page.request.get(`/api/v1/console/orgs/${org}/context`)).status()).toBe(200);
  const second = await page.request.get(`${process.env.SPX_E2E_CONSOLE_SECOND_URL}/api/v1/console/orgs/${org}/context`);
  expect(second.status()).toBe(200);
  await page.goto(`/management/platform/users`);
  await expect(page.getByText("当前账号没有访问该页面的权限。", { exact: true })).toBeVisible();
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

test("管理中心保留应用外壳，自定义角色可创建、配置权限和查看影响", async ({ page }) => {
  await browserLogin(page, "admin", adminPassword);
  await expect(page.getByRole("button", { name: "管理中心", exact: true })).toBeVisible();
  await page.getByRole("button", { name: "管理中心", exact: true }).click();
  await page.getByRole("menuitem", { name: "管理中心", exact: true }).click();
  await page.getByText("角色与权限", { exact: true }).click();
  await page.getByRole("button", { name: "创建角色", exact: true }).click();
  await page.getByPlaceholder("例如 security_auditor").fill(`audit_${randomBytes(4).toString("hex")}`);
  const roleName = `审计角色 ${randomBytes(3).toString("hex")}`;
  await page.getByRole("dialog").getByLabel("角色名称", {exact:true}).fill(roleName);
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
});

test("跨标签退出清除身份，浏览器不持久化认证材料", async ({ page, context }) => {
  await browserLogin(page, "admin", adminPassword);
  await expect(page.getByRole("button", { name: "管理中心", exact: true })).toBeVisible();
  const other = await context.newPage();
  await other.goto("/dashboard");
  await expect(other.getByRole("button", { name: "管理中心", exact: true })).toBeVisible();
  const stored = await page.evaluate(() =>
    Object.keys(localStorage).filter((key) => /token|session|userStatus|userRole/i.test(key)),
  );
  expect(stored).toEqual([]);
  await page.getByLabel("个人账号").click();
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
      hidden: ["组织成员", "角色与权限", "组织审计", "用户管理"],
    },
    {
      name: "组织管理员",
      actions: [],
      role: orgAdmin.id,
      visible: ["组织成员", "角色与权限", "组织审计"],
      hidden: ["用户管理"],
    },
    {
      name: "自定义审计员",
      actions: ["iam.audit.read"],
      role: undefined,
      visible: ["组织审计"],
      hidden: ["组织成员", "角色与权限", "用户管理"],
    },
    {
      name: "自定义角色管理员",
      actions: ["iam.roles.read", "iam.roles.manage", "iam.roles.assign", "iam.members.read"],
      role: undefined,
      visible: ["组织成员", "角色与权限"],
      hidden: ["组织审计", "用户管理"],
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
      await expect(page.getByRole("button", { name: "管理中心", exact: true })).toHaveCount(0);
      await page.goto(`/management/orgs/${org}/audit`);
      await expect(page.getByText("当前账号没有访问该页面的权限。", { exact: true })).toBeVisible();
    } else {
      await page.getByRole("button", { name: "管理中心", exact: true }).click();
      await page.getByRole("menuitem", { name: "管理中心", exact: true }).click();
      const sidebar = page.getByRole("navigation", { name: "主导航" });
      for (const label of scenario.visible)
        await expect(sidebar.getByRole("link", { name: label, exact: true })).toBeVisible();
      for (const label of scenario.hidden)
        await expect(sidebar.getByRole("link", { name: label, exact: true })).toHaveCount(0);
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
  await expect(page.getByText("当前账号没有访问该页面的权限。", { exact: true })).toBeVisible();
  await admin.dispose();
});

test("组织创建选择已有管理员，两个标签各自保留组织上下文", async ({ page, context }) => {
  const admin = await adminClient();
  const initial = (await (await admin.get("/api/v1/iam/me")).json()).data.organizations[0];
  const suffix = randomBytes(4).toString("hex");
  const name = `浏览器组织 ${suffix}`;
  await browserLogin(page, "admin", adminPassword);
  await expect(page.getByRole("button", { name: "管理中心", exact: true })).toBeVisible();
  await page.goto("/management/platform/organizations");
  await page.getByRole("button", { name: "创建组织", exact: true }).click();
  const dialog = page.getByRole("dialog");
  await dialog.getByLabel("组织名称", { exact: true }).fill(name);
  await dialog.getByPlaceholder("例如 security-team").fill(`team-${suffix}`);
  await dialog.getByRole("combobox", { name: "初始组织管理员", exact: true }).click();
  await page.getByRole("option").filter({ hasText: "(admin)" }).click();
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
  await expect(page.getByLabel("当前组织")).toHaveValue(created.id);
  await expect(other.getByLabel("当前组织")).toHaveValue(initial.id);
  expect((await other.request.get(`/api/v1/console/orgs/${initial.id}/context`)).status()).toBe(200);
  await other.reload();
  await expect(other).toHaveURL(new RegExp(`org_id=${initial.id}`));
  await expect(other.getByRole("heading", { name: "安全智驾工作台", exact: true })).toBeVisible();
  await other.close();
  await admin.dispose();
});
