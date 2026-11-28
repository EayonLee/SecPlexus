"use client";

import { useEffect, useRef, useState } from "react";

type RuleRequestState<T> = {
  key?: string;
  data?: T;
  error?: unknown;
  isPending?: boolean;
};

// 样本和含样本的响应只由当前编辑会话持有。
export function useRuleRequest<T>(currentKey: string) {
  const active = useRef<{ key: string; controller: AbortController } | null>(null);
  const [state, setState] = useState<RuleRequestState<T>>({});

  useEffect(() => () => active.current?.controller.abort(), []);
  useEffect(() => {
    const pending = active.current;
    if (pending && pending.key !== currentKey) {
      pending.controller.abort();
      active.current = null;
    }
  }, [currentKey]);

  function reset() {
    active.current?.controller.abort();
    active.current = null;
    setState({});
  }

  async function run(key: string, send: (signal: AbortSignal) => Promise<T>, onError?: (error: unknown) => void) {
    active.current?.controller.abort();
    const controller = new AbortController();
    active.current = { key, controller };
    setState({ key, isPending: true });
    try {
      const data = await send(controller.signal);
      if (controller.signal.aborted) return;
      setState({ key, data });
      return data;
    } catch (error) {
      if (!controller.signal.aborted) {
        if (error instanceof Error && error.name === "AbortError") {
          setState({});
          return;
        }
        setState({ key, error });
        onError?.(error);
      }
    } finally {
      if (active.current?.controller === controller) active.current = null;
    }
  }

  const current = state.key === currentKey ? state : {};
  return {
    data: current.data,
    error: current.error,
    isError: current.error !== undefined,
    isPending: current.isPending ?? false,
    reset,
    run,
  };
}
