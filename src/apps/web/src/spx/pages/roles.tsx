"use client";

import { useState } from "react";

import { useMutation, useQuery } from "@tanstack/react-query";
import { PencilIcon, PlusIcon, SearchIcon } from "lucide-react";
import { useTranslations } from "next-intl";
import { z } from "zod";

import { permissions } from "@/auth/permissions.gen";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Field, FieldGroup, FieldLabel, FieldLegend, FieldSet } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { InputGroup, InputGroupAddon, InputGroupInput } from "@/components/ui/input-group";
import { Textarea } from "@/components/ui/textarea";
import {
  createRole,
  deleteRole,
  listRoles,
  permissions as permissionCatalog,
  setRolePermissions,
  updateRole,
} from "@/spx/api/iam/management";
import type { ApiRole } from "@/spx/api/iam/schemas";

import { Can, useCan } from "../components/access";
import { DataTable, useAction, usePagedQuery } from "../components/data";
import { FormDialog } from "../components/forms";
import { PageHeader } from "../components/page-header";
import { SheetPanel } from "../components/sheet-panel";
import { Toolbar } from "../components/toolbar";
import { managementChanged } from "../iam/management";
import { sessionScope } from "../iam/query";
import { useIdentity } from "../iam/session";

type PermissionCatalog = ReturnType<typeof usePermissionCatalog>;

function usePermissionCatalog() {
  const auth = useIdentity();
  const query = useQuery({
    queryKey: ["iam", "permissions", ...sessionScope(auth)],
    queryFn: ({ signal }) => permissionCatalog(signal),
    enabled: auth.status === "authenticated",
  });

  return query.data?.items ?? [];
}

// PermissionPicker 是创建与编辑共用的权限勾选区，按资源类型分组。
// 抽屉加宽后按两列排布，避免单个分组被拉成一条很长的窄列。
function PermissionPicker({
  catalog,
  selected,
  onChange,
  disabled,
}: {
  catalog: PermissionCatalog;
  selected: string[];
  onChange: (next: string[]) => void;
  disabled?: boolean;
}) {
  const t = useTranslations("Management");
  const resources = [...new Set(catalog.map((item) => item.permission.resource_type))];

  return (
    <div className="grid gap-x-8 gap-y-6 sm:grid-cols-2">
      {resources.map((resource) => (
        <FieldSet key={resource}>
          {/* 分组标题必须弱于上方的「权限」标题，否则层级倒挂。 */}
          <FieldLegend variant="label" className="text-muted-foreground font-medium">
            {({ users: t("user"), roles: t("role"), audit: t("audit") } as Record<string, string>)[resource] ||
              resource}
          </FieldLegend>
          {catalog
            .filter((item) => item.permission.resource_type === resource)
            .map(({ permission, can_delegate }) => (
              <Field key={permission.key} orientation="horizontal">
                <Checkbox
                  id={`permission-${permission.key}`}
                  checked={selected.includes(permission.key)}
                  disabled={disabled === true || !can_delegate}
                  onCheckedChange={(checked) =>
                    onChange(
                      checked === true
                        ? [...selected, permission.key]
                        : selected.filter((key) => key !== permission.key),
                    )
                  }
                />
                <FieldLabel htmlFor={`permission-${permission.key}`}>
                  {permission.description || permission.key}
                </FieldLabel>
              </Field>
            ))}
        </FieldSet>
      ))}
    </div>
  );
}

// CreateRoleSheet 从右侧创建角色：名称、说明与权限一次提交。
function CreateRoleSheet({ catalog, onClose }: { catalog: PermissionCatalog; onClose: () => void }) {
  const t = useTranslations("Management");
  const common = useTranslations("Common");
  const perform = useAction();
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [grants, setGrants] = useState<string[]>([]);
  const create = useMutation({ mutationFn: createRole, onSuccess: managementChanged });
  const canSubmit = name.trim().length > 0 && !create.isPending;

  return (
    <SheetPanel
      title={t("createRole")}
      onClose={onClose}
      footer={
        <>
          <Button size="sm" variant="outline" onClick={onClose} disabled={create.isPending}>
            {common("cancel")}
          </Button>
          <Button
            size="sm"
            disabled={!canSubmit}
            onClick={() =>
              void perform(async () => {
                await create.mutateAsync({ name, description: description || undefined, permissions: grants });
                onClose();
              })
            }
          >
            {t("createRole")}
          </Button>
        </>
      }
    >
      <div className="flex flex-col gap-8 px-6 py-5">
        <FieldGroup>
          <Field>
            <FieldLabel htmlFor="new-role-name">{t("roleName")}</FieldLabel>
            <Input
              id="new-role-name"
              value={name}
              maxLength={100}
              placeholder={t("exampleRoleName")}
              onChange={(event) => setName(event.target.value)}
            />
          </Field>
          <Field>
            <FieldLabel htmlFor="new-role-description">{t("description")}</FieldLabel>
            <Textarea
              id="new-role-description"
              value={description}
              maxLength={1000}
              onChange={(event) => setDescription(event.target.value)}
            />
          </Field>
        </FieldGroup>

        <section className="flex flex-col gap-4">
          <h3 className="text-sm font-medium">{t("permissions")}</h3>
          <PermissionPicker catalog={catalog} selected={grants} onChange={setGrants} disabled={create.isPending} />
        </section>
      </div>
    </SheetPanel>
  );
}

// EditRoleSheet 按「角色信息 / 角色权限」两段展示，两段各自保存；
// 角色成员属于用户管理，不在这里重复展示。
function EditRoleSheet({ role, catalog, onClose }: { role: ApiRole; catalog: PermissionCatalog; onClose: () => void }) {
  const t = useTranslations("Management");
  const common = useTranslations("Common");
  const perform = useAction();

  const [name, setName] = useState(role.name);
  const [description, setDescription] = useState(role.description);
  const [grants, setGrants] = useState(role.permissions);

  const edit = useMutation({ mutationFn: updateRole, onSuccess: managementChanged });
  const grantsMutation = useMutation({ mutationFn: setRolePermissions, onSuccess: managementChanged });
  const busy = edit.isPending || grantsMutation.isPending;

  return (
    <SheetPanel
      title={t("editRole")}
      description={role.name}
      onClose={onClose}
      footer={
        <Button size="sm" variant="outline" onClick={onClose} disabled={busy}>
          {common("close")}
        </Button>
      }
    >
      <div className="flex flex-col gap-8 px-6 py-5">
        <section className="flex flex-col gap-4">
          <h3 className="text-sm font-medium">{t("roleInfo")}</h3>
          <FieldGroup>
            <Field>
              <FieldLabel htmlFor="role-name">{t("roleName")}</FieldLabel>
              <Input id="role-name" value={name} maxLength={100} onChange={(event) => setName(event.target.value)} />
            </Field>
            <Field>
              <FieldLabel htmlFor="role-description">{t("description")}</FieldLabel>
              <Textarea
                id="role-description"
                value={description}
                maxLength={1000}
                onChange={(event) => setDescription(event.target.value)}
              />
            </Field>
          </FieldGroup>
          <div>
            <Button
              size="sm"
              disabled={busy || (name === role.name && description === role.description)}
              onClick={() =>
                void perform(async () => {
                  await edit.mutateAsync({ id: role.id, body: { name, description } });
                })
              }
            >
              {t("saveRole")}
            </Button>
          </div>
        </section>

        <section className="flex flex-col gap-4 border-t pt-6">
          <h3 className="text-sm font-medium">{t("permissions")}</h3>
          <PermissionPicker catalog={catalog} selected={grants} onChange={setGrants} disabled={busy} />
          <div>
            <Button
              size="sm"
              disabled={busy || [...grants].sort().join() === [...role.permissions].sort().join()}
              onClick={() =>
                void perform(async () => {
                  await grantsMutation.mutateAsync({ id: role.id, body: { permissions: grants } });
                })
              }
            >
              {t("savePermissions")}
            </Button>
          </div>
        </section>
      </div>
    </SheetPanel>
  );
}

// BuiltinRoleSheet 只读展示内置角色的权限：权限目录来自接口，页面不写死任何行。
function BuiltinRoleSheet({
  role,
  catalog,
  onClose,
}: {
  role: ApiRole;
  catalog: PermissionCatalog;
  onClose: () => void;
}) {
  const t = useTranslations("Management");
  const common = useTranslations("Common");

  return (
    <SheetPanel
      title={role.name}
      onClose={onClose}
      footer={
        <Button size="sm" variant="outline" onClick={onClose}>
          {common("close")}
        </Button>
      }
    >
      <div className="flex flex-col gap-3 px-6 py-5">
        {role.permissions.length ? (
          role.permissions.map((key) => (
            <div key={key} className="flex items-center gap-2">
              <Badge variant="secondary">{key}</Badge>
              <span className="text-muted-foreground text-sm">
                {catalog.find((item) => item.permission.key === key)?.permission.description ?? ""}
              </span>
            </div>
          ))
        ) : (
          <p className="text-muted-foreground text-sm">{t("none")}</p>
        )}
      </div>
    </SheetPanel>
  );
}

export function RolesPage() {
  const t = useTranslations("Management");
  const navigation = useTranslations("Navigation");
  const canManage = useCan(permissions.IAMRolesManage);
  const catalog = usePermissionCatalog();

  const [term, setTerm] = useState("");
  const [applied, setApplied] = useState("");
  const state = usePagedQuery<ApiRole>(["iam", "roles", applied], (query, signal) =>
    listRoles({ ...query, q: applied || undefined }, signal),
  );

  const [creating, setCreating] = useState(false);
  const [editing, setEditing] = useState<ApiRole>();
  const [deleting, setDeleting] = useState<ApiRole>();
  const remove = useMutation({ mutationFn: deleteRole, onSuccess: managementChanged });

  return (
    <div className="flex flex-col gap-4">
      <PageHeader title={navigation("roles")} />

      <Toolbar
        actions={
          <Can action={permissions.IAMRolesManage}>
            <Button size="sm" onClick={() => setCreating(true)}>
              <PlusIcon data-icon="inline-start" />
              {t("createRole")}
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
            placeholder={t("searchRolesPlaceholder")}
            aria-label={t("searchRolesPlaceholder")}
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
          {
            title: t("roleName"),
            cell: (role) => (
              <div className="flex items-center gap-2">
                <span>{role.name}</span>
                {role.builtin && <Badge variant="secondary">{t("builtin")}</Badge>}
              </div>
            ),
          },
          { title: t("description"), cell: (role) => role.description || "—" },
          { title: t("userCount"), cell: (role) => role.user_count },
          {
            title: t("action"),
            cell: (role) => {
              let deleteTitle: string | undefined;
              if (role.builtin) deleteTitle = t("builtinRoleLocked");
              else if (role.user_count > 0) deleteTitle = t("deleteRoleHasUsers");

              return (
                <div className="flex items-center gap-2">
                  <Button variant="ghost" size="sm" onClick={() => setEditing(role)}>
                    <PencilIcon data-icon="inline-start" />
                    {t("edit")}
                  </Button>
                  {canManage && (
                    <Button
                      variant="ghost"
                      size="sm"
                      disabled={role.builtin || role.user_count > 0 || remove.isPending}
                      title={deleteTitle}
                      onClick={() => setDeleting(role)}
                    >
                      {t("delete")}
                    </Button>
                  )}
                </div>
              );
            },
          },
        ]}
      />

      {deleting && (
        <FormDialog
          title={t("deleteRole")}
          description={t("deleteRoleDescription", { name: deleting.name })}
          confirmVariant="destructive"
          schema={z.object({})}
          defaultValues={{}}
          onClose={() => setDeleting(undefined)}
          onSubmit={async () => {
            await remove.mutateAsync({ id: deleting.id });
            setDeleting(undefined);
          }}
        />
      )}

      {creating && <CreateRoleSheet catalog={catalog} onClose={() => setCreating(false)} />}
      {editing &&
        (editing.builtin ? (
          <BuiltinRoleSheet role={editing} catalog={catalog} onClose={() => setEditing(undefined)} />
        ) : (
          <EditRoleSheet role={editing} catalog={catalog} onClose={() => setEditing(undefined)} />
        ))}
    </div>
  );
}
