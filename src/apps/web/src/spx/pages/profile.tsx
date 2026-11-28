"use client";

import { useState } from "react";

import { useRouter } from "next/navigation";

import { CircleCheck, Clock3, Ellipsis, LockKeyhole, LogOut, Mail, Pencil, Settings2, ShieldCheck } from "lucide-react";
import { useFormatter, useTranslations } from "next-intl";
import { toast } from "sonner";
import { z } from "zod";

import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Separator } from "@/components/ui/separator";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { zChangePasswordBody } from "@/spx/api/iam/schemas";

import { useAction } from "../components/data";
import { FormDialog, FormInput } from "../components/forms";
import { PageHeader } from "../components/page-header";
import { changePassword, signOut, useIdentity } from "../iam/session";
import { ProfileTimeZoneDialog } from "./profile-time-zone-dialog";

const passwordSchema = zChangePasswordBody
  .extend({ confirm_password: z.string() })
  .refine((value) => value.new_password === value.confirm_password, {
    path: ["confirm_password"],
    message: "passwordMismatch",
  });

// 结构来自模板的 Profile Header、线形标签栏及 Overview 两栏。
export function ProfilePage() {
  const t = useTranslations("Account");
  const format = useFormatter();
  const navigation = useTranslations("Navigation");
  const management = useTranslations("Management");
  const auth = useIdentity();
  const router = useRouter();
  const perform = useAction();
  const [dialog, setDialog] = useState<{ type: "timezone" | "password"; opener: HTMLButtonElement }>();
  const principal = auth.principal;
  if (!principal) return null;

  const zone = auth.timezone;
  const displayTimeZone = !zone || zone === "browser" ? t("timeZoneBrowser") : zone;
  const timeZone = zone === "utc" ? t("timeZoneUtc") : displayTimeZone;

  const dateOptions = { dateStyle: "medium", timeStyle: "short" } as const;
  const signedIn = format.dateTime(new Date(principal.authenticated_at), dateOptions);
  const createdAt = auth.created_at ? format.dateTime(new Date(auth.created_at), dateOptions) : "—";
  const fields = [
    { label: management("username"), value: principal.username },
    { label: t("displayName"), value: principal.display_name },
    { label: management("email"), value: auth.email || "—" },
    { label: t("accountId"), value: principal.user_id },
    { label: management("createdAt"), value: createdAt },
    { label: t("displayTimeZone"), value: timeZone },
  ];
  const accountDetails = (
    <dl className="grid gap-5 sm:grid-cols-2 xl:grid-cols-3 xl:gap-x-12">
      {fields.map(({ label, value }) => (
        <div key={label} className="flex min-w-0 flex-col gap-1">
          <dt className="text-muted-foreground text-xs">{label}</dt>
          <dd className="text-sm [overflow-wrap:anywhere]">{value}</dd>
        </div>
      ))}
    </dl>
  );

  return (
    <div className="flex min-w-0 flex-col gap-4">
      <div className="flex flex-col gap-5 lg:flex-row lg:items-end lg:justify-between">
        <div className="flex min-w-0 items-center gap-4">
          <div className="grid size-18 shrink-0 place-items-center sm:size-23">
            <Avatar className="size-16 sm:size-20">
              <AvatarFallback>{[...principal.display_name].slice(0, 2).join("").toUpperCase()}</AvatarFallback>
            </Avatar>
          </div>
          <div className="flex min-w-0 flex-col gap-2">
            <PageHeader title={principal.display_name} description={auth.email || `@${principal.username}`} />
          </div>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          {auth.email && (
            <Button asChild size="sm" variant="outline">
              <a href={`mailto:${auth.email}`}>
                <Mail data-icon="inline-start" />
                {management("email")}
              </a>
            </Button>
          )}
          <Button size="sm" onClick={(event) => setDialog({ type: "timezone", opener: event.currentTarget })}>
            <Pencil data-icon="inline-start" />
            {t("changeTimeZone")}
          </Button>
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button aria-label={t("profileActions")} variant="outline" size="icon-sm">
                <Ellipsis />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="w-48">
              <DropdownMenuGroup>
                <DropdownMenuItem
                  onSelect={() =>
                    void perform(async () => {
                      await signOut();
                      router.replace("/login");
                    })
                  }
                >
                  <LogOut />
                  {navigation("signOut")}
                </DropdownMenuItem>
              </DropdownMenuGroup>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </div>

      <Card className="py-0">
        <Tabs defaultValue="overview" className="min-h-0 flex-1 gap-0">
          <div className="touch-pan-x scrollbar-none overflow-x-auto overscroll-x-contain border-b px-4">
            <TabsList
              variant="line"
              className="w-max min-w-full justify-start gap-4 *:data-[slot=tabs-trigger]:flex-none"
            >
              <TabsTrigger value="overview">{t("overview")}</TabsTrigger>
              <TabsTrigger value="security">{t("passwordAndLogin")}</TabsTrigger>
            </TabsList>
          </div>
          <TabsContent value="overview" className="px-4">
            <div className="grid lg:grid-cols-[minmax(0,1fr)_auto_18rem]">
              <div className="min-w-0 py-4 lg:pr-6">
                <section className="flex flex-col gap-2">
                  <h2 className="font-heading text-base font-medium">{t("profileIdentity")}</h2>
                  {accountDetails}
                </section>
                <Separator className="my-4" />
                <section className="flex flex-col gap-2">
                  <h2 className="font-heading text-base font-medium">{management("roles")}</h2>
                  <div className="flex flex-wrap items-center gap-2">
                    {auth.roles.map((role) => (
                      <Badge key={role.id} variant="outline" className="rounded-sm">
                        <ShieldCheck />
                        {role.name}
                      </Badge>
                    ))}
                    {!auth.roles.length && (
                      <span className="text-muted-foreground text-sm">{management("noRoles")}</span>
                    )}
                  </div>
                </section>
              </div>
              <Separator orientation="vertical" className="hidden lg:block" />
              <aside className="min-w-0 border-t py-4 lg:border-t-0 lg:pl-6">
                <section className="flex flex-col gap-3">
                  <h2 className="font-heading text-sm font-medium">{t("recordStatus")}</h2>
                  <div className="flex items-start gap-2">
                    <CircleCheck aria-hidden="true" className="text-muted-foreground mt-0.5 size-4 shrink-0" />
                    <div>
                      <p className="text-sm font-medium">{management("active")}</p>
                    </div>
                  </div>
                  {principal.configured && (
                    <div className="flex items-start gap-2">
                      <Settings2 aria-hidden="true" className="text-muted-foreground mt-0.5 size-4 shrink-0" />
                      <div>
                        <p className="text-sm font-medium">{t("accountManagement")}</p>
                        <p className="text-muted-foreground text-xs">{t("deploymentConfiguration")}</p>
                      </div>
                    </div>
                  )}
                </section>
                <Separator className="my-4" />
                <section className="flex flex-col gap-3">
                  <h2 className="font-heading text-sm font-medium">{t("currentSession")}</h2>
                  <div className="flex items-start gap-2">
                    <Clock3 aria-hidden="true" className="text-muted-foreground mt-0.5 size-4 shrink-0" />
                    <div>
                      <p className="text-sm font-medium">{t("signedInAt")}</p>
                      <p className="text-muted-foreground text-xs">{signedIn}</p>
                    </div>
                  </div>
                </section>
              </aside>
            </div>
          </TabsContent>
          <TabsContent value="security" className="p-4">
            {principal.configured ? (
              <Tooltip>
                <TooltipTrigger asChild>
                  <Button variant="ghost" size="sm" className="text-muted-foreground">
                    <LockKeyhole data-icon="inline-start" />
                    {t("configurationManaged")}
                  </Button>
                </TooltipTrigger>
                <TooltipContent>{t("configurationPasswordHelp")}</TooltipContent>
              </Tooltip>
            ) : (
              <Button size="sm" onClick={(event) => setDialog({ type: "password", opener: event.currentTarget })}>
                <LockKeyhole data-icon="inline-start" />
                {t("changePassword")}
              </Button>
            )}
          </TabsContent>
        </Tabs>
      </Card>

      {dialog?.type === "timezone" && (
        <ProfileTimeZoneDialog returnFocus={dialog.opener} onClose={() => setDialog(undefined)} />
      )}
      {dialog?.type === "password" && !principal.configured && (
        <FormDialog
          title={t("changePassword")}
          layout="product"
          returnFocus={dialog.opener}
          schema={passwordSchema}
          defaultValues={{ old_password: "", new_password: "", confirm_password: "" }}
          submitLabel={t("changeAndSignOut")}
          onClose={() => setDialog(undefined)}
          onSubmit={async (form) => {
            await changePassword(form.old_password, form.new_password);
            toast.success(t("passwordChanged"));
            router.replace("/login");
          }}
        >
          {(form) => (
            <>
              <FormInput
                required
                type="password"
                autoComplete="current-password"
                label={t("currentPassword")}
                registration={form.register("old_password")}
                error={form.formState.errors.old_password}
              />
              <FormInput
                required
                type="password"
                label={t("newPassword")}
                registration={form.register("new_password")}
                error={form.formState.errors.new_password}
              />
              <FormInput
                required
                type="password"
                label={t("confirmPassword")}
                registration={form.register("confirm_password")}
                error={form.formState.errors.confirm_password}
                errorText={t("passwordMismatch")}
              />
            </>
          )}
        </FormDialog>
      )}
    </div>
  );
}
