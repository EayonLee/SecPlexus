"use client";

import { useState } from "react";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { PlusIcon, Trash2Icon } from "lucide-react";
import { useTranslations } from "next-intl";

import { permissions } from "@/auth/permissions.gen";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import {
  createMapping,
  deleteMapping,
  listMappingsTyped,
  updateMapping,
  validateMapping,
} from "@/spx/api/ingest/mappings";
import type { SampleResult } from "@/spx/api/ingest/schemas";

import { Can, useCan } from "../components/access";
import { ErrorState } from "../components/error-state";
import { PageHeader } from "../components/page-header";

// 最小可用规则模板：新建时给一份能编译通过的骨架，避免用户从空白开始。
const template = {
  id: "custom",
  name: "custom",
  parse: { mode: "json" },
  field_map: { src: "src_endpoint.ip" },
  constants: { class_uid: 2004 },
};

// MappingsPage 是解析映射的管理页：列出映射并在线编辑规则 JSON。
//
// 规则内容以 JSON 文本编辑而不是可视化字段表：映射的表达力取决于规则本身，
// 文本框加服务端编译校验已经足够，且与 API 契约一一对应。
export function MappingsPage() {
  const t = useTranslations("Mappings");
  const queryClient = useQueryClient();
  const canWrite = useCan(permissions.IngestMappingWrite);

  const [name, setName] = useState("");
  const [definition, setDefinition] = useState(() => JSON.stringify(template, null, 2));
  const [editingID, setEditingID] = useState("");
  const [error, setError] = useState("");
  const [samples, setSamples] = useState("");
  const [result, setResult] = useState<SampleResult | null>(null);

  const mappings = useQuery({ queryKey: ["ingest", "mappings"], queryFn: listMappingsTyped });

  const save = useMutation({
    mutationFn: async () => {
      let parsed: unknown;
      try {
        parsed = JSON.parse(definition);
      } catch {
        throw new Error(t("invalidJSON"));
      }

      const body = { name: name.trim(), spec: parsed as Record<string, unknown> };

      if (editingID === "") return createMapping(body);

      return updateMapping(editingID, body);
    },
    onSuccess: async () => {
      setError("");
      setName("");
      setEditingID("");
      setDefinition(JSON.stringify(template, null, 2));
      await queryClient.invalidateQueries({ queryKey: ["ingest", "mappings"] });
    },
    onError: (failure: Error) => setError(failure.message),
  });

  const verify = useMutation({
    mutationFn: async () => {
      let parsed: unknown;
      try {
        parsed = JSON.parse(definition);
      } catch {
        throw new Error(t("invalidJSON"));
      }

      const lines = samples
        .split("\n")
        .map((line) => line.trim())
        .filter((line) => line !== "");

      if (lines.length === 0) throw new Error(t("samplesRequired"));

      return validateMapping(parsed, lines);
    },
    onSuccess: (next) => {
      setError("");
      setResult(next);
    },
    onError: (failure: Error) => {
      setResult(null);
      setError(failure.message);
    },
  });

  const remove = useMutation({
    mutationFn: deleteMapping,
    onSuccess: async () => {
      setError("");
      await queryClient.invalidateQueries({ queryKey: ["ingest", "mappings"] });
    },
    onError: (failure: Error) => setError(failure.message),
  });

  return (
    <div className="flex flex-col gap-4 md:gap-6">
      <PageHeader title={t("title")} />

      <Can action={permissions.IngestMappingWrite}>
        <div className="flex flex-col gap-2">
          <div className="flex flex-wrap items-center gap-2">
            <Input
              className="max-w-xs"
              placeholder={t("namePlaceholder")}
              value={name}
              onChange={(event) => setName(event.target.value)}
            />
            <Button size="sm" disabled={name.trim() === "" || save.isPending} onClick={() => save.mutate()}>
              <PlusIcon /> {editingID === "" ? t("create") : t("save")}
            </Button>
            {editingID !== "" && (
              <Button
                size="sm"
                variant="outline"
                onClick={() => {
                  setEditingID("");
                  setName("");
                  setDefinition(JSON.stringify(template, null, 2));
                }}
              >
                {t("cancel")}
              </Button>
            )}
          </div>
          <textarea
            aria-label={t("samples")}
            className="min-h-24 w-full rounded-md border border-input bg-transparent p-3 font-mono text-xs"
            placeholder={t("samplesPlaceholder")}
            spellCheck={false}
            value={samples}
            onChange={(event) => setSamples(event.target.value)}
          />
          <Button size="sm" variant="outline" disabled={verify.isPending} onClick={() => verify.mutate()}>
            {t("verify")}
          </Button>
          <textarea
            aria-label={t("definition")}
            className="min-h-64 w-full rounded-md border border-input bg-transparent p-3 font-mono text-xs"
            spellCheck={false}
            value={definition}
            onChange={(event) => setDefinition(event.target.value)}
          />
        </div>
      </Can>

      {error !== "" && <p className="text-sm text-destructive">{error}</p>}

      {result && (
        <div className="flex flex-col gap-1 rounded-md border p-3 text-sm">
          {result.compiled ? (
            <p>{t("matchedOf", { matched: result.matched, total: result.total })}</p>
          ) : (
            <p className="text-destructive">{result.compile_error}</p>
          )}
          {(result.failures ?? []).map((failure) => (
            <p key={failure.index} className="font-mono text-xs text-destructive">
              #{failure.index + 1} {failure.reason}
            </p>
          ))}
        </div>
      )}

      {mappings.isPending && <Skeleton className="h-24 w-full" />}
      {mappings.isError && <ErrorState title={t("loadFailed")} description={t("loadFailedDescription")} />}

      {mappings.data && (
        <div className="overflow-hidden rounded-md border">
          <table className="w-full text-sm">
            <thead className="bg-muted/40 text-left">
              <tr>
                <th className="p-3 font-medium">{t("name")}</th>
                <th className="p-3 font-medium">{t("version")}</th>
                <th className="p-3" />
              </tr>
            </thead>
            <tbody>
              {mappings.data.map((mapping) => (
                <tr key={mapping.id} className="border-t">
                  <td className="p-3">{mapping.name}</td>
                  <td className="p-3">
                    <Badge variant="secondary">v{mapping.version}</Badge>
                  </td>
                  <td className="p-3 text-right">
                    {canWrite && (
                      <div className="flex items-center justify-end gap-1">
                        <Button
                          size="sm"
                          variant="ghost"
                          onClick={() => {
                            setEditingID(mapping.id);
                            setName(mapping.name);
                          }}
                        >
                          {t("edit")}
                        </Button>
                        <Button
                          size="icon"
                          variant="ghost"
                          aria-label={t("delete")}
                          disabled={remove.isPending}
                          onClick={() => remove.mutate(mapping.id)}
                        >
                          <Trash2Icon />
                        </Button>
                      </div>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}

// 说明：编辑时规则内容需要重新填写 JSON。列表接口不返回 definition，
// 这是刻意的——列表不该为编辑页承担整份规则的传输成本。
