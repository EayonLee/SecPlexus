"use client";

import { useState } from "react";

import { zodResolver } from "@hookform/resolvers/zod";
import { useQuery } from "@tanstack/react-query";
import { Eye, Pencil, PlusIcon, Trash2 } from "lucide-react";
import { useFormatter, useTranslations } from "next-intl";
import { Controller, useForm, useFormState } from "react-hook-form";
import { z } from "zod";

import { permissions } from "@/auth/permissions.gen";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Field, FieldGroup, FieldLabel, FieldLegend, FieldSet } from "@/components/ui/field";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  createRole,
  deleteRole,
  listRoles,
  permissions as permissionCatalog,
  setRolePermissions,
  updateRole,
} from "@/spx/api/iam/management";
import { type ApiRole, zCreateRoleBody, zSetRolePermissionsBody, zUpdateRoleBody } from "@/spx/api/iam/schemas";

import { useCan } from "../components/access";
import { CollectionBoundary, CollectionState } from "../components/collection-state";
import { DataTable, useAction, usePagedQuery } from "../components/data";
import { useDirectorySearch } from "../components/directory-search";
import { FormDialog, FormInput } from "../components/forms";
import { ListSelect } from "../components/list-controls";
import { PageHeader } from "../components/page-header";
import { RecordMenu } from "../components/record-menu";
import { SearchField } from "../components/search-field";
import { SheetPanel } from "../components/sheet-panel";
import { ListSurface, Toolbar } from "../components/toolbar";
import { managementChanged } from "../iam/management";
import { usePermissionDescription } from "../iam/permission-labels";
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

  return query;
}

function useResourceLabels() {
  const t = useTranslations("Management");
  return new Map([
    ["users", t("user")],
    ["roles", t("role")],
    ["audit", t("audit")],
    ["events", t("eventResource")],
    ["permissions", t("permissionCatalog")],
    ["input", t("inputResource")],
    ["mapping", t("mappingResource")],
  ]);
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
  const labels = useResourceLabels();
  const describePermission = usePermissionDescription();
  const items = catalog.data?.items ?? [];
  const resources = [...new Set(items.map((item) => item.permission.resource_type))];

  return (
    <CollectionBoundary query={catalog} hasContent={items.length > 0}>
      {!items.length && <CollectionState kind="empty" />}
      <div className="grid gap-x-4 gap-y-5">
        {resources.map((resource) => (
          <FieldSet key={resource}>
            {/* 分组标题必须弱于上方的「权限」标题，否则层级倒挂。 */}
            <FieldLegend variant="label" className="text-muted-foreground font-medium">
              {labels.get(resource) ?? resource}
            </FieldLegend>
            {items
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
                    {describePermission(permission.key, permission.description) || permission.key}
                  </FieldLabel>
                </Field>
              ))}
          </FieldSet>
        ))}
      </div>
    </CollectionBoundary>
  );
}

function CreateRoleSheet({ catalog, onClose }: { catalog: PermissionCatalog; onClose: () => void }) {
  const t = useTranslations("Management");

  return (
    <FormDialog
      variant="sheet"
      layout="management"
      title={t("createRole")}
      submitLabel={t("createRole")}
      submitDisabled={!catalog.isSuccess}
      schema={zCreateRoleBody}
      defaultValues={{ name: "", description: "", permissions: [] }}
      onClose={onClose}
      onSubmit={async (values) => {
        await createRole(values);
        await managementChanged();
      }}
    >
      {(form) => (
        <>
          <FormInput
            required
            label={t("roleName")}
            registration={form.register("name")}
            error={form.formState.errors.name}
          />
          <FormInput
            label={t("description")}
            type="textarea"
            registration={form.register("description")}
            error={form.formState.errors.description}
          />
          <section className="col-span-full flex flex-col gap-4 border-t pt-6">
            <h3 className="text-sm font-medium">{t("permissions")}</h3>
            <Controller
              name="permissions"
              control={form.control}
              render={({ field }) => (
                <PermissionPicker
                  catalog={catalog}
                  selected={field.value}
                  onChange={field.onChange}
                  disabled={form.formState.isSubmitting}
                />
              )}
            />
          </section>
        </>
      )}
    </FormDialog>
  );
}

function EditRoleSheet({
  role,
  catalog,
  opener,
  onClose,
}: {
  role: ApiRole;
  catalog: PermissionCatalog;
  opener: HTMLElement | null;
  onClose: () => void;
}) {
  const t = useTranslations("Management");
  const common = useTranslations("Common");
  const perform = useAction();

  const [savedGrants, setSavedGrants] = useState(role.permissions);
  const [grants, setGrants] = useState(role.permissions);
  const [savingGrants, setSavingGrants] = useState(false);
  const details = useForm<z.infer<typeof zUpdateRoleBody>>({
    resolver: zodResolver(zUpdateRoleBody),
    defaultValues: { name: role.name, description: role.description },
  });
  const {
    errors: detailsErrors,
    defaultValues: savedDetails,
    isDirty,
    isSubmitting,
  } = useFormState({
    control: details.control,
  });
  const busy = isSubmitting || savingGrants;
  const grantsChanged = grants.length !== savedGrants.length || grants.some((key) => !savedGrants.includes(key));

  return (
    <SheetPanel
      layout="management"
      title={t("editRole")}
      description={savedDetails?.name}
      returnFocus={opener}
      onClose={() => {
        if (!busy) onClose();
      }}
      footer={
        <Button size="sm" variant="outline" onClick={onClose} disabled={busy}>
          {common("close")}
        </Button>
      }
    >
      <div className="flex flex-col gap-5 py-4">
        <section className="flex flex-col gap-4">
          <h3 className="text-sm font-medium">{t("roleInfo")}</h3>
          <form
            noValidate
            onSubmit={details.handleSubmit((values) =>
              perform(async () => {
                await updateRole({ id: role.id, body: values });
                details.reset(values);
                await managementChanged();
              }),
            )}
          >
            <fieldset disabled={busy}>
              <FieldGroup>
                <FormInput
                  required
                  label={t("roleName")}
                  registration={details.register("name")}
                  error={detailsErrors.name}
                />
                <FormInput
                  label={t("description")}
                  type="textarea"
                  registration={details.register("description")}
                  error={detailsErrors.description}
                />
                <div>
                  <Button type="submit" size="sm" disabled={busy || !isDirty}>
                    {t("saveRole")}
                  </Button>
                </div>
              </FieldGroup>
            </fieldset>
          </form>
        </section>

        <section className="flex flex-col gap-4 border-t pt-6">
          <h3 className="text-sm font-medium">{t("permissions")}</h3>
          <PermissionPicker catalog={catalog} selected={grants} onChange={setGrants} disabled={busy} />
          <div>
            <Button
              size="sm"
              disabled={busy || !catalog.isSuccess || !grantsChanged}
              onClick={() => {
                setSavingGrants(true);
                void perform(async () => {
                  await setRolePermissions({
                    id: role.id,
                    body: zSetRolePermissionsBody.parse({ permissions: grants }),
                  });
                  setSavedGrants(grants);
                  await managementChanged();
                }).finally(() => setSavingGrants(false));
              }}
            >
              {t("savePermissions")}
            </Button>
          </div>
        </section>
      </div>
    </SheetPanel>
  );
}

function RoleDetailsSheet({
  role,
  catalog,
  opener,
  onClose,
}: {
  role: ApiRole;
  catalog: PermissionCatalog;
  opener: HTMLElement | null;
  onClose: () => void;
}) {
  const t = useTranslations("Management");
  const format = useFormatter();
  const describePermission = usePermissionDescription();

  return (
    <SheetPanel layout="management" title={role.name} returnFocus={opener} onClose={onClose}>
      <div className="flex flex-col gap-5 py-4">
        <dl className="grid grid-cols-2 gap-4">
          <div>
            <dt className="text-muted-foreground text-xs">{t("roleType")}</dt>
            <dd className="text-sm">{t(role.builtin ? "builtin" : "customRole")}</dd>
          </div>
          <div>
            <dt className="text-muted-foreground text-xs">{t("userCount")}</dt>
            <dd className="text-sm">{format.number(role.user_count)}</dd>
          </div>
          {role.description && (
            <div className="col-span-2">
              <dt className="text-muted-foreground text-xs">{t("description")}</dt>
              <dd className="text-sm whitespace-pre-wrap [overflow-wrap:anywhere]">{role.description}</dd>
            </div>
          )}
        </dl>
        <section className="flex flex-col gap-3 border-t pt-4">
          <h3 className="text-sm font-medium">{t("permissions")}</h3>
          {role.permissions.length ? (
            <ul className="divide-y rounded-lg border">
              {role.permissions.map((key) => (
                <li key={key} className="flex flex-col items-start gap-1.5 px-3 py-2">
                  <Badge variant="secondary" className="max-w-full whitespace-normal [overflow-wrap:anywhere]">
                    {key}
                  </Badge>
                  <span className="text-muted-foreground text-sm">
                    {describePermission(
                      key,
                      catalog.data?.items.find((item) => item.permission.key === key)?.permission.description,
                    )}
                  </span>
                </li>
              ))}
            </ul>
          ) : (
            <CollectionState kind="empty" title={t("none")} />
          )}
        </section>
      </div>
    </SheetPanel>
  );
}

export function RolesPage() {
  const t = useTranslations("Management");
  const labels = useResourceLabels();
  const describePermission = usePermissionDescription();
  const navigation = useTranslations("Navigation");
  const format = useFormatter();
  const canManage = useCan(permissions.IAMRolesManage);
  const catalog = usePermissionCatalog();

  const { term, setTerm, applied, setApplied } = useDirectorySearch();
  const [typeFilter, setTypeFilter] = useState<"all" | "builtin" | "custom">("all");
  const state = usePagedQuery<ApiRole>(["iam", "roles", applied, typeFilter], (query, signal) =>
    listRoles({ ...query, q: applied || undefined, type: typeFilter === "all" ? undefined : typeFilter }, signal),
  );

  const restartPage = () => state.setPagination((page) => ({ ...page, pageIndex: 0 }));
  const search = (value: string) => {
    setTerm(value);
    setApplied(value);
    restartPage();
  };

  const [creating, setCreating] = useState(false);
  const permissionGroups = new Map(
    catalog.data?.items.map((item) => [item.permission.key, item.permission.resource_type]),
  );
  const [selected, setSelected] = useState<{
    role: ApiRole;
    mode: "view" | "edit";
    opener: HTMLElement | null;
  }>();
  const [deleting, setDeleting] = useState<{ role: ApiRole; opener: HTMLElement | null }>();

  return (
    <div className="flex flex-col gap-4">
      <PageHeader title={navigation("roles")} />
      <Tabs defaultValue="roles" className="gap-4">
        <TabsList
          variant="line"
          className="w-full justify-start gap-2 border-b ps-0 *:data-[slot=tabs-trigger]:flex-none"
        >
          <TabsTrigger value="roles">{t("roles")}</TabsTrigger>
          <TabsTrigger value="catalog">{t("permissionCatalog")}</TabsTrigger>
        </TabsList>
        <TabsContent value="roles" className="flex flex-col gap-4">
          <ListSurface>
            <Toolbar
              actions={
                canManage && (
                  <Button size="sm" onClick={() => setCreating(true)}>
                    <PlusIcon data-icon="inline-start" />
                    {t("createRole")}
                  </Button>
                )
              }
            >
              <SearchField
                value={term}
                onChange={(value) => {
                  setTerm(value);
                  restartPage();
                }}
                onSubmit={search}
                placeholder={t("searchRolesPlaceholder")}
                className="sm:w-82"
              />
              <ListSelect<"all" | "builtin" | "custom">
                label={t("roleType")}
                value={typeFilter}
                options={[
                  { value: "all", label: t("allRoleTypes") },
                  { value: "builtin", label: t("builtinRoles") },
                  { value: "custom", label: t("customRoles") },
                ]}
                onChange={(value) => {
                  setTypeFilter(value);
                  restartPage();
                }}
              />
            </Toolbar>

            <DataTable
              onClear={
                applied || typeFilter !== "all"
                  ? () => {
                      setTypeFilter("all");
                      search("");
                    }
                  : undefined
              }
              state={state}
              groupBy={(role) => t(role.builtin ? "builtinRoles" : "customRoles")}
              columns={[
                {
                  title: t("roleName"),
                  className: "w-auto @xl/list:w-[28%]",
                  cell: (role) => (
                    <div className="min-w-0 whitespace-normal [overflow-wrap:anywhere]">
                      <div className="font-medium">{role.name}</div>
                      {role.description && (
                        <div className="text-muted-foreground mt-0.5 line-clamp-2 text-xs">{role.description}</div>
                      )}
                    </div>
                  ),
                },
                {
                  title: t("roleType"),
                  className: "hidden w-[16%] @xl/list:table-cell",
                  cell: (role) => (
                    <Badge variant="outline" className="rounded-sm">
                      {t(role.builtin ? "builtin" : "customRole")}
                    </Badge>
                  ),
                },
                {
                  title: t("userCount"),
                  className: "hidden w-[10%] @lg/list:table-cell",
                  cell: (role) => format.number(role.user_count),
                },
                {
                  title: t("permissionCatalog"),
                  className: "hidden @lg/list:table-cell",
                  cell: (role) => {
                    const groups = [
                      ...new Set(
                        role.permissions.map(
                          (key) => permissionGroups.get(key) ?? key.split(".").slice(0, 2).join("."),
                        ),
                      ),
                    ];
                    return groups.length ? (
                      <div className="flex flex-wrap items-center gap-2 whitespace-normal [overflow-wrap:anywhere]">
                        {groups.slice(0, 3).map((group) => (
                          <Badge key={group} variant="outline" className="rounded-sm">
                            {labels.get(group) ?? group}
                          </Badge>
                        ))}
                        {groups.length > 3 && <span className="tabular-nums">+{format.number(groups.length - 3)}</span>}
                      </div>
                    ) : (
                      "—"
                    );
                  },
                },
                {
                  title: t("updatedAt"),
                  className: "hidden w-[16%] @2xl/list:table-cell",
                  cell: (role) =>
                    role.updated_at ? format.dateTime(new Date(role.updated_at), { dateStyle: "medium" }) : "—",
                },
                {
                  title: t("action"),
                  className: "w-14",
                  header: <span className="sr-only">{t("action")}</span>,
                  cell: (role) => {
                    let deleteLock: string | undefined;
                    if (role.builtin) deleteLock = t("builtinRoleLocked");
                    else if (role.user_count > 0) deleteLock = t("deleteRoleHasUsers");

                    return (
                      <RecordMenu
                        orientation="vertical"
                        label={t("recordActions", { name: role.name })}
                        actions={[
                          {
                            label: t("viewDetails"),
                            icon: Eye,
                            onSelect: (opener) => setSelected({ role, mode: "view", opener }),
                          },
                          ...(canManage && !role.builtin
                            ? [
                                {
                                  label: t("editRole"),
                                  icon: Pencil,
                                  onSelect: (opener: HTMLButtonElement | null) =>
                                    setSelected({ role, mode: "edit", opener }),
                                },
                              ]
                            : []),
                        ]}
                        destructive={
                          canManage
                            ? {
                                label: t("delete"),
                                icon: Trash2,
                                disabledReason: deleteLock,
                                onSelect: (opener) => setDeleting({ role, opener }),
                              }
                            : undefined
                        }
                      />
                    );
                  },
                },
              ]}
            />
          </ListSurface>
        </TabsContent>
        <TabsContent value="catalog">
          <ListSurface>
            <CollectionBoundary query={catalog} hasContent={Boolean(catalog.data?.items.length)}>
              <ul className="divide-y">
                {catalog.data?.items.map(({ permission, can_delegate }) => (
                  <li key={permission.key} className="flex flex-wrap items-start justify-between gap-2 px-4 py-3">
                    <div className="min-w-0">
                      <p className="text-sm font-medium">
                        {describePermission(permission.key, permission.description)}
                      </p>
                      <code className="text-muted-foreground text-xs [overflow-wrap:anywhere]">{permission.key}</code>
                    </div>
                    <Badge variant="outline" className="rounded-sm">
                      {t(can_delegate ? "delegable" : "restrictedPermission")}
                    </Badge>
                  </li>
                ))}
              </ul>
            </CollectionBoundary>
          </ListSurface>
        </TabsContent>
      </Tabs>

      {deleting && (
        <FormDialog
          variant="alert"
          title={t("deleteRole")}
          description={t("deleteRoleDescription", { name: deleting.role.name })}
          submitLabel={t("delete")}
          returnFocus={deleting.opener}
          schema={z.object({})}
          defaultValues={{}}
          onClose={() => setDeleting(undefined)}
          onSubmit={async () => {
            await deleteRole({ id: deleting.role.id });
            if (state.items.length === 1 && state.pagination.pageIndex > 0) {
              state.setPagination({ ...state.pagination, pageIndex: state.pagination.pageIndex - 1 });
            }
            await managementChanged();
          }}
        />
      )}

      {creating && <CreateRoleSheet catalog={catalog} onClose={() => setCreating(false)} />}
      {selected &&
        (selected.mode === "edit" ? (
          <EditRoleSheet
            role={selected.role}
            catalog={catalog}
            opener={selected.opener}
            onClose={() => setSelected(undefined)}
          />
        ) : (
          <RoleDetailsSheet
            role={selected.role}
            catalog={catalog}
            opener={selected.opener}
            onClose={() => setSelected(undefined)}
          />
        ))}
    </div>
  );
}
