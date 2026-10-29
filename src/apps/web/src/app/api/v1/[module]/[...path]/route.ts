import { apiOrigins } from "@/spx/server/config";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

async function proxy(request: Request) {
  const requestID = crypto.randomUUID();
  const started = Date.now();
  const url = new URL(request.url);
  const module = url.pathname.split("/")[3];
  if (module !== "iam" && module !== "console") {
    return Response.json(
      { code: "not_found", message: "接口不存在", data: null, request_id: requestID },
      { status: 404, headers: { "X-Request-ID": requestID, "Cache-Control": "no-store" } },
    );
  }
  const headers = new Headers();
  for (const name of [
    "accept",
    "content-type",
    "cookie",
    "origin",
    "user-agent",
    "x-csrf-token",
    "x-request-id",
    "last-event-id",
  ]) {
    const value = request.headers.get(name);
    if (value) headers.set(name, value);
  }
  try {
    const init: RequestInit & { duplex?: "half" } = {
      method: request.method,
      headers,
      redirect: "manual",
      cache: "no-store",
      signal: request.signal,
    };
    if (!["GET", "HEAD"].includes(request.method)) {
      init.body = request.body;
      init.duplex = "half";
    }
    const response = await fetch(`${apiOrigins[module]}${url.pathname}${url.search}`, init);
    const outgoing = new Headers();
    for (const name of [
      "content-type",
      "cache-control",
      "vary",
      "retry-after",
      "allow",
      "x-request-id",
      "content-disposition",
    ]) {
      const value = response.headers.get(name);
      if (value) outgoing.set(name, value);
    }
    for (const cookie of response.headers.getSetCookie()) outgoing.append("set-cookie", cookie);
    // 仅记录模块和上游标识；完整 URL、Cookie、请求体不能进入日志。
    const upstreamID = response.headers.get("x-request-id");
    console.info(
      JSON.stringify({
        time: new Date().toISOString(),
        level: "INFO",
        service: "web",
        event: "web.proxy_forwarded",
        request_id: requestID,
        module,
        method: request.method,
        status: response.status,
        upstream_request_id: upstreamID && /^[0-9a-f-]{36}$/i.test(upstreamID) ? upstreamID : undefined,
        duration_ms: Date.now() - started,
      }),
    );
    return new Response(response.body, { status: response.status, headers: outgoing });
  } catch (error) {
    const cause = error instanceof Error ? error.cause : undefined;
    const code = cause instanceof Error && "code" in cause ? cause.code : undefined;
    let reason = request.signal.aborted ? "client_canceled" : "upstream_fetch_failed";
    if (
      typeof code === "string" &&
      [
        "ECONNREFUSED",
        "ETIMEDOUT",
        "ENOTFOUND",
        "ECONNRESET",
        "EHOSTUNREACH",
        "DEPTH_ZERO_SELF_SIGNED_CERT",
        "ERR_TLS_CERT_ALTNAME_INVALID",
      ].includes(code)
    )
      reason = code;
    console.error(
      JSON.stringify({
        time: new Date().toISOString(),
        level: "ERROR",
        service: "web",
        event: "web.proxy_failed",
        request_id: requestID,
        module,
        method: request.method,
        reason,
        duration_ms: Date.now() - started,
      }),
    );
    return Response.json(
      { code: "service_unavailable", message: "服务暂时不可用", data: null, request_id: requestID },
      { status: 503, headers: { "X-Request-ID": requestID, "Cache-Control": "no-store" } },
    );
  }
}

export { proxy as GET, proxy as POST, proxy as PUT, proxy as PATCH, proxy as DELETE, proxy as HEAD, proxy as OPTIONS };
