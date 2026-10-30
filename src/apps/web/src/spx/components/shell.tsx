"use client";
import type { CSSProperties, ReactNode } from "react";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";

import {
  ArrowLeft,
  Command,
  KeyRound,
  LayoutDashboard,
  LogOut,
  type LucideIcon,
  Settings,
  UserRound,
} from "lucide-react";
import { useTranslations } from "next-intl";

import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Separator } from "@/components/ui/separator";
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarInset,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarProvider,
  SidebarTrigger,
  useSidebar,
} from "@/components/ui/sidebar";
import { usePreferencesStore } from "@/stores/preferences/preferences-provider";

import { visibleManagementPages, visibleManagementSections } from "../iam/navigation";
import { signOut, useIdentity } from "../iam/session";
import { useAction } from "./data";
import { LanguageSwitcher } from "./language-switcher";
import { LayoutControls } from "./layout-controls";
import { ThemeSwitcher } from "./theme-switcher";

export function Shell({ children }: { children: ReactNode }) {
  const perform = useAction();

  const t = useTranslations("Navigation");

  const auth = useIdentity();
  const router = useRouter();
  const pathname = usePathname();
  const variant = usePreferencesStore((state) => state.values.sidebar_variant);
  const collapsible = usePreferencesStore((state) => state.values.sidebar_collapsible);
  const pages = visibleManagementPages(auth);
  const management = pathname === "/management" || pathname.startsWith("/management/");
  const settings = pathname === "/settings" || pathname.startsWith("/settings/");
  const sections = visibleManagementSections(auth);
  let navigationLabel = t("mainNavigation");
  if (management) navigationLabel = t("managementNavigation");
  if (settings) navigationLabel = t("accountSettings");
  const workspace = auth.org_id ? `/dashboard?org_id=${encodeURIComponent(auth.org_id)}` : "/dashboard";
  return (
    <SidebarProvider style={{ "--sidebar-width": "calc(var(--spacing) * 68)" } as CSSProperties}>
      <Sidebar variant={variant} collapsible={collapsible}>
        <SidebarHeader>
          <SidebarMenu>
            <SidebarMenuItem>
              <SidebarLink href="/dashboard" icon={Command}>
                <span className="text-base font-semibold">SecPlexus</span>
              </SidebarLink>
            </SidebarMenuItem>
          </SidebarMenu>
        </SidebarHeader>
        <SidebarContent role="navigation" aria-label={navigationLabel}>
          {!management && !settings && (
            <SidebarGroup>
              <SidebarGroupLabel>{t("dashboards")}</SidebarGroupLabel>
              <SidebarMenu>
                <SidebarMenuItem>
                  <SidebarLink href="/dashboard" icon={LayoutDashboard} isActive={pathname === "/dashboard"}>
                    <span>{t("dashboard")}</span>
                  </SidebarLink>
                </SidebarMenuItem>
              </SidebarMenu>
            </SidebarGroup>
          )}
          {management && (
            <SidebarGroup>
              <SidebarGroupLabel>{t("managementCenter")}</SidebarGroupLabel>
              <SidebarMenu>
                {sections.map((section) => (
                  <SidebarMenuItem key={section.id}>
                    <SidebarLink
                      href={section.path(auth.org_id)}
                      icon={section.icon}
                      isActive={pages.some((page) => page.group === section.id && page.path(auth.org_id) === pathname)}
                    >
                      <span>{t(section.id)}</span>
                    </SidebarLink>
                  </SidebarMenuItem>
                ))}
              </SidebarMenu>
            </SidebarGroup>
          )}
          {settings && (
            <SidebarGroup>
              <SidebarGroupLabel>{t("accountSettings")}</SidebarGroupLabel>
              <SidebarMenu>
                {(
                  [
                    { id: "profile", title: "profile", icon: UserRound },
                    { id: "account", title: "account", icon: KeyRound },
                  ] as const
                ).map((item) => (
                  <SidebarMenuItem key={item.id}>
                    <SidebarLink
                      href={`/settings/${item.id}`}
                      icon={item.icon}
                      isActive={pathname === `/settings/${item.id}`}
                    >
                      <span className="whitespace-nowrap">{t(item.title)}</span>
                    </SidebarLink>
                  </SidebarMenuItem>
                ))}
              </SidebarMenu>
            </SidebarGroup>
          )}
        </SidebarContent>
        {(management || settings) && (
          <SidebarFooter>
            <SidebarMenu>
              <SidebarMenuItem>
                <SidebarLink href={workspace} icon={ArrowLeft}>
                  <span>{t("backToWorkspace")}</span>
                </SidebarLink>
              </SidebarMenuItem>
            </SidebarMenu>
          </SidebarFooter>
        )}
      </Sidebar>
      <SidebarInset className="min-w-0 overflow-x-clip [--dashboard-header-height:--spacing(12)] peer-data-[variant=inset]:border [html[data-content-layout=centered]_&>*]:mx-auto [html[data-content-layout=centered]_&>*]:w-full [html[data-content-layout=centered]_&>*]:max-w-screen-2xl">
        <header className="[html[data-navbar-style=sticky]_&]:bg-background/50 flex h-12 shrink-0 items-center gap-2 border-b transition-[width,height] ease-linear group-has-data-[collapsible=icon]/sidebar-wrapper:h-12 [html[data-navbar-style=sticky]_&]:sticky [html[data-navbar-style=sticky]_&]:top-0 [html[data-navbar-style=sticky]_&]:z-50 [html[data-navbar-style=sticky]_&]:overflow-hidden [html[data-navbar-style=sticky]_&]:rounded-t-[inherit] [html[data-navbar-style=sticky]_&]:backdrop-blur-md">
          <div className="flex w-full items-center justify-between gap-2 px-4 lg:px-6">
            <div className="flex min-w-0 items-center gap-2">
              <SidebarTrigger />
              <Separator orientation="vertical" className="mx-2 data-[orientation=vertical]:h-4" />
              {!!auth.organizations.length && (
                <Select
                  value={auth.org_id || ""}
                  onValueChange={(org) => {
                    if (!org) return;
                    if (pathname.startsWith("/management/orgs/")) {
                      router.push(pathname.replace(/^(\/management\/orgs\/)[^/]+/, `$1${org}`));
                      return;
                    }
                    router.push(`${settings ? pathname : "/dashboard"}?org_id=${org}`);
                  }}
                >
                  <SelectTrigger aria-label={t("currentOrganization")} className="w-56">
                    <SelectValue placeholder={t("chooseOrganization")} />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectGroup>
                      {auth.organizations.map((org) => (
                        <SelectItem key={org.id} value={org.id}>
                          {org.name}
                        </SelectItem>
                      ))}
                    </SelectGroup>
                  </SelectContent>
                </Select>
              )}
            </div>
            <div className="flex items-center gap-2">
              <LanguageSwitcher />
              <LayoutControls />
              <ThemeSwitcher />
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button variant="ghost" size="icon" aria-label={t("accountMenu")}>
                    <Avatar className="size-8 rounded-lg">
                      <AvatarFallback>{auth.principal?.display_name.slice(0, 1)}</AvatarFallback>
                    </Avatar>
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent
                  align="end"
                  className="min-w-56 [&_[data-slot=dropdown-menu-item]]:whitespace-nowrap"
                >
                  <DropdownMenuGroup>
                    <DropdownMenuItem asChild>
                      <Link href="/settings/profile">
                        <UserRound />
                        {t("accountSettings")}
                      </Link>
                    </DropdownMenuItem>
                  </DropdownMenuGroup>
                  {!!pages.length && (
                    <DropdownMenuGroup>
                      <DropdownMenuItem asChild>
                        <Link href={pages[0].path(auth.org_id)}>
                          <Settings />
                          {t("managementCenter")}
                        </Link>
                      </DropdownMenuItem>
                    </DropdownMenuGroup>
                  )}
                  <DropdownMenuSeparator />
                  <DropdownMenuGroup>
                    <DropdownMenuItem
                      onSelect={() =>
                        void perform(async () => {
                          await signOut();
                          router.replace("/login");
                        })
                      }
                    >
                      <LogOut />
                      {t("signOut")}
                    </DropdownMenuItem>
                  </DropdownMenuGroup>
                </DropdownMenuContent>
              </DropdownMenu>
            </div>
          </div>
        </header>
        <main className="min-h-0 min-w-0 flex-1 overflow-x-hidden p-4 md:p-6">{children}</main>
      </SidebarInset>
    </SidebarProvider>
  );
}

/** SidebarLink 在原生导航完成时关闭移动抽屉，保持桌面侧栏状态。 */
function SidebarLink({
  href,
  icon: Icon,
  isActive,
  children,
}: {
  href: string;
  icon: LucideIcon;
  isActive?: boolean;
  children: ReactNode;
}) {
  const { setOpenMobile } = useSidebar();
  return (
    <SidebarMenuButton asChild isActive={isActive}>
      <Link
        href={href}
        onNavigate={() => {
          setOpenMobile(false);
        }}
      >
        <Icon />
        {children}
      </Link>
    </SidebarMenuButton>
  );
}
