"use client";

import { useState } from "react";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { CheckIcon, PencilIcon, PlusIcon, Trash2Icon, XIcon } from "lucide-react";
import { useTranslations } from "next-intl";

import { permissions } from "@/auth/permissions.gen";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { Switch } from "@/components/ui/switch";
import { createInput, deleteInput, listInputsTyped, updateInput } from "@/spx/api/ingest/inputs";
import { listMappingsTyped } from "@/spx/api/ingest/mappings";
import type { ApiInput, ApiMapping } from "@/spx/api/ingest/schemas";

import { Can, useCan } from "../components/access";
import { ErrorState } from "../components/error-state";
import { PageHeader } from "../components/page-header";

const protocols = ["http", "syslog", "kafka"] as const;

// IngestPage 是数据接入点的管理页：列出接入点并允许新建与删除。
//
// 令牌只在新建立后显示一次：服务端仅保存摘要，页面刷新后再也取不回，
// 因此提示必须明确，不能让用户以为随时能查。
export function IngestPage() {
  const t = useTranslations("Ingest");
  const queryClient = useQueryClient();

  const [name, setName] = useState("");
  const [protocol, setProtocol] = useState<(typeof protocols)[number]>("http");
  const [port, setPort] = useState("");
  const [token, setToken] = useState("");
  const [editing, setEditing] = useState("");
  const [draftName, setDraftName] = useState("");

  const canWrite = useCan(permissions.IngestInputWrite);

  const inputs = useQuery({ queryKey: ["ingest", "inputs"], queryFn: listInputsTyped });
  const mappings = useQuery({ queryKey: ["ingest", "mappings"], queryFn: listMappingsTyped });

  const create = useMutation({
    mutationFn: createInput,
    onSuccess: async (result) => {
      setToken(result.token ?? "");
      setName("");
      setPort("");
      await queryClient.invalidateQueries({ queryKey: ["ingest", "inputs"] });
    },
  });

  const update = useMutation({
    mutationFn: ({ id, body }: { id: string; body: Parameters<typeof updateInput>[1] }) => updateInput(id, body),
    onSuccess: async () => {
      setEditing("");
      await queryClient.invalidateQueries({ queryKey: ["ingest", "inputs"] });
    },
  });

  const remove = useMutation({
    mutationFn: deleteInput,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["ingest", "inputs"] });
    },
  });

  return (
    <div className="flex flex-col gap-4 md:gap-6">
      <PageHeader
        title={t("title")}
        actions={
          <Can action={permissions.IngestInputWrite}>
            <Button
              size="sm"
              disabled={name.trim() === "" || create.isPending}
              onClick={() => {
                create.mutate({
                  name: name.trim(),
                  protocol,
                  ...(protocol === "syslog" && port !== "" ? { syslog_port: Number(port) } : {}),
                });
              }}
            >
              <PlusIcon /> {t("create")}
            </Button>
          </Can>
        }
      />

      <Can action={permissions.IngestInputWrite}>
        <div className="flex flex-wrap items-center gap-2">
          <Input
            className="max-w-xs"
            placeholder={t("namePlaceholder")}
            value={name}
            onChange={(event) => setName(event.target.value)}
          />
          <select
            aria-label={t("protocol")}
            className="h-9 rounded-md border border-input bg-transparent px-3 text-sm"
            value={protocol}
            onChange={(event) => setProtocol(event.target.value as (typeof protocols)[number])}
          >
            {protocols.map((option) => (
              <option key={option} value={option}>
                {option}
              </option>
            ))}
          </select>
          {protocol === "syslog" && (
            <Input
              className="max-w-32"
              inputMode="numeric"
              placeholder={t("port")}
              value={port}
              onChange={(event) => setPort(event.target.value.replace(/\D/g, ""))}
            />
          )}
        </div>
      </Can>

      {token !== "" && (
        <div className="rounded-md border border-amber-500/40 bg-amber-500/10 p-3 text-sm">
          <p className="font-medium">{t("tokenOnce")}</p>
          <code className="mt-1 block break-all font-mono text-xs">{token}</code>
        </div>
      )}

      {inputs.isPending && <Skeleton className="h-24 w-full" />}
      {inputs.isError && <ErrorState title={t("loadFailed")} description={t("loadFailedDescription")} />}

      {inputs.data && (
        <div className="overflow-hidden rounded-md border">
          <table className="w-full text-sm">
            <thead className="bg-muted/40 text-left">
              <tr>
                <th className="p-3 font-medium">{t("name")}</th>
                <th className="p-3 font-medium">{t("protocol")}</th>
                <th className="p-3 font-medium">{t("endpoint")}</th>
                <th className="p-3 font-medium">{t("mapping")}</th>
                <th className="p-3 font-medium">{t("status")}</th>
                <th className="p-3" />
              </tr>
            </thead>
            <tbody>
              {inputs.data.map((item) => (
                <tr key={item.id} className="border-t">
                  <td className="p-3">{item.name}</td>
                  <td className="p-3 font-mono text-xs">{item.protocol}</td>
                  <td className="p-3 font-mono text-xs">{endpointOf(item, t("none"))}</td>
                  <td className="p-3">
                    <Can action={permissions.IngestInputWrite}>
                      <select
                        aria-label={t("mapping")}
                        className="h-8 max-w-44 rounded-md border border-input bg-transparent px-2 text-xs"
                        value={item.mapping_id ?? ""}
                        disabled={update.isPending || mappings.isPending}
                        onChange={(event) =>
                          update.mutate({
                            id: item.id,
                            body: { mapping_id: event.target.value === "" ? null : event.target.value },
                          })
                        }
                      >
                        <option value="">{t("mappingNone")}</option>
                        {(mappings.data ?? []).map((mapping) => (
                          <option key={mapping.id} value={mapping.id}>
                            {mapping.name} (v{mapping.version})
                          </option>
                        ))}
                      </select>
                    </Can>
                    {!canWrite && (
                      <span className="font-mono text-xs">{mappingNameOf(item, mappings.data, t("mappingNone"))}</span>
                    )}
                  </td>
                  <td className="p-3">
                    <Badge variant={item.enabled ? "default" : "secondary"}>
                      {item.enabled ? t("enabled") : t("disabled")}
                    </Badge>
                  </td>
                  <td className="p-3 text-right">
                    <div className="flex items-center justify-end gap-1">
                      <Can action={permissions.IngestInputWrite}>
                        {editing === item.id ? (
                          <>
                            <Input
                              className="h-8 w-40"
                              aria-label={t("rename")}
                              value={draftName}
                              onChange={(event) => setDraftName(event.target.value)}
                            />
                            <Button
                              size="icon"
                              variant="ghost"
                              aria-label={t("save")}
                              disabled={draftName.trim() === "" || update.isPending}
                              onClick={() => update.mutate({ id: item.id, body: { name: draftName.trim() } })}
                            >
                              <CheckIcon />
                            </Button>
                            <Button size="icon" variant="ghost" aria-label={t("cancel")} onClick={() => setEditing("")}>
                              <XIcon />
                            </Button>
                          </>
                        ) : (
                          <>
                            <Switch
                              aria-label={t("toggle")}
                              checked={item.enabled}
                              disabled={update.isPending}
                              onCheckedChange={(next) => update.mutate({ id: item.id, body: { enabled: next } })}
                            />
                            <Button
                              size="icon"
                              variant="ghost"
                              aria-label={t("rename")}
                              onClick={() => {
                                setEditing(item.id);
                                setDraftName(item.name);
                              }}
                            >
                              <PencilIcon />
                            </Button>
                            <Button
                              size="icon"
                              variant="ghost"
                              aria-label={t("delete")}
                              disabled={remove.isPending}
                              onClick={() => remove.mutate(item.id)}
                            >
                              <Trash2Icon />
                            </Button>
                          </>
                        )}
                      </Can>
                    </div>
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

// mappingNameOf 把 mapping_id 显示成名字；未绑定或列表尚未加载时显示占位。
function mappingNameOf(item: ApiInput, mappings: ApiMapping[] | undefined, none: string): string {
  if (item.mapping_id === null) return none;

  const matched = (mappings ?? []).find((mapping) => mapping.id === item.mapping_id);

  return matched ? matched.name : item.mapping_id;
}

// endpointOf 展示接入点的实际落点：端口或 topic，而不是抽象描述。
function endpointOf(item: ApiInput, none: string): string {
  if (item.protocol === "syslog" && item.syslog_port !== null) return `udp/tcp:${String(item.syslog_port)}`;
  if (item.protocol === "kafka" && item.kafka_topic !== null) return item.kafka_topic;
  if (item.protocol === "http") return "bearer token";

  return none;
}
