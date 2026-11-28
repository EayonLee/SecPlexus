"use client";
import { useState } from "react";

import { zodResolver } from "@hookform/resolvers/zod";
import { useQuery } from "@tanstack/react-query";
import { Download, Eye, Grid2X2, List, Pencil, PlusIcon, SlidersHorizontal, Trash2 } from "lucide-react";
import { useFormatter, useTranslations } from "next-intl";
import { useForm, useFormState } from "react-hook-form";
import { z } from "zod";

import { permissions } from "@/auth/permissions.gen";
import { Avatar, AvatarBadge, AvatarFallback } from "@/components/ui/avatar";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { CardAction, CardContent, CardHeader } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Field, FieldGroup, FieldLabel, FieldLegend, FieldSet } from "@/components/ui/field";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
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
import { CollectionBoundary, CollectionState } from "../components/collection-state";
import { DataTable, useAction, usePagedQuery } from "../components/data";
import { useDirectorySearch } from "../components/directory-search";
import { FormDialog, FormInput } from "../components/forms";
import { ListSelect } from "../components/list-controls";
import { PageTitle } from "../components/page-header";
import { RecordMenu } from "../components/record-menu";
import { SearchField } from "../components/search-field";
import { SheetPanel } from "../components/sheet-panel";
import { StatusBadge } from "../components/status-badge";
import { ListSurface } from "../components/toolbar";
import { managementChanged } from "../iam/management";
import { sessionScope } from "../iam/query";
import { useIdentity } from "../iam/session";

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

const userDetailsSchema = zUpdateUserBody.pick({ display_name: true, email: true });

export function UsersPage() {
  const t = useTranslations("Management");
  const navigation = useTranslations("Navigation");
  const common = useTranslations("Common");
  const format = useFormatter();
  const auth = useIdentity();
  const canManage = useCan(permissions.IAMUsersManage);
  const canAssign = useCan(permissions.IAMUsersRoles);
  const canReset = useCan(permissions.IAMUsersReset);
  const canReadRoles = useCan(permissions.IAMRolesRead);

  const { term, setTerm, applied, setApplied } = useDirectorySearch();
  const [statusFilter, setStatusFilter] = useState<"all" | ApiUser["status"]>("all");
  const [roleFilter, setRoleFilter] = useState("");
  const [checked, setChecked] = useState<string[]>([]);
  const [display, setDisplay] = useState("list");
  const [hidden, setHidden] = useState<string[]>([]);
  const roleOptions = useQuery({
    queryKey: ["iam", "roles", "assignment", ...sessionScope(auth)],
    queryFn: ({ signal }) => listRoles({ limit: 100 }, signal),
    enabled: canReadRoles,
  });
  const state = usePagedQuery<ApiUser>(["iam", "users", applied, statusFilter, roleFilter], (query, signal) =>
    listUsers(
      {
        ...query,
        q: applied || undefined,
        status: statusFilter === "all" ? undefined : statusFilter,
        role_id: roleFilter || undefined,
      },
      signal,
    ),
  );
  const restartPage = () => {
    setChecked([]);
    state.setPagination((page) => ({ ...page, pageIndex: 0 }));
  };
  const search = (value: string) => {
    setTerm(value);
    setApplied(value);
    restartPage();
  };

  const [creating, setCreating] = useState(false);
  const [selected, setSelected] = useState<{ user: ApiUser; edit: boolean; opener: HTMLElement | null }>();
  const [deleting, setDeleting] = useState<{ user: ApiUser; opener: HTMLElement | null }>();

  const selectedUsers = state.items.filter((user) => checked.includes(user.id));
  const allChecked = state.items.length > 0 && selectedUsers.length === state.items.length;
  const filtersActive = Boolean(applied || roleFilter || statusFilter !== "all");

  const exportUsers = () => {
    const records = selectedUsers.length ? selectedUsers : state.items;
    const rows = [
      [t("username"), t("displayName"), t("email"), t("roles"), t("status"), t("createdAt")],
      ...records.map((user) => [
        user.username,
        user.display_name,
        user.email ?? "",
        user.roles.map((role) => role.name).join(", "),
        t(user.status),
        user.created_at ?? "",
      ]),
    ];
    const csv = rows
      .map((row) =>
        row
          .map((value) => {
            const safe = /^[\s]*[=+@-]|^[\t\r\n]/.test(value) ? `'${value}` : value;
            return `"${safe.replaceAll('"', '""')}"`;
          })
          .join(","),
      )
      .join("\r\n");
    const url = URL.createObjectURL(new Blob(["\uFEFF", csv], { type: "text/csv;charset=utf-8" }));
    const download = document.createElement("a");
    download.href = url;
    download.download = "secplexus-users.csv";
    download.click();
    window.setTimeout(() => URL.revokeObjectURL(url), 0);
  };

  const actions = (user: ApiUser) => {
    let deleteLock: string | undefined;
    if (user.configured) deleteLock = t("configurationManaged");
    else if (auth.principal?.user_id === user.id) deleteLock = t("cannotDeleteSelf");

    return (
      <RecordMenu
        label={t("recordActions", { name: user.display_name })}
        actions={[
          { label: t("viewDetails"), icon: Eye, onSelect: (opener) => setSelected({ user, edit: false, opener }) },
          ...((!user.configured && (canManage || canReset)) || (canAssign && canReadRoles)
            ? [
                {
                  label: t("editUser"),
                  icon: Pencil,
                  onSelect: (opener: HTMLButtonElement | null) => setSelected({ user, edit: true, opener }),
                },
              ]
            : []),
        ]}
        destructive={
          canManage
            ? {
                label: t("deleteUser"),
                icon: Trash2,
                disabledReason: deleteLock,
                onSelect: (opener) => setDeleting({ user, opener }),
              }
            : undefined
        }
      />
    );
  };

  const toggle = (id: string, value: boolean) => {
    const ids = selectedUsers.map((user) => user.id);
    setChecked(value ? [...ids, id] : ids.filter((selectedId) => selectedId !== id));
  };
  const optionalColumns = [
    {
      key: "roles",
      title: t("roles"),
      className: "hidden @lg/list:table-cell",
      cell: (user: ApiUser) => <UserRoles roles={user.roles} />,
    },
    {
      key: "status",
      title: t("status"),
      className: "hidden @sm/list:table-cell",
      cell: (user: ApiUser) => <UserStatus user={user} />,
    },
    {
      key: "createdAt",
      title: t("createdAt"),
      className: "hidden @2xl/list:table-cell",
      cell: (user: ApiUser) =>
        user.created_at ? format.dateTime(new Date(user.created_at), { dateStyle: "medium", timeStyle: "short" }) : "—",
    },
  ];

  return (
    <>
      <ListSurface>
        <CardHeader className="items-center gap-3 border-b py-4 has-data-[slot=card-action]:grid-cols-1 @3xl/list:has-data-[slot=card-action]:grid-cols-[1fr_auto]">
          <PageTitle>{navigation("users")}</PageTitle>
          <CardAction className="col-start-1 row-start-auto flex flex-wrap justify-start gap-2 self-center justify-self-stretch @3xl/list:col-start-2 @3xl/list:row-span-1 @3xl/list:row-start-1 @3xl/list:justify-end">
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button size="sm" variant="outline">
                  <SlidersHorizontal />
                  {t("columns")}
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                {optionalColumns.map((column) => (
                  <DropdownMenuCheckboxItem
                    key={column.key}
                    checked={!hidden.includes(column.key)}
                    onCheckedChange={(value) =>
                      setHidden((keys) => (value ? keys.filter((key) => key !== column.key) : [...keys, column.key]))
                    }
                  >
                    {column.title}
                  </DropdownMenuCheckboxItem>
                ))}
              </DropdownMenuContent>
            </DropdownMenu>
            <Button variant="outline" size="sm" disabled={!state.items.length || state.loading} onClick={exportUsers}>
              <Download />
              {t("exportUsers")}
            </Button>
            <Can action={permissions.IAMUsersManage}>
              <Button size="sm" onClick={() => setCreating(true)}>
                <PlusIcon />
                {t("addUser")}
              </Button>
            </Can>
          </CardAction>
        </CardHeader>
        <CardContent className="flex flex-col gap-4 px-0 pt-4">
          <div className="flex flex-wrap items-center gap-3 px-4">
            <SearchField
              value={term}
              onChange={(value) => {
                setTerm(value);
                restartPage();
              }}
              onSubmit={search}
              placeholder={t("searchUsersPlaceholder")}
            />
            {canReadRoles && (
              <ListSelect
                label={t("roles")}
                value={roleFilter || "all"}
                options={[
                  { value: "all", label: t("allRoles") },
                  ...(roleOptions.data?.items ?? []).map((role) => ({ value: role.id, label: role.name })),
                ]}
                onChange={(value) => {
                  setRoleFilter(value === "all" ? "" : value);
                  restartPage();
                }}
              />
            )}
            <ListSelect<"all" | ApiUser["status"]>
              label={t("status")}
              value={statusFilter}
              options={[
                { value: "all", label: t("allStatuses") },
                { value: "active", label: t("active") },
                { value: "disabled", label: t("disabled") },
              ]}
              onChange={(value) => {
                setStatusFilter(value);
                restartPage();
              }}
            />
            {selectedUsers.length > 0 && (
              <span role="status" className="text-muted-foreground text-sm tabular-nums">
                {common("selectedCount", { count: selectedUsers.length })}
              </span>
            )}
            <Tabs value={display} onValueChange={setDisplay} className="ml-auto">
              <TabsList>
                <TabsTrigger value="list" aria-label={t("listView")}>
                  <List />
                </TabsTrigger>
                <TabsTrigger value="grid" aria-label={t("gridView")}>
                  <Grid2X2 />
                </TabsTrigger>
              </TabsList>
            </Tabs>
          </div>
          <DataTable
            onClear={
              filtersActive
                ? () => {
                    setRoleFilter("");
                    setStatusFilter("all");
                    search("");
                  }
                : undefined
            }
            state={state}
            isSelected={(user) => checked.includes(user.id)}
            onPageChange={() => setChecked([])}
            card={
              display === "grid"
                ? (user) => (
                    <div className="flex flex-col gap-3">
                      <div className="flex items-center gap-3">
                        <Checkbox
                          aria-label={t("selectUser", { name: user.display_name })}
                          checked={checked.includes(user.id)}
                          onCheckedChange={(value) => toggle(user.id, Boolean(value))}
                        />
                        <div className="min-w-0 flex-1">
                          <UserIdentity user={user} />
                        </div>
                        {actions(user)}
                      </div>
                      <div className="flex flex-wrap items-center justify-between gap-2">
                        <UserRoles roles={user.roles} />
                        <UserStatus user={user} />
                      </div>
                    </div>
                  )
                : undefined
            }
            columns={[
              {
                title: t("selectUsers"),
                className: "w-12",
                header: (
                  <Checkbox
                    aria-label={t("selectUsers")}
                    checked={allChecked || (selectedUsers.length > 0 && "indeterminate")}
                    disabled={!state.items.length}
                    onCheckedChange={(value) => setChecked(value ? state.items.map((user) => user.id) : [])}
                  />
                ),
                cell: (user) => (
                  <Checkbox
                    aria-label={t("selectUser", { name: user.display_name })}
                    checked={checked.includes(user.id)}
                    onCheckedChange={(value) => toggle(user.id, Boolean(value))}
                  />
                ),
              },
              { title: t("identity"), cell: (user) => <UserIdentity user={user} /> },
              ...optionalColumns.filter((column) => !hidden.includes(column.key)),
              { title: t("action"), className: "w-0", cell: actions },
            ]}
          />
        </CardContent>
      </ListSurface>

      {creating && <CreateUserDialog onClose={() => setCreating(false)} />}
      {selected && (
        <UserSheet
          user={selected.user}
          edit={selected.edit}
          opener={selected.opener}
          onClose={() => setSelected(undefined)}
        />
      )}
      {deleting && (
        <FormDialog
          variant="alert"
          title={t("deleteUser")}
          description={t("deleteUserDescription", { name: deleting.user.display_name })}
          submitLabel={t("delete")}
          returnFocus={deleting.opener}
          schema={z.object({})}
          defaultValues={{}}
          onClose={() => setDeleting(undefined)}
          onSubmit={async () => {
            await deleteUser({ id: deleting.user.id });
            if (state.items.length === 1 && state.pagination.pageIndex > 0) {
              state.setPagination({ ...state.pagination, pageIndex: state.pagination.pageIndex - 1 });
            }
            await managementChanged();
          }}
        />
      )}
    </>
  );
}

function UserIdentity({ user }: { user: ApiUser }) {
  return (
    <div className="flex min-w-0 items-center gap-3">
      <Avatar size="lg" aria-hidden="true" className="after:border-status-info/25">
        <AvatarFallback className="bg-status-info/15 text-status-info font-medium">
          {[...user.display_name].slice(0, 2).join("").toUpperCase()}
        </AvatarFallback>
        <AvatarBadge
          className={user.status === "active" ? "bg-status-success ring-card" : "bg-muted-foreground ring-card"}
        />
      </Avatar>
      <div className="min-w-0 whitespace-normal [overflow-wrap:anywhere]">
        <div className="text-foreground text-sm font-medium">{user.display_name}</div>
        <div className="text-muted-foreground text-sm">{user.email || user.username}</div>
        {user.email && !user.email.startsWith(`${user.username}@`) && (
          <div className="text-muted-foreground text-xs">{user.username}</div>
        )}
      </div>
    </div>
  );
}

function UserStatus({ user }: { user: ApiUser }) {
  const t = useTranslations("Management");
  return (
    <StatusBadge status={user.status}>
      <span aria-hidden="true" className="size-1.5 rounded-full bg-current" />
      {t(user.status)}
    </StatusBadge>
  );
}

function UserRoles({ roles }: { roles: ApiUser["roles"] }) {
  const t = useTranslations("Management");
  return (
    <div className="flex flex-wrap items-center gap-1">
      {roles.length ? (
        roles.map((role) => (
          <Badge key={role.id} variant="secondary" className="max-w-full whitespace-normal [overflow-wrap:anywhere]">
            {role.name}
          </Badge>
        ))
      ) : (
        <span className="text-muted-foreground text-sm">{t("noRoles")}</span>
      )}
    </div>
  );
}

function CreateUserDialog({ onClose }: { onClose: () => void }) {
  const t = useTranslations("Management");

  return (
    <FormDialog
      layout="product"
      submitLabel={t("createUser")}
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
            required
            label={t("username")}
            registration={form.register("username")}
            error={form.formState.errors.username}
          />
          <FormInput
            required
            label={t("displayName")}
            registration={form.register("display_name")}
            error={form.formState.errors.display_name}
          />
          <FormInput
            fullRow
            label={t("email")}
            type="email"
            registration={form.register("email")}
            error={form.formState.errors.email}
          />
          <FormInput
            required
            label={t("password")}
            type="password"
            registration={form.register("password")}
            error={form.formState.errors.password}
          />
          <FormInput
            required
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

function UserSheet({
  user,
  edit,
  opener,
  onClose,
}: {
  user: ApiUser;
  edit: boolean;
  opener: HTMLElement | null;
  onClose: () => void;
}) {
  const t = useTranslations("Management");
  const common = useTranslations("Common");
  const perform = useAction();
  const auth = useIdentity();
  const canManage = useCan(permissions.IAMUsersManage);
  const canAssign = useCan(permissions.IAMUsersRoles);
  const canResetPassword = useCan(permissions.IAMUsersReset);
  const canReadRoles = useCan(permissions.IAMRolesRead);
  const self = auth.principal?.user_id === user.id;

  const [status, setStatus] = useState(user.status);
  const [assigned, setAssigned] = useState(user.roles);
  const [selection, setSelection] = useState(user.roles.map((role) => role.id));
  const [changingStatus, setChangingStatus] = useState(false);
  const [assigning, setAssigning] = useState(false);
  const details = useForm<z.infer<typeof userDetailsSchema>>({
    resolver: zodResolver(userDetailsSchema),
    defaultValues: { display_name: user.display_name, email: user.email ?? "" },
  });
  const {
    errors: detailsErrors,
    defaultValues: savedDetails,
    isDirty,
    isSubmitting,
  } = useFormState({
    control: details.control,
  });
  const busy = changingStatus || isSubmitting;
  const roleQuery = useQuery({
    queryKey: ["iam", "roles", "assignment", ...sessionScope(auth)],
    queryFn: ({ signal }) => listRoles({ limit: 100 }, signal),
    enabled: edit && canAssign && canReadRoles,
  });
  const roles = roleQuery.data?.items ?? [];
  const dirty =
    selection.length !== assigned.length || selection.some((id) => !assigned.some((role) => role.id === id));

  const resetForm = useForm<z.infer<typeof resetPasswordFormSchema>>({
    resolver: zodResolver(resetPasswordFormSchema),
    defaultValues: { password: "", confirm_password: "" },
  });

  const { errors: resetErrors, isSubmitting: resettingPassword } = useFormState({ control: resetForm.control });
  const saving = busy || assigning || resettingPassword;

  // 禁用按钮同时给出原因，避免使用者对着灰按钮猜。
  let statusLock: string | undefined;
  if (user.configured) statusLock = t("configurationManaged");
  else if (self && status === "active") statusLock = t("cannotDisableSelf");

  return (
    <SheetPanel
      layout="management"
      title={t(edit ? "editUser" : "userDetails")}
      description={edit ? (savedDetails?.display_name ?? user.display_name) : undefined}
      returnFocus={opener}
      onClose={() => {
        if (!saving) onClose();
      }}
      footer={
        edit && (
          <Button size="sm" variant="outline" onClick={onClose} disabled={saving}>
            {common("close")}
          </Button>
        )
      }
    >
      <div className="flex flex-col gap-5 py-4">
        <section className="flex flex-col gap-4">
          <dl className="grid gap-3 @min-[28rem]:grid-cols-2">
            <div>
              <dt className="text-muted-foreground text-xs">{t("username")}</dt>
              <dd className="text-sm [overflow-wrap:anywhere]">{user.username}</dd>
            </div>
            {(!edit || !canManage || user.configured) && (
              <>
                <div>
                  <dt className="text-muted-foreground text-xs">{t("displayName")}</dt>
                  <dd className="text-sm [overflow-wrap:anywhere]">{user.display_name}</dd>
                </div>
                <div className="col-span-full">
                  <dt className="text-muted-foreground text-xs">{t("email")}</dt>
                  <dd className="text-sm [overflow-wrap:anywhere]">{user.email || "—"}</dd>
                </div>
              </>
            )}
          </dl>
          {edit && canManage && !user.configured && (
            <form
              noValidate
              onSubmit={details.handleSubmit((values) =>
                perform(async () => {
                  await updateUser({ id: user.id, body: { ...values, status } });
                  details.reset(values);
                  await managementChanged();
                }),
              )}
            >
              <fieldset disabled={busy}>
                <FieldGroup>
                  <div className="grid gap-4 @min-[28rem]:grid-cols-2">
                    <FormInput
                      required
                      label={t("displayName")}
                      registration={details.register("display_name")}
                      error={detailsErrors.display_name}
                    />
                    <FormInput
                      label={t("email")}
                      type="email"
                      registration={details.register("email")}
                      error={detailsErrors.email}
                    />
                  </div>
                  <div>
                    <Button size="sm" type="submit" disabled={!isDirty || busy}>
                      {t("save")}
                    </Button>
                  </div>
                </FieldGroup>
              </fieldset>
            </form>
          )}
        </section>

        <section className="flex flex-col gap-3">
          <h3 className="text-sm font-medium">{t("status")}</h3>
          <div className="flex flex-wrap items-center gap-2">
            <StatusBadge status={status}>{status === "active" ? t("active") : t("disabled")}</StatusBadge>
            {edit && canManage && (
              <Button
                size="sm"
                variant="outline"
                disabled={user.configured || (self && status === "active") || busy}
                title={statusLock}
                onClick={() => {
                  setChangingStatus(true);
                  void perform(async () => {
                    const next = status === "active" ? "disabled" : "active";
                    await updateUser({
                      id: user.id,
                      body: {
                        display_name: savedDetails?.display_name ?? user.display_name,
                        status: next,
                      },
                    });
                    setStatus(next);
                    await managementChanged();
                  }).finally(() => setChangingStatus(false));
                }}
              >
                {status === "active" ? t("disable") : t("enable")}
              </Button>
            )}
            {user.configured && <span className="text-muted-foreground text-xs">{t("configurationManaged")}</span>}
          </div>
        </section>

        <section className="flex flex-col gap-3 border-t pt-4">
          <h3 className="text-sm font-medium">{t("roles")}</h3>
          {edit && canAssign && canReadRoles ? (
            <FieldSet>
              <FieldLegend variant="label" className="sr-only">
                {t("roles")}
              </FieldLegend>
              <CollectionBoundary query={roleQuery} hasContent={roles.length > 0}>
                {roles.length === 0 && <CollectionState kind="empty" title={t("noRoles")} />}
                {roles.map((role) => {
                  // 配置管理员的内置角色由部署配置保证，勾选状态因此在页面上锁定。
                  const locked = user.configured && role.builtin;
                  return (
                    <Field key={role.id} orientation="horizontal">
                      <Checkbox
                        id={`user-role-${user.id}-${role.id}`}
                        checked={selection.includes(role.id)}
                        disabled={locked || assigning}
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
              </CollectionBoundary>
              <div>
                <Button
                  size="sm"
                  disabled={!dirty || assigning || !roleQuery.isSuccess}
                  onClick={() => {
                    setAssigning(true);
                    void perform(async () => {
                      await setUserRoles({ id: user.id, body: { role_ids: selection } });
                      setAssigned(
                        roles
                          .filter((role) => selection.includes(role.id))
                          .map((role) => ({ id: role.id, name: role.name })),
                      );
                      await managementChanged();
                    }).finally(() => setAssigning(false));
                  }}
                >
                  {t("save")}
                </Button>
              </div>
            </FieldSet>
          ) : (
            <UserRoles roles={assigned} />
          )}
        </section>

        {edit && canResetPassword && !user.configured && (
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
              <fieldset disabled={resettingPassword}>
                <FieldGroup>
                  <div className="grid gap-4 @min-[28rem]:grid-cols-2">
                    <FormInput
                      required
                      label={t("newPassword")}
                      type="password"
                      registration={resetForm.register("password")}
                      error={resetErrors.password}
                    />
                    <FormInput
                      required
                      label={t("confirmPassword")}
                      type="password"
                      registration={resetForm.register("confirm_password")}
                      error={resetErrors.confirm_password}
                      errorText={
                        resetErrors.confirm_password?.message === "passwordMismatch" ? t("passwordMismatch") : undefined
                      }
                    />
                  </div>
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
      </div>
    </SheetPanel>
  );
}
