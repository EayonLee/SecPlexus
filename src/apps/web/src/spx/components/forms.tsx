"use client";
import { type ReactNode, useEffect, useId, useRef, useState } from "react";

import { zodResolver } from "@hookform/resolvers/zod";
import { useTranslations } from "next-intl";
import {
  type DefaultValues,
  type FieldValues,
  type FieldError as FormError,
  type UseFormRegisterReturn,
  type UseFormReturn,
  useForm,
  useFormState,
} from "react-hook-form";
import type { z } from "zod";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Field, FieldDescription, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Spinner } from "@/components/ui/spinner";
import { Textarea } from "@/components/ui/textarea";

import { APIError } from "../api/client";
import { zReauthenticateBody } from "../api/iam/schemas";
import { errorMessageKey, reauthenticate } from "../iam/session";

/** FormDialog 保留业务 RHF 草稿，在同一对话框中完成服务端要求的身份确认。 */
export function FormDialog<TInput extends FieldValues, TOutput extends FieldValues>({
  title,
  description,
  schema,
  defaultValues,
  children,
  onClose,
  onSubmit,
  submitDisabled = false,
}: {
  title: string;
  description?: string;
  schema: z.ZodType<TOutput, TInput>;
  defaultValues: DefaultValues<TInput>;
  children?: (form: UseFormReturn<TInput, unknown, TOutput>) => ReactNode;
  submitDisabled?: boolean;
  onClose: () => void;
  onSubmit: (values: TOutput) => Promise<unknown>;
}) {
  const t = useTranslations("Common");
  const errors = useTranslations("Errors");
  const descriptionID = useId();
  const form = useForm<TInput, unknown, TOutput>({ resolver: zodResolver(schema), defaultValues });
  const businessState = useFormState({ control: form.control });
  const account = useTranslations("Account");
  const confirmation = useForm<z.infer<typeof zReauthenticateBody>>({
    resolver: zodResolver(zReauthenticateBody),
    defaultValues: { password: "" },
  });
  const [confirming, setConfirming] = useState(false);
  const previousStep = useRef<boolean>(false);
  const submit = useRef<HTMLButtonElement>(null);
  const busy = businessState.isSubmitting || confirmation.formState.isSubmitting;
  useEffect(() => {
    if (busy) return;
    if (confirming) confirmation.setFocus("password");
    else if (previousStep.current) submit.current?.focus();
    previousStep.current = confirming;
  }, [busy, confirming, confirmation]);
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !busy) {
          form.reset();
          confirmation.reset();
          onClose();
        }
      }}
    >
      <DialogContent
        aria-describedby={description && !confirming ? descriptionID : undefined}
        onEscapeKeyDown={(event) => {
          // Base UI 的候选弹层与 Radix Dialog 分别处理 Escape，先让候选框关闭。
          const target = event.target;
          if (
            target instanceof HTMLElement &&
            target.getAttribute("role") === "combobox" &&
            target.getAttribute("aria-expanded") === "true"
          )
            event.preventDefault();
        }}
      >
        <DialogHeader>
          <DialogTitle>{confirming ? account("confirmCurrentPassword") : title}</DialogTitle>
          {description && !confirming && <DialogDescription id={descriptionID}>{description}</DialogDescription>}
        </DialogHeader>
        <form
          hidden={confirming}
          inert={confirming}
          className="motion-safe:animate-in motion-safe:fade-in-0 motion-safe:slide-in-from-left-2 motion-safe:duration-150"
          noValidate
          onSubmit={form.handleSubmit(async (values) => {
            form.clearErrors("root");
            try {
              await onSubmit(values);
              form.reset();
              onClose();
            } catch (error) {
              if (error instanceof APIError && error.code === "iam.recent_auth_required") {
                setConfirming(true);
                return;
              }
              if (!(error instanceof DOMException && error.name === "AbortError"))
                form.setError("root", { message: errors(errorMessageKey(error)) });
            }
          })}
        >
          <fieldset disabled={busy} className="flex flex-col gap-5">
            <FieldGroup>{children?.({ ...form, formState: businessState })}</FieldGroup>
            {businessState.errors.root && <FieldError>{businessState.errors.root.message}</FieldError>}
            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                onClick={() => {
                  form.reset();
                  onClose();
                }}
                disabled={busy}
              >
                {t("cancel")}
              </Button>
              <Button ref={submit} type="submit" disabled={busy || submitDisabled}>
                {busy && <Spinner data-icon="inline-start" />}
                {t("confirm")}
              </Button>
            </DialogFooter>
          </fieldset>
        </form>
        {confirming && (
          <form
            noValidate
            className="motion-safe:animate-in motion-safe:fade-in-0 motion-safe:slide-in-from-right-2 motion-safe:duration-150"
            onSubmit={confirmation.handleSubmit(async (values) => {
              confirmation.clearErrors("root");
              try {
                await reauthenticate(values.password);
                confirmation.reset();
                setConfirming(false);
              } catch (error) {
                if (!(error instanceof DOMException && error.name === "AbortError"))
                  confirmation.setError("root", { message: errors(errorMessageKey(error)) });
              }
            })}
          >
            <fieldset disabled={busy} className="flex flex-col gap-5">
              <FieldGroup>
                <Field data-invalid={!!confirmation.formState.errors.password}>
                  <FieldLabel htmlFor={`${descriptionID}-password`}>{account("currentPassword")}</FieldLabel>
                  <Input
                    id={`${descriptionID}-password`}
                    {...confirmation.register("password")}
                    type="password"
                    autoComplete="current-password"
                    required
                    aria-invalid={!!confirmation.formState.errors.password}
                  />
                  {confirmation.formState.errors.password && <FieldError>{errors("invalidRequest")}</FieldError>}
                </Field>
              </FieldGroup>
              {confirmation.formState.errors.root && (
                <FieldError>{confirmation.formState.errors.root.message}</FieldError>
              )}
              <DialogFooter>
                <Button
                  type="button"
                  variant="outline"
                  onClick={() => {
                    confirmation.reset();
                    setConfirming(false);
                  }}
                >
                  {t("back")}
                </Button>
                <Button type="submit" disabled={busy}>
                  {busy && <Spinner data-icon="inline-start" />}
                  {t("confirmAndContinue")}
                </Button>
              </DialogFooter>
            </fieldset>
          </form>
        )}
      </DialogContent>
    </Dialog>
  );
}

/** FormInput 复用现有 Field 和输入样式，错误文案由 next-intl 提供。 */
export function FormInput({
  label,
  registration,
  error,
  errorText,
  type = "text",
  description,
  placeholder,
}: {
  label: string;
  registration: UseFormRegisterReturn;
  error?: FormError;
  errorText?: string;
  type?: "text" | "password" | "email" | "textarea";
  description?: string;
  placeholder?: string;
}) {
  const id = useId();
  const errors = useTranslations("Errors");
  return (
    <Field data-invalid={!!error}>
      <FieldLabel htmlFor={id}>{label}</FieldLabel>
      {type === "textarea" ? (
        <Textarea id={id} {...registration} aria-invalid={!!error} />
      ) : (
        <Input
          id={id}
          {...registration}
          type={type}
          placeholder={placeholder}
          autoComplete={type === "password" ? "new-password" : "off"}
          aria-invalid={!!error}
        />
      )}
      {description && <FieldDescription>{description}</FieldDescription>}
      {error && <FieldError>{errorText ?? errors("invalidRequest")}</FieldError>}
    </Field>
  );
}
