"use client";

import { useEffect, useState, type ReactNode } from "react";

import { Card, CardContent } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import type { ApiOrganization } from "@/spx/api/iam/schemas";

import { OrganizationTree } from "./organization-tree";

// 组织层级树默认展开到根节点；父级 id 为空的那个组织就是根。
function rootOrganization(items: ApiOrganization[]): ApiOrganization | undefined {
  return items.find((item) => !item.parent_id) ?? items[0];
}

/**
 * SplitPane 是组织管理与用户管理共用的「左侧层级树 + 右侧内容」布局。
 * 选中项由内部状态持有，默认落在根组织，因此右侧默认展示全量。
 */
export function OrganizationSplitPane({
  items,
  loading,
  heading,
  description,
  children,
}: {
  items: ApiOrganization[];
  loading?: boolean;
  heading: string;
  description: string;
  children: (selected: ApiOrganization | undefined) => ReactNode;
}) {
  const [selectedID, setSelectedID] = useState<string>();

  // 数据到达后补齐默认选中，避免首帧闪空。
  useEffect(() => {
    if (selectedID) return;
    const root = rootOrganization(items);
    if (root) setSelectedID(root.id);
  }, [items, selectedID]);

  const selected = items.find((item) => item.id === selectedID);

  return (
    <div className="flex flex-col gap-4 lg:flex-row lg:items-start">
      <Card className="w-full shrink-0 lg:w-64">
        <CardContent className="p-3">
          <h3 className="mb-1 px-1 text-sm font-medium">{heading}</h3>
          <p className="text-muted-foreground mb-2 px-1 text-xs leading-relaxed">{description}</p>
          {loading ? (
            <div className="flex flex-col gap-2">
              <Skeleton className="h-7 w-full" />
              <Skeleton className="h-7 w-4/5" />
              <Skeleton className="h-7 w-3/5" />
            </div>
          ) : (
            <OrganizationTree
              items={items}
              selectedID={selectedID}
              variant="selectable"
              onSelect={(node) => setSelectedID(node.id)}
              className="max-h-[32rem]"
            />
          )}
        </CardContent>
      </Card>

      <div className="flex min-w-0 flex-1 flex-col gap-4">{children(selected)}</div>
    </div>
  );
}
