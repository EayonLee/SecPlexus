"use client";
import { type ReactNode, useCallback, useState } from "react";

import { type QueryKey, useQuery } from "@tanstack/react-query";
import { type ColumnDef, useTable } from "@tanstack/react-table";
import { useTranslations } from "next-intl";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Empty, EmptyDescription, EmptyHeader, EmptyTitle } from "@/components/ui/empty";
import { Spinner } from "@/components/ui/spinner";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { type DataTableFeatures, dataTableFeatures } from "@/lib/data-table-features";

import { errorMessageKey, useIdentity } from "../iam/session";

export interface PageData<T> {
  items: T[];
  total: number;
}
/** usePagedQuery 仅组合受控页码和身份作用域；服务端状态完全交给 Query。 */
export function usePagedQuery<T>(
  key: QueryKey,
  fetchPage: (pagination: { limit: number; offset: number }, signal: AbortSignal) => Promise<PageData<T>>,
) {
  const auth = useIdentity();
  const version = auth.organizations.find((org) => org.id === auth.org_id)?.authz_version;
  const [page, setPage] = useState(1);
  const query = useQuery({
    queryKey: [...key, auth.principal?.session_id, auth.principal?.auth_version, auth.org_id, version, page],
    enabled: auth.status === "authenticated",
    queryFn: ({ signal }) => fetchPage({ limit: 20, offset: (page - 1) * 20 }, signal),
  });
  return {
    items: query.data?.items ?? [],
    total: query.data?.total ?? 0,
    loading: query.isFetching,
    error: query.error ? errorMessageKey(query.error) : ("" as const),
    page,
    setPage,
    reload: () => {
      void query.refetch();
    },
  };
}
export function DataTable<T extends { id: string }>({
  state,
  columns,
}: {
  state: ReturnType<typeof usePagedQuery<T>>;
  columns: { title: string; cell: (row: T) => ReactNode }[];
}) {
  const t = useTranslations("Common");
  const errors = useTranslations("Errors");

  const tableColumns: ColumnDef<DataTableFeatures, T>[] = columns.map((column) => ({
    id: column.title,
    header: column.title,
    cell: ({ row }) => column.cell(row.original),
  }));
  const table = useTable({
    features: dataTableFeatures,
    data: state.items,
    columns: tableColumns,
    getRowId: (row) => row.id,
    manualPagination: true,
    rowCount: state.total,
    state: { pagination: { pageIndex: state.page - 1, pageSize: 20 } },
    onPaginationChange: (updater) => {
      const current = { pageIndex: state.page - 1, pageSize: 20 };
      const next = typeof updater === "function" ? updater(current) : updater;
      state.setPage(next.pageIndex + 1);
    },
  });
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
              table.getRowModel().rows.map((row) => (
                <TableRow key={row.id}>
                  {columns.map((column) => (
                    <TableCell key={column.title}>{column.cell(row.original)}</TableCell>
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
          <Button variant="outline" disabled={state.page <= 1 || state.loading} onClick={() => table.previousPage()}>
            {t("previousPage")}
          </Button>
          <Button
            variant="outline"
            disabled={state.page * 20 >= state.total || state.loading}
            onClick={() => table.nextPage()}
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
