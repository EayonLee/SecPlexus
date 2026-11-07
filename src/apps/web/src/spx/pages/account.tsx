"use client";
import { useRouter } from "next/navigation";

import { zodResolver } from "@hookform/resolvers/zod";
import { LockKeyhole } from "lucide-react";
import { useTranslations } from "next-intl";
import { useForm } from "react-hook-form";
import { toast } from "sonner";
import { z } from "zod";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Field, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { zChangePasswordBody } from "@/spx/api/iam/schemas";

import { useAction } from "../components/data";
import { changePassword, signOut, useIdentity } from "../iam/session";

export function AccountPage() {
  const perform = useAction();

  const t = useTranslations("Account");
  const errors = useTranslations("Errors");

  const auth = useIdentity();
  const router = useRouter();
  const passwordSchema = zChangePasswordBody
    .extend({ confirm_password: z.string() })
    .refine((value) => value.new_password === value.confirm_password, {
      path: ["confirm_password"],
      message: t("passwordMismatch"),
    });
  const passwordForm = useForm<z.infer<typeof passwordSchema>>({
    resolver: zodResolver(passwordSchema),
    defaultValues: { old_password: "", new_password: "", confirm_password: "" },
  });
  const busy = passwordForm.formState.isSubmitting;
  return (
    <div className="@container/main flex flex-col gap-4 md:gap-6">
      <header>
        <h1 className="font-semibold text-2xl tracking-tight">{t("account")}</h1>
      </header>

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-3">
        <Card className="lg:col-span-2">
          <CardHeader>
            <CardTitle>{t("username")}</CardTitle>
            <CardDescription>
              {t("accountId")}: {auth.principal?.user_id}
            </CardDescription>
          </CardHeader>
          <CardContent>
            <Input id="account-username" value={auth.principal?.username ?? ""} readOnly />
          </CardContent>
        </Card>

        <Card className="h-fit">
          <CardHeader>
            <CardTitle>{t("account")}</CardTitle>
            <CardDescription>@{auth.principal?.username}</CardDescription>
          </CardHeader>
        </Card>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>{t("passwordAndLogin")}</CardTitle>
        </CardHeader>
        <CardContent>
          {auth.principal?.is_super_admin ? (
            <Tooltip>
              <TooltipTrigger asChild>
                <button
                  type="button"
                  className="text-muted-foreground focus-visible:outline-ring inline-flex w-fit items-center gap-2 rounded-sm text-sm focus-visible:outline-2"
                >
                  <LockKeyhole className="size-4" />
                  {t("configurationManaged")}
                </button>
              </TooltipTrigger>
              <TooltipContent>{t("configurationPasswordHelp")}</TooltipContent>
            </Tooltip>
          ) : (
            <form
              className="max-w-lg"
              noValidate
              onSubmit={passwordForm.handleSubmit(async (values) => {
                await perform(async () => {
                  await changePassword(values.old_password, values.new_password);
                  passwordForm.reset();
                  toast.success(t("passwordChanged"));
                  router.replace("/login");
                });
              })}
            >
              <fieldset disabled={busy}>
                <FieldGroup>
                  <Field>
                    <FieldLabel htmlFor="old-password">{t("currentPassword")}</FieldLabel>
                    <Input
                      id="old-password"
                      {...passwordForm.register("old_password")}
                      type="password"
                      autoComplete="current-password"
                      required
                    />
                  </Field>
                  <Field>
                    <FieldLabel htmlFor="new-password">{t("newPassword")}</FieldLabel>
                    <Input
                      id="new-password"
                      {...passwordForm.register("new_password")}
                      type="password"
                      autoComplete="new-password"
                      minLength={8}
                      maxLength={128}
                      required
                    />
                  </Field>
                  <Field>
                    <FieldLabel htmlFor="confirm-password">{t("confirmPassword")}</FieldLabel>
                    <Input
                      id="confirm-password"
                      {...passwordForm.register("confirm_password")}
                      type="password"
                      autoComplete="new-password"
                      required
                    />
                  </Field>
                  {Object.keys(passwordForm.formState.errors).length > 0 && (
                    <FieldError>
                      {passwordForm.formState.errors.confirm_password?.message ?? errors("invalidRequest")}
                    </FieldError>
                  )}
                  <Field orientation="horizontal">
                    <Button type="submit" disabled={busy}>
                      {t("changeAndSignOut")}
                    </Button>
                    <Button
                      type="button"
                      variant="outline"
                      onClick={() =>
                        void perform(async () => {
                          await signOut();
                          router.replace("/login");
                        })
                      }
                    >
                      {t("signOut")}
                    </Button>
                  </Field>
                </FieldGroup>
              </fieldset>
            </form>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
