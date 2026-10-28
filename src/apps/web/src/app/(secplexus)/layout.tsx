import { type ReactNode, Suspense } from "react";

import type { Metadata } from "next";

import { AuthBoundary } from "@/spx/components/access";
import { Shell } from "@/spx/components/shell";
import { QueryProvider } from "@/spx/iam/query";
export const metadata: Metadata = { title: "SecPlexus" };
export default function Layout({ children }: { children: ReactNode }) {
  return (
    <Suspense>
      <QueryProvider>
        <AuthBoundary>
          <Shell>{children}</Shell>
        </AuthBoundary>
      </QueryProvider>
    </Suspense>
  );
}
