"use client";
import type { CSSProperties, ReactNode } from "react";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";

import { Command, LayoutDashboard, LogOut, Settings, Shield, User } from "lucide-react";
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
import { NativeSelect, NativeSelectOption } from "@/components/ui/native-select";
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
} from "@/components/ui/sidebar";
import { usePreferencesStore } from "@/stores/preferences/preferences-provider";

import { visibleManagementPages } from "../iam/navigation";
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
  return (
    <SidebarProvider style={{ "--sidebar-width": "calc(var(--spacing) * 68)" } as CSSProperties}>
      <Sidebar variant={variant} collapsible={collapsible}>
        <SidebarHeader>
          <SidebarMenu>
            <SidebarMenuItem>
              <SidebarMenuButton asChild>
                <Link href="/dashboard">
                  <Command />
                  <span className="text-base font-semibold">SecPlexus</span>
                </Link>
              </SidebarMenuButton>
            </SidebarMenuItem>
          </SidebarMenu>
        </SidebarHeader>
        <SidebarContent role="navigation" aria-label={t("mainNavigation")}>
          <SidebarGroup>
            <SidebarGroupLabel>{t("dashboards")}</SidebarGroupLabel>
            <SidebarMenu>
              <SidebarMenuItem>
                <SidebarMenuButton asChild isActive={pathname === "/dashboard"}>
                  <Link href="/dashboard">
                    <LayoutDashboard />
                    <span>{t("dashboard")}</span>
                  </Link>
                </SidebarMenuButton>
              </SidebarMenuItem>
            </SidebarMenu>
          </SidebarGroup>
          {[...new Set(pages.map((page) => page.group))].map((group) => (
            <SidebarGroup key={group}>
              <SidebarGroupLabel>{t(group)}</SidebarGroupLabel>
              <SidebarMenu>
                {pages
                  .filter((page) => page.group === group)
                  .map((page) => (
                    <SidebarMenuItem key={page.id}>
                      <SidebarMenuButton asChild isActive={pathname === page.path(auth.org_id)}>
                        <Link href={page.path(auth.org_id)}>
                          <Shield />
                          <span>{t(page.title)}</span>
                        </Link>
                      </SidebarMenuButton>
                    </SidebarMenuItem>
                  ))}
              </SidebarMenu>
            </SidebarGroup>
          ))}
        </SidebarContent>
        <SidebarFooter>
          <SidebarMenu>
            <SidebarMenuItem>
              <SidebarMenuButton asChild isActive={pathname === "/account/security"}>
                <Link href="/account/security">
                  <User />
                  <span>{t("accountSecurity")}</span>
                </Link>
              </SidebarMenuButton>
            </SidebarMenuItem>
          </SidebarMenu>
        </SidebarFooter>
      </Sidebar>
      <SidebarInset className="min-w-0 overflow-x-clip [--dashboard-header-height:--spacing(12)] [html[data-content-layout=centered]_&>*]:mx-auto [html[data-content-layout=centered]_&>*]:w-full [html[data-content-layout=centered]_&>*]:max-w-screen-2xl peer-data-[variant=inset]:border">
        <header className="flex h-12 shrink-0 items-center gap-2 border-b transition-[width,height] ease-linear group-has-data-[collapsible=icon]/sidebar-wrapper:h-12 [html[data-navbar-style=sticky]_&]:sticky [html[data-navbar-style=sticky]_&]:top-0 [html[data-navbar-style=sticky]_&]:z-50 [html[data-navbar-style=sticky]_&]:overflow-hidden [html[data-navbar-style=sticky]_&]:rounded-t-[inherit] [html[data-navbar-style=sticky]_&]:bg-background/50 [html[data-navbar-style=sticky]_&]:backdrop-blur-md">
          <div className="flex w-full items-center justify-between gap-2 px-4 lg:px-6">
            <div className="flex min-w-0 items-center gap-2">
              <SidebarTrigger />
              <Separator orientation="vertical" className="mx-2 data-[orientation=vertical]:h-4" />
              {!!auth.organizations.length && (
                <NativeSelect
                  aria-label={t("currentOrganization")}
                  value={auth.org_id || ""}
                  onChange={(event) => {
                    const org = event.target.value;
                    router.push(
                      pathname.startsWith("/management/orgs/")
                        ? pathname.replace(/^(\/management\/orgs\/)[^/]+/, `$1${org}`)
                        : `/dashboard?org_id=${org}`,
                    );
                  }}
                >
                  <NativeSelectOption value="" disabled>
                    {t("chooseOrganization")}
                  </NativeSelectOption>
                  {auth.organizations.map((org) => (
                    <NativeSelectOption key={org.id} value={org.id}>
                      {org.name}
                    </NativeSelectOption>
                  ))}
                </NativeSelect>
              )}
            </div>
            <div className="flex items-center gap-2">
              {!!pages.length && (
                <DropdownMenu>
                  <DropdownMenuTrigger asChild>
                    <Button variant="ghost" size="icon" aria-label={t("managementCenter")}>
                      <Settings />
                    </Button>
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="end">
                    <DropdownMenuGroup>
                      <DropdownMenuItem onSelect={() => router.push(pages[0].path(auth.org_id))}>
                        {t("managementCenter")}
                      </DropdownMenuItem>
                    </DropdownMenuGroup>
                  </DropdownMenuContent>
                </DropdownMenu>
              )}
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
                <DropdownMenuContent align="end">
                  <DropdownMenuGroup>
                    <DropdownMenuItem onSelect={() => router.push("/account/profile")}>{t("profile")}</DropdownMenuItem>
                    <DropdownMenuItem onSelect={() => router.push("/account/security")}>
                      {t("accountSecurity")}
                    </DropdownMenuItem>
                  </DropdownMenuGroup>
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
