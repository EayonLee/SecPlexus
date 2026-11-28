import { timingSafeEqual } from "node:crypto";
import { isIP } from "node:net";

import { apiOrigins, edgeToken } from "@/spx/server/config";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

async function proxy(request: Request) {
  const requestID = crypto.randomUUID();
  const started = Date.now();
  const url = new URL(request.url);
  const module = url.pathname.split("/")[3];
  if (module !== "iam" && module !== "console" && module !== "ingest") {
    return Response.json(
      { code: "not_found", message: "接口不存在", data: null, request_id: requestID },
      { status: 404, headers: { "X-Request-ID": requestID, "Cache-Control": "no-store" } },
    );
  }
  const upstream = apiOrigins[module];
  if (!upstream) {
    return Response.json(
      { code: "service_unavailable", message: "服务暂时不可用", data: null, request_id: requestID },
      { status: 503, headers: { "X-Request-ID": requestID, "Cache-Control": "no-store" } },
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
  // 入口覆盖客户端提供的同名头，凭据只用于这一跳，不再透传。
  if (module === "iam" && edgeToken) {
    const credential = request.headers.get("x-spx-edge-token");
    const clientIP = request.headers.get("x-spx-client-ip");
    if (
      credential && /^[0-9a-f]{64}$/.test(credential) && clientIP && isIP(clientIP) &&
      timingSafeEqual(Buffer.from(credential, "hex"), Buffer.from(edgeToken, "hex"))
    ) {
      headers.set("x-forwarded-for", clientIP);
    }
  }
  // 设备推送使用设备令牌。管理 API 继续只透传用户会话和 CSRF。
  if (request.method === "POST" && /^\/api\/v1\/ingest\/events\/[^/]+\/?$/.test(url.pathname)) {
    const credential = request.headers.get("authorization");
    if (credential) headers.set("authorization", credential);
  }
  try {
    const init: RequestInit & { duplex?: "half" } = {
      method: request.method,
      headers,
      redirect: "manual",
      cache: "no-store",
      signal: request.signal,
    };
    if (module === "ingest") {
      // Ingest 限制请求体；流式发送使 Web 无需为设备并发推送缓冲完整报文。
      if (request.body) {
        init.body = request.body;
        init.duplex = "half";
      }
    } else if (!["GET", "HEAD"].includes(request.method) && request.body) {
      // 有界缓冲保留上游未读请求体就返回的 401/405。限制与 Go 请求边界一致。
      const limit = /^\/api\/v1\/iam\/auth\/(?:login|reauthenticate)\/?$/.test(url.pathname) ? 2048 : 32 * 1024;
      const chunks: Buffer[] = [];
      let received = 0;
      const reader = request.body.getReader();
      try {
        while (true) {
          const { done, value } = await reader.read();
          if (done) break;
          received += value.byteLength;
          if (received > limit) {
            await reader.cancel().catch(() => undefined);
            return Response.json(
              { code: "payload_too_large", message: "Request body is too large", data: null, request_id: requestID },
              { status: 413, headers: { "X-Request-ID": requestID, "Cache-Control": "no-store" } },
            );
          }
          chunks.push(Buffer.from(value));
        }
      } finally {
        reader.releaseLock();
      }
      init.body = Buffer.concat(chunks, received);
    }
    const response = await fetch(`${upstream}${url.pathname}${url.search}`, init);
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
        error_name: error instanceof Error ? error.name : undefined,
        cause_name: cause instanceof Error ? cause.name : undefined,
        cause_code: typeof code === "string" ? code : undefined,
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
