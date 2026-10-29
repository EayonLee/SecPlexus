"use client";
import { useState } from "react";

import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQuery } from "@tanstack/react-query";
import { useTranslations } from "next-intl";
import { useForm } from "react-hook-form";
import { toast } from "sonner";
import { z } from "zod";

import { permissions } from "@/auth/permissions.gen";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Field, FieldError, FieldGroup, FieldLabel, FieldLegend, FieldSet } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { NativeSelect, NativeSelectOption } from "@/components/ui/native-select";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Textarea } from "@/components/ui/textarea";
import {
  listRoleMembersQueryKey,
  listRolesOptions,
  listRolesQueryKey,
  permissionsQueryKey,
  setRolePermissionsMutation,
} from "@/spx/api/iam/@tanstack/react-query.gen";
import {
  createRole,
  deleteRole,
  getRole,
  listRoleMembers,
  permissions as permissionCatalog,
  previewPermission,
  updateRole,
} from "@/spx/api/iam/sdk.gen";
import type { ApiPreview, ApiRole } from "@/spx/api/iam/types.gen";
import {
  zCreateRoleBody,
  zPreviewPermissionBody,
  zSetRolePermissionsBody,
  zUpdateRoleBody,
} from "@/spx/api/iam/zod.gen";

import { Can, useCan } from "../components/access";
import { DataTable, useAction } from "../components/data";
import { FormDialog, FormInput } from "../components/forms";
import { identityScope, queryClient } from "../iam/query";
import { dataOf, errorMessageKey, mutationHeaders, refreshIdentity, useIdentity } from "../iam/session";
import { useRoleLabel } from "../iam/use-role-label";

export function RolesPage() {
  const roleLabel = useRoleLabel();
  const roleForm = useForm<z.infer<typeof zUpdateRoleBody>>({ resolver: zodResolver(zUpdateRoleBody) });
  const perform = useAction();

  const t = useTranslations("Management");
  const errors = useTranslations("Errors");
  const permissionText = useTranslations("Permissions");
  const permissionLabels = Object.fromEntries(Object.values(permissions).map((key) => [key, permissionText(key)]));

  const auth = useIdentity();
  const org = auth.org_id ?? "";
  const [page, setPage] = useState(1);
  const options = listRolesOptions({ path: { org_id: org }, query: { limit: 20, offset: (page - 1) * 20 } });
  // 使用生成键的 tags 元数据隔离身份/授权版本，不发送额外身份 HTTP 头。
  options.queryKey[0].tags = identityScope(auth).map((value) => value ?? "");
  const roleQuery = useQuery({
    ...options,
    enabled: auth.status === "authenticated",
    select: (response) => response.data,
  });
  const state = {
    items: roleQuery.data?.items ?? [],
    total: roleQuery.data?.total ?? 0,
    loading: roleQuery.isFetching,
    error: roleQuery.error ? errorMessageKey(roleQuery.error) : ("" as const),
    page,
    setPage,
    reload: () => {
      void roleQuery.refetch();
    },
  };
  const canMembers = useCan(permissions.IAMMembersRead);
  const [creating, setCreating] = useState(false);
  const [editing, setEditing] = useState<ApiRole | undefined>(undefined);
  const [removing, setRemoving] = useState<ApiRole | undefined>(undefined);

  const [grants, setGrants] = useState<string[]>([]);

  const [preview, setPreview] = useState<ApiPreview | undefined>(undefined);
  const canEdit = editing?.allowed_actions.includes(permissions.IAMRolesManage) ?? false;
  const catalogQuery = useQuery({
    queryKey: [...permissionsQueryKey({ query: { org_id: org } }), ...identityScope(auth)],
    queryFn: ({ signal }) => dataOf(permissionCatalog({ query: { org_id: org }, signal, throwOnError: true })),
    enabled: auth.status === "authenticated",
  });
  const catalog = catalogQuery.data?.items ?? [];
  const memberQuery = useQuery({
    queryKey: [
      ...listRoleMembersQueryKey({ path: { org_id: org, id: editing?.id ?? "" }, query: { limit: 100 } }),
      ...identityScope(auth),
    ],
    enabled: !!editing && canMembers,
    queryFn: ({ signal }) =>
      dataOf(
        listRoleMembers({
          path: { org_id: org, id: editing?.id ?? "" },
          query: { limit: 100 },
          signal,
          throwOnError: true,
        }),
      ),
  });
  const members = memberQuery.data?.items ?? [];
  const grantsMutation = useMutation({
    ...setRolePermissionsMutation({ headers: mutationHeaders }),
    onSuccess: async (_data, variables) => {
      await queryClient.invalidateQueries({
        queryKey: listRolesQueryKey({ path: { org_id: org } }),
        refetchType: "none",
      });
      await reloadRole(variables.path.id);
      toast.success(t("permissionsSaved"));
    },
  });
  function open(role: ApiRole) {
    roleForm.reset({ name: role.name, description: role.description, status: role.status, version: role.version });
    setEditing(role);
    setGrants(role.permissions);
    setPreview(undefined);
  }
  async function reloadRole(id: string) {
    await refreshIdentity(org);
    state.reload();
    if (!useIdentity.getState().capabilities.includes(permissions.IAMRolesRead)) {
      setEditing(undefined);
      return;
    }
    const role = await dataOf(getRole({ throwOnError: true, path: { org_id: org, id } }));
    roleForm.reset({ name: role.name, description: role.description, status: role.status, version: role.version });
    setEditing(role);
    setGrants(role.permissions);
    setPreview(undefined);
  }
  return (
    <div className="flex flex-col gap-4">
      <div className="flex gap-2">
        <Can action={permissions.IAMRolesManage}>
          <Button onClick={() => setCreating(true)}>{t("createRole")}</Button>
        </Can>
        <Button variant="outline" onClick={state.reload}>
          {t("refresh")}
        </Button>
      </div>
      <DataTable
        state={state}
        columns={[
          { title: t("roleName"), cell: (role) => roleLabel(role) },
          { title: t("key"), cell: (role) => role.code },
          {
            title: t("type"),
            cell: (role) => (
              <Badge variant="secondary">{role.kind === "system" ? t("systemRole") : t("customRole")}</Badge>
            ),
          },
          { title: t("memberCount"), cell: (role) => role.member_count },
          { title: t("status"), cell: (role) => (role.status === "active" ? t("active") : t("disabled")) },
          {
            title: t("action"),
            cell: (role) => (
              <div className="flex gap-2">
                <Button variant="ghost" onClick={() => open(role)}>
                  {t("view")}
                </Button>
                {role.allowed_actions.includes(permissions.IAMRolesManage) && (
                  <Button variant="ghost" onClick={() => setRemoving(role)}>
                    {t("delete")}
                  </Button>
                )}
              </div>
            ),
          },
        ]}
      />
      {creating && (
        <FormDialog
          title={t("createCustomRole")}
          schema={zCreateRoleBody.required({ code: true })}
          defaultValues={{ code: "", name: "", description: "" }}
          onClose={() => setCreating(false)}
          onSubmit={async (values) => {
            await dataOf(
              createRole({
                throwOnError: true,
                path: { org_id: org },
                headers: mutationHeaders,
                body: values,
              }),
            );
            state.reload();
            await refreshIdentity(org);
          }}
        >
          {(form) => (
            <>
              <FormInput
                label={t("roleCode")}
                registration={form.register("code")}
                error={form.formState.errors.code}
                placeholder={t("exampleRoleCode")}
                description={t("roleCodeDescription")}
              />
              <FormInput
                label={t("roleName")}
                registration={form.register("name")}
                error={form.formState.errors.name}
                placeholder={t("exampleRoleName")}
              />
              <FormInput
                label={t("description")}
                type="textarea"
                registration={form.register("description")}
                error={form.formState.errors.description}
              />
            </>
          )}
        </FormDialog>
      )}
      {removing && (
        <FormDialog
          title={t("deleteRole")}
          description={t("deleteRoleDescription", { name: removing.name })}
          schema={z.object({})}
          defaultValues={{}}
          onClose={() => setRemoving(undefined)}
          onSubmit={async () => {
            await deleteRole({
              throwOnError: true,
              path: { org_id: org, id: removing.id },
              query: { version: removing.version },
              headers: mutationHeaders,
            });
            state.reload();
          }}
        />
      )}
      {editing && (
        <Dialog
          open
          onOpenChange={(value) => {
            if (!value && !grantsMutation.isPending && !roleForm.formState.isSubmitting) {
              setEditing(undefined);
              setPreview(undefined);
            }
          }}
        >
          <DialogContent className="max-h-[85dvh] overflow-y-auto sm:max-w-3xl">
            <DialogHeader>
              <DialogTitle>{roleLabel(editing)}</DialogTitle>
              <DialogDescription>{t("roleDescription")}</DialogDescription>
            </DialogHeader>
            <Tabs defaultValue="information">
              <TabsList>
                <TabsTrigger value="information">{t("roleInfo")}</TabsTrigger>
                <TabsTrigger value="permissions">{t("permissions")}</TabsTrigger>
                {canMembers && (
                  <TabsTrigger value="members">{t("membersTab", { count: editing.member_count })}</TabsTrigger>
                )}
              </TabsList>
              <TabsContent value="information">
                <form
                  noValidate
                  onSubmit={roleForm.handleSubmit(async (body) => {
                    await perform(async () => {
                      await updateRole({
                        path: { org_id: org, id: editing.id },
                        body,
                        headers: mutationHeaders,
                        throwOnError: true,
                      });
                      await reloadRole(editing.id);
                    }, t("roleSaved"));
                  })}
                >
                  <fieldset disabled={!canEdit || roleForm.formState.isSubmitting || grantsMutation.isPending}>
                    <FieldGroup>
                      <Field>
                        <FieldLabel htmlFor="role-code">{t("roleCode")}</FieldLabel>
                        <Input id="role-code" disabled value={editing.code} />
                      </Field>
                      <Field>
                        <FieldLabel htmlFor="role-name">{t("roleName")}</FieldLabel>
                        <Input
                          key={editing.version}
                          id="role-name"
                          {...roleForm.register("name")}
                          required
                          maxLength={100}
                        />
                      </Field>
                      <Field>
                        <FieldLabel htmlFor="role-description">{t("description")}</FieldLabel>
                        <Textarea
                          key={editing.version}
                          id="role-description"
                          {...roleForm.register("description")}
                          maxLength={1000}
                        />
                      </Field>
                      <Field>
                        <FieldLabel htmlFor="role-status">{t("status")}</FieldLabel>
                        <NativeSelect key={editing.version} id="role-status" {...roleForm.register("status")}>
                          <NativeSelectOption value="active">{t("active")}</NativeSelectOption>
                          <NativeSelectOption value="disabled">{t("disabled")}</NativeSelectOption>
                        </NativeSelect>
                      </Field>
                      {Object.keys(roleForm.formState.errors).length > 0 && (
                        <FieldError>{errors("invalidRequest")}</FieldError>
                      )}
                      {canEdit && <Button type="submit">{t("saveRole")}</Button>}
                    </FieldGroup>
                  </fieldset>
                </form>
              </TabsContent>
              <TabsContent value="permissions" className="flex flex-col gap-5">
                <p className="text-muted-foreground text-sm">{t("permissionDescription")}</p>
                {[...new Set(catalog.map((item) => item.permission.resource_type))].map((resource) => (
                  <FieldSet key={resource}>
                    <FieldLegend>
                      {(
                        {
                          organizations: t("organization"),
                          members: t("member"),
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
                            checked={grants.includes(permission.key)}
                            disabled={!canEdit || (!can_delegate && !editing.permissions.includes(permission.key))}
                            onCheckedChange={(checked) => {
                              setGrants((current) =>
                                checked === true
                                  ? [...current, permission.key]
                                  : current.filter((key) => key !== permission.key),
                              );
                              setPreview(undefined);
                            }}
                          />
                          <FieldLabel htmlFor={`permission-${permission.key}`}>
                            {permissionLabels[permission.key] ?? permission.key}
                          </FieldLabel>
                        </Field>
                      ))}
                  </FieldSet>
                ))}
                {canEdit && (
                  <div className="flex flex-wrap gap-2">
                    <Button
                      disabled={grantsMutation.isPending || roleForm.formState.isSubmitting}
                      onClick={() =>
                        void perform(async () => {
                          await grantsMutation.mutateAsync({
                            path: { org_id: org, id: editing.id },
                            headers: mutationHeaders,
                            body: zSetRolePermissionsBody.parse({ permissions: grants, version: editing.version }),
                          });
                        })
                      }
                    >
                      {t("savePermissions")}
                    </Button>
                    {canMembers && (
                      <Button
                        variant="outline"
                        onClick={() =>
                          void perform(async () => {
                            setPreview(
                              await dataOf(
                                previewPermission({
                                  throwOnError: true,
                                  path: { org_id: org },
                                  headers: mutationHeaders,
                                  body: zPreviewPermissionBody.parse({
                                    role_id: editing.id,
                                    permissions: grants,
                                    version: editing.version,
                                  }),
                                }),
                              ),
                            );
                          })
                        }
                      >
                        {t("previewImpact")}
                      </Button>
                    )}
                  </div>
                )}
                {preview && (
                  <p>
                    {t("impact", {
                      count: preview.affected_members,
                      added: preview.added.map((key) => permissionLabels[key] ?? key).join(", ") || t("none"),
                      removed: preview.removed.map((key) => permissionLabels[key] ?? key).join(", ") || t("none"),
                    })}
                  </p>
                )}
              </TabsContent>
              {canMembers && (
                <TabsContent value="members">
                  <Table>
                    <TableHeader>
                      <TableRow>
                        <TableHead>{t("username")}</TableHead>
                        <TableHead>{t("displayName")}</TableHead>
                        <TableHead>{t("status")}</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {members.map((member) => (
                        <TableRow key={member.id}>
                          <TableCell>{member.username}</TableCell>
                          <TableCell>{member.display_name}</TableCell>
                          <TableCell>{member.status === "active" ? t("active") : t("disabled")}</TableCell>
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                </TabsContent>
              )}
            </Tabs>
          </DialogContent>
        </Dialog>
      )}
    </div>
  );
}
