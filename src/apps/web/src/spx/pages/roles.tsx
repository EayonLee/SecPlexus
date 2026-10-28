"use client";
import { useEffect, useState } from "react";

import { useFormatter, useTranslations } from "next-intl";
import { toast } from "sonner";

import type { components } from "@/auth/api.gen";
import { permissions } from "@/auth/permissions.gen";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Field, FieldGroup, FieldLabel, FieldLegend, FieldSet } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { NativeSelect, NativeSelectOption } from "@/components/ui/native-select";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Textarea } from "@/components/ui/textarea";

import { Can, useCan } from "../components/access";
import { DataTable, type PageData, useAction, useList } from "../components/data";
import { FormDialog } from "../components/forms";
import { api, refreshIdentity, useIdentity } from "../iam/session";
import { useRoleLabel } from "../iam/use-role-label";

type Role = components["schemas"]["APIRole"];
type Member = components["schemas"]["APIMember"];
type PermissionItem = components["schemas"]["APIPermissionItem"];
type Preview = components["schemas"]["APIPreview"];
export function RolesPage() {
  const roleLabel = useRoleLabel();
  const perform = useAction();

  const t = useTranslations("Management");
  const format = useFormatter();
  const permissionText = useTranslations("Permissions");
  const permissionLabels = Object.fromEntries(Object.values(permissions).map((key) => [key, permissionText(key)]));

  const auth = useIdentity();
  const org = auth.org_id;
  const base = `/iam/orgs/${org}/roles`;
  const authzVersion = auth.organizations.find((entry) => entry.id === org)?.authz_version;
  const state = useList<Role>(base);
  const canMembers = useCan(permissions.IAMMembersRead);
  const [creating, setCreating] = useState(false);
  const [editing, setEditing] = useState<Role>();
  const [removing, setRemoving] = useState<Role>();
  const [catalog, setCatalog] = useState<PermissionItem[]>([]);
  const [grants, setGrants] = useState<string[]>([]);
  const [members, setMembers] = useState<Member[]>([]);
  const [preview, setPreview] = useState<Preview>();
  const canEdit = editing?.allowed_actions?.includes(permissions.IAMRolesManage) || false;
  // biome-ignore lint/correctness/useExhaustiveDependencies: 授权版本变化后重新获取可委派权限。
  useEffect(() => {
    const controller = new AbortController();
    void perform(async () => {
      const data = await api<{ items: PermissionItem[] }>(`/iam/permissions?org_id=${org}`, {
        signal: controller.signal,
      });
      if (!controller.signal.aborted) setCatalog(data.items);
    });
    return () => controller.abort();
  }, [org, authzVersion]);
  async function open(role: Role) {
    setEditing(role);
    setGrants(role.permissions);
    setPreview(undefined);
    setMembers([]);
    if (canMembers) {
      const data = await api<PageData<Member>>(`${base}/${role.id}/members?limit=100`);
      setMembers(data.items);
    }
  }
  async function reloadRole(id: string) {
    await refreshIdentity(org);
    state.reload();
    if (!useIdentity.getState().capabilities.includes(permissions.IAMRolesRead)) {
      setEditing(undefined);
      return;
    }
    const role = await api<Role>(`${base}/${id}`);
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
                <Button variant="ghost" onClick={() => void perform(() => open(role))}>
                  {t("view")}
                </Button>
                {role.allowed_actions?.includes(permissions.IAMRolesManage) && (
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
          fields={[
            {
              name: "code",
              label: t("roleCode"),
              required: true,
              pattern: "[a-z][a-z0-9_]{2,63}",
              placeholder: t("exampleRoleCode"),
              description: t("roleCodeDescription"),
            },
            { name: "name", label: t("roleName"), required: true, maxLength: 100, placeholder: t("exampleRoleName") },
            { name: "description", label: t("description"), type: "textarea", maxLength: 1000 },
          ]}
          onClose={() => setCreating(false)}
          onSubmit={async (values) => {
            await api(base, { method: "POST", body: values });
            state.reload();
            await refreshIdentity(org);
          }}
        />
      )}
      {removing && (
        <FormDialog
          title={t("deleteRole")}
          description={t("deleteRoleDescription", { name: removing.name })}
          fields={[]}
          onClose={() => setRemoving(undefined)}
          onSubmit={async () => {
            await api(`${base}/${removing.id}?version=${removing.version}`, { method: "DELETE" });
            state.reload();
          }}
        />
      )}
      {editing && (
        <Dialog
          open
          onOpenChange={(value) => {
            if (!value) {
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
                  onSubmit={(event) => {
                    event.preventDefault();
                    const values = new FormData(event.currentTarget);
                    void perform(async () => {
                      await api(`${base}/${editing.id}`, {
                        method: "PATCH",
                        body: {
                          name: values.get("name"),
                          description: values.get("description"),
                          status: values.get("status"),
                          version: editing.version,
                        },
                      });
                      await reloadRole(editing.id);
                    }, t("roleSaved"));
                  }}
                >
                  <fieldset disabled={!canEdit}>
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
                          name="name"
                          required
                          maxLength={100}
                          defaultValue={editing.name}
                        />
                      </Field>
                      <Field>
                        <FieldLabel htmlFor="role-description">{t("description")}</FieldLabel>
                        <Textarea
                          key={editing.version}
                          id="role-description"
                          name="description"
                          maxLength={1000}
                          defaultValue={editing.description}
                        />
                      </Field>
                      <Field>
                        <FieldLabel htmlFor="role-status">{t("status")}</FieldLabel>
                        <NativeSelect
                          key={editing.version}
                          id="role-status"
                          name="status"
                          defaultValue={editing.status}
                        >
                          <NativeSelectOption value="active">{t("active")}</NativeSelectOption>
                          <NativeSelectOption value="disabled">{t("disabled")}</NativeSelectOption>
                        </NativeSelect>
                      </Field>
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
                      onClick={() =>
                        void perform(async () => {
                          await api(`${base}/${editing.id}/permissions`, {
                            method: "PUT",
                            body: { permissions: grants, version: editing.version },
                          });
                          await reloadRole(editing.id);
                          toast.success(t("permissionsSaved"));
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
                              await api<Preview>(`/iam/orgs/${org}/permission-preview`, {
                                method: "POST",
                                body: { role_id: editing.id, permissions: grants, version: editing.version },
                              }),
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
                      added: format.list(preview.added.map((key) => permissionLabels[key] ?? key)) || t("none"),
                      removed: format.list(preview.removed.map((key) => permissionLabels[key] ?? key)) || t("none"),
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
