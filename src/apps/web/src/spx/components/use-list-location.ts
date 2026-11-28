"use client";

import { useSearchParams } from "next/navigation";

import { pageSizes } from "./list-controls";

/** 使用同一组 URL 参数承载有界管理列表的筛选、翻页和详情返回。 */
export function useListLocation(defaultSize = pageSizes[0]) {
  const params = useSearchParams();
  function update(key: string, value: string) {
    const url = new URL(window.location.href);
    if (key !== "page") url.searchParams.delete("page");
    if (!value) url.searchParams.delete(key);
    else url.searchParams.set(key, value);
    window.history.replaceState(null, "", url);
  }

  const requestedSize = Number(params.get("size"));
  const limit = pageSizes.includes(requestedSize) ? requestedSize : defaultSize;
  const requestedPage = Number(params.get("page"));
  const maxIndex = Math.floor(100_000 / limit);
  const index = Number.isSafeInteger(requestedPage) ? Math.min(Math.max(0, requestedPage - 1), maxIndex) : 0;
  const offset = index * limit;

  function pageControls(page?: { offset: number; total: number }) {
    const current = page ? Math.floor(page.offset / limit) : index;
    return {
      index: current,
      size: limit,
      total: page?.total,
      hasNext: page !== undefined && current < maxIndex && (current + 1) * limit < page.total,
      onPage: (next: number) => update("page", next ? String(Math.min(next, maxIndex) + 1) : ""),
      onSize: (n: number) => update("size", n === defaultSize ? "" : String(n)),
    };
  }
  return { params, update, pageControls, limit, offset };
}
