"use client";
import { type ReactNode, useCallback, useState } from "react";

import { type QueryKey, useQuery } from "@tanstack/react-query";
import { type ColumnDef, type PaginationState, useTable } from "@tanstack/react-table";
import { ChevronLeft, ChevronRight, ChevronsLeft, ChevronsRight } from "lucide-react";
import { useTranslations } from "next-intl";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Empty, EmptyDescription, EmptyHeader, EmptyTitle } from "@/components/ui/empty";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Spinner } from "@/components/ui/spinner";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { type DataTableFeatures, dataTableFeatures } from "@/lib/data-table-features";

import { identityScope } from "../iam/query";
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
  const [pagination, setPagination] = useState<PaginationState>({ pageIndex: 0, pageSize: 10 });
  const query = useQuery({
    queryKey: [...key, ...identityScope(auth), pagination.pageIndex, pagination.pageSize],
    enabled: auth.status === "authenticated",
    queryFn: ({ signal }) =>
      fetchPage({ limit: pagination.pageSize, offset: pagination.pageIndex * pagination.pageSize }, signal),
  });
  return {
    items: query.data?.items ?? [],
    total: query.data?.total ?? 0,
    loading: query.isFetching,
    error: query.error ? errorMessageKey(query.error) : ("" as const),
    pagination,
    setPagination,
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
    state: { pagination: state.pagination },
    onPaginationChange: state.setPagination,
  });
  return (
    <div className="flex min-w-0 flex-col gap-4">
      {state.error && <p role="alert">{errors(state.error)}</p>}
      <div className="overflow-hidden border-y">
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
      <div className="flex items-center justify-between gap-6">
        <p className="text-muted-foreground text-sm whitespace-nowrap">
          {t("pagination", { total: state.total, page: state.pagination.pageIndex + 1 })}
        </p>
        <div className="flex items-center gap-6">
          <div className="flex items-center gap-2">
            <Label className="whitespace-nowrap">{t("rowsPerPage")}</Label>
            <Select
              value={String(state.pagination.pageSize)}
              onValueChange={(value) => state.setPagination({ pageIndex: 0, pageSize: Number(value) })}
            >
              <SelectTrigger size="sm" className="w-20" aria-label={t("rowsPerPage")}>
                <SelectValue />
              </SelectTrigger>
              <SelectContent side="top">
                <SelectGroup>
                  {[10, 20, 30, 40, 50].map((size) => (
                    <SelectItem key={size} value={String(size)}>
                      {size}
                    </SelectItem>
                  ))}
                </SelectGroup>
              </SelectContent>
            </Select>
          </div>
          <p className="text-sm whitespace-nowrap tabular-nums">
            {t("pageInfo", { page: state.pagination.pageIndex + 1, total: Math.max(1, table.getPageCount()) })}
          </p>
          <div className="flex items-center gap-2">
            <Button
              variant="outline"
              size="icon-sm"
              aria-label={t("firstPage")}
              disabled={!table.getCanPreviousPage() || state.loading}
              onClick={() => table.setPageIndex(0)}
            >
              <ChevronsLeft />
            </Button>
            <Button
              variant="outline"
              size="icon-sm"
              aria-label={t("previousPage")}
              disabled={!table.getCanPreviousPage() || state.loading}
              onClick={() => table.previousPage()}
            >
              <ChevronLeft />
            </Button>
            <Button
              variant="outline"
              size="icon-sm"
              aria-label={t("nextPage")}
              disabled={!table.getCanNextPage() || state.loading}
              onClick={() => table.nextPage()}
            >
              <ChevronRight />
            </Button>
            <Button
              variant="outline"
              size="icon-sm"
              aria-label={t("lastPage")}
              disabled={!table.getCanNextPage() || state.loading}
              onClick={() => table.setPageIndex(table.getPageCount() - 1)}
            >
              <ChevronsRight />
            </Button>
          </div>
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
