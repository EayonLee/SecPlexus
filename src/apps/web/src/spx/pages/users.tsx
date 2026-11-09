"use client";
import { useState } from "react";

import { useMutation, useQuery } from "@tanstack/react-query";
import { useTranslations } from "next-intl";
import { Controller } from "react-hook-form";
import { z } from "zod";

import { permissions } from "@/auth/permissions.gen";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Field, FieldError, FieldLabel, FieldLegend, FieldSet } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import {
  createOrganizationUser,
  deleteOrganizationUser,
  listOrganizationTree,
  listOrganizationUsers,
  listRoles,
  setOrganizationUserRoles,
  updateOrganizationUser,
} from "@/spx/api/iam/management";
import type { ApiOrganization, ApiOrganizationUser } from "@/spx/api/iam/schemas";
import {
  zCreateOrganizationUserBody,
  zSetOrganizationUserRolesBody,
  zUpdateOrganizationUserBody,
} from "@/spx/api/iam/schemas";

import { Can, useCan } from "../components/access";
import { DataTable, useAction, usePagedQuery } from "../components/data";
import { FormDialog, FormInput } from "../components/forms";
import { OrganizationSplitPane } from "../components/organization-split-pane";
import { StatusBadge } from "../components/status-badge";
import { UserPicker } from "../components/user-picker";
import { managementChanged } from "../iam/management";
import { identityScope } from "../iam/query";
import { errorMessageKey, useIdentity } from "../iam/session";

// 创建用户有两种入口：新建账号，或把已有账号加入组织。
// 表单 schema 固定为宽集合，按当前模式在提交前收敛必填项。
const createUserFormSchema = zCreateOrganizationUserBody
  .pick({ user_id: true, username: true, display_name: true, password: true })
  .superRefine((value, ctx) => {
    const existingMode = !!value.user_id && !value.username;
    if (existingMode) return;
    for (const key of ["username", "display_name", "password"] as const) {
      if (!value[key]) ctx.addIssue({ code: "custom", path: [key], message: "required" });
    }
  });

// UsersPage 是组织与平台统一后的用户管理：
// 左侧组织层级树，右侧是所选组织子树下的用户；默认选中根组织即全部用户。
export function UsersPage() {
  const t = useTranslations("Management");
  const auth = useIdentity();

  const tree = useQuery({
    queryKey: ["iam", "organizations", "tree"],
    queryFn: ({ signal }) => listOrganizationTree(signal),
  });
  const items = tree.data?.items ?? [];

  return (
    <div className="flex flex-col gap-6">
      <h2 className="text-base font-medium">{t("userManage")}</h2>

      {!tree.isPending && !items.length ? (
        <Skeleton className="h-64 w-full" />
      ) : (
        <OrganizationSplitPane
          items={items}
          loading={tree.isPending}
          heading={t("organizationTree")}
          description={t("organizationTreeDescription")}
        >
          {(selected) => <UserList root={selected} organizations={items} fallbackOrgID={auth.org_id} />}
        </OrganizationSplitPane>
      )}
    </div>
  );
}

function UserList({
  root,
  organizations,
  fallbackOrgID,
}: {
  root: ApiOrganization | undefined;
  organizations: ApiOrganization[];
  fallbackOrgID?: string;
}) {
  const perform = useAction();
  const t = useTranslations("Management");
  const errors = useTranslations("Errors");
  const auth = useIdentity();

  // 组织范围由左侧选中项决定；选中根组织时 scope=subtree 即全部用户。
  const scopeOrg = root?.id ?? fallbackOrgID ?? "";
  const [term, setTerm] = useState("");
  const [applied, setApplied] = useState("");

  const state = usePagedQuery<ApiOrganizationUser>(["iam", "users", scopeOrg, applied], (query, signal) =>
    listOrganizationUsers(scopeOrg, { ...query, scope: "subtree", q: applied || undefined }, signal),
  );

  const [creating, setCreating] = useState(false);
  const [createOrg, setCreateOrg] = useState<string | undefined>(root?.id);
  const [existing, setExisting] = useState(false);
  const [assigning, setAssigning] = useState<ApiOrganizationUser | undefined>(undefined);
  const [removing, setRemoving] = useState<ApiOrganizationUser | undefined>(undefined);

  const canAssign = useCan(permissions.IAMRolesAssign);
  const canReadRoles = useCan(permissions.IAMRolesRead);
  const roleQuery = useQuery({
    queryKey: ["iam", "roles", scopeOrg, "assignment", ...identityScope(auth)],
    queryFn: ({ signal }) => listRoles(scopeOrg, { limit: 100 }, signal),
    enabled: !!assigning && canReadRoles,
  });
  const roles = roleQuery.data?.items ?? [];

  const addExisting = useMutation({ mutationFn: createOrganizationUser, onSuccess: managementChanged });
  const edit = useMutation({ mutationFn: updateOrganizationUser, onSuccess: managementChanged });
  const remove = useMutation({ mutationFn: deleteOrganizationUser, onSuccess: managementChanged });
  const assign = useMutation({ mutationFn: setOrganizationUserRoles, onSuccess: managementChanged });
  const [selected, setSelected] = useState<string[]>([]);
  const version = auth.organizations.find((entry) => entry.id === scopeOrg)?.authz_epoch;
  const [assignOrg, setAssignOrg] = useState<string | undefined>();
  const assignVersion = auth.organizations.find((entry) => entry.id === (assignOrg ?? scopeOrg))?.authz_epoch;

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-2">
          <Input
            className="h-9 w-56"
            value={term}
            placeholder={t("searchUsersPlaceholder")}
            onChange={(event) => setTerm(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === "Enter") setApplied(term);
            }}
          />
          <Button variant="outline" onClick={() => setApplied(term)}>
            {t("auditSearch")}
          </Button>
        </div>
        <Can action={permissions.IAMUsersManage}>
          <Button
            onClick={() => {
              setCreateOrg(root?.id);
              setExisting(false);
              setCreating(true);
            }}
          >
            {t("addUser")}
          </Button>
        </Can>
      </div>

      <DataTable
        state={state}
        columns={[
          { title: t("username"), cell: (user) => user.username },
          { title: t("displayName"), cell: (user) => user.display_name },
          {
            title: t("userOrganization"),
            cell: (user) => <span className="text-muted-foreground text-sm">{user.org_slug ?? "—"}</span>,
          },
          {
            title: t("status"),
            cell: (user) => (
              <StatusBadge status={user.status}>{user.status === "active" ? t("active") : t("disabled")}</StatusBadge>
            ),
          },
          {
            title: t("action"),
            cell: (user) =>
              user.is_super_admin ? (
                <StatusBadge status="system">{t("configurationManaged")}</StatusBadge>
              ) : (
                <div className="flex gap-2">
                  {canAssign && canReadRoles && user.org_id && (
                    <Button
                      variant="ghost"
                      onClick={() =>
                        void perform(async () => {
                          setAssignOrg(user.org_id);
                          setSelected(user.role_ids ?? []);
                          setAssigning(user);
                        })
                      }
                    >
                      {t("assignRoles")}
                    </Button>
                  )}
                  <Can action={permissions.IAMUsersManage}>
                    <Button
                      variant="ghost"
                      disabled={edit.isPending}
                      onClick={() =>
                        void perform(async () => {
                          await edit.mutateAsync({
                            orgId: user.org_id ?? scopeOrg,
                            id: user.id,
                            body: zUpdateOrganizationUserBody.parse({
                              status: user.status === "active" ? "disabled" : "active",
                              authz_epoch: version,
                            }),
                          });
                        })
                      }
                    >
                      {user.status === "active" ? t("disabled") : t("active")}
                    </Button>
                    <Button variant="ghost" onClick={() => setRemoving(user)}>
                      {t("remove")}
                    </Button>
                  </Can>
                </div>
              ),
          },
        ]}
      />

      {creating && (
        <FormDialog
          key={existing ? "existing" : "new"}
          title={t("addUser")}
          schema={createUserFormSchema}
          defaultValues={{ user_id: "", username: "", display_name: "", password: "" }}
          onClose={() => setCreating(false)}
          onSubmit={async (body) => {
            await addExisting.mutateAsync({ orgId: createOrg ?? scopeOrg, body });
            await managementChanged();
          }}
        >
          {(form) => (
            <>
              <Field>
                <FieldLabel htmlFor="user-organization">{t("userOrganization")}</FieldLabel>
                <Select value={createOrg ?? scopeOrg} onValueChange={setCreateOrg}>
                  <SelectTrigger id="user-organization">
                    <SelectValue placeholder={t("chooseOrganization")} />
                  </SelectTrigger>
                  <SelectContent>
                    {organizations.map((org) => (
                      <SelectItem key={org.id} value={org.id}>
                        {org.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </Field>
              {auth.principal?.is_super_admin && (
                <Field orientation="horizontal">
                  <Checkbox
                    id="existing-user"
                    checked={existing}
                    onCheckedChange={(checked) => setExisting(checked === true)}
                  />
                  <FieldLabel htmlFor="existing-user">{t("addExistingAccount")}</FieldLabel>
                </Field>
              )}
              {existing ? (
                <Field>
                  <FieldLabel htmlFor="existing-user-picker">{t("existingAccount")}</FieldLabel>
                  <Controller
                    control={form.control}
                    name="user_id"
                    render={({ field }) => (
                      <UserPicker
                        id="existing-user-picker"
                        name={field.name}
                        ref={field.ref}
                        onBlur={field.onBlur}
                        required
                        invalid={!!form.formState.errors.user_id}
                        value={field.value}
                        onValueChange={field.onChange}
                      />
                    )}
                  />
                </Field>
              ) : (
                <>
                  <FormInput
                    label={t("newUsername")}
                    registration={form.register("username")}
                    error={form.formState.errors.username}
                  />
                  <FormInput
                    label={t("displayName")}
                    registration={form.register("display_name")}
                    error={form.formState.errors.display_name}
                  />
                  <FormInput
                    label={t("password")}
                    type="password"
                    registration={form.register("password")}
                    error={form.formState.errors.password}
                  />
                </>
              )}
            </>
          )}
        </FormDialog>
      )}

      {assigning && (
        <FormDialog
          title={t("assignUserRoles", { name: assigning.display_name })}
          submitDisabled={!roleQuery.isSuccess}
          schema={z.object({})}
          defaultValues={{}}
          onClose={() => setAssigning(undefined)}
          onSubmit={async () => {
            await assign.mutateAsync({
              orgId: assignOrg ?? scopeOrg,
              id: assigning.id,
              body: zSetOrganizationUserRolesBody.parse({ role_ids: selected, authz_epoch: assignVersion }),
            });
          }}
        >
          {() => (
            <FieldSet>
              <FieldLegend>{t("userRoles")}</FieldLegend>
              {roleQuery.error && <FieldError>{errors(errorMessageKey(roleQuery.error))}</FieldError>}
              {roles.map((role) => (
                <Field key={role.id} orientation="horizontal">
                  <Checkbox
                    id={`user-role-${role.id}`}
                    checked={selected.includes(role.id)}
                    onCheckedChange={(checked) =>
                      setSelected((current) =>
                        checked === true ? [...current, role.id] : current.filter((id) => id !== role.id),
                      )
                    }
                  />
                  <FieldLabel htmlFor={`user-role-${role.id}`}>{role.name}</FieldLabel>
                </Field>
              ))}
            </FieldSet>
          )}
        </FormDialog>
      )}

      {removing && (
        <FormDialog
          title={t("removeUser")}
          description={t("removeUserDescription", { name: removing.display_name })}
          schema={z.object({})}
          defaultValues={{}}
          onClose={() => setRemoving(undefined)}
          onSubmit={async () => {
            await remove.mutateAsync({
              orgId: removing.org_id ?? scopeOrg,
              id: removing.id,
              version: version ?? "",
            });
          }}
        />
      )}
    </div>
  );
}
