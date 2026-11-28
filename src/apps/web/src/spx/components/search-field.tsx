import { cn } from "cn";
import { SearchIcon } from "lucide-react";

import { Button } from "@/components/ui/button";
import { InputGroup, InputGroupAddon, InputGroupInput } from "@/components/ui/input-group";

/**
 * SearchField 是列表页的关键词输入与提交按钮。
 *
 * 各页原本各自拼一份 InputGroup + 按钮，宽度、高度和回车行为逐页漂移；
 * 这里收成一处：回车与点击都提交当前输入，调用方只负责保存已提交的值。
 */
export function SearchField({
  value,
  onChange,
  onSubmit,
  placeholder,
  submitLabel,
  className,
}: {
  value: string;
  onChange: (value: string) => void;
  onSubmit?: (value: string) => void;
  placeholder: string;
  submitLabel?: string;
  className?: string;
}) {
  return (
    <>
      <InputGroup className={cn("h-7 w-full rounded-md sm:w-64", className)}>
        <InputGroupAddon>
          <SearchIcon />
        </InputGroupAddon>
        <InputGroupInput
          type="search"
          className="h-7 min-w-0 truncate"
          value={value}
          placeholder={placeholder}
          title={value || placeholder}
          aria-label={placeholder}
          onChange={(event) => onChange(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === "Enter" && !event.nativeEvent.isComposing) onSubmit?.(event.currentTarget.value);
          }}
        />
      </InputGroup>
      {onSubmit && submitLabel && (
        <Button size="sm" variant="outline" onClick={() => onSubmit(value)}>
          {submitLabel}
        </Button>
      )}
    </>
  );
}
