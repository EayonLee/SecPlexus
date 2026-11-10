import { z } from "zod";

// 账号字段规则必须与 Go account 包完全一致：
// 改动任何一条都要同步 src/apps/iam/internal/account 下的实现与测试。
const usernamePattern = /^[A-Za-z][A-Za-z0-9_.-]{1,37}[A-Za-z0-9]$/;
const consecutiveSeparators = /[._-]{2,}/;
const emailPattern =
  /^[a-zA-Z0-9.!#$%&'*+/=?^_`{|}~-]+@[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$/;

/** 用户名：3–39 位、字母开头字母数字结尾、无连续分隔符。 */
export const zUsername = z
  .string()
  .regex(usernamePattern, { message: "username" })
  .refine((value) => !consecutiveSeparators.test(value), { message: "username" });

/** 显示名称：trim 后 1–100 码点。 */
export const zDisplayName = z
  .string()
  .transform((value) => value.trim())
  .pipe(z.string().min(1).max(100));

/** 邮箱：可选；空串表示未设置；整址 ≤254、本地部 ≤64，与 Go account.ValidateEmail 一致。 */
export const zEmail = z.union([
  z.literal(""),
  z
    .string()
    .max(254)
    .regex(emailPattern, { message: "email" })
    .refine((value) => value.lastIndexOf("@") <= 64, { message: "email" }),
]);

/** 密码：12–128 码点。 */
export const zPassword = z.string().min(12).max(128);

// 仅声明 Web 实际消费的数据及表单输入，类型由 Zod 推导。
export const zApiAuditEvent = z.object({
  action: z.string(),
  actor_id: z.string().nullish(),
  actor_name: z.string().nullish(),
  actor_type: z.string(),
  id: z.string(),
  metadata: z.record(z.string(), z.unknown()).optional(),
  outcome: z.string(),
  request_id: z.string(),
  target_id: z.string().nullish(),
  target_type: z.string().nullish(),
  occurred_at: z.iso.datetime({ offset: true }),
});

export const zApicsrfContext = z.object({
  csrf_token: z.string(),
});

export const zApiCreated = z.object({
  id: z.string(),
});

// 角色引用与用户角色、本人角色共用同一形状：授权只由用户与角色表达。
export const zApiRoleRef = z.object({
  id: z.string(),
  name: z.string(),
});

export const zApiPermission = z.object({
  action: z.string(),
  delegable: z.boolean(),
  description: z.string(),
  key: z.string(),
  module: z.string(),
  requires_recent_auth: z.boolean(),
  resource_type: z.string(),
  risk_level: z.string(),
  status: z.string(),
});

export const zApiPermissionItem = z.object({
  can_delegate: z.boolean(),
  permission: zApiPermission,
});

export const zApiPrincipal = z.object({
  auth_epoch: z.string(),
  authenticated_at: z.iso.datetime({ offset: true }),
  configured: z.boolean(),
  display_name: z.string(),
  session_id: z.string(),
  user_id: z.string(),
  username: z.string(),
});

export const zApiLoginResponse = z.object({
  principal: zApiPrincipal,
});

// 时区是展示偏好，由 /me 一并返回；它不是授权作用域，缺省时按浏览器时区渲染。
export const zApiMe = z.object({
  permissions: z.array(z.string()),
  principal: zApiPrincipal,
  roles: z.array(zApiRoleRef),
  timezone: z.string().nullish(),
});

export const zApiProfileUpdateBody = z.object({
  timezone: z.string().max(64),
});

export const zApiProfileUpdateResponse = z.object({
  timezone: z.string().nullish(),
});

export const zApiRole = z.object({
  builtin: z.boolean(),
  description: z.string(),
  id: z.string(),
  name: z.string(),
  permissions: z.array(z.string()),
  user_count: z.int().gte(0).lte(9007199254740991),
});

export const zApiUser = z.object({
  configured: z.boolean(),
  display_name: z.string(),
  email: z.string().nullable(),
  id: z.string(),
  roles: z.array(zApiRoleRef),
  status: z.enum(["active", "disabled"]),
  username: z.string(),
});

// 表单交互由 RHF/Zod 校验；服务端 Go DTO 与业务用例仍独立拥有最终输入规则。
export const zLoginBody = z
  .object({
    username: z.string(),
    password: z.string().max(128),
  })
  .strict();

export const zReauthenticateBody = z
  .object({
    password: z.string(),
  })
  .strict();

export const zChangePasswordBody = z
  .object({
    old_password: z.string(),
    new_password: zPassword,
  })
  .strict();

export const zCreateUserBody = z
  .object({
    display_name: zDisplayName,
    email: zEmail.optional(),
    password: zPassword,
    username: zUsername,
  })
  .strict();

export const zUpdateUserBody = z
  .object({
    display_name: zDisplayName,
    email: zEmail.optional(),
    status: z.enum(["active", "disabled"]),
  })
  .strict();

export const zResetUserPasswordBody = z
  .object({
    password: zPassword,
  })
  .strict();

export const zCreateRoleBody = z
  .object({
    description: z.string().max(1000).optional(),
    name: z.string().min(1).max(100),
    permissions: z.array(z.string()).max(100),
  })
  .strict();

export const zUpdateRoleBody = z
  .object({
    description: z.string().max(1000).optional(),
    name: z.string().min(1).max(100),
  })
  .strict();

const zPermissionKeys = z
  .array(z.string())
  .max(200)
  .refine((values) => new Set(values).size === values.length);

export const zSetRolePermissionsBody = z
  .object({
    permissions: zPermissionKeys,
  })
  .strict();

const zRoleIds = z
  .array(z.uuid())
  .max(50)
  .refine((values) => new Set(values).size === values.length);

export const zSetUserRolesBody = z
  .object({
    role_ids: zRoleIds,
  })
  .strict();

export type ApiAuditEvent = z.infer<typeof zApiAuditEvent>;

export type ApiPermission = z.infer<typeof zApiPermission>;

export type ApiPermissionItem = z.infer<typeof zApiPermissionItem>;

export type ApiPrincipal = z.infer<typeof zApiPrincipal>;

export type ApiMe = z.infer<typeof zApiMe>;

export type ApiRole = z.infer<typeof zApiRole>;

export type ApiUser = z.infer<typeof zApiUser>;

export const listQuerySchema = z.object({
  limit: z.number().int().min(1).max(100).optional(),
  offset: z.number().int().min(0).max(100000).optional(),
  q: z.string().optional(),
});
export type ListQuery = z.infer<typeof listQuerySchema>;

// 审计只向前翻页：总行数随时间单调增长，游标比深 OFFSET 便宜且没有跳页漂移。
export const auditQuerySchema = z.object({
  limit: z.number().int().min(1).max(100).optional(),
  cursor: z.string().max(128).optional(),
  actor: z.string().optional(),
  outcome: z.enum(["success", "failure", "denied"]).optional(),
  from: z.string().optional(),
  to: z.string().optional(),
});
export type AuditQuery = z.infer<typeof auditQuerySchema>;

/** 审计一页：next_cursor 为空表示没有更早的事件。 */
export const zApiAuditPage = z.object({
  items: z.array(zApiAuditEvent),
  limit: z.int(),
  next_cursor: z.string(),
});
export type ApiAuditPage = z.infer<typeof zApiAuditPage>;
