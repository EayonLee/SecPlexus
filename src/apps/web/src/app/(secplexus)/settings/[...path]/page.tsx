import { ManagementPage } from "@/spx/pages/management";

export default async function Page({ params }: { params: Promise<{ path: string[] }> }) {
  const { path } = await params;
  return <ManagementPage pathname={`/settings/${path.join("/")}`} />;
}
