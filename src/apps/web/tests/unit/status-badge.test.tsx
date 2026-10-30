import { renderToStaticMarkup } from "react-dom/server";
import { expect, test } from "vitest";

import { StatusBadge } from "../../src/spx/components/status-badge";

test("status badges retain labels and consistent semantic variants", () => {
  for (const [status, variant] of [
    ["active", "success"],
    ["success", "success"],
    ["disabled", "destructive"],
    ["suspended", "destructive"],
    ["failure", "destructive"],
    ["denied", "warning"],
    ["current", "info"],
    ["custom", "secondary"],
  ]) {
    const html = renderToStaticMarkup(<StatusBadge status={status}>Visible status</StatusBadge>);
    expect(html).toContain(`data-variant="${variant}"`);
    expect(html).toContain("Visible status");
  }
});
