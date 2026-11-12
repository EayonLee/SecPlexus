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
}: {
  value: string;
  onChange: (value: string) => void;
  onSubmit: () => void;
  placeholder: string;
  submitLabel: string;
}) {
  return (
    <>
      <InputGroup className="w-64">
        <InputGroupAddon>
          <SearchIcon />
        </InputGroupAddon>
        <InputGroupInput
          className="h-8"
          value={value}
          placeholder={placeholder}
          aria-label={placeholder}
          onChange={(event) => onChange(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === "Enter") onSubmit();
          }}
        />
      </InputGroup>
      <Button size="sm" variant="outline" onClick={onSubmit}>
        {submitLabel}
      </Button>
    </>
  );
}
