import { EmptyBlock, ErrorBlock, LoadingBlock, StatCard, StatusBadge } from '@/components/app/display-primitives';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from '@/components/ui/card';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import { useI18n } from '@/i18n';
import { compactValue, formatDateTime } from '@/lib/workspace-formatters';

export function RunnersView({ viewState, runners, onTerminateRunner }) {
  const { t } = useI18n();
  const activeRunners = runners.filter((runner) => ['active', 'running', 'launching'].includes(String(runner.status || '').toLowerCase())).length;
  const expiringSoon = runners.filter((runner) => {
    const timestamp = new Date(runner.expiresAt).getTime();
    const remaining = timestamp - Date.now();
    return Number.isFinite(timestamp) && remaining >= 0 && remaining <= 60 * 60 * 1000;
  }).length;
  const repoCount = new Set(runners.map((runner) => `${runner.repoOwner}/${runner.repoName}`)).size;

  return (
    <div className="flex flex-col gap-6">
      <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
        <StatCard label={t('runners.stats.total.label')} value={runners.length} note={t('runners.stats.total.note')} />
        <StatCard
          label={t('runners.stats.active.label')}
          value={activeRunners}
          note={t('runners.stats.active.note')}
          accent={activeRunners > 0}
        />
        <StatCard
          label={t('runners.stats.expiringSoon.label')}
          value={expiringSoon}
          note={t('runners.stats.expiringSoon.note')}
          accent={expiringSoon > 0}
        />
        <StatCard
          label={t('runners.stats.repositories.label')}
          value={repoCount}
          note={t('runners.stats.repositories.note')}
        />
      </div>

      <Card className="border bg-card/95">
        <CardHeader className="border-b">
          <div>
            <CardTitle>{t('runners.title')}</CardTitle>
            <CardDescription>{t('runners.description')}</CardDescription>
          </div>
        </CardHeader>
        <CardContent className="flex flex-col gap-4 pt-4">
          {viewState?.status === 'loading' ? (
            <LoadingBlock
              title={t('viewState.loading.title', { view: t('runners.title') })}
              body={t('viewState.loading.body', { view: t('runners.title') })}
            />
          ) : viewState?.status === 'error' ? (
            <ErrorBlock
              title={t('viewState.error.title', { view: t('runners.title') })}
              body={viewState.error || t('viewState.error.body')}
            />
          ) : viewState?.status === 'empty' ? (
            <EmptyBlock title={t('runners.empty.title')} body={t('runners.empty.body')} />
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('runners.table.name')}</TableHead>
                  <TableHead>{t('runners.table.status')}</TableHead>
                  <TableHead>{t('runners.table.repository')}</TableHead>
                  <TableHead>{t('runners.table.lifecycle')}</TableHead>
                  <TableHead className="text-right">{t('runners.table.actions')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {runners.map((runner) => (
                  <TableRow key={runner.id}>
                    <TableCell className="font-medium">{runner.runnerName}</TableCell>
                    <TableCell><StatusBadge value={runner.status} /></TableCell>
                      <TableCell>{runner.repoOwner}/{runner.repoName}</TableCell>
                      <TableCell className="max-w-72 whitespace-normal">
                        <div className="flex flex-col gap-1">
                          <span>{formatDateTime(runner.expiresAt)}</span>
                          <span className="font-mono text-xs text-muted-foreground">{compactValue(runner.instanceOcid)}</span>
                          <span className="text-xs text-muted-foreground">
                            {t('runners.table.sourceValue', {
                              source: String(runner.source || '').trim() || 'ondemand'
                            })}
                          </span>
                          {String(runner.source || '').toLowerCase() === 'warm' ? (
                            <>
                              <span className="text-xs text-muted-foreground">
                                {t('runners.table.warmStateValue', {
                                  state: String(runner.warmState || '').trim() || t('common.notSet')
                                })}
                              </span>
                              <span className="text-xs text-muted-foreground">
                                {t('runners.table.warmTargetValue', {
                                  target: [runner.warmRepoOwner || runner.repoOwner, runner.warmRepoName || runner.repoName]
                                    .filter(Boolean)
                                    .join('/') || t('common.notSet')
                                })}
                              </span>
                              <span className="text-xs text-muted-foreground">
                                {t('runners.table.warmPolicyValue', {
                                  id: runner.warmPolicyId || t('common.notSet')
                                })}
                              </span>
                            </>
                          ) : null}
                        </div>
                      </TableCell>
                    <TableCell>
                      <div className="flex justify-end">
                        <Button variant="ghost" size="sm" onClick={() => void onTerminateRunner(runner.id)}>
                          {t('runners.actions.terminate')}
                        </Button>
                      </div>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
        <CardFooter className="justify-between gap-3 text-sm text-muted-foreground">
          <p>{t('runners.footer.primary')}</p>
          <p>
            {activeRunners > 0
              ? t('runners.footer.active', { count: activeRunners })
              : t('runners.footer.none')}
          </p>
        </CardFooter>
      </Card>
    </div>
  );
}
