import { z } from "zod";

const port = z.coerce.number().int().min(1).max(65535);
const host = z.union([z.hostname(), z.ipv4(), z.ipv6()]);
/** @param {Record<string, string | undefined>} environment */
export function readWebConfiguration(environment) {
  const targetHost = host.refine((value) => !["0.0.0.0", "::"].includes(value));
  const parsed = z
    .object({
      SPX_WEB_PORT: port,
      SPX_WEB_BIND_HOST: z.union([z.ipv4(), z.ipv6(), z.literal("localhost")]).default("0.0.0.0"),
      SPX_IAM_HOST: targetHost,
      SPX_IAM_PORT: port,
      SPX_CONSOLE_HOST: targetHost,
      SPX_CONSOLE_PORT: port,
    })
    .safeParse(environment);
  if (!parsed.success) {
    throw new Error(
      "Invalid Web configuration: " +
        parsed.error.issues
          .map((issue) => {
            const key = issue.path.join(".");
            return `${key}: ${key.endsWith("PORT") ? "expected an integer from 1 to 65535" : "expected a valid host (connection targets cannot be wildcard addresses)"}`;
          })
          .join("; "),
    );
  }
  const configuration = parsed.data;
  const webPort = configuration.SPX_WEB_PORT;
  const bindHost = configuration.SPX_WEB_BIND_HOST;
  const origins = [
    [configuration.SPX_IAM_HOST, configuration.SPX_IAM_PORT],
    [configuration.SPX_CONSOLE_HOST, configuration.SPX_CONSOLE_PORT],
  ].map(
    ([hostname, port]) =>
      new URL(`http://${String(hostname).includes(":") ? `[${hostname}]` : hostname}:${port}`).origin,
  );
  return { webPort, bindHost, apiOrigins: { iam: origins[0], console: origins[1] } };
}
