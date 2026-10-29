"use client";
import { type ReactNode, useId } from "react";

import { zodResolver } from "@hookform/resolvers/zod";
import { useTranslations } from "next-intl";
import {
  type DefaultValues,
  type FieldValues,
  type FieldError as FormError,
  type UseFormRegisterReturn,
  type UseFormReturn,
  useForm,
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

import { errorMessageKey } from "../iam/session";

/** FormDialog 只组合对话框与 RHF 生命周期；字段和生成协议类型由业务页面声明。 */
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
  const form = useForm<TInput, unknown, TOutput>({ resolver: zodResolver(schema), defaultValues });
  const busy = form.formState.isSubmitting;
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !busy) {
          form.reset();
          onClose();
        }
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>{description ?? t("formDescription")}</DialogDescription>
        </DialogHeader>
        <form
          noValidate
          onSubmit={form.handleSubmit(async (values) => {
            form.clearErrors("root");
            try {
              await onSubmit(values);
              form.reset();
              onClose();
            } catch (error) {
              if (!(error instanceof DOMException && error.name === "AbortError"))
                form.setError("root", { message: errors(errorMessageKey(error)) });
            }
          })}
        >
          <fieldset disabled={busy} className="flex flex-col gap-5">
            <FieldGroup>{children?.(form)}</FieldGroup>
            {form.formState.errors.root && <FieldError>{form.formState.errors.root.message}</FieldError>}
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
              <Button type="submit" disabled={busy || submitDisabled}>
                {busy && <Spinner data-icon="inline-start" />}
                {t("confirm")}
              </Button>
            </DialogFooter>
          </fieldset>
        </form>
      </DialogContent>
    </Dialog>
  );
}

/** FormInput 复用现有 Field 和输入样式，错误文案由 next-intl 提供。 */
export function FormInput({
  label,
  registration,
  error,
  type = "text",
  description,
  placeholder,
}: {
  label: string;
  registration: UseFormRegisterReturn;
  error?: FormError;
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
      {error && <FieldError>{errors("invalidRequest")}</FieldError>}
    </Field>
  );
}
