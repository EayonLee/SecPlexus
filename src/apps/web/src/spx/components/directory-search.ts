import { useEffect, useState } from "react";

/** 打字调节服务端查询，回车可以立即提交。 */
export function useDirectorySearch() {
  const [term, setTerm] = useState("");
  const [applied, setApplied] = useState("");
  useEffect(() => {
    if (term === applied) return;
    const timer = window.setTimeout(() => setApplied(term), 250);
    return () => window.clearTimeout(timer);
  }, [term, applied]);
  return { term, setTerm, applied, setApplied };
}
