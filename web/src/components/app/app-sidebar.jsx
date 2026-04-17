import { useEffect, useMemo, useState } from 'react';
import { LogOutIcon } from 'lucide-react';

import { BrandLockup } from '@/components/app/brand-logo';
import { LocaleSwitcher } from '@/components/app/locale-switcher';
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuBadge,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarSeparator,
  useSidebar
} from '@/components/ui/sidebar';
import { useI18n } from '@/i18n';
import { cn } from '@/lib/utils';
import { NAV_ITEMS, SETTINGS_NAV_ITEM } from '@/lib/workspace-constants';

const PRIMARY_NAV_IDS = new Set(['overview', 'policies', 'runners', 'jobs']);
const SECONDARY_NAV_IDS = new Set(['docs', 'runner-images', 'events']);

function resolveNavCount(itemId, policies, runners, runnerImages, jobs, logs) {
  switch (itemId) {
    case 'policies':
      return policies.length;
    case 'runners':
      return runners.length;
    case 'runner-images':
      return runnerImages.recipes.length;
    case 'jobs':
      return jobs.length;
    case 'events':
      return logs.length;
    default:
      return 0;
  }
}

function SidebarUtilityButton({
  icon: Icon,
  label,
  isActive = false,
  onClick,
  badge = 0,
  destructive = false,
  labelMode = 'stacked'
}) {
  const stacked = labelMode === 'stacked';
  const inline = labelMode === 'inline';

  return (
    <div className="relative">
      <SidebarMenuButton
        isActive={isActive}
        tooltip={label}
        aria-label={label}
        title={label}
        className={cn(
          'rounded-xl border border-transparent bg-sidebar-accent/35 shadow-none hover:bg-sidebar-accent',
          stacked
            ? 'h-auto min-h-14 flex-col justify-center gap-1.5 px-2 py-2 text-center group-data-[collapsible=icon]:size-10 group-data-[collapsible=icon]:px-0 group-data-[collapsible=icon]:py-0'
            : 'h-10 justify-start gap-2 px-3 group-data-[collapsible=icon]:size-10 group-data-[collapsible=icon]:justify-center group-data-[collapsible=icon]:px-0',
          destructive && 'text-destructive hover:bg-destructive/10 hover:text-destructive',
          isActive && !destructive && 'border-sidebar-border bg-background shadow-sm'
        )}
        onClick={onClick}
      >
        <Icon />
        <span
          className={cn(
            'font-medium text-current',
            stacked
              ? 'text-[11px] leading-4 group-data-[collapsible=icon]:hidden'
              : 'truncate text-xs group-data-[collapsible=icon]:hidden'
          )}
        >
          {label}
        </span>
        {inline && badge > 0 ? (
          <span className="ml-auto rounded-md bg-sidebar-primary px-1.5 py-0.5 text-[10px] font-semibold text-sidebar-primary-foreground group-data-[collapsible=icon]:hidden">
            {badge}
          </span>
        ) : null}
      </SidebarMenuButton>
      {stacked && badge > 0 ? (
        <span className="pointer-events-none absolute -top-1 -right-1 flex min-w-5 items-center justify-center rounded-full bg-sidebar-primary px-1.5 py-0.5 text-[10px] font-semibold text-sidebar-primary-foreground">
          {badge}
        </span>
      ) : null}
    </div>
  );
}

export function AppSidebar({ view, setView, policies, runners, runnerImages, jobs, logs, onLogout }) {
  const { t } = useI18n();
  const { isMobile, openMobile, setOpenMobile } = useSidebar();
  const [pendingView, setPendingView] = useState(null);
  const SettingsIcon = SETTINGS_NAV_ITEM.icon;
  const primaryItems = useMemo(() => NAV_ITEMS.filter((item) => PRIMARY_NAV_IDS.has(item.id)), []);
  const secondaryItems = useMemo(() => NAV_ITEMS.filter((item) => SECONDARY_NAV_IDS.has(item.id)), []);

  useEffect(() => {
    if (!isMobile || openMobile || pendingView == null) {
      return;
    }

    setView(pendingView);
    setPendingView(null);
  }, [isMobile, openMobile, pendingView, setView]);

  const handleViewSelect = (nextView) => {
    if (!isMobile) {
      setView(nextView);
      return;
    }

    if (nextView === view) {
      setOpenMobile(false);
      return;
    }

    setPendingView(nextView);
    setOpenMobile(false);
  };

  return (
    <Sidebar variant="inset" collapsible="icon" className="border-r border-sidebar-border/70">
      <SidebarHeader className="border-b px-3 py-4">
        <BrandLockup
          className="rounded-lg px-2 group-data-[collapsible=icon]:justify-center group-data-[collapsible=icon]:px-0"
          markClassName="size-10 rounded-xl border-sidebar-border/80 bg-sidebar-accent/75 p-1.5"
          titleClassName="group-data-[collapsible=icon]:hidden"
          subtitleClassName="group-data-[collapsible=icon]:hidden"
        />
      </SidebarHeader>
      <SidebarContent className="px-2 py-3">
        <SidebarGroup>
          <SidebarGroupLabel>{t('nav.group.primary')}</SidebarGroupLabel>
          <SidebarGroupContent>
            <SidebarMenu>
              {primaryItems.map((item) => {
                const Icon = item.icon;
                const count = resolveNavCount(item.id, policies, runners, runnerImages, jobs, logs);
                return (
                  <SidebarMenuItem key={item.id}>
                    <SidebarMenuButton
                      isActive={item.id === view}
                      tooltip={t(item.labelKey)}
                      onClick={() => handleViewSelect(item.id)}
                    >
                      <Icon />
                      <span>{t(item.labelKey)}</span>
                    </SidebarMenuButton>
                    {count > 0 ? <SidebarMenuBadge>{count}</SidebarMenuBadge> : null}
                  </SidebarMenuItem>
                );
              })}
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>

        <SidebarSeparator className="my-3" />

        <SidebarGroup>
          <SidebarGroupLabel>{t('nav.group.secondary')}</SidebarGroupLabel>
          <SidebarGroupContent>
            <div className="flex flex-col gap-2">
              {secondaryItems.map((item) => (
                <SidebarUtilityButton
                  key={item.id}
                  icon={item.icon}
                  label={t(item.labelKey)}
                  isActive={item.id === view}
                  badge={resolveNavCount(item.id, policies, runners, runnerImages, jobs, logs)}
                  labelMode="inline"
                  onClick={() => handleViewSelect(item.id)}
                />
              ))}
            </div>
          </SidebarGroupContent>
        </SidebarGroup>
      </SidebarContent>
      <SidebarFooter className="gap-3 border-t px-3 py-4">
        <LocaleSwitcher className="w-full group-data-[collapsible=icon]:hidden" />
        <div className="flex items-center gap-2 group-data-[collapsible=icon]:flex-col">
          <SidebarUtilityButton
            icon={SettingsIcon}
            label={t(SETTINGS_NAV_ITEM.labelKey)}
            isActive={SETTINGS_NAV_ITEM.id === view}
            labelMode="inline"
            onClick={() => handleViewSelect(SETTINGS_NAV_ITEM.id)}
          />
          <SidebarUtilityButton
            icon={LogOutIcon}
            label={t('common.signOut')}
            destructive
            labelMode="inline"
            onClick={onLogout}
          />
        </div>
      </SidebarFooter>
    </Sidebar>
  );
}
