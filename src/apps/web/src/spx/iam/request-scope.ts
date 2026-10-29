let version = 0;
let controller = new AbortController();

/** requestScope 给请求及身份刷新提供同一个周期快照。 */
export function requestScope() {
  return { version, signal: controller.signal };
}

/** advanceRequestScope 使先前周期的网络和异步响应失效。 */
export function advanceRequestScope() {
  version++;
  controller.abort();
  controller = new AbortController();
}

export function assertRequestScope(expected: number) {
  if (expected !== version) throw new DOMException("Identity changed", "AbortError");
}
