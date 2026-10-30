"use client";
import { useMemo } from "react";

import type { ColumnDef } from "@tanstack/react-table";
import { Subscribe } from "@tanstack/react-table";
import { EllipsisVertical } from "lucide-react";
import { useTranslations } from "next-intl";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import type { DataTableFeatures } from "@/lib/data-table-features";
import { StatusBadge } from "@/spx/components/status-badge";

import type { RecentLeadRow } from "./schema";

export function useRecentLeadsColumns(): ColumnDef<DataTableFeatures, RecentLeadRow>[] {
  const t = useTranslations("Dashboard");

  return useMemo<ColumnDef<DataTableFeatures, RecentLeadRow>[]>(
    () => [
      {
        id: "select",
        header: ({ table }) => (
          <div className="flex items-center justify-center">
            <Subscribe
              source={table.atoms.rowSelection}
              selector={() =>
                table.getIsAllPageRowsSelected() ||
                (table.getIsSomePageRowsSelected() && !table.getIsAllPageRowsSelected() && "indeterminate")
              }
            >
              {(checked) => (
                <Checkbox
                  checked={checked}
                  onCheckedChange={(value) => table.toggleAllPageRowsSelected(!!value)}
                  aria-label={t("selectAll")}
                />
              )}
            </Subscribe>
          </div>
        ),
        cell: ({ row }) => (
          <div className="flex items-center justify-center">
            <Subscribe source={row.table.atoms.rowSelection} selector={(selection) => Boolean(selection?.[row.id])}>
              {(checked) => (
                <Checkbox
                  checked={checked}
                  onCheckedChange={(value) => row.toggleSelected(!!value)}
                  aria-label={t("selectRow")}
                />
              )}
            </Subscribe>
          </div>
        ),
        enableHiding: false,
      },
      {
        accessorKey: "id",
        header: t("reference"),
        cell: ({ row }) => <span className="tabular-nums">{row.original.id}</span>,
        enableHiding: false,
      },
      {
        accessorKey: "name",
        header: t("name"),
        cell: ({ row }) => row.original.name,
        enableHiding: false,
      },
      {
        accessorKey: "company",
        header: t("company"),
        cell: ({ row }) => row.original.company,
      },
      {
        accessorKey: "status",
        header: t("status"),
        cell: ({ row }) => <StatusBadge status={row.original.statusKey}>{row.original.status}</StatusBadge>,
      },
      {
        accessorKey: "source",
        header: t("source"),
        cell: ({ row }) => <Badge variant="outline">{row.original.source}</Badge>,
      },
      {
        accessorKey: "lastActivity",
        header: t("lastActivity"),
        cell: ({ row }) => <span className="text-muted-foreground tabular-nums">{row.original.lastActivity}</span>,
      },
      {
        id: "actions",
        cell: () => (
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button variant="ghost" size="icon" className="text-muted-foreground flex size-8">
                <EllipsisVertical />
                <span className="sr-only">{t("openMenu")}</span>
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="w-32">
              <DropdownMenuGroup>
                <DropdownMenuItem>{t("view")}</DropdownMenuItem>
                <DropdownMenuItem>{t("assign")}</DropdownMenuItem>
                <DropdownMenuItem>{t("archive")}</DropdownMenuItem>
              </DropdownMenuGroup>
              <DropdownMenuSeparator />
              <DropdownMenuGroup>
                <DropdownMenuItem variant="destructive">{t("delete")}</DropdownMenuItem>
              </DropdownMenuGroup>
            </DropdownMenuContent>
          </DropdownMenu>
        ),
        enableHiding: false,
      },
    ],
    [t],
  );
}
