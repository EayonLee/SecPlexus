import axios, { type AxiosRequestConfig, type AxiosResponse, isAxiosError, isCancel } from "axios";
import { z } from "zod";

import { resultCodes, resultStatuses } from "@/auth/result.gen";

import { assertRequestScope, requestScope } from "../iam/request-scope";

const fieldErrorsSchema = z
  .array(
    z.object({
      field: z.string().min(1).max(128),
      code: z.string().min(1).max(64),
      limit: z.int().positive().optional(),
    }),
  )
  .max(32);

/** APIError 向页面传递结果码、字段错误、HTTP 状态和排障标识。 */
export class APIError extends Error {
  constructor(
    readonly status: number,
    readonly code: string,
    readonly requestID = "",
    readonly path = "",
    readonly fieldErrors: z.infer<typeof fieldErrorsSchema> = [],
  ) {
    super(code);
  }
}

declare module "axios" {
  interface AxiosRequestConfig<D, P> {
    spx?: { status: number; schema: z.ZodType; epoch?: number };
  }
}

const errorSchema = z.object({
  code: z.enum(Object.values(resultCodes)),
  message: z.string(),
  data: z.null(),
  request_id: z.string().min(1),
  field_errors: fieldErrorsSchema.optional().catch(undefined),
});

/** http 的传输、JSON 解码、超时和取消由 Axios 实现。 */
export const http = axios.create({
  baseURL: typeof window === "undefined" ? "" : window.location.origin,
  timeout: 15000,
  responseType: "json",
  withCredentials: true,
  headers: { Accept: "application/json" },
});

http.interceptors.request.use((config) => {
  const scope = requestScope();
  if (config.spx) config.spx.epoch = scope.version;
  config.signal = config.signal instanceof AbortSignal ? AbortSignal.any([config.signal, scope.signal]) : scope.signal;
  return config;
});

http.interceptors.response.use(
  async (response) => {
    const boundary = response.config.spx;
    if (!boundary || boundary.epoch === undefined) throw new Error("Missing API response model");
    assertRequestScope(boundary.epoch);
    const requestID = z.uuid().safeParse(response.headers["x-request-id"]).data ?? "";
    const rejected = () => new APIError(response.status, "internal_error", requestID, response.config.url);
    if (response.status !== boundary.status) throw rejected();
    if (response.status === 204) {
      if (response.data !== "" && response.data !== undefined) throw rejected();
      response.data = await boundary.schema.parseAsync(undefined);
    } else {
      const envelope = z.object({
        code: z.literal("ok"),
        message: z.string(),
        request_id: z.string().min(1),
        data: boundary.schema,
      });
      const parsed = await envelope.safeParseAsync(response.data);
      assertRequestScope(boundary.epoch);
      if (!parsed.success) {
        console.error(
          JSON.stringify({
            service: "web",
            event: "api.protocol_failed",
            route: response.config.url,
            status: response.status,
            request_id: requestID,
            issues: parsed.error.issues.map((issue) => ({ code: issue.code, path: issue.path })),
          }),
        );
        throw rejected();
      }
      response.data = parsed.data.data;
    }
    assertRequestScope(boundary.epoch);
    return response;
  },
  (error: unknown) => {
    if (isCancel(error)) throw new DOMException("Request canceled", "AbortError");
    if (!isAxiosError(error)) throw error;
    if (error.config?.spx?.epoch !== undefined) assertRequestScope(error.config.spx.epoch);
    const response = error.response;
    const status = response?.status ?? 503;
    const parsed = errorSchema.safeParse(response?.data);
    let code = "internal_error";
    if (status === 401) code = "unauthenticated";
    if (status === 503) code = "service_unavailable";
    if (parsed.success && resultStatuses[parsed.data.code] === status) code = parsed.data.code;
    const requestID = parsed.success
      ? parsed.data.request_id
      : (z.uuid().safeParse(response?.headers["x-request-id"]).data ?? "");
    const fieldErrors = parsed.success && parsed.data.code === code ? parsed.data.field_errors : undefined;
    throw new APIError(status, code, requestID, error.config?.url, fieldErrors);
  },
);

/** request 只适配平台结果与调用方模型，不实现另一套 HTTP/JSON 执行器。 */
export async function request<S extends z.ZodType>(
  config: AxiosRequestConfig,
  schema: S,
  status = 200,
): Promise<z.output<S>> {
  const response = await http.request<unknown, AxiosResponse<z.output<S>>>({ ...config, spx: { status, schema } });
  return response.data;
}

export function pageSchema<S extends z.ZodType>(item: S) {
  return z.object({
    items: z.array(item),
    total: z.int().min(0).max(Number.MAX_SAFE_INTEGER),
    limit: z.int(),
    offset: z.int(),
  });
}
