import { notFound } from "next/navigation";

import { IngestWorkspace } from "@/spx/pages/ingest-workspace";

export default async function Page({ params }: { params: Promise<{ path?: string[] }> }) {
  const { path = [] } = await params;
  const [section, id] = path;
  if (path.length > 2 || (section && section !== "inputs" && section !== "parsing-rules")) notFound();
  return <IngestWorkspace section={section} id={id} />;
}
