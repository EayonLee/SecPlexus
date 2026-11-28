"use client";
import Link from "next/link";
import { redirect, useSearchParams } from "next/navigation";

import { PlusIcon } from "lucide-react";
import { useTranslations } from "next-intl";

import { permissions } from "@/auth/permissions.gen";
import { Button } from "@/components/ui/button";

import { AccessDenied, useCan } from "../components/access";
import { PageHeader } from "../components/page-header";
import { ListSurface } from "../components/toolbar";
import { visibleIngestPages } from "../iam/navigation";
import { useIdentity } from "../iam/session";
import { IngestPage } from "./ingest";
import { MappingsPage } from "./mappings";

export function IngestWorkspace({ section, id }: { section?: string; id?: string }) {
  const params = useSearchParams();
  const labels = useTranslations();
  const pages = visibleIngestPages(useIdentity());
  const t = useTranslations("Navigation");
  const canCreate = useCan(section === "parsing-rules" ? permissions.IngestParsingRuleWrite : permissions.IngestInputWrite);
  if (!section && pages[0]) redirect(pages[0].path);
  const active = pages.find((page) => page.path === `/ingest/${section}`);
  if (!active || (id === "new" && !canCreate)) return <AccessDenied />;
  const content = section === "inputs" ? <IngestPage key={id ?? "list"} id={id} /> : <MappingsPage id={id} />;
  if (id) return content;
  return (
    <div className="flex min-w-0 flex-col gap-4">
      <PageHeader
        title={t(active.title)}
        actions={
          canCreate && (
            <Button size="sm" asChild>
              <Link href={`${active.path}/new${params.size ? `?${params}` : ""}`}>
                <PlusIcon data-icon="inline-start" />
                {section === "inputs" ? labels("Ingest.createInput") : labels("Mappings.createMapping")}
              </Link>
            </Button>
          )
        }
      />
      {section === "inputs" ? <ListSurface>{content}</ListSurface> : content}
    </div>
  );
}
