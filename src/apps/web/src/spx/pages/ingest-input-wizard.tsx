"use client";

import { useEffect, useState } from "react";

import { useSearchParams } from "next/navigation";

import { useMutation, useQuery } from "@tanstack/react-query";
import { ArrowRight } from "lucide-react";
import { useLocale, useTranslations } from "next-intl";
import { useForm, useFormState, useWatch } from "react-hook-form";

import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Field, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import { APIError } from "@/spx/api/client";
import { listEventClasses } from "@/spx/api/ingest/event-schema";
import { createInput, listInputPage } from "@/spx/api/ingest/inputs";
import type { ApiInput, ApiMappingSummary } from "@/spx/api/ingest/schemas";
import { zCreateInputBody } from "@/spx/api/ingest/schemas";

import { IngestSection, SourceIdentityFields } from "../components/ingest-section";
import { PageHeader } from "../components/page-header";
import {
  channelIcons,
  DetailFields,
  protocols,
  RulePicker,
  SampleTrial,
  unboundRule,
  useMappingDetail,
  useMappingTrial,
  WizardProgress,
} from "./ingest-input-controls";

export function InputWizard({
  canVerify,
  onClose,
  onCreated,
}: {
  canVerify: boolean;
  onClose: () => void;
  onCreated: (input: ApiInput, token?: string) => void;
}) {
  const t = useTranslations("Ingest");
  const mixed = useTranslations("MixedRule");
  const locale = useLocale();

  const [step, setStep] = useState(0);
  const mt = useTranslations("Mappings");
  const channel = useSearchParams().get("via");
  const initialProtocol = protocols.find((value) => value === channel) ?? "http";
  const form = useForm({
    defaultValues: {
      mappingID: unboundRule,
      name: "",
      vendor: "",
      product: "",
      protocol: initialProtocol,
      port: "",
      brokers: "",
      topic: "",
      startPosition: "",
      kafkaCredential: "",
    },
  });
  const values = useWatch({ control: form.control });
  const { protocol = initialProtocol, mappingID = unboundRule } = values;
  const { errors, dirtyFields } = useFormState({ control: form.control });
  const [conflictField, setConflictField] = useState<"name" | "port" | "kafkaCredential">();
  useEffect(() => {
    if (conflictField) form.setFocus(conflictField);
  }, [conflictField, form]);
  function chooseMapping(id: string, rule?: ApiMappingSummary) {
    form.setValue("mappingID", id);
    for (const field of ["vendor", "product"] as const) {
      if (!dirtyFields[field]) form.setValue(field, rule?.[field] ?? "");
    }
  }
  const selectedRule = useMappingDetail(mappingID);
  const selected = selectedRule.data;
  const trial = useMappingTrial(selected);
  const catalog = useQuery({
    queryKey: ["ingest", "event-schema"],
    queryFn: ({ signal }) => listEventClasses(signal),
    enabled: !!selected,
  });
  const eventTypes = selected?.class_uids
    ?.map((uid) => catalog.data?.classes.find((item) => item.class_uid === uid)?.name[locale])
    .filter(Boolean)
    .join(" · ");
  const create = useMutation({
    mutationFn: async (body: Parameters<typeof createInput>[0]) => {
      const result = await createInput(body);
      onCreated(result.input, result.token);
    },
    onError: async (error, body) => {
      if (!(error instanceof APIError)) return;
      if (error.fieldErrors.some((entry) => entry.field === "kafka_credential")) {
        setStep(1);
        form.setError("kafkaCredential", { message: t("kafkaCredentialUnavailable") });
        setConflictField("kafkaCredential");
        return;
      }
      if (error.status !== 409) return;
      const sameName = await listInputPage({ limit: 1, name: body.name }).catch(() => undefined);
      const samePort =
        !sameName?.total && body.protocol === "syslog"
          ? await listInputPage({ limit: 1, syslog_port: body.syslog_port }).catch(() => undefined)
          : undefined;
      let field: "name" | "port" | undefined;
      if (sameName?.total) field = "name";
      else if (samePort?.total) field = "port";
      if (!field) return;
      setStep(field === "name" ? 0 : 1);
      form.setError(field, { message: t(field === "name" ? "nameTaken" : "portTaken") });
      setConflictField(field);
    },
  });
  const advance = () => {
    const values = form.getValues();
    form.clearErrors();
    const parsed = zCreateInputBody.safeParse({
      name: values.name,
      vendor: values.vendor,
      product: values.product,
      protocol: values.protocol,
      parsing_rule_id: selected?.id,
      expected_rule_version: selected?.version,
      syslog_port: Number(values.port),
      kafka_topic: values.topic,
      kafka_credential: values.kafkaCredential.trim() || undefined,
      kafka_start_position: values.startPosition,
      kafka_brokers: values.brokers
        .split(/[,\n]/)
        .map((broker) => broker.trim())
        .filter(Boolean),
    });
    if (!parsed.success) {
      const fieldErrors = {
        name: ["name", "nameRequired"],
        syslog_port: ["port", "portInvalid"],
        kafka_topic: ["topic", "topicRequired"],
        kafka_credential: ["kafkaCredential", "kafkaCredentialInvalid"],
        kafka_brokers: ["brokers", "brokersRequired"],
        kafka_start_position: ["startPosition", "startPositionRequired"],
      } as const;
      for (const [index, issue] of parsed.error.issues.entries()) {
        const key = issue.path[0];
        if (key === "name" || key === "syslog_port" || key === "kafka_topic" || key === "kafka_credential" || key === "kafka_brokers" || key === "kafka_start_position") {
          const [field, message] = fieldErrors[key];
          form.setError(field, { type: "validate", message: t(message) }, { shouldFocus: index === 0 });
        }
      }
      return;
    }
    if (step === 1) setStep(2);
    else if (step === 2) setStep(3);
    else if (mappingID === unboundRule || (selected && trial.verified)) create.mutate(parsed.data);
  };

  const direction = { http: "pushHttp", syslog: "pushSyslog", kafka: "pullKafka" } as const;
  function next() {
    if (create.isError) create.reset();
    setConflictField(undefined);
    if (step === 0) {
      if (!form.getValues("name").trim()) {
        form.setError("name", { message: t("nameRequired") }, { shouldFocus: true });
        return;
      }
      form.clearErrors();
      setStep(1);
      return;
    }
    advance();
  }
  return (
    <section className="@container flex min-w-0 flex-col gap-6">
      <PageHeader title={t("createInput")} />
      <Card className="min-w-0 gap-4 p-4 sm:p-6">
        <WizardProgress step={step} onNavigate={create.isPending ? undefined : setStep} />
        <div className="grid min-w-0 items-start gap-8 @min-[54rem]:grid-cols-[minmax(0,1.35fr)_minmax(0,0.75fr)]">
          {step === 0 && (
            <IngestSection title={t("logSource")}>
              <FieldGroup>
                <Field data-invalid={!!errors.name}>
                  <FieldLabel required htmlFor="wizard-name">
                    {t("name")}
                  </FieldLabel>
                  <Input
                    id="wizard-name"
                    aria-required="true"
                    maxLength={100}
                    aria-invalid={!!errors.name}
                    aria-describedby={errors.name ? "wizard-name-error" : undefined}
                    {...form.register("name")}
                  />
                  <FieldError id="wizard-name-error" errors={[errors.name]} />
                </Field>
                <SourceIdentityFields
                  maxLength={120}
                  vendor={form.register("vendor")}
                  product={form.register("product")}
                />
                <RulePicker value={mappingID} selected={selected} onChange={chooseMapping} disabled={!canVerify} />
              </FieldGroup>
            </IngestSection>
          )}
          {step === 1 && (
            <div className="grid gap-6">
              <IngestSection title={t("protocol")} required>
                <RadioGroup
                  value={protocol}
                  aria-label={t("protocol")}
                  aria-required="true"
                  onValueChange={(value) => {
                    const channel = protocols.find((item) => item === value);
                    if (channel) form.setValue("protocol", channel);
                  }}
                  className="grid gap-2 @min-[32rem]:grid-cols-3"
                >
                  {protocols.map((option) => {
                    const Icon = channelIcons[option];
                    return (
                      <label
                        key={option}
                        htmlFor={`via-${option}`}
                        className="flex cursor-pointer flex-col gap-2 rounded-lg border p-3 has-[[data-state=checked]]:border-primary has-[[data-state=checked]]:bg-primary/5 hover:bg-muted/40"
                      >
                        <span className="flex items-center justify-between gap-2">
                          <Icon aria-hidden className="size-4 text-muted-foreground" />
                          <RadioGroupItem id={`via-${option}`} value={option} aria-label={t(option)} />
                        </span>
                        <span className="font-medium">{t(option)}</span>
                        <span className="text-xs text-muted-foreground">{t(direction[option])}</span>
                      </label>
                    );
                  })}
                </RadioGroup>
              </IngestSection>
              <IngestSection title={t("connection")}>
                <FieldGroup>
                  {protocol === "http" && (
                    <DetailFields
                      rows={[
                        { label: t("protocol"), value: "HTTP" },
                        { label: t("tokenLabel"), value: t("afterCreate") },
                      ]}
                    />
                  )}
                  {protocol === "syslog" && (
                    <Field data-invalid={!!errors.port}>
                      <FieldLabel required htmlFor="wizard-port">
                        {t("port")}
                      </FieldLabel>
                      <Input
                        id="wizard-port"
                        aria-required="true"
                        inputMode="numeric"
                        aria-invalid={!!errors.port}
                        aria-describedby={errors.port ? "wizard-port-error" : undefined}
                        {...form.register("port")}
                      />
                      <FieldError id="wizard-port-error" errors={[errors.port]} />
                    </Field>
                  )}
                  {protocol === "kafka" && (
                    <>
                      <Field data-invalid={!!errors.brokers}>
                        <FieldLabel required htmlFor="wizard-brokers">
                          {t("brokers")}
                        </FieldLabel>
                        <Textarea
                          id="wizard-brokers"
                          aria-required="true"
                          placeholder="broker.example:9092"
                          aria-invalid={!!errors.brokers}
                          aria-describedby={errors.brokers ? "wizard-brokers-error" : undefined}
                          {...form.register("brokers")}
                        />
                        <FieldError id="wizard-brokers-error" errors={[errors.brokers]} />
                      </Field>
                      <Field data-invalid={!!errors.topic}>
                        <FieldLabel required htmlFor="wizard-topic">
                          {t("topic")}
                        </FieldLabel>
                        <Input
                          id="wizard-topic"
                          aria-required="true"
                          maxLength={249}
                          aria-invalid={!!errors.topic}
                          aria-describedby={errors.topic ? "wizard-topic-error" : undefined}
                          {...form.register("topic")}
                        />
                        <FieldError id="wizard-topic-error" errors={[errors.topic]} />
                      </Field>
                      <Field data-invalid={!!errors.kafkaCredential}>
                        <FieldLabel htmlFor="wizard-kafka-credential">{t("kafkaCredential")}</FieldLabel>
                        <Input
                          id="wizard-kafka-credential"
                          placeholder={t("kafkaCredentialPlaceholder")}
                          maxLength={64}
                          aria-invalid={!!errors.kafkaCredential}
                          aria-describedby={errors.kafkaCredential ? "wizard-kafka-credential-error" : undefined}
                          {...form.register("kafkaCredential")}
                        />
                        <FieldError id="wizard-kafka-credential-error" errors={[errors.kafkaCredential]} />
                      </Field>
                      <Field data-invalid={!!errors.startPosition}>
                        <FieldLabel required htmlFor="wizard-kafka-start">{t("startPosition")}</FieldLabel>
                        <Select
                          value={values.startPosition}
                          onValueChange={(position) => {
                            form.setValue("startPosition", position);
                            form.clearErrors("startPosition");
                          }}
                        >
                          <SelectTrigger
                            id="wizard-kafka-start"
                            ref={form.register("startPosition").ref}
                            aria-required="true"
                            aria-invalid={!!errors.startPosition}
                            aria-describedby={errors.startPosition ? "wizard-kafka-start-error" : undefined}
                            className="w-full"
                          >
                            <SelectValue placeholder={t("startPositionRequired")} />
                          </SelectTrigger>
                          <SelectContent>
                            <SelectItem value="earliest">{t("startEarliest")}</SelectItem>
                            <SelectItem value="since_created">{t("startSinceCreated")}</SelectItem>
                          </SelectContent>
                        </Select>
                        <FieldError id="wizard-kafka-start-error" errors={[errors.startPosition]} />
                      </Field>
                    </>
                  )}
                </FieldGroup>
              </IngestSection>
            </div>
          )}
          {step === 2 && (
            <IngestSection title={t("verifyStep")}>
              {selected ? (
                <SampleTrial trial={trial} />
              ) : (
                <Alert>
                  <AlertDescription>{mixed("unboundNotice")}</AlertDescription>
                </Alert>
              )}
            </IngestSection>
          )}
          {step === 3 && (
            <div className="grid gap-6">
              <IngestSection
                title={t("sourceConnection")}
                action={
                  <Button variant="outline" size="sm" disabled={create.isPending} onClick={() => setStep(0)}>
                    {t("editBasic")}
                  </Button>
                }
              >
                <DetailFields
                  rows={[
                    { label: t("name"), value: values.name },
                    { label: t("protocol"), value: t(protocol) },
                    { label: mt("vendor"), value: values.vendor || "—" },
                    { label: mt("product"), value: values.product || "—" },
                    ...(protocol === "syslog" ? [{ label: t("port"), value: values.port }] : []),
                    ...(protocol === "kafka"
                      ? [
                          { label: t("brokers"), value: values.brokers },
                          { label: t("topic"), value: values.topic },
                          ...(values.kafkaCredential ? [{ label: t("kafkaCredential"), value: values.kafkaCredential }] : []),
                          { label: t("startPosition"), value: t(values.startPosition === "earliest" ? "startEarliest" : "startSinceCreated") },
                        ]
                      : []),
                  ]}
                />
              </IngestSection>
              {mappingID !== unboundRule && !selected && (
                <Alert variant={selectedRule.isError ? "destructive" : "default"}>
                  <AlertDescription>
                    {selectedRule.isError ? mt("loadFailed") : t("loadingRule")}
                    {selectedRule.isError && (
                      <Button size="sm" variant="link" onClick={() => void selectedRule.refetch()}>
                        {t("retry")}
                      </Button>
                    )}
                  </AlertDescription>
                </Alert>
              )}
              {mappingID === unboundRule && (
                <Alert>
                  <AlertDescription>{mixed("unboundNotice")}</AlertDescription>
                </Alert>
              )}
            </div>
          )}
          <IngestSection title={t("parseConfig")}>
            <DetailFields
              rows={[
                { label: t("name"), value: values.name || "—" },
                { label: t("protocol"), value: t(protocol) },
                { label: t("mapping"), value: selected?.name ?? (mappingID === unboundRule ? t("mappingNone") : "") },
                ...(eventTypes ? [{ label: mixed("eventTypes"), value: eventTypes }] : []),
                ...(selected
                  ? [
                      {
                        label: t("verifyStep"),
                        value: trial.result
                          ? mixed("matchedSamples", { matched: trial.result.matched, total: trial.result.total })
                          : t("unverified"),
                      },
                    ]
                  : []),
              ]}
            />
          </IngestSection>
        </div>
        {create.isError && (
          <Alert variant="destructive">
            <AlertDescription>
              {create.error instanceof APIError && create.error.status === 409 ? t("conflict") : t("checkBeforeRetry")}{" "}
              <Button size="sm" variant="link" onClick={onClose}>
                {t("backToList")}
              </Button>
            </AlertDescription>
          </Alert>
        )}
        <div className="flex justify-between gap-3 border-t pt-4">
          <Button variant="outline" disabled={create.isPending} onClick={() => (step ? setStep(step - 1) : onClose())}>
            {t(step ? "back" : "cancel")}
          </Button>
          <Button
            disabled={create.isPending || (step >= 2 && mappingID !== unboundRule && !trial.verified)}
            onClick={next}
          >
            {t(step === 3 ? "createEnable" : "next")}
            {step < 3 && <ArrowRight />}
          </Button>
        </div>
      </Card>
    </section>
  );
}
