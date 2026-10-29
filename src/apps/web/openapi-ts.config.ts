import { defineConfig } from "@hey-api/openapi-ts";

import path from "node:path";

export default defineConfig(
  ["iam", "console"].map((module) => ({
    input: path.join(process.env.SPX_CONTRACT_INPUT || "../../contracts", module, "openapi.json"),
    output: path.join(process.env.SPX_CONTRACT_OUTPUT || "src/spx/api", module),
    plugins: [
      "@hey-api/typescript",
      "@hey-api/client-fetch",
      { name: "@hey-api/sdk", validator: { response: "zod" } },
      { name: "zod", requests: true, responses: true, dates: { offset: true, local: false } },
      { name: "@tanstack/react-query", queryOptions: true, mutationOptions: true, mutationKeys: false },
    ],
  })),
);
