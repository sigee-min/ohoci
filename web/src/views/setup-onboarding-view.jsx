import { BookOpenTextIcon, CheckCircle2Icon, LogOutIcon, RefreshCwIcon } from 'lucide-react';

import { BrandLockup } from '@/components/app/brand-logo';
import { LocaleSwitcher } from '@/components/app/locale-switcher';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { useI18n } from '@/i18n';
import { SETUP_STEP_META, SETUP_STEP_ORDER } from '@/lib/workspace-constants';
import { cn } from '@/lib/utils';

function badgeVariantForStep(step, isCurrentStep) {
  if (step.completed) {
    return 'default';
  }
  if (isCurrentStep) {
    return 'secondary';
  }
  return 'outline';
}

function SetupRailCard({
  setupStatus,
  activeStepId,
  currentStepId,
  completedCount,
  remainingCount,
  onSelectStep,
  refreshing,
  onRefresh,
  onLogout,
  mobile = false
}) {
  const { t } = useI18n();
  return (
    <Card className="border bg-card/92">
      <CardHeader className="border-b">
        <div className="flex flex-col gap-2">
          <BrandLockup
            markClassName="size-11 rounded-[1.05rem] p-1.5"
            titleClassName="text-base"
            subtitleClassName="text-xs"
          />
          <CardTitle>{t('setup.rail.title')}</CardTitle>
          <CardDescription>{t('setup.rail.description')}</CardDescription>
        </div>
      </CardHeader>
      <CardContent className="flex flex-col gap-4 pt-4">
        <div className="flex items-center justify-between rounded-xl border bg-background/70 px-4 py-3">
          <div>
            <p className="text-sm font-medium">{t('setup.rail.completedLabel')}</p>
            <p className="text-sm text-muted-foreground">
              {t('setup.rail.completedCount', { completed: completedCount, total: SETUP_STEP_ORDER.length })}
            </p>
          </div>
          <Badge variant={remainingCount === 0 ? 'default' : 'secondary'}>
            {remainingCount === 0 ? t('setup.rail.remaining.ready') : t('setup.rail.remaining.left', { count: remainingCount })}
          </Badge>
        </div>

        <div className="flex flex-col gap-2">
          {SETUP_STEP_ORDER.map((stepId, index) => {
            const meta = SETUP_STEP_META[stepId];
            const step = setupStatus.steps?.[stepId] || { completed: false, missing: [] };
            const Icon = meta.icon;
            const selectable = step.completed || stepId === currentStepId;

            return (
              <Button
                key={stepId}
                type="button"
                variant="ghost"
                className={cn(
                  'h-auto justify-start whitespace-normal rounded-xl border px-4 text-left',
                  mobile ? 'min-h-0 py-3' : 'py-2.5',
                  activeStepId === stepId && 'border-foreground/20 bg-accent/60',
                  !selectable && 'cursor-not-allowed opacity-70'
                )}
                onClick={() => onSelectStep(stepId)}
                disabled={!selectable}
              >
                <div className="flex w-full items-start gap-3">
                  <div className="mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-full bg-background ring-1 ring-border/70">
                    {step.completed ? <CheckCircle2Icon /> : <Icon />}
                  </div>
                  <div className="min-w-0 flex-1">
                    <div className="flex flex-wrap items-start gap-2">
                      <p className="min-w-0 flex-1 text-sm font-medium">
                        {index + 1}. {t(meta.labelKey)}
                      </p>
                      <Badge variant={badgeVariantForStep(step, stepId === currentStepId)} className="shrink-0">
                        {step.completed
                          ? t('setup.rail.step.done')
                          : stepId === currentStepId
                            ? t('setup.rail.step.required')
                            : t('setup.rail.step.locked')}
                      </Badge>
                    </div>
                  </div>
                </div>
              </Button>
            );
          })}
        </div>

        <div className="flex flex-col gap-3 border-t pt-4">
          <LocaleSwitcher />
          <div className="flex items-center gap-2">
            <Button
              type="button"
              variant="outline"
              size="icon-sm"
              onClick={onRefresh}
              disabled={refreshing}
              aria-busy={refreshing}
              aria-label={t('common.refresh')}
              title={t('common.refresh')}
            >
              <RefreshCwIcon className={cn(refreshing && 'animate-spin')} />
            </Button>
            <Button type="button" variant="outline" size="icon-sm" asChild>
              <a href="/docs" aria-label={t('common.openDocs')} title={t('common.openDocs')}>
                <BookOpenTextIcon />
              </a>
            </Button>
            <Button
              type="button"
              variant="ghost"
              size="icon-sm"
              onClick={onLogout}
              aria-label={t('common.signOut')}
              title={t('common.signOut')}
            >
              <LogOutIcon />
            </Button>
          </div>
        </div>
      </CardContent>
    </Card>
  );
}

export function SetupOnboardingView({
  setupStatus,
  activeStepId,
  currentStepId,
  onSelectStep,
  refreshing,
  onRefresh,
  onLogout,
  children
}) {
  const completedCount = SETUP_STEP_ORDER.filter((stepId) => setupStatus.steps?.[stepId]?.completed).length;
  const remainingCount = SETUP_STEP_ORDER.length - completedCount;

  return (
    <div className="bg-background">
      <div className="mx-auto flex w-full max-w-[1320px] flex-col gap-4 px-4 py-4 sm:py-5 lg:grid lg:grid-cols-[280px_minmax(0,1fr)] lg:items-start lg:gap-6 lg:px-6 xl:grid-cols-[296px_minmax(0,1fr)] xl:px-8">
        <div className="lg:hidden">
          <SetupRailCard
            setupStatus={setupStatus}
            activeStepId={activeStepId}
            currentStepId={currentStepId}
            completedCount={completedCount}
            remainingCount={remainingCount}
            onSelectStep={onSelectStep}
            refreshing={refreshing}
            onRefresh={onRefresh}
            onLogout={onLogout}
            mobile
          />
        </div>

        <aside className="hidden lg:block">
          <div className="sticky top-5">
            <SetupRailCard
              setupStatus={setupStatus}
              activeStepId={activeStepId}
              currentStepId={currentStepId}
              completedCount={completedCount}
              remainingCount={remainingCount}
              onSelectStep={onSelectStep}
              refreshing={refreshing}
              onRefresh={onRefresh}
              onLogout={onLogout}
            />
          </div>
        </aside>

        <main className="min-w-0 pb-8 lg:pb-10">
          {children}
        </main>
      </div>
    </div>
  );
}
