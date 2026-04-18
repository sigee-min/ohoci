import { BookOpenTextIcon, CheckCircle2Icon, LogOutIcon, RefreshCwIcon } from 'lucide-react';

import { BrandLockup } from '@/components/app/brand-logo';
import { LocaleSwitcher } from '@/components/app/locale-switcher';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { useI18n } from '@/i18n';
import { SETUP_FLOW_GROUPS, SETUP_FLOW_TASK_META } from '@/lib/workspace-constants';
import { cn } from '@/lib/utils';

function taskBadgeVariant(status) {
  switch (status) {
    case 'complete':
      return 'default';
    case 'current':
      return 'secondary';
    default:
      return 'outline';
  }
}

function taskStatusLabel(status, t) {
  switch (status) {
    case 'complete':
      return t('setup.shell.task.complete');
    case 'current':
      return t('setup.shell.task.current');
    default:
      return t('setup.shell.task.next');
  }
}

export function SetupShell({
  setupFlow,
  activeTaskId,
  onSelectTask,
  refreshing,
  onRefresh,
  onLogout,
  children
}) {
  const { t } = useI18n();
  const activeTask = setupFlow.tasks.find((task) => task.id === activeTaskId) || setupFlow.tasks[0] || null;
  const currentTask = setupFlow.tasks.find((task) => task.status === 'current') || activeTask;

  return (
    <div className="min-h-svh bg-background">
      <header className="border-b bg-background/90 backdrop-blur supports-[backdrop-filter]:bg-background/80">
        <div className="mx-auto flex w-full max-w-[1360px] flex-col gap-3 px-4 py-3 md:flex-row md:items-center md:justify-between md:px-6 lg:px-8">
          <BrandLockup
            className="min-w-0"
            title={t('setup.shell.title')}
            subtitle={null}
            markClassName="size-11 rounded-[1.05rem] p-1.5"
          />
          <div className="flex flex-wrap items-center gap-2">
            <LocaleSwitcher />
            <Button variant="outline" size="sm" onClick={onRefresh} disabled={refreshing} aria-busy={refreshing}>
              <RefreshCwIcon data-icon="inline-start" className={refreshing ? 'animate-spin' : undefined} />
              {t('common.refresh')}
            </Button>
            <Button variant="outline" size="sm" asChild>
              <a href="/docs">
                <BookOpenTextIcon data-icon="inline-start" />
                {t('common.openDocs')}
              </a>
            </Button>
            <Button variant="ghost" size="sm" onClick={onLogout}>
              <LogOutIcon data-icon="inline-start" />
              {t('common.signOut')}
            </Button>
          </div>
        </div>
      </header>

      <main className="mx-auto flex w-full max-w-[1360px] flex-col gap-4 px-4 py-4 sm:py-5 md:px-6 lg:grid lg:grid-cols-[292px_minmax(0,1fr)] lg:items-start lg:gap-6 lg:px-8">
        <aside className="min-w-0">
          <Card className="border bg-card/95 lg:sticky lg:top-5">
            <CardHeader className="border-b">
              <div className="space-y-2">
                <Badge variant="secondary" className="w-fit">
                  {setupFlow.blockingTasks.length === 0
                    ? t('setup.shell.ready')
                    : t('setup.shell.remainingCount', { count: setupFlow.blockingTasks.length })}
                </Badge>
                <CardTitle>{t('setup.shell.checklistTitle')}</CardTitle>
              </div>
            </CardHeader>
            <CardContent className="flex flex-col gap-4 pt-4">
              {SETUP_FLOW_GROUPS.map((group) => {
                const groupTasks = setupFlow.tasks.filter((task) => SETUP_FLOW_TASK_META[task.id]?.groupId === group.id);
                if (!groupTasks.length) {
                  return null;
                }

                return (
                  <section key={group.id} className="space-y-2">
                    <p className="text-xs font-semibold uppercase tracking-[0.18em] text-muted-foreground">
                      {t(group.titleKey)}
                    </p>
                    <div className="flex flex-col gap-2">
                      {groupTasks.map((task, index) => {
                        const meta = SETUP_FLOW_TASK_META[task.id];
                        const Icon = meta.icon;

                        return (
                          <Button
                            key={task.id}
                            type="button"
                            variant="ghost"
                            className={cn(
                              'h-auto justify-start rounded-xl border px-4 py-3 text-left whitespace-normal',
                              task.id === activeTaskId && 'border-foreground/20 bg-accent/60',
                              !task.isEditable && 'cursor-not-allowed opacity-70'
                            )}
                            disabled={!task.isEditable}
                            onClick={() => onSelectTask(task.id)}
                          >
                            <div className="flex w-full items-start gap-3">
                              <div className="mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-full bg-background ring-1 ring-border/70">
                                {task.complete ? <CheckCircle2Icon /> : <Icon />}
                              </div>
                              <div className="min-w-0 flex-1">
                                <div className="flex flex-wrap items-start gap-2">
                                  <p className="min-w-0 flex-1 text-sm font-medium">
                                    {index + 1}. {t(meta.titleKey)}
                                  </p>
                                  <Badge variant={taskBadgeVariant(task.status)} className="shrink-0">
                                    {taskStatusLabel(task.status, t)}
                                  </Badge>
                                </div>
                              </div>
                            </div>
                          </Button>
                        );
                      })}
                    </div>
                  </section>
                );
              })}
            </CardContent>
          </Card>
        </aside>

        <section className="min-w-0 space-y-4 pb-8 lg:pb-10">
          {activeTask ? (
            <div className="space-y-3">
              <div className="space-y-2">
                <div className="flex flex-wrap gap-2">
                  <Badge variant="secondary">
                    {setupFlow.blockingTasks.length === 0
                      ? t('setup.shell.ready')
                      : t('setup.shell.remainingCount', { count: setupFlow.blockingTasks.length })}
                  </Badge>
                  {currentTask && currentTask.id !== activeTask.id ? (
                    <Badge variant="outline">
                      {t('common.current')}: {t(SETUP_FLOW_TASK_META[currentTask.id].titleKey)}
                    </Badge>
                  ) : null}
                </div>
                <h1 className="text-xl font-semibold tracking-tight">{t(SETUP_FLOW_TASK_META[activeTask.id].titleKey)}</h1>
              </div>
              {children}
            </div>
          ) : children}
        </section>
      </main>
    </div>
  );
}
