import { expect, test } from "vitest";

import nextConfig from "../../next.config.mjs";

test("the system settings entry redirects to its profile page", async () => {
  const redirects = (await nextConfig.redirects?.()) ?? [];
  expect(redirects.filter(({ destination }) => destination === "/settings/profile")).toEqual([
    { source: "/settings", destination: "/settings/profile", permanent: true },
  ]);
});
