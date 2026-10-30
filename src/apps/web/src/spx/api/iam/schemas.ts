import { z } from "zod";

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

export const zApiMember = z.object({
  is_super_admin: z.boolean(),
  display_name: z.string(),
  id: z.string(),
  role_ids: z.array(z.string()).optional(),
  status: z.enum(["active", "disabled"]),
  user_id: z.string(),
  username: z.string(),
});

export const zApiOrganization = z.object({
  authz_version: z.string(),
  id: z.string(),
  name: z.string(),
  slug: z.string(),
  status: z.string().optional(),
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
  auth_version: z.string(),
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
});

export const zApiRole = z.object({
  allowed_actions: z.array(z.string()),
  code: z.string(),
  description: z.string(),
  id: z.string(),
  kind: z.string(),
  member_count: z.int().gte(0).lte(9007199254740991),
  name: z.string(),
  permissions: z.array(z.string()),
  status: z.enum(["active", "disabled"]),
  version: z.string(),
});

export const zApiUser = z.object({
  auth_version: z.string(),
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
    new_password: z.string().min(8).max(128),
  })
  .strict();

export const zCreateUserBody = z
  .object({
    username: z.string().regex(/^[A-Za-z][A-Za-z0-9_.-]{2,63}$/),
    display_name: z.string().min(1).max(100),
    email: z.string().optional(),
    password: z.string().min(8).max(128),
  })
  .strict();

export const zUpdateUserBody = z
  .object({
    display_name: z.string().min(1).max(100),
    email: z.string().optional(),
    status: z.enum(["active", "disabled"]),
    auth_version: z.string().regex(/^[1-9][0-9]*$/),
  })
  .strict();

export const zResetUserPasswordBody = z
  .object({
    password: z.string().min(8).max(128),
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
  })
  .strict();

export const zUpdateOrganizationBody = z
  .object({
    name: organizationName,
    status: z.enum(["active", "suspended"]),
    authz_version: z.string().regex(/^[1-9][0-9]*$/),
  })
  .strict();

export const zUpdateCurrentOrganizationBody = z
  .object({
    name: organizationName,
    authz_version: z.string().regex(/^[1-9][0-9]*$/),
  })
  .strict();

export const zCreateMemberBody = z
  .object({
    user_id: z.uuid().optional(),
    username: z
      .string()
      .regex(/^[A-Za-z][A-Za-z0-9_.-]{2,63}$/)
      .optional(),
    display_name: z.string().min(1).max(100).optional(),
    email: z.string().optional(),
    password: z.string().min(8).max(128).optional(),
  })
  .strict();

export const zUpdateMemberBody = z
  .object({
    status: z.enum(["active", "disabled"]),
    authz_version: z.string().regex(/^[1-9][0-9]*$/),
  })
  .strict();

export const zCreateRoleBody = z
  .object({
    code: z.string().regex(/^[a-z][a-z0-9_]{2,63}$/),
    name: z.string().min(1).max(100),
    description: z.string().max(1000).optional(),
    status: z.enum(["active", "disabled"]).optional(),
  })
  .strict();

export const zUpdateRoleBody = z
  .object({
    name: z.string().min(1).max(100),
    description: z.string().max(1000).optional(),
    status: z.enum(["active", "disabled"]),
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

export const zSetMemberRolesBody = z
  .object({
    role_ids: zApiRoleIds,
    authz_version: z.string().regex(/^[1-9][0-9]*$/),
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
    authz_version: z
      .string()
      .regex(/^[1-9][0-9]*$/)
      .optional(),
  })
  .strict();

export type ApiAuditEvent = z.infer<typeof zApiAuditEvent>;

export type ApiMember = z.infer<typeof zApiMember>;

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
});
export type ListQuery = z.infer<typeof listQuerySchema>;
