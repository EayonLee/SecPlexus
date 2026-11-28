"use client";
import { type ReactNode, useCallback, useState } from "react";

import { type QueryKey, useQuery } from "@tanstack/react-query";
import { type ColumnDef, type PaginationState, useTable } from "@tanstack/react-table";
import { cn } from "cn";
import { useTranslations } from "next-intl";
import { toast } from "sonner";

import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { type DataTableFeatures, dataTableFeatures } from "@/lib/data-table-features";

import { APIError } from "../api/client";
import { sessionScope } from "../iam/query";
import { errorMessageKey, useIdentity } from "../iam/session";
import { CollectionBoundary, CollectionState } from "./collection-state";
import { ListPagination } from "./list-controls";
import { requestReauth } from "./reauth";

// 全局约定：所有列（含操作列）一律左对齐，操作按钮不加 justify-end，
// 避免同一张表里出现两种对齐轴。
export interface PageData<T> {
  items: T[];
  total: number;
}
/** usePagedQuery 仅组合受控页码和会话作用域；服务端状态完全交给 Query。 */
export function usePagedQuery<T>(
  key: QueryKey,
  fetchPage: (pagination: { limit: number; offset: number }, signal: AbortSignal) => Promise<PageData<T>>,
) {
  const auth = useIdentity();
  const [pagination, setPagination] = useState<PaginationState>({ pageIndex: 0, pageSize: 10 });
  const query = useQuery({
    queryKey: [...key, ...sessionScope(auth), pagination.pageIndex, pagination.pageSize],
    enabled: auth.status === "authenticated",
    queryFn: ({ signal }) =>
      fetchPage({ limit: pagination.pageSize, offset: pagination.pageIndex * pagination.pageSize }, signal),
  });
  return {
    query,
    items: query.data?.items ?? [],
    total: query.data?.total ?? 0,
    loading: query.isFetching,
    pagination,
    setPagination,
  };
}
export function DataTable<T extends { id: string }>({
  state,
  columns,
  onClear,
  groupBy,
  card,
  onPageChange,
  isSelected,
}: {
  onPageChange?: () => void;
  isSelected?: (row: T) => boolean;
  card?: (row: T) => ReactNode;
  onClear?: () => void;
  groupBy?: (row: T) => string;
  state: ReturnType<typeof usePagedQuery<T>>;
  columns: { title: string; header?: ReactNode; className?: string; cell: (row: T) => ReactNode }[];
}) {
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
  const rows = table.getRowModel().rows;
  const groups = new Map<string, typeof rows>();
  for (const row of rows) {
    const label = groupBy ? groupBy(row.original) : "";
    const group = groups.get(label);
    if (group) group.push(row);
    else groups.set(label, [row]);
  }

  return (
    <CollectionBoundary query={state.query} hasContent={state.items.length > 0}>
      <div className="flex min-w-0 flex-col">
        {card && rows.length ? (
          <div className="grid gap-3 border-y p-4 @sm/list:grid-cols-2 @3xl/list:grid-cols-3">
            {rows.map((row) => (
              <div
                key={row.id}
                data-state={isSelected?.(row.original) ? "selected" : undefined}
                className="border-border data-[state=selected]:bg-primary/10 rounded-xl border p-4"
              >
                {card(row.original)}
              </div>
            ))}
          </div>
        ) : (
          <div className="border-y">
            <Table className={groupBy ? "w-full border-collapse @lg/list:table-fixed" : undefined}>
              <TableHeader>
                <TableRow className={groupBy ? "[&>:not(:last-child)]:border-r" : undefined}>
                  {columns.map((column) => (
                    <TableHead
                      key={column.title}
                      className={cn(groupBy ? "px-4 text-sm" : "h-[53px] px-4 font-normal", column.className)}
                    >
                      {column.header ?? column.title}
                    </TableHead>
                  ))}
                </TableRow>
              </TableHeader>
              {rows.length ? (
                [...groups].map(([label, group]) => (
                  <TableBody key={label} aria-label={label || undefined}>
                    {label && (
                      <TableRow className="bg-muted h-10">
                        <TableHead scope="rowgroup" colSpan={columns.length} className="text-muted-foreground px-4">
                          {label}
                          <span className="text-muted-foreground ml-2 rounded-sm border px-2 text-xs tabular-nums">
                            {group.length}
                          </span>
                        </TableHead>
                      </TableRow>
                    )}
                    {group.map((row) => (
                      <TableRow
                        key={row.id}
                        data-state={isSelected?.(row.original) ? "selected" : undefined}
                        className={
                          groupBy
                            ? "h-12 hover:bg-muted/20 [&>:not(:last-child)]:border-r"
                            : "border-border/60 hover:bg-muted/20"
                        }
                      >
                        {columns.map((column) => (
                          <TableCell
                            key={column.title}
                            className={cn("px-4", groupBy ? "py-2" : "py-4", column.className)}
                          >
                            {column.cell(row.original)}
                          </TableCell>
                        ))}
                      </TableRow>
                    ))}
                  </TableBody>
                ))
              ) : (
                <TableBody>
                  <TableRow>
                    <TableCell colSpan={columns.length} className="p-0">
                      {onClear ? (
                        <CollectionState kind="filtered" onClear={onClear} />
                      ) : (
                        <CollectionState kind="empty" />
                      )}
                    </TableCell>
                  </TableRow>
                </TableBody>
              )}
            </Table>
          </div>
        )}
        <ListPagination
          hasNext={table.getCanNextPage()}
          index={state.pagination.pageIndex}
          size={state.pagination.pageSize}
          total={state.total}
          loading={state.loading}
          onPage={(index) => {
            onPageChange?.();
            table.setPageIndex(index);
          }}
          onSize={(pageSize) => {
            onPageChange?.();
            state.setPagination({ pageIndex: 0, pageSize });
          }}
        />
      </div>
    </CollectionBoundary>
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
        // 高风险操作可能要求近期密码确认：确认成功后重放一次，用户不必重填表单。
        if (error instanceof APIError && error.code === "iam.recent_auth_required") {
          if (!(await requestReauth())) return false;

          try {
            await action();
            if (success) toast.success(success);
            return true;
          } catch (retryError) {
            if (!(retryError instanceof DOMException && retryError.name === "AbortError"))
              toast.error(errors(errorMessageKey(retryError)));
            return false;
          }
        }

        if (!(error instanceof DOMException && error.name === "AbortError"))
          toast.error(errors(errorMessageKey(error)));
        return false;
      }
    },
    [errors],
  );
}
