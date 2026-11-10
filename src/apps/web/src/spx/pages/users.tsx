"use client";
import { useState } from "react";

import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQuery } from "@tanstack/react-query";
import { PlusIcon, SearchIcon, UserRoundCogIcon } from "lucide-react";
import { useTranslations } from "next-intl";
import { useForm } from "react-hook-form";
import { z } from "zod";

import { permissions } from "@/auth/permissions.gen";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Field, FieldError, FieldGroup, FieldLabel, FieldLegend, FieldSet } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { InputGroup, InputGroupAddon, InputGroupInput } from "@/components/ui/input-group";
import { Skeleton } from "@/components/ui/skeleton";
import {
  createUser,
  deleteUser,
  listRoles,
  listUsers,
  resetUserPassword,
  setUserRoles,
  updateUser,
} from "@/spx/api/iam/management";
import type { ApiUser } from "@/spx/api/iam/schemas";
import { zCreateUserBody, zDisplayName, zEmail, zPassword, zUpdateUserBody, zUsername } from "@/spx/api/iam/schemas";

import { Can, useCan } from "../components/access";
import { DataTable, useAction, usePagedQuery } from "../components/data";
import { FormDialog, FormInput } from "../components/forms";
import { PageHeader } from "../components/page-header";
import { SheetPanel } from "../components/sheet-panel";
import { StatusBadge } from "../components/status-badge";
import { Toolbar } from "../components/toolbar";
import { managementChanged } from "../iam/management";
import { sessionScope } from "../iam/query";
import { errorMessageKey, useIdentity } from "../iam/session";

// 新建用户表单与创建账号接口一一对应；confirm_password 只存在于表单侧，
// 提交前被剥离，避免把展示字段写进契约。
const createUserFormSchema = z
  .object({
    username: zUsername,
    display_name: zDisplayName,
    email: zEmail,
    password: zPassword,
    confirm_password: z.string(),
  })
  .refine((value) => value.password === value.confirm_password, {
    path: ["confirm_password"],
    message: "passwordMismatch",
  });

// 重置密码需要两次输入一致，规则与 Go account 包一致：12–128 码点。
const resetPasswordFormSchema = z
  .object({
    password: zPassword,
    confirm_password: z.string(),
  })
  .refine((value) => value.password === value.confirm_password, {
    path: ["confirm_password"],
    message: "passwordMismatch",
  });

// UsersPage 是唯一的用户管理列表：一行一个用户，角色与状态在管理抽屉里维护。
export function UsersPage() {
  const t = useTranslations("Management");
  const navigation = useTranslations("Navigation");

  const [term, setTerm] = useState("");
  const [applied, setApplied] = useState("");
  const state = usePagedQuery<ApiUser>(["iam", "users", applied], (query, signal) =>
    listUsers({ ...query, q: applied || undefined }, signal),
  );

  const [creating, setCreating] = useState(false);
  const [managing, setManaging] = useState<ApiUser>();

  return (
    <div className="flex flex-col gap-4">
      <PageHeader title={navigation("users")} />

      <Toolbar
        actions={
          <Can action={permissions.IAMUsersManage}>
            <Button size="sm" onClick={() => setCreating(true)}>
              <PlusIcon data-icon="inline-start" />
              {t("addUser")}
            </Button>
          </Can>
        }
      >
        <InputGroup className="w-64">
          <InputGroupAddon>
            <SearchIcon />
          </InputGroupAddon>
          <InputGroupInput
            className="h-8"
            value={term}
            placeholder={t("searchUsersPlaceholder")}
            aria-label={t("searchUsersPlaceholder")}
            onChange={(event) => setTerm(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === "Enter") setApplied(term);
            }}
          />
        </InputGroup>
        <Button size="sm" variant="outline" onClick={() => setApplied(term)}>
          {t("search")}
        </Button>
      </Toolbar>

      <DataTable
        state={state}
        columns={[
          { title: t("username"), cell: (user) => user.username },
          { title: t("displayName"), cell: (user) => user.display_name },
          { title: t("email"), cell: (user) => user.email || "—" },
          {
            title: t("roles"),
            cell: (user) => (
              <div className="flex flex-wrap items-center gap-1">
                {user.roles.length ? (
                  user.roles.map((role) => (
                    <Badge key={role.id} variant="secondary">
                      {role.name}
                    </Badge>
                  ))
                ) : (
                  <span className="text-muted-foreground text-sm">{t("noRoles")}</span>
                )}
              </div>
            ),
          },
          {
            title: t("status"),
            cell: (user) => (
              <StatusBadge status={user.status}>{user.status === "active" ? t("active") : t("disabled")}</StatusBadge>
            ),
          },
          {
            title: t("action"),
            cell: (user) => (
              <Button variant="ghost" size="sm" onClick={() => setManaging(user)}>
                <UserRoundCogIcon data-icon="inline-start" />
                {t("manage")}
              </Button>
            ),
          },
        ]}
      />

      {creating && <CreateUserSheet onClose={() => setCreating(false)} />}
      {managing && <ManageUserSheet user={managing} onClose={() => setManaging(undefined)} />}
    </div>
  );
}

function CreateUserSheet({ onClose }: { onClose: () => void }) {
  const t = useTranslations("Management");

  return (
    <FormDialog
      variant="sheet"
      title={t("createUser")}
      schema={createUserFormSchema}
      defaultValues={{ username: "", display_name: "", email: "", password: "", confirm_password: "" }}
      onClose={onClose}
      onSubmit={async (form) => {
        await createUser(
          zCreateUserBody.parse({
            username: form.username,
            display_name: form.display_name,
            email: form.email || undefined,
            password: form.password,
          }),
        );
        await managementChanged();
      }}
    >
      {(form) => (
        <>
          <FormInput
            label={t("username")}
            registration={form.register("username")}
            error={form.formState.errors.username}
          />
          <FormInput
            label={t("displayName")}
            registration={form.register("display_name")}
            error={form.formState.errors.display_name}
          />
          <FormInput
            label={t("email")}
            type="email"
            registration={form.register("email")}
            error={form.formState.errors.email}
          />
          <FormInput
            label={t("password")}
            type="password"
            registration={form.register("password")}
            error={form.formState.errors.password}
          />
          <FormInput
            label={t("confirmPassword")}
            type="password"
            registration={form.register("confirm_password")}
            error={form.formState.errors.confirm_password}
            errorText={
              form.formState.errors.confirm_password?.message === "passwordMismatch" ? t("passwordMismatch") : undefined
            }
          />
        </>
      )}
    </FormDialog>
  );
}

// ManageUserSheet 把单个用户的全部操作收在一个抽屉里：
// 状态、角色勾选、重置密码与删除各自独立提交，互不影响。
function ManageUserSheet({ user, onClose }: { user: ApiUser; onClose: () => void }) {
  const t = useTranslations("Management");
  const common = useTranslations("Common");
  const errors = useTranslations("Errors");
  const perform = useAction();
  const auth = useIdentity();
  const canManage = useCan(permissions.IAMUsersManage);
  const canAssign = useCan(permissions.IAMUsersRoles);
  const canReset = useCan(permissions.IAMUsersReset);
  const canReadRoles = useCan(permissions.IAMRolesRead);

  const self = auth.principal?.user_id === user.id;

  const [status, setStatus] = useState(user.status);
  const [assigned, setAssigned] = useState(user.roles);
  const [selection, setSelection] = useState(user.roles.map((role) => role.id));
  const [deleting, setDeleting] = useState(false);

  const update = useMutation({ mutationFn: updateUser, onSuccess: managementChanged });
  const assign = useMutation({ mutationFn: setUserRoles, onSuccess: managementChanged });
  const remove = useMutation({ mutationFn: deleteUser, onSuccess: managementChanged });

  const roleQuery = useQuery({
    queryKey: ["iam", "roles", "assignment", ...sessionScope(auth)],
    queryFn: ({ signal }) => listRoles({ limit: 100 }, signal),
    enabled: canAssign && canReadRoles,
  });
  const roles = roleQuery.data?.items ?? [];
  const dirty =
    selection.length !== assigned.length || selection.some((id) => !assigned.some((role) => role.id === id));

  const resetForm = useForm<z.infer<typeof resetPasswordFormSchema>>({
    resolver: zodResolver(resetPasswordFormSchema),
    defaultValues: { password: "", confirm_password: "" },
  });

  // 禁用按钮同时给出原因，避免使用者对着灰按钮猜。
  let statusLock: string | undefined;
  if (user.configured) statusLock = t("configurationManaged");
  else if (self && status === "active") statusLock = t("cannotDisableSelf");

  let deleteLock: string | undefined;
  if (user.configured) deleteLock = t("configurationManaged");
  else if (self) deleteLock = t("cannotDeleteSelf");

  return (
    <SheetPanel
      title={t("manageUser")}
      description={user.display_name}
      onClose={onClose}
      footer={
        <Button size="sm" variant="outline" onClick={onClose}>
          {common("close")}
        </Button>
      }
    >
      <div className="flex flex-col gap-6 px-6 py-5">
        <section className="flex flex-col gap-3">
          <h3 className="text-sm font-medium">{t("status")}</h3>
          <div className="flex flex-wrap items-center gap-2">
            <StatusBadge status={status}>{status === "active" ? t("active") : t("disabled")}</StatusBadge>
            {canManage && (
              <Button
                size="sm"
                variant="outline"
                disabled={user.configured || (self && status === "active") || update.isPending}
                title={statusLock}
                onClick={() =>
                  void perform(async () => {
                    const next = status === "active" ? "disabled" : "active";
                    await update.mutateAsync({
                      id: user.id,
                      body: zUpdateUserBody.parse({ display_name: user.display_name, status: next }),
                    });
                    setStatus(next);
                  })
                }
              >
                {status === "active" ? t("disable") : t("enable")}
              </Button>
            )}
          </div>
        </section>

        <section className="flex flex-col gap-3">
          <h3 className="text-sm font-medium">{t("roles")}</h3>
          {canAssign && canReadRoles ? (
            <FieldSet>
              <FieldLegend variant="label" className="sr-only">
                {t("roles")}
              </FieldLegend>
              {roleQuery.isPending && <Skeleton className="h-16 w-full" />}
              {roles.length === 0 && !roleQuery.isPending && (
                <p className="text-muted-foreground text-sm">{t("noRoles")}</p>
              )}
              {roles.map((role) => {
                // 配置管理员的内置角色由部署配置保证，勾选状态因此在页面上锁定。
                const locked = user.configured && role.builtin;
                return (
                  <Field key={role.id} orientation="horizontal">
                    <Checkbox
                      id={`user-role-${user.id}-${role.id}`}
                      checked={selection.includes(role.id)}
                      disabled={locked}
                      title={locked ? t("configurationManaged") : undefined}
                      onCheckedChange={(checked) =>
                        setSelection((current) =>
                          checked === true ? [...current, role.id] : current.filter((id) => id !== role.id),
                        )
                      }
                    />
                    <FieldLabel htmlFor={`user-role-${user.id}-${role.id}`}>{role.name}</FieldLabel>
                  </Field>
                );
              })}
              {roleQuery.error && <FieldError>{errors(errorMessageKey(roleQuery.error))}</FieldError>}
              <div>
                <Button
                  size="sm"
                  disabled={!dirty || assign.isPending || !roleQuery.isSuccess}
                  onClick={() =>
                    void perform(async () => {
                      await assign.mutateAsync({ id: user.id, body: { role_ids: selection } });
                      setAssigned(
                        roles
                          .filter((role) => selection.includes(role.id))
                          .map((role) => ({ id: role.id, name: role.name })),
                      );
                    })
                  }
                >
                  {t("save")}
                </Button>
              </div>
            </FieldSet>
          ) : (
            <div className="flex flex-wrap items-center gap-1">
              {assigned.length ? (
                assigned.map((role) => (
                  <Badge key={role.id} variant="secondary">
                    {role.name}
                  </Badge>
                ))
              ) : (
                <span className="text-muted-foreground text-sm">{t("noRoles")}</span>
              )}
            </div>
          )}
        </section>

        {canReset && !user.configured && (
          <section className="flex flex-col gap-3 border-t pt-4">
            <h3 className="text-sm font-medium">{t("resetPassword")}</h3>
            <form
              noValidate
              onSubmit={resetForm.handleSubmit(async (values) => {
                await perform(async () => {
                  await resetUserPassword({ id: user.id, body: { password: values.password } });
                  resetForm.reset();
                });
              })}
            >
              <fieldset disabled={resetForm.formState.isSubmitting}>
                <FieldGroup>
                  <Field data-invalid={!!resetForm.formState.errors.password}>
                    <FieldLabel htmlFor={`reset-password-${user.id}`}>{t("newPassword")}</FieldLabel>
                    <Input
                      id={`reset-password-${user.id}`}
                      {...resetForm.register("password")}
                      type="password"
                      autoComplete="new-password"
                      minLength={12}
                      maxLength={128}
                      aria-invalid={!!resetForm.formState.errors.password}
                    />
                    {resetForm.formState.errors.password && <FieldError>{errors("invalidRequest")}</FieldError>}
                  </Field>
                  <Field data-invalid={!!resetForm.formState.errors.confirm_password}>
                    <FieldLabel htmlFor={`reset-confirm-${user.id}`}>{t("confirmPassword")}</FieldLabel>
                    <Input
                      id={`reset-confirm-${user.id}`}
                      {...resetForm.register("confirm_password")}
                      type="password"
                      autoComplete="new-password"
                      aria-invalid={!!resetForm.formState.errors.confirm_password}
                    />
                    {resetForm.formState.errors.confirm_password && <FieldError>{t("passwordMismatch")}</FieldError>}
                  </Field>
                  <div>
                    <Button type="submit" size="sm">
                      {t("resetPassword")}
                    </Button>
                  </div>
                </FieldGroup>
              </fieldset>
            </form>
          </section>
        )}

        {canManage && (
          <section className="flex flex-col gap-3 border-t pt-4">
            <Button
              size="sm"
              variant="destructive"
              disabled={user.configured || self || remove.isPending}
              title={deleteLock}
              onClick={() => setDeleting(true)}
            >
              {t("deleteUser")}
            </Button>
          </section>
        )}
      </div>

      {deleting && (
        <FormDialog
          title={t("deleteUser")}
          description={t("deleteUserDescription", { name: user.display_name })}
          confirmVariant="destructive"
          schema={z.object({})}
          defaultValues={{}}
          onClose={() => setDeleting(false)}
          onSubmit={async () => {
            await remove.mutateAsync({ id: user.id });
            onClose();
          }}
        />
      )}
    </SheetPanel>
  );
}
