"use client";
import { type ReactNode, useCallback, useEffect, useState } from "react";

import { useTranslations } from "next-intl";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Empty, EmptyDescription, EmptyHeader, EmptyTitle } from "@/components/ui/empty";
import { Spinner } from "@/components/ui/spinner";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";

import { api, errorMessageKey, useIdentity } from "../iam/session";

export interface PageData<T> {
  items: T[];
  total: number;
}
export function useList<T>(url: string) {
  const auth = useIdentity();
  const version = auth.organizations.find((org) => org.id === auth.org_id)?.authz_version;
  const [page, setPage] = useState(1);
  const [revision, setRevision] = useState(0);
  const [data, setData] = useState<PageData<T>>({ items: [], total: 0 });
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<ReturnType<typeof errorMessageKey> | "">("");
  // biome-ignore lint/correctness/useExhaustiveDependencies: 授权版本和手动刷新都会使列表快照失效。
  useEffect(() => {
    const controller = new AbortController();
    setLoading(true);
    setError("");
    setData({ items: [], total: 0 });
    const separator = url.includes("?") ? "&" : "?";
    void api<PageData<T>>(`${url}${separator}limit=20&offset=${(page - 1) * 20}`, { signal: controller.signal })
      .then((result) => {
        if (!controller.signal.aborted) setData(result);
      })
      .catch((failure: unknown) => {
        if (!controller.signal.aborted) setError(errorMessageKey(failure));
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => controller.abort();
  }, [url, page, version, revision]);
  const reload = useCallback(() => setRevision((value) => value + 1), []);
  return { ...data, loading, error, page, setPage, reload };
}
export function DataTable<T extends { id: string }>({
  state,
  columns,
}: {
  state: ReturnType<typeof useList<T>>;
  columns: { title: string; cell: (row: T) => ReactNode }[];
}) {
  const t = useTranslations("Common");
  const errors = useTranslations("Errors");

  return (
    <div className="flex min-w-0 flex-col gap-4">
      {state.error && <p role="alert">{errors(state.error)}</p>}
      <div className="overflow-hidden rounded-lg border">
        <Table>
          <TableHeader>
            <TableRow>
              {columns.map((column) => (
                <TableHead key={column.title}>{column.title}</TableHead>
              ))}
            </TableRow>
          </TableHeader>
          <TableBody>
            {state.loading ? (
              <TableRow>
                <TableCell colSpan={columns.length}>
                  <Spinner />
                </TableCell>
              </TableRow>
            ) : state.items.length ? (
              state.items.map((row) => (
                <TableRow key={row.id}>
                  {columns.map((column) => (
                    <TableCell key={column.title}>{column.cell(row)}</TableCell>
                  ))}
                </TableRow>
              ))
            ) : (
              <TableRow>
                <TableCell colSpan={columns.length}>
                  <Empty>
                    <EmptyHeader>
                      <EmptyTitle>{t("empty")}</EmptyTitle>
                      <EmptyDescription>{t("emptyDescription")}</EmptyDescription>
                    </EmptyHeader>
                  </Empty>
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
      </div>
      <div className="flex flex-wrap items-center justify-between gap-2">
        <p className="text-muted-foreground text-sm">{t("pagination", { total: state.total, page: state.page })}</p>
        <div className="flex gap-2">
          <Button
            variant="outline"
            disabled={state.page <= 1 || state.loading}
            onClick={() => state.setPage(state.page - 1)}
          >
            {t("previousPage")}
          </Button>
          <Button
            variant="outline"
            disabled={state.page * 20 >= state.total || state.loading}
            onClick={() => state.setPage(state.page + 1)}
          >
            {t("nextPage")}
          </Button>
        </div>
      </div>
    </div>
  );
}
export function useAction() {
  const errors = useTranslations("Errors");
  return useCallback(
    async (action: () => Promise<unknown>, success?: string) => {
      try {
        await action();
        if (success) toast.success(success);
        return true;
      } catch (error) {
        if (!(error instanceof DOMException && error.name === "AbortError"))
          toast.error(errors(errorMessageKey(error)));
        return false;
      }
    },
    [errors],
  );
}
