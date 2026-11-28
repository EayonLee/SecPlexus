"use client";

import { useRef } from "react";

import { Ellipsis, EllipsisVertical, type LucideIcon } from "lucide-react";

import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";

type Action = {
  label: string;
  icon: LucideIcon;
  onSelect: (trigger: HTMLButtonElement | null) => void;
  disabledReason?: string;
};

export function RecordMenu({
  label,
  actions,
  destructive,
  orientation,
}: {
  label: string;
  actions: Action[];
  destructive?: Action;
  orientation?: "vertical";
}) {
  const trigger = useRef<HTMLButtonElement>(null);
  const openedOverlay = useRef(false);
  const select = (action: Action["onSelect"]) => {
    openedOverlay.current = true;
    action(trigger.current);
  };
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button ref={trigger} aria-label={label} size="icon-sm" variant="ghost">
          {orientation === "vertical" ? <EllipsisVertical /> : <Ellipsis />}
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent
        align="end"
        onCloseAutoFocus={(event) => {
          if (!openedOverlay.current) return;
          event.preventDefault();
          openedOverlay.current = false;
        }}
      >
        <DropdownMenuGroup>
          {actions.map(({ label, icon: Icon, onSelect }) => (
            <DropdownMenuItem key={label} onSelect={() => select(onSelect)}>
              <Icon />
              {label}
            </DropdownMenuItem>
          ))}
        </DropdownMenuGroup>
        {destructive && (
          <>
            <DropdownMenuSeparator />
            <DropdownMenuGroup>
              <DropdownMenuItem
                variant="destructive"
                disabled={!!destructive.disabledReason}
                title={destructive.disabledReason}
                onSelect={() => select(destructive.onSelect)}
              >
                <destructive.icon />
                {destructive.label}
              </DropdownMenuItem>
            </DropdownMenuGroup>
          </>
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
