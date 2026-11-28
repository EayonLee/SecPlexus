"use client";
import { type ComponentProps, type ReactNode, useId, useRef } from "react";

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

import {
  AlertDialog,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
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
import { errorMessageKey } from "../iam/session";
import { requestReauth } from "./reauth";
import { SheetPanel } from "./sheet-panel";

// 短表单使用对话框，长表单使用右侧抽屉，删除使用危险操作确认框。
export function FormDialog<TInput extends FieldValues, TOutput extends FieldValues>({
  title,
  description,
  schema,
  defaultValues,
  children,
  onClose,
  onSubmit,
  submitDisabled = false,
  variant = "dialog",
  confirmVariant = "default",
  submitLabel,
  layout,
  returnFocus,
}: {
  title: string;
  description?: string;
  schema: z.ZodType<TOutput, TInput>;
  defaultValues: DefaultValues<TInput>;
  children?: (form: UseFormReturn<TInput, unknown, TOutput>) => ReactNode;
  submitDisabled?: boolean;
  onClose: () => void;
  onSubmit: (values: TOutput) => Promise<unknown>;
  variant?: "dialog" | "sheet" | "alert";
  confirmVariant?: "default" | "destructive";
  submitLabel?: string;
  layout?: "product" | "management";
  returnFocus?: HTMLElement | null;
}) {
  const t = useTranslations("Common");
  const compact = layout === "product";
  const errors = useTranslations("Errors");
  const descriptionID = useId();
  const formID = useId();
  const form = useForm<TInput, unknown, TOutput>({ resolver: zodResolver(schema), defaultValues });
  const businessState = useFormState({ control: form.control });
  const busy = businessState.isSubmitting;
  const opener = useRef<HTMLElement | null>(null);
  const rememberOpener = () => {
    if (document.activeElement instanceof HTMLElement) opener.current = document.activeElement;
  };
  const restoreFocus = (event: Event) => {
    const target = returnFocus ?? opener.current;
    if (target?.isConnected) {
      event.preventDefault();
      requestAnimationFrame(() => {
        if (target.isConnected) target.focus({ preventScroll: true });
      });
    }
  };

  // save 在需要近期密码确认时先走全局确认框，确认成功后重放一次提交。
  // 表单本身不承载确认字段：同一个确认框覆盖抽屉与对话框两种形态。
  const save = async (values: TOutput) => {
    try {
      await onSubmit(values);
    } catch (error) {
      if (!(error instanceof APIError) || error.code !== "iam.recent_auth_required") throw error;
      if (!(await requestReauth())) throw error;
      await onSubmit(values);
    }
  };

  const close = () => {
    if (busy) return;
    form.reset();
    onClose();
  };

  const body = (
    <form
      id={formID}
      noValidate
      className={compact ? "-mx-4 min-h-0 overflow-y-auto px-4 py-4" : undefined}
      onSubmit={form.handleSubmit(async (values) => {
        form.clearErrors("root");
        try {
          await save(values);
          form.reset();
          onClose();
        } catch (error) {
          if (!(error instanceof DOMException && error.name === "AbortError"))
            form.setError("root", { message: errors(errorMessageKey(error)) });
        }
      })}
    >
      <fieldset disabled={busy} className="flex flex-col gap-5">
        <FieldGroup>{children?.({ ...form, formState: businessState })}</FieldGroup>
        {businessState.errors.root && <FieldError>{businessState.errors.root.message}</FieldError>}
      </fieldset>
    </form>
  );

  const footer = (
    <>
      <Button type="button" variant="outline" onClick={close} disabled={busy}>
        {t("cancel")}
      </Button>
      <Button type="submit" form={formID} variant={confirmVariant} disabled={busy || submitDisabled}>
        {busy && <Spinner data-icon="inline-start" />}
        {submitLabel ?? t("confirm")}
      </Button>
    </>
  );

  if (variant === "sheet")
    return (
      <SheetPanel
        title={title}
        description={description}
        footer={footer}
        onClose={close}
        layout={layout === "management" ? layout : undefined}
        returnFocus={returnFocus}
      >
        <div className="py-5">{body}</div>
      </SheetPanel>
    );

  if (variant === "alert" && description)
    return (
      <AlertDialog
        open
        onOpenChange={(open) => {
          if (!open) close();
        }}
      >
        <AlertDialogContent onOpenAutoFocus={rememberOpener} onCloseAutoFocus={restoreFocus}>
          <AlertDialogHeader>
            <AlertDialogTitle>{title}</AlertDialogTitle>
            <AlertDialogDescription>{description}</AlertDialogDescription>
          </AlertDialogHeader>
          {body}
          <AlertDialogFooter>
            <AlertDialogCancel disabled={busy}>{t("cancel")}</AlertDialogCancel>
            <Button type="submit" form={formID} variant="destructive" disabled={busy || submitDisabled}>
              {busy && <Spinner data-icon="inline-start" />}
              {submitLabel ?? t("confirm")}
            </Button>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    );

  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) close();
      }}
    >
      <DialogContent
        className={compact ? "flex max-h-[90dvh] flex-col gap-0 sm:max-w-lg" : undefined}
        aria-describedby={description ? descriptionID : undefined}
        onOpenAutoFocus={rememberOpener}
        onCloseAutoFocus={restoreFocus}
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
        <DialogHeader className={compact ? "-mx-4 shrink-0 border-b px-4 pb-4 pr-12" : undefined}>
          <DialogTitle>{title}</DialogTitle>
          {description && <DialogDescription id={descriptionID}>{description}</DialogDescription>}
        </DialogHeader>
        {body}
        <DialogFooter className={compact ? "shrink-0 bg-popover" : undefined}>{footer}</DialogFooter>
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
  fullRow = false,
  description,
  placeholder,
  autoComplete,
  textareaVariant,
  maxLength,
  required,
  readOnly,
}: {
  label: string;
  registration: UseFormRegisterReturn;
  error?: FormError;
  errorText?: string;
  fullRow?: boolean;
  type?: "text" | "password" | "email" | "textarea";
  description?: string;
  placeholder?: string;
  autoComplete?: string;
  textareaVariant?: ComponentProps<typeof Textarea>["variant"];
  maxLength?: number;
  required?: boolean;
  readOnly?: boolean;
}) {
  const id = useId();
  const errors = useTranslations("Errors");
  const descriptionID = description ? `${id}-description` : undefined;
  const errorID = error ? `${id}-error` : undefined;
  const describedBy = [descriptionID, errorID].filter(Boolean).join(" ") || undefined;
  return (
    <Field data-full-row={fullRow} data-invalid={!!error}>
      <FieldLabel htmlFor={id} required={required}>
        {label}
      </FieldLabel>
      {type === "textarea" ? (
        <Textarea
          id={id}
          {...registration}
          variant={textareaVariant}
          placeholder={placeholder}
          readOnly={readOnly}
          aria-required={required}
          aria-invalid={!!error}
          aria-describedby={describedBy}
        />
      ) : (
        <Input
          id={id}
          {...registration}
          type={type}
          placeholder={placeholder}
          autoComplete={autoComplete ?? (type === "password" ? "new-password" : "off")}
          maxLength={maxLength}
          readOnly={readOnly}
          aria-required={required}
          aria-invalid={!!error}
          aria-describedby={describedBy}
        />
      )}
      {description && <FieldDescription id={descriptionID}>{description}</FieldDescription>}
      {error && <FieldError id={errorID}>{errorText ?? errors("invalidRequest")}</FieldError>}
    </Field>
  );
}
