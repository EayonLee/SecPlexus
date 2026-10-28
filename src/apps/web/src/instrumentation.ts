import type { Instrumentation } from "next";

/** register 接入当前 Next 进程的生命周期日志；容器由运行平台收集和轮转 stdout。 */
export function register() {
  console.info(
    JSON.stringify({ time: new Date().toISOString(), level: "INFO", service: "web", event: "service.started" }),
  );
}

/** onRequestError 只记录框架路由和诊断标识，不输出请求对象、Cookie、查询值或原始异常消息。 */
export const onRequestError: Instrumentation.onRequestError = (error, _request, context) => {
  const digest =
    error instanceof Error &&
    "digest" in error &&
    typeof error.digest === "string" &&
    /^[\w-]{1,64}$/.test(error.digest)
      ? error.digest
      : undefined;
  const kind =
    error instanceof Error &&
    ["Error", "TypeError", "ReferenceError", "RangeError", "SyntaxError", "URIError", "AggregateError"].includes(
      error.name,
    )
      ? error.name
      : "Error";

  // digest 可与 Next 的服务端错误记录关联；不把用户可控的 message 当作诊断字段。
  console.error(
    JSON.stringify({
      time: new Date().toISOString(),
      level: "ERROR",
      service: "web",
      event: "web.request_failed",
      request_id: crypto.randomUUID(),
      route: context.routePath,
      route_type: context.routeType,
      router: context.routerKind,
      error_kind: kind,
      digest,
    }),
  );
};
