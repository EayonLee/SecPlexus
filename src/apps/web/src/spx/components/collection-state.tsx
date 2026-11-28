"use client";

import type { ComponentType, ReactNode } from "react";

import type { UseQueryResult } from "@tanstack/react-query";
import { FolderOpen, LoaderCircle, SearchX, Unplug } from "lucide-react";
import { useTranslations } from "next-intl";

import { Button } from "@/components/ui/button";
import { Empty, EmptyContent, EmptyDescription, EmptyHeader, EmptyMedia } from "@/components/ui/empty";

import { errorMessageKey } from "../iam/session";
import { PageHeader, PageTitle } from "./page-header";

export function StatePanel({
  title,
  description,
  icon: Icon,
  actions,
  kind = "empty",
  placement = "section",
  pageHeading = false,
}: {
  title: string;
  description?: string;
  icon: ComponentType<{ className?: string }>;
  actions?: ReactNode;
  kind?: "error" | "empty" | "filtered" | "loading";
  pageHeading?: boolean;
  placement?: "section" | "viewport" | "notice";
}) {
  return (
    <Empty
      data-state-kind={kind}
      data-placement={placement}
      role={kind === "error" ? "alert" : "status"}
      aria-busy={kind === "loading" || undefined}
      className="state-panel bg-muted/20 whitespace-normal"
    >
      <EmptyHeader>
        <EmptyMedia
          variant="icon"
          className={
            kind === "error" ? "bg-status-warning/10 text-status-warning size-11" : "bg-primary/10 text-primary size-11"
          }
        >
          <Icon className={kind === "loading" ? "size-5 motion-safe:animate-spin" : "size-5"} />
        </EmptyMedia>
        {pageHeading || placement === "viewport" ? (
          <PageTitle>{title}</PageTitle>
        ) : (
          <h2 className="text-foreground text-base font-semibold tracking-tight">{title}</h2>
        )}
        {description && <EmptyDescription>{description}</EmptyDescription>}
      </EmptyHeader>
      {actions && <EmptyContent className="flex-row flex-wrap justify-center">{actions}</EmptyContent>}
    </Empty>
  );
}

type CollectionProps = { title?: string; description?: string } & (
  | { kind: "error"; onRetry: () => void; retrying?: boolean; retainContent?: boolean }
  | { kind: "loading" }
  | { kind: "empty"; actions?: ReactNode }
  | { kind: "filtered"; onClear: () => void }
);

export function CollectionState(props: CollectionProps) {
  const t = useTranslations("Common");
  if (props.kind === "error")
    return (
      <StatePanel
        kind="error"
        placement={props.retainContent ? "notice" : "section"}
        icon={Unplug}
        title={props.title ?? t("collectionError")}
        description={props.description}
        actions={
          <Button type="button" variant="outline" size="sm" disabled={props.retrying} onClick={props.onRetry}>
            {t("retry")}
          </Button>
        }
      />
    );
  if (props.kind === "loading") return <StatePanel kind="loading" icon={LoaderCircle} title={t("loading")} />;
  return (
    <StatePanel
      kind={props.kind}
      icon={props.kind === "filtered" ? SearchX : FolderOpen}
      title={props.title ?? t(props.kind === "filtered" ? "noMatches" : "empty")}
      description={props.description}
      actions={
        props.kind === "filtered" ? (
          <Button type="button" size="sm" variant="outline" onClick={props.onClear}>
            {t("clearFilters")}
          </Button>
        ) : (
          props.actions
        )
      }
    />
  );
}

// 首次请求占用列表内容区。刷新失败时保留可用内容，让重试始终针对原查询。
export function CollectionBoundary({
  query,
  hasContent,
  title,
  pageTitle,
  children,
}: {
  query: Pick<UseQueryResult, "error" | "isPending" | "isError" | "isFetching" | "refetch">;
  hasContent: boolean;
  title?: string;
  pageTitle?: string;
  children?: ReactNode;
}) {
  const errors = useTranslations("Errors");
  const showContent = hasContent || (!query.isPending && !query.isError);
  return (
    <>
      {!showContent && pageTitle && <PageHeader title={pageTitle} />}
      {query.isError && (
        <CollectionState
          kind="error"
          title={title}
          description={errors(errorMessageKey(query.error))}
          onRetry={() => void query.refetch()}
          retrying={query.isFetching}
          retainContent={hasContent}
        />
      )}
      {query.isPending && !showContent && <CollectionState kind="loading" />}
      {showContent && children}
    </>
  );
}
