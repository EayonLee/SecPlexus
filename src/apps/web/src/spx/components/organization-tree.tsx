"use client";

import { useMemo, useState } from "react";

import { cn } from "cn";
import { BuildingIcon, CheckIcon, ChevronRightIcon } from "lucide-react";

import { ScrollArea } from "@/components/ui/scroll-area";
import type { ApiOrganization } from "@/spx/api/iam/schemas";

/** 把扁平组织列表组装成树，保持输入顺序。 */
function buildTree(items: ApiOrganization[]) {
  const byParent = new Map<string | null, ApiOrganization[]>();
  for (const item of items) {
    const key = item.parent_id ?? null;
    const bucket = byParent.get(key) ?? [];
    bucket.push(item);
    byParent.set(key, bucket);
  }

  return byParent;
}

/** 返回从根到目标组织的 id 链，用于打开时展开到当前节点。 */
export function ancestorIDs(items: ApiOrganization[], targetID: string): string[] {
  const byID = new Map(items.map((item) => [item.id, item]));
  const chain: string[] = [];
  let current = byID.get(targetID);
  while (current?.parent_id) {
    chain.unshift(current.parent_id);
    current = byID.get(current.parent_id);
  }

  return chain;
}

/** 返回目标组织及其全部后代的 id，用于禁止把节点挂到自身子树下。 */
export function descendantIDs(items: ApiOrganization[], targetID: string): Set<string> {
  const byParent = buildTree(items);
  const blocked = new Set<string>([targetID]);
  const walk = (id: string) => {
    for (const child of byParent.get(id) ?? []) {
      if (blocked.has(child.id)) continue;
      blocked.add(child.id);
      walk(child.id);
    }
  };
  walk(targetID);

  return blocked;
}

type OrganizationTreeProps = {
  items: ApiOrganization[];
  currentID?: string;
  selectedID?: string;
  onSelect?: (org: ApiOrganization) => void;
  disabledIDs?: Set<string>;
  variant?: "navigation" | "selectable" | "readonly";
  className?: string;
};

/**
 * 组织树。三种用法共用同一渲染：
 * - navigation：点击切换当前组织
 * - selectable：点击选中，用于父组织选择器
 * - readonly：纯展示，节点不可点击
 */
export function OrganizationTree({
  items,
  currentID,
  selectedID,
  onSelect,
  disabledIDs,
  variant = "navigation",
  className,
}: OrganizationTreeProps) {
  const tree = useMemo(() => buildTree(items), [items]);
  const path = useMemo(
    () => new Set(ancestorIDs(items, selectedID ?? currentID ?? "")),
    [items, selectedID, currentID],
  );
  const [collapsed, setCollapsed] = useState<Set<string>>(() => new Set());

  function toggle(id: string) {
    setCollapsed((current) => {
      const next = new Set(current);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }

  function render(parentID: string | null, level: number) {
    const children = tree.get(parentID) ?? [];

    return children.map((node) => {
      const hasChildren = (tree.get(node.id) ?? []).length > 0;
      const open = hasChildren && !collapsed.has(node.id);
      const isCurrent = node.id === currentID;
      const isSelected = node.id === selectedID;
      const disabled = disabledIDs?.has(node.id) ?? false;
      const interactive = variant !== "readonly" && !disabled;

      return (
        <li key={node.id}>
          <div className="flex items-center gap-0.5" style={{ paddingInlineStart: `${level * 14}px` }}>
            {hasChildren ? (
              <button
                type="button"
                onClick={() => toggle(node.id)}
                aria-label={node.name}
                aria-expanded={open}
                className="text-muted-foreground hover:text-foreground focus-visible:ring-ring/50 inline-flex size-5 shrink-0 items-center justify-center rounded-sm transition-colors focus-visible:ring-[2px] focus-visible:outline-none"
              >
                <ChevronRightIcon className={cn("size-3.5 transition-transform duration-150", open && "rotate-90")} />
              </button>
            ) : (
              <span aria-hidden className="size-5 shrink-0" />
            )}

            <button
              type="button"
              onClick={interactive ? () => onSelect?.(node) : undefined}
              disabled={!interactive}
              aria-current={isCurrent ? "true" : undefined}
              aria-pressed={variant === "selectable" ? isSelected : undefined}
              className={cn(
                "focus-visible:ring-ring/50 flex min-w-0 flex-1 items-center gap-1.5 rounded-sm px-1.5 py-1 text-left text-sm transition-colors focus-visible:ring-[2px] focus-visible:outline-none",
                interactive && "hover:bg-accent",
                !interactive && "cursor-default",
                disabled && "text-muted-foreground/50 cursor-not-allowed",
                isCurrent && "bg-accent font-medium",
                isSelected && "bg-primary/10 text-primary font-medium",
              )}
            >
              <BuildingIcon aria-hidden className={cn("size-3.5 shrink-0", isSelected ? "opacity-100" : "opacity-50")} />
              <span className="truncate">{node.name}</span>
              {isSelected && <CheckIcon aria-hidden className="ms-auto size-3.5 shrink-0" />}
            </button>
          </div>

          {open && <ul className="flex flex-col">{render(node.id, level + 1)}</ul>}
        </li>
      );
    });
  }

  return (
    <ScrollArea className={cn("max-h-80", className)}>
      <ul className="flex flex-col gap-0.5 pe-2">{render(null, 0)}</ul>
    </ScrollArea>
  );
}
