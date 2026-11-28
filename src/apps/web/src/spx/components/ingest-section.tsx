import type { ReactNode } from "react";

import { useTranslations } from "next-intl";
import type { FieldError, UseFormRegisterReturn } from "react-hook-form";

import { FieldGroup } from "@/components/ui/field";
import { Separator } from "@/components/ui/separator";

import { FormInput } from "./forms";

export function IngestSection({
  title,
  action,
  children,
  required,
}: {
  title: string;
  action?: ReactNode;
  children: ReactNode;
  required?: boolean;
}) {
  return (
    <section className="flex min-w-0 flex-col gap-4" aria-label={title}>
      <div className="flex min-h-8 items-center justify-between gap-3">
        <h2 className="font-heading text-base font-semibold">
          {title}
          {required && <span aria-hidden="true" className="ms-2 text-destructive before:content-['*']" />}
        </h2>
        {action}
      </div>
      <Separator />
      {children}
    </section>
  );
}

export function SourceIdentityFields({
  vendor,
  product,
  errors,
  errorText,
  maxLength,
  required,
  readOnly,
}: {
  vendor: UseFormRegisterReturn;
  product: UseFormRegisterReturn;
  errors?: Partial<Record<"vendor" | "product", FieldError>>;
  errorText?: Partial<Record<"vendor" | "product", string>>;
  maxLength?: number;
  required?: boolean;
  readOnly?: boolean;
}) {
  const t = useTranslations("Mappings");
  return (
    <FieldGroup className="grid gap-4 sm:grid-cols-2">
      {(
        [
          ["vendor", vendor],
          ["product", product],
        ] as const
      ).map(([key, registration]) => (
        <FormInput
          key={key}
          label={t(key)}
          registration={registration}
          error={errors?.[key]}
          errorText={errorText?.[key]}
          maxLength={maxLength}
          required={required}
          readOnly={readOnly}
        />
      ))}
    </FieldGroup>
  );
}
