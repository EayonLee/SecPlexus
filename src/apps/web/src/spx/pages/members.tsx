"use client";
import { useState } from "react";

import { useTranslations } from "next-intl";

import type { components } from "@/auth/api.gen";
import { permissions } from "@/auth/permissions.gen";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Field, FieldDescription, FieldLabel, FieldLegend, FieldSet } from "@/components/ui/field";

import { Can, useCan } from "../components/access";
import { DataTable, type PageData, useAction, useList } from "../components/data";
import { FormDialog } from "../components/forms";
import { UserPicker } from "../components/user-picker";
import { api, refreshIdentity, useIdentity } from "../iam/session";
import { useRoleLabel } from "../iam/use-role-label";

type Member = components["schemas"]["APIMember"];
type Role = components["schemas"]["APIRole"];
export function MembersPage() {
  const roleLabel = useRoleLabel();
  const perform = useAction();

  const t = useTranslations("Management");

  const auth = useIdentity();
  const org = auth.org_id;
  const base = `/iam/orgs/${org}/members`;
  const state = useList<Member>(base);
  const [creating, setCreating] = useState(false);
  const [existing, setExisting] = useState(false);
  const [assigning, setAssigning] = useState<Member>();
  const [removing, setRemoving] = useState<Member>();
  const [roles, setRoles] = useState<Role[]>([]);
  const [selected, setSelected] = useState<string[]>([]);
  const canAssign = useCan(permissions.IAMRolesAssign);
  const canReadRoles = useCan(permissions.IAMRolesRead);
  const version = auth.organizations.find((entry) => entry.id === org)?.authz_version;
  async function changed() {
    await refreshIdentity(org);
    state.reload();
  }
  return (
    <div className="flex flex-col gap-4">
      <div className="flex gap-2">
        <Can action={permissions.IAMMembersManage}>
          <Button
            onClick={() => {
              setExisting(false);
              setCreating(true);
            }}
          >
            {t("addMember")}
          </Button>
        </Can>
        <Button variant="outline" onClick={() => void perform(changed)}>
          {t("refresh")}
        </Button>
      </div>
      <DataTable
        state={state}
        columns={[
          { title: t("username"), cell: (member) => member.username },
          { title: t("displayName"), cell: (member) => member.display_name },
          { title: t("status"), cell: (member) => (member.status === "active" ? t("active") : t("disabled")) },
          {
            title: t("action"),
            cell: (member) => (
              <div className="flex gap-2">
                {canAssign && canReadRoles && (
                  <Button
                    variant="ghost"
                    onClick={() =>
                      void perform(async () => {
                        const data = await api<PageData<Role>>(`/iam/orgs/${org}/roles?limit=100`);
                        setRoles(data.items);
                        setSelected(member.role_ids ?? []);
                        await refreshIdentity(org);
                        setAssigning(member);
                      })
                    }
                  >
                    {t("assignRoles")}
                  </Button>
                )}
                <Can action={permissions.IAMMembersManage}>
                  <Button
                    variant="ghost"
                    onClick={() =>
                      void perform(async () => {
                        await api(`${base}/${member.id}`, {
                          method: "PATCH",
                          body: { status: member.status === "active" ? "disabled" : "active", authz_version: version },
                        });
                        await changed();
                      })
                    }
                  >
                    {member.status === "active" ? t("disabled") : t("active")}
                  </Button>
                  <Button variant="ghost" onClick={() => setRemoving(member)}>
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
          key={String(existing)}
          title={t("addOrgMember")}
          fields={
            existing
              ? []
              : [
                  {
                    name: "username",
                    label: t("newUsername"),
                    required: true,
                    pattern: "[A-Za-z][A-Za-z0-9_.-]{2,63}",
                  },
                  { name: "display_name", label: t("displayName"), required: true, maxLength: 100 },
                  {
                    name: "password",
                    label: t("password"),
                    type: "password",
                    required: true,
                    minLength: 8,
                    maxLength: 128,
                  },
                ]
          }
          onClose={() => setCreating(false)}
          onSubmit={async (values) => {
            await api(base, { method: "POST", body: values });
            await changed();
          }}
        >
          {auth.principal?.is_super_admin && (
            <Field orientation="horizontal">
              <Checkbox
                id="existing-member"
                checked={existing}
                onCheckedChange={(checked) => setExisting(checked === true)}
              />
              <FieldLabel htmlFor="existing-member">{t("addExistingAccount")}</FieldLabel>
            </Field>
          )}
          {existing && (
            <Field>
              <FieldLabel>{t("existingAccount")}</FieldLabel>
              <UserPicker required />
              <FieldDescription>{t("existingAccountDescription")}</FieldDescription>
            </Field>
          )}
        </FormDialog>
      )}
      {assigning && (
        <FormDialog
          title={t("assignMemberRoles", { name: assigning.display_name })}
          fields={[]}
          onClose={() => setAssigning(undefined)}
          onSubmit={async () => {
            await api(`${base}/${assigning.id}/roles`, {
              method: "PUT",
              body: { role_ids: selected, authz_version: version },
            });
            await changed();
          }}
        >
          <FieldSet>
            <FieldLegend>{t("memberRoles")}</FieldLegend>
            {roles.map((role) => (
              <Field key={role.id} orientation="horizontal">
                <Checkbox
                  id={`member-role-${role.id}`}
                  checked={selected.includes(role.id)}
                  disabled={role.status !== "active"}
                  onCheckedChange={(checked) =>
                    setSelected((current) =>
                      checked === true ? [...current, role.id] : current.filter((id) => id !== role.id),
                    )
                  }
                />
                <FieldLabel htmlFor={`member-role-${role.id}`}>{roleLabel(role)}</FieldLabel>
              </Field>
            ))}
          </FieldSet>
        </FormDialog>
      )}
      {removing && (
        <FormDialog
          title={t("removeMember")}
          description={t("removeMemberDescription", { name: removing.display_name })}
          fields={[]}
          onClose={() => setRemoving(undefined)}
          onSubmit={async () => {
            await api(`${base}/${removing.id}?authz_version=${version}`, { method: "DELETE" });
            await changed();
          }}
        />
      )}
    </div>
  );
}
