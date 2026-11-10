"use client";
import type { ComponentType, ReactNode } from "react";

import { cn } from "cn";

// 终态页面（未找到 / 出错 / 依赖不可用）共用一套版面：
// 同一处改动即可让所有入口保持一致，避免每个场景各写一版。
export function ErrorState({
  icon: Icon,
  title,
  description,
  actions,
  variant = "page",
  className,
}: {
  icon?: ComponentType<{ className?: string }>;
  title: string;
  description: string;
  actions?: ReactNode;
  variant?: "page" | "inline";
  className?: string;
}) {
  return (
    <div
      className={cn(
        "flex flex-col items-center justify-center gap-4 px-6 text-center",
        variant === "page" ? "min-h-dvh" : "py-20",
        className,
      )}
    >
      {Icon && (
        <div className="bg-muted flex size-12 items-center justify-center rounded-full">
          <Icon className="text-muted-foreground size-6" />
        </div>
      )}
      <div className="space-y-1">
        <h1 className="font-heading text-xl font-semibold tracking-tight">{title}</h1>
        <p className="text-muted-foreground max-w-md text-sm">{description}</p>
      </div>
      {actions && <div className="flex flex-wrap items-center justify-center gap-2">{actions}</div>}
    </div>
  );
}
