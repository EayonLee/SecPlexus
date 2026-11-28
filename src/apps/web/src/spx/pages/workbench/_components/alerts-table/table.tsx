"use client";
import * as React from "react";

import {
  type ColumnFiltersState,
  type ColumnVisibilityState,
  type PaginationState,
  useTable,
} from "@tanstack/react-table";
import { ChevronDownIcon, Download, Settings2 } from "lucide-react";
import { useTranslations } from "next-intl";

import { Button } from "@/components/ui/button";
import { CardAction, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { dataTableFeatures } from "@/lib/data-table-features";
import { CollectionState } from "@/spx/components/collection-state";
import { ListPagination } from "@/spx/components/list-controls";
import { ListSurface } from "@/spx/components/toolbar";

import { useRecentLeadsColumns } from "./columns";
import type { RecentLeadRow } from "./schema";

export function AlertsTable({ data }: { data: RecentLeadRow[] }) {
  const t = useTranslations("Dashboard");
  const common = useTranslations("Common");

  const recentLeadsColumns = useRecentLeadsColumns();
  const [rowSelection, setRowSelection] = React.useState({});
  const [columnVisibility, setColumnVisibility] = React.useState<ColumnVisibilityState>({});
  const [columnFilters, setColumnFilters] = React.useState<ColumnFiltersState>([]);
  const [pagination, setPagination] = React.useState<PaginationState>({
    pageIndex: 0,
    pageSize: 10,
  });

  const table = useTable({
    features: dataTableFeatures,
    data,
    columns: recentLeadsColumns,
    state: {
      rowSelection,
      columnVisibility,
      columnFilters,
      pagination,
    },
    getRowId: (row) => row.id,
    enableRowSelection: true,
    onRowSelectionChange: setRowSelection,
    onColumnVisibilityChange: setColumnVisibility,
    onColumnFiltersChange: setColumnFilters,
    onPaginationChange: setPagination,
  });

  const selectedCount = table.getFilteredSelectedRowModel().rows.length;
  return (
    <ListSurface>
      <CardHeader className="border-b py-4">
        <CardTitle>{t("recentLeads")}</CardTitle>
        <CardAction>
          <div className="flex flex-wrap items-center gap-2">
            {selectedCount > 0 && (
              <span role="status" className="text-muted-foreground text-sm tabular-nums">
                {common("selectedCount", { count: selectedCount })}
              </span>
            )}
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button variant="outline" size="sm">
                  <Settings2 data-icon="inline-start" />
                  {t("columns")}
                  <ChevronDownIcon data-icon="inline-end" />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end" className="w-36">
                <DropdownMenuLabel>{t("toggleColumns")}</DropdownMenuLabel>
                <DropdownMenuSeparator />
                <DropdownMenuGroup>
                  {table.getAllColumns().map((column) => {
                    const { header } = column.columnDef;
                    if (!column.getCanHide() || typeof header !== "string") return null;
                    return (
                      <DropdownMenuCheckboxItem
                        key={column.id}
                        checked={column.getIsVisible()}
                        onCheckedChange={(value) => column.toggleVisibility(!!value)}
                      >
                        {header}
                      </DropdownMenuCheckboxItem>
                    );
                  })}
                </DropdownMenuGroup>
              </DropdownMenuContent>
            </DropdownMenu>
            <Button variant="outline" size="sm">
              <Download data-icon="inline-start" />
              <span className="hidden lg:inline">{t("export")}</span>
            </Button>
          </div>
        </CardAction>
      </CardHeader>
      <CardContent className="px-0">
        <Table>
          <TableHeader>
            {table.getHeaderGroups().map((headerGroup) => (
              <TableRow key={headerGroup.id}>
                {headerGroup.headers.map((header) => (
                  <TableHead key={header.id} colSpan={header.colSpan}>
                    {header.isPlaceholder ? null : <table.FlexRender header={header} />}
                  </TableHead>
                ))}
              </TableRow>
            ))}
          </TableHeader>
          <TableBody className="**:data-[slot=table-cell]:first:w-8">
            {table.getRowModel().rows.length ? (
              table.getRowModel().rows.map((row) => (
                <TableRow key={row.id} data-state={table.state.rowSelection[row.id] && "selected"}>
                  {row.getVisibleCells().map((cell) => (
                    <TableCell key={cell.id}>
                      <table.FlexRender cell={cell} />
                    </TableCell>
                  ))}
                </TableRow>
              ))
            ) : (
              <TableRow>
                <TableCell colSpan={table.getVisibleLeafColumns().length} className="p-0">
                  {columnFilters.length ? (
                    <CollectionState kind="filtered" onClear={() => setColumnFilters([])} />
                  ) : (
                    <CollectionState kind="empty" title={t("noResults")} />
                  )}
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
        <div className="border-t">
          <ListPagination
            index={table.state.pagination.pageIndex}
            size={table.state.pagination.pageSize}
            total={table.getFilteredRowModel().rows.length}
            hasNext={table.getCanNextPage()}
            onPage={(index) => table.setPageIndex(index)}
            onSize={(size) => {
              table.setPageSize(size);
              table.setPageIndex(0);
            }}
          />
        </div>
      </CardContent>
    </ListSurface>
  );
}
