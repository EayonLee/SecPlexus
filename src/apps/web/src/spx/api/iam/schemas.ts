import { z } from "zod";

// 账号字段规则必须与 Go identity 包完全一致：
// 改动任何一条都要同步 src/apps/iam/internal/identity 下的实现与测试。
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

/** 邮箱：可选；空串表示未设置；整址 ≤254、本地部 ≤64，与 Go identity.ValidateEmail 一致。 */
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
  actor_name: z.string().nullish(),
  actor_type: z.string().optional(),
  id: z.string(),
  metadata: z.record(z.string(), z.unknown()).optional(),
  occurred_at: z.iso.datetime({ offset: true }),
  org_id: z.string().nullish(),
  outcome: z.string(),
  reason_code: z.string().nullish(),
  request_id: z.string(),
  target_id: z.string().nullish(),
  target_type: z.string().optional(),
});

export const zApicsrfContext = z.object({
  csrf_token: z.string(),
});

export const zApiCreated = z.object({
  id: z.string(),
});

export const zApiOrganizationUser = z.object({
  is_super_admin: z.boolean(),
  display_name: z.string(),
  id: z.string(),
  org_id: z.string().optional(),
  org_slug: z.string().optional(),
  role_ids: z.array(z.string()).optional(),
  source: z.enum(["direct", "subtree", "inherited"]).optional(),
  status: z.enum(["active", "disabled"]),
  user_id: z.string(),
  username: z.string(),
});

export const zApiOrganization = z.object({
  authz_epoch: z.string(),
  depth: z.int(),
  id: z.string(),
  name: z.string(),
  parent_id: z.string().nullish(),
  path: z.array(z.string()).nullish(),
  slug: z.string(),
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
  scope: z.string(),
  status: z.string(),
});

export const zApiPermissionItem = z.object({
  can_delegate: z.boolean(),
  permission: zApiPermission,
});

export const zApiPreview = z.object({
  added: z.array(z.string()),
  affected_members: z.int().gte(0).lte(9007199254740991),
  after: z.array(z.string()),
  before: z.array(z.string()),
  removed: z.array(z.string()),
});

export const zApiPrincipal = z.object({
  auth_epoch: z.string(),
  authenticated_at: z.iso.datetime({ offset: true }),
  display_name: z.string(),
  is_super_admin: z.boolean(),
  reauthenticated_at: z.iso.datetime({ offset: true }).optional(),
  session_id: z.string(),
  user_id: z.string(),
  username: z.string(),
});

export const zApiLoginResponse = z.object({
  principal: zApiPrincipal,
});

export const zApiMe = z.object({
  capabilities: z.array(z.string()),
  org_id: z.string().optional(),
  organizations: z.array(zApiOrganization),
  platform_capabilities: z.array(z.string()),
  principal: zApiPrincipal,
  timezone: z.string().nullish(),
});

export const zApiProfileUpdateBody = z.object({
  timezone: z.string().max(64),
});

export const zApiProfileUpdateResponse = z.object({
  timezone: z.string().nullish(),
});

export const zApiRole = z.object({
  allowed_actions: z.array(z.string()),
  description: z.string(),
  id: z.string(),
  member_count: z.int().gte(0).lte(9007199254740991),
  name: z.string(),
  permissions: z.array(z.string()),
  version: z.string(),
});

export const zApiUser = z.object({
  auth_epoch: z.string(),
  display_name: z.string(),
  email: z.string().nullable(),
  id: z.string(),
  is_super_admin: z.boolean(),
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
    username: zUsername,
    display_name: zDisplayName,
    email: zEmail.optional(),
    password: zPassword,
    org_id: z.uuid(),
    role_ids: z.array(z.uuid()).min(1),
  })
  .strict();

// 创建成员表单：在请求体之外增加确认密码，仅用于交互校验，不发送到服务端。
export const zCreateUserForm = zCreateUserBody
  .extend({ confirm_password: z.string() })
  .refine((value) => value.password === value.confirm_password, {
    path: ["confirm_password"],
    message: "passwordMismatch",
  });

export const zUpdateUserBody = z
  .object({
    display_name: zDisplayName,
    email: zEmail.optional(),
    status: z.enum(["active", "disabled"]),
    auth_epoch: z.string().regex(/^[1-9][0-9]*$/),
  })
  .strict();

export const zResetUserPasswordBody = z
  .object({
    password: zPassword,
  })
  .strict();

// Names are normalized once at the request boundary; Unicode-aware regex counts code points.
const organizationName = z
  .string()
  .refine((value) => !/\p{Cc}/u.test(value))
  .transform((value) => value.trim())
  .pipe(z.string().regex(/^.{1,30}$/u));

export const zCreateOrganizationBody = z
  .object({
    slug: z.string().regex(/^[a-z][a-z0-9_-]{1,28}[a-z0-9]$/),
    name: organizationName,
    admin_user_id: z.uuid(),
    parent_id: z.uuid().optional(),
  })
  .strict();

export const zUpdateOrganizationBody = z
  .object({
    name: organizationName,
    authz_epoch: z.string().regex(/^[1-9][0-9]*$/),
    parent_id: z.uuid().optional(),
  })
  .strict();

export const zCreateOrganizationUserBody = z
  .object({
    user_id: z.uuid().optional(),
    username: zUsername.optional(),
    display_name: zDisplayName.optional(),
    email: zEmail.optional(),
    password: zPassword.optional(),
  })
  .strict();

export const zUpdateOrganizationUserBody = z
  .object({
    status: z.enum(["active", "disabled"]),
    authz_epoch: z.string().regex(/^[1-9][0-9]*$/),
  })
  .strict();

export const zCreateRoleBody = z
  .object({
    name: z.string().min(1).max(100),
    description: z.string().max(1000).optional(),
    permissions: z.array(z.string()).max(100),
  })
  .strict();

export const zUpdateRoleBody = z
  .object({
    name: z.string().min(1).max(100),
    description: z.string().max(1000).optional(),
    version: z.string().regex(/^[1-9][0-9]*$/),
  })
  .strict();

const zApiPermissions = z
  .array(z.string())
  .max(200)
  .refine((values) => new Set(values).size === values.length);

export const zSetRolePermissionsBody = z
  .object({
    permissions: zApiPermissions,
    version: z.string().regex(/^[1-9][0-9]*$/),
  })
  .strict();

const zApiRoleIds = z
  .array(z.uuid())
  .max(50)
  .refine((values) => new Set(values).size === values.length);

export const zSetOrganizationUserRolesBody = z
  .object({
    role_ids: zApiRoleIds,
    authz_epoch: z.string().regex(/^[1-9][0-9]*$/),
  })
  .strict();

export const zPreviewPermissionBody = z
  .object({
    role_id: z.uuid().optional(),
    permissions: zApiPermissions.optional(),
    version: z
      .string()
      .regex(/^[1-9][0-9]*$/)
      .optional(),
    membership_id: z.uuid().optional(),
    role_ids: zApiRoleIds.optional(),
    authz_epoch: z
      .string()
      .regex(/^[1-9][0-9]*$/)
      .optional(),
  })
  .strict();

export type ApiAuditEvent = z.infer<typeof zApiAuditEvent>;

export type ApiOrganizationUser = z.infer<typeof zApiOrganizationUser>;

export type ApiOrganization = z.infer<typeof zApiOrganization>;

export type ApiPermission = z.infer<typeof zApiPermission>;

export type ApiPermissionItem = z.infer<typeof zApiPermissionItem>;

export type ApiPreview = z.infer<typeof zApiPreview>;

export type ApiPrincipal = z.infer<typeof zApiPrincipal>;

export type ApiMe = z.infer<typeof zApiMe>;

export type ApiRole = z.infer<typeof zApiRole>;

export type ApiUser = z.infer<typeof zApiUser>;

export const listQuerySchema = z.object({
  limit: z.number().int().min(1).max(100).optional(),
  offset: z.number().int().min(0).max(100000).optional(),
  q: z.string().optional(),
  org_id: z.string().optional(),
  scope: z.enum(["direct", "subtree"]).optional(),
});
export type ListQuery = z.infer<typeof listQuerySchema>;

export const auditQuerySchema = z.object({
  limit: z.number().int().min(1).max(100).optional(),
  offset: z.number().int().min(0).max(100000).optional(),
  actor: z.string().optional(),
  outcome: z.enum(["success", "failure", "denied"]).optional(),
  from: z.string().optional(),
  to: z.string().optional(),
});
export type AuditQuery = z.infer<typeof auditQuerySchema>;

export const zOrganizationTree = z.object({
  items: z.array(zApiOrganization),
  truncated: z.boolean().optional(),
});
export type OrganizationTreeResponse = z.infer<typeof zOrganizationTree>;
