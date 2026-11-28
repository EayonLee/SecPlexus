"use client";
import type { ComponentType, ReactNode } from "react";

import { CircleAlert } from "lucide-react";

import { StatePanel } from "./collection-state";

export function ErrorState({
  icon = CircleAlert,
  title,
  description,
  actions,
  viewport = false,
}: {
  icon?: ComponentType<{ className?: string }>;
  title: string;
  description: string;
  actions?: ReactNode;
  viewport?: boolean;
}) {
  return (
    <StatePanel
      pageHeading
      kind="error"
      icon={icon}
      title={title}
      description={description}
      actions={actions}
      placement={viewport ? "viewport" : "section"}
    />
  );
}
