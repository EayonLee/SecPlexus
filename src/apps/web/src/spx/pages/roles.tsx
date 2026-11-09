"use client";

import { useState } from "react";

import { useMutation, useQuery } from "@tanstack/react-query";
import { PencilIcon, PlusIcon, SearchIcon } from "lucide-react";
import { useTranslations } from "next-intl";

import { permissions } from "@/auth/permissions.gen";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Field, FieldGroup, FieldLabel, FieldLegend, FieldSet } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { Textarea } from "@/components/ui/textarea";
import {
  createRole,
  deleteRole,
  listRoleUsers,
  listRoles,
  permissions as permissionCatalog,
  setRolePermissions,
  updateRole,
} from "@/spx/api/iam/management";
import type { ApiOrganizationUser, ApiRole } from "@/spx/api/iam/schemas";

import { Can, useCan } from "../components/access";
import { DataTable, useAction, usePagedQuery } from "../components/data";
import { managementChanged } from "../iam/management";
import { identityScope } from "../iam/query";
import { useIdentity } from "../iam/session";

type PermissionCatalog = ReturnType<typeof usePermissionCatalog>;

function usePermissionCatalog() {
  const auth = useIdentity();
  const org = auth.org_id ?? "";
  const query = useQuery({
    queryKey: ["iam", "permissions", org, ...identityScope(auth)],
    queryFn: ({ signal }) => permissionCatalog(org, signal),
    enabled: auth.status === "authenticated",
  });

  return query.data?.items ?? [];
}

// PermissionPicker 是创建与编辑共用的权限勾选区，按资源类型分组。
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
    <div className="flex flex-col gap-5">
      {resources.map((resource) => (
        <FieldSet key={resource}>
          {/* 分组标题必须弱于上方的「权限」标题，否则层级倒挂。 */}
          <FieldLegend variant="label" className="text-muted-foreground font-medium">
            {(
              {
                organizations: t("organization"),
                users: t("user"),
                roles: t("role"),
                audit: t("audit"),
              } as Record<string, string>
            )[resource] || resource}
          </FieldLegend>
          {catalog
            .filter((item) => item.permission.resource_type === resource)
            .map(({ permission, can_delegate }) => (
              <Field key={permission.key} orientation="horizontal">
                <Checkbox
                  id={`permission-${permission.key}`}
                  checked={selected.includes(permission.key)}
                  disabled={disabled || !can_delegate}
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

function RoleMembers({ orgID, roleID }: { orgID: string; roleID: string }) {
  const t = useTranslations("Management");
  const state = usePagedQuery<ApiOrganizationUser>(["iam", "role-users", orgID, roleID], (query, signal) =>
    listRoleUsers(orgID, roleID, query, signal),
  );

  return (
    <DataTable
      state={state}
      columns={[
        { title: t("username"), cell: (member) => member.username },
        { title: t("displayName"), cell: (member) => member.display_name },
      ]}
    />
  );
}

// CreateRoleSheet 从右侧创建角色：名称、说明与权限一次提交。
function CreateRoleSheet({
  orgID,
  catalog,
  onClose,
}: {
  orgID: string;
  catalog: PermissionCatalog;
  onClose: () => void;
}) {
  const t = useTranslations("Management");
  const perform = useAction();
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [grants, setGrants] = useState<string[]>([]);
  const create = useMutation({ mutationFn: createRole, onSuccess: managementChanged });
  const canSubmit = name.trim().length > 0 && !create.isPending;

  return (
    <Sheet
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      <SheetContent className="flex w-full flex-col gap-0 overflow-y-auto sm:max-w-lg">
        <SheetHeader>
          <SheetTitle>{t("createRole")}</SheetTitle>
          <SheetDescription>{t("createRoleDescription")}</SheetDescription>
        </SheetHeader>

        <div className="flex flex-1 flex-col gap-6 px-4 pb-4">
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

          <section className="flex flex-col gap-3">
            <h3 className="text-sm font-medium">{t("permissions")}</h3>
            <PermissionPicker catalog={catalog} selected={grants} onChange={setGrants} />
          </section>
        </div>

        <SheetFooter>
          <Button
            disabled={!canSubmit}
            onClick={() =>
              void perform(async () => {
                await create.mutateAsync({
                  orgId: orgID,
                  body: { name, description: description || undefined, permissions: grants },
                });
                onClose();
              })
            }
          >
            {t("createRole")}
          </Button>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  );
}

// EditRoleSheet 按「角色信息 / 角色权限 / 角色成员」三段展示；
// 信息与权限可改，成员只读。
function EditRoleSheet({
  orgID,
  role,
  catalog,
  onClose,
}: {
  orgID: string;
  role: ApiRole;
  catalog: PermissionCatalog;
  onClose: () => void;
}) {
  const t = useTranslations("Management");
  const perform = useAction();
  const canMembers = useCan(permissions.IAMUsersRead);

  const [name, setName] = useState(role.name);
  const [description, setDescription] = useState(role.description);
  const [grants, setGrants] = useState(role.permissions);

  const edit = useMutation({ mutationFn: updateRole, onSuccess: managementChanged });
  const grantsMutation = useMutation({ mutationFn: setRolePermissions, onSuccess: managementChanged });
  const busy = edit.isPending || grantsMutation.isPending;

  return (
    <Sheet
      open
      onOpenChange={(open) => {
        if (!open && !busy) onClose();
      }}
    >
      <SheetContent className="flex w-full flex-col gap-0 overflow-y-auto sm:max-w-xl">
        <SheetHeader>
          <SheetTitle>{t("editRole")}</SheetTitle>
          <SheetDescription>{role.name}</SheetDescription>
        </SheetHeader>

        <div className="flex flex-1 flex-col gap-8 px-4 pb-4">
          <section className="flex flex-col gap-3">
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
              <Field orientation="horizontal">
                <Button
                  disabled={busy || (name === role.name && description === role.description)}
                  onClick={() =>
                    void perform(async () => {
                      await edit.mutateAsync({
                        orgId: orgID,
                        id: role.id,
                        body: { name, description, version: role.version },
                      });
                    })
                  }
                >
                  {t("saveRole")}
                </Button>
              </Field>
            </FieldGroup>
          </section>

          <section className="flex flex-col gap-3">
            <h3 className="text-sm font-medium">{t("permissions")}</h3>
            <PermissionPicker catalog={catalog} selected={grants} onChange={setGrants} disabled={busy} />
            <div>
              <Button
                disabled={busy || [...grants].sort().join() === [...role.permissions].sort().join()}
                onClick={() =>
                  void perform(async () => {
                    await grantsMutation.mutateAsync({
                      orgId: orgID,
                      id: role.id,
                      body: { permissions: grants, version: role.version },
                    });
                  })
                }
              >
                {t("savePermissions")}
              </Button>
            </div>
          </section>

          {canMembers && (
            <section className="flex flex-col gap-3">
              <h3 className="text-sm font-medium">{t("roleMembers")}</h3>
              <RoleMembers orgID={orgID} roleID={role.id} />
            </section>
          )}
        </div>
      </SheetContent>
    </Sheet>
  );
}

export function RolesPage() {
  const t = useTranslations("Management");
  const perform = useAction();

  const auth = useIdentity();
  const org = auth.org_id ?? "";
  const catalog = usePermissionCatalog();

  const [term, setTerm] = useState("");
  const [applied, setApplied] = useState("");
  const state = usePagedQuery<ApiRole>(["iam", "roles", org, applied], (query, signal) =>
    listRoles(org, { ...query, q: applied || undefined }, signal),
  );

  const [creating, setCreating] = useState(false);
  const [editing, setEditing] = useState<ApiRole>();
  const remove = useMutation({ mutationFn: deleteRole, onSuccess: managementChanged });

  return (
    <div className="flex flex-col gap-4">
      <h2 className="text-base font-medium">{t("rolesTitle")}</h2>
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-2">
          <Input
            className="h-9 w-56"
            value={term}
            placeholder={t("searchRolesPlaceholder")}
            onChange={(event) => setTerm(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === "Enter") setApplied(term);
            }}
          />
          <Button variant="outline" onClick={() => setApplied(term)}>
            <SearchIcon data-icon="inline-start" />
            {t("auditSearch")}
          </Button>
        </div>
        <Can action={permissions.IAMRolesManage}>
          <Button onClick={() => setCreating(true)}>
            <PlusIcon data-icon="inline-start" />
            {t("createRole")}
          </Button>
        </Can>
      </div>

      <DataTable
        state={state}
        columns={[
          { title: t("roleName"), cell: (role) => role.name },
          { title: t("description"), cell: (role) => role.description || "—" },
          { title: t("memberCount"), cell: (role) => role.member_count },
          {
            title: t("action"),
            cell: (role) => (
              <div className="flex gap-2">
                <Button variant="ghost" onClick={() => setEditing(role)}>
                  <PencilIcon data-icon="inline-start" />
                  {t("edit")}
                </Button>
                {role.allowed_actions.includes(permissions.IAMRolesManage) && (
                  <Button
                    variant="ghost"
                    disabled={role.member_count > 0 || remove.isPending}
                    title={role.member_count > 0 ? t("deleteRoleHasMembers") : undefined}
                    onClick={() =>
                      void perform(async () => {
                        await remove.mutateAsync({ orgId: org, id: role.id, version: role.version });
                      })
                    }
                  >
                    {t("delete")}
                  </Button>
                )}
              </div>
            ),
          },
        ]}
      />

      {creating && <CreateRoleSheet orgID={org} catalog={catalog} onClose={() => setCreating(false)} />}
      {editing && <EditRoleSheet orgID={org} role={editing} catalog={catalog} onClose={() => setEditing(undefined)} />}
    </div>
  );
}
