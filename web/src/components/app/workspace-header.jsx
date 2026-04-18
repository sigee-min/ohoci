import { Clock3Icon, RefreshCwIcon } from "lucide-react";

import { BrandMark } from "@/components/app/brand-logo";
import { BusyButtonContent } from "@/components/app/busy-button-content";
import { LocaleSwitcher } from "@/components/app/locale-switcher";
import { Button } from "@/components/ui/button";
import { SidebarTrigger } from "@/components/ui/sidebar";
import { useI18n } from "@/i18n";

export function WorkspaceHeader({
  currentView,
  ociAuthStatus,
  refreshing,
  onRefresh,
  onCleanup,
}) {
  const { t } = useI18n();

  if (currentView?.id === "docs") {
    return (
      <header className="sticky top-0 z-10 border-b bg-background/90 backdrop-blur supports-[backdrop-filter]:bg-background/80">
        <div className="mx-auto flex w-full max-w-[1360px] flex-col gap-3 px-4 py-3 md:px-6 lg:px-8">
          <div className="flex min-w-0 flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
            <div className="flex min-w-0 items-start gap-3">
              <SidebarTrigger className="-ml-1 mt-0.5 shrink-0" />
              <BrandMark className="mt-0.5 size-9 rounded-xl p-1.5 md:hidden" />
              <div className="min-w-0 flex flex-1 flex-col">
                <h1 className="text-lg font-semibold tracking-tight sm:text-xl">
                  {t(currentView.labelKey)}
                </h1>
              </div>
            </div>
            <LocaleSwitcher className="self-start" />
          </div>
        </div>
      </header>
    );
  }

  return (
    <header className="sticky top-0 z-10 border-b bg-background/90 backdrop-blur supports-[backdrop-filter]:bg-background/80">
      <div className="mx-auto flex w-full max-w-[1360px] flex-col gap-3 px-4 py-3 md:px-6 lg:px-8">
        <div className="flex min-w-0 items-start gap-3">
          <SidebarTrigger className="-ml-1 mt-0.5 shrink-0" />
          <BrandMark className="mt-0.5 size-9 rounded-xl p-1.5 md:hidden" />
          <div className="min-w-0 flex flex-1 flex-col">
            <h1 className="text-lg font-semibold tracking-tight sm:text-xl">
              {t(currentView.labelKey)}
            </h1>
          </div>
        </div>
        <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-end">
          <LocaleSwitcher className="w-fit" />
          <Button
            variant="outline"
            size="sm"
            onClick={onRefresh}
            disabled={refreshing}
            aria-busy={refreshing}
          >
            <BusyButtonContent
              busy={refreshing}
              label={t("common.refresh")}
              icon={RefreshCwIcon}
              busyIcon={RefreshCwIcon}
              spin
            />
          </Button>
          <Button variant="outline" size="sm" onClick={onCleanup}>
            <Clock3Icon data-icon="inline-start" />
            {t("workspaceHeader.cleanup")}
          </Button>
        </div>
      </div>
    </header>
  );
}
