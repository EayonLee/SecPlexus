import { z } from "zod";

const port = z.coerce.number().int().min(1).max(65535);
const host = z.union([z.hostname(), z.ipv4(), z.ipv6()]);

/** @param {string} hostname @param {number} port */
function serviceOrigin(hostname, port) {
  return new URL(`http://${hostname.includes(":") ? `[${hostname}]` : hostname}:${port}`).origin;
}
/** @param {Record<string, string | undefined>} environment */
export function readWebConfiguration(environment) {
  // 上游地址供 Web 服务端调用。监听地址可以使用通配地址。
  const targetHost = host.refine((value) => !["0.0.0.0", "::"].includes(value));
  const parsed = z
    .object({
      SPX_WEB_PORT: port,
      SPX_WEB_BIND_HOST: z.union([z.ipv4(), z.ipv6(), z.literal("localhost")]).default("0.0.0.0"),
      SPX_WEB_EDGE_TOKEN: z.string().regex(/^[0-9a-f]{64}$/).or(z.literal("")).optional(),
      SPX_IAM_HOST: targetHost,
      SPX_IAM_PORT: port,
      SPX_CONSOLE_HOST: targetHost,
      SPX_CONSOLE_PORT: port,
      SPX_INGEST_HOST: targetHost.or(z.literal("")).optional(),
      SPX_INGEST_PORT: z.literal("").or(port).optional(),
      SPX_INGEST_ORIGIN: z.string().optional(),
    })
    .safeParse(
      environment.SPX_INGEST_ORIGIN
        ? { ...environment, SPX_INGEST_HOST: undefined, SPX_INGEST_PORT: undefined }
        : environment,
    );
  if (!parsed.success) {
    throw new Error(
      "Invalid Web configuration: " +
        parsed.error.issues
          .map((issue) => {
            const key = issue.path.join(".");
            if (key === "SPX_WEB_EDGE_TOKEN") return `${key}: expected a 64-character hexadecimal token`;
            return `${key}: ${key.endsWith("PORT") ? "expected an integer from 1 to 65535" : "expected a valid host (connection targets cannot be wildcard addresses)"}`;
          })
          .join("; "),
    );
  }
  const configuration = parsed.data;
  const webPort = configuration.SPX_WEB_PORT;
  const bindHost = configuration.SPX_WEB_BIND_HOST;
  let ingest;
  // 独立部署的 Ingest 可以用完整来源指定 HTTPS。
  if (configuration.SPX_INGEST_ORIGIN) {
    const url = URL.parse(configuration.SPX_INGEST_ORIGIN);
    if (
      !url ||
      !["http:", "https:"].includes(url.protocol) ||
      !url.hostname ||
      ["0.0.0.0", "[::]"].includes(url.hostname) ||
      url.username ||
      url.password ||
      url.pathname !== "/" ||
      url.search ||
      url.hash
    ) {
      throw new Error("SPX_INGEST_ORIGIN must be an HTTP(S) origin without a path, query or credentials.");
    }
    ingest = url.origin;
  } else if (configuration.SPX_INGEST_HOST || configuration.SPX_INGEST_PORT) {
    if (!configuration.SPX_INGEST_HOST || !configuration.SPX_INGEST_PORT) {
      throw new Error("SPX_INGEST_HOST and SPX_INGEST_PORT must be set together.");
    }
    ingest = serviceOrigin(configuration.SPX_INGEST_HOST, configuration.SPX_INGEST_PORT);
  }
  return {
    webPort,
    bindHost,
    edgeToken: configuration.SPX_WEB_EDGE_TOKEN,
    apiOrigins: {
      iam: serviceOrigin(configuration.SPX_IAM_HOST, configuration.SPX_IAM_PORT),
      console: serviceOrigin(configuration.SPX_CONSOLE_HOST, configuration.SPX_CONSOLE_PORT),
      ingest,
    },
  };
}
