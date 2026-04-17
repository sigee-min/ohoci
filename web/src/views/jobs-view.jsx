import { useState } from 'react';

import { EmptyBlock, ErrorBlock, LoadingBlock, StatCard, StatusBadge } from '@/components/app/display-primitives';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from '@/components/ui/card';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import { useI18n } from '@/i18n';
import { normalizeOperatorText } from '@/lib/operator-text';
import { compactValue, formatDateTime } from '@/lib/workspace-formatters';

const DIAGNOSTIC_STAGE_ORDER = [
  'setup_ready',
  'repo_allowed',
  'policy_match',
  'capacity_ok',
  'budget_ok',
  'warm_candidate',
  'launch_required',
  'runner_registration',
  'runner_attachment',
  'cleanup'
];

function diagnosticStateVariant(state) {
  switch (String(state || '').trim().toLowerCase()) {
    case 'blocked':
      return 'destructive';
    case 'passed':
      return 'secondary';
    case 'degraded':
      return 'outline';
    case 'skipped':
      return 'outline';
    default:
      return 'outline';
  }
}

function summarizeDiagnosticSummary(summaryCode, t) {
  return normalizeOperatorText(summaryCode, { keyPrefixes: ['operator.diagnostic.summary'] }) || t('jobs.diagnostics.summaryFallback');
}

function summarizeDiagnosticStage(stageName, t) {
  return normalizeOperatorText(stageName, { keyPrefixes: ['operator.diagnostic.stage'] }) || t('common.notSet');
}

function summarizeDiagnosticCode(code) {
  return normalizeOperatorText(code, {
    keyPrefixes: ['operator.diagnostic.code', 'operator.billing.reason', 'operator.githubDrift.issue']
  });
}

export function JobsView({
  viewState,
  jobs,
  jobDiagnosticsByJobId,
  jobDiagnosticsErrorsByJobId,
  jobDiagnosticsLoadingId,
  onLoadJobDiagnostics
}) {
  const { t } = useI18n();
  const [selectedJobId, setSelectedJobId] = useState('');
  const queuedJobs = jobs.filter((job) => String(job.status || '').toLowerCase() === 'queued').length;
  const failedJobs = jobs.filter((job) => ['failed', 'error'].includes(String(job.status || '').toLowerCase())).length;
  const repoCount = new Set(jobs.map((job) => `${job.repoOwner}/${job.repoName}`)).size;
  const selectedJob = jobs.find((job) => String(job.id || '') === selectedJobId) || null;
  const selectedDiagnostic = selectedJobId ? jobDiagnosticsByJobId?.[selectedJobId] || selectedJob?.diagnostic || null : null;
  const selectedDiagnosticError = selectedJobId ? jobDiagnosticsErrorsByJobId?.[selectedJobId] : '';
  const diagnosticsLoading = Boolean(selectedJobId) && jobDiagnosticsLoadingId === selectedJobId;

  async function handleDiagnosticsClick(job) {
    const targetJobId = String(job?.id || '').trim();
    if (!targetJobId) {
      return;
    }

    if (selectedJobId === targetJobId) {
      setSelectedJobId('');
      return;
    }

    setSelectedJobId(targetJobId);

    if (!jobDiagnosticsByJobId?.[targetJobId] && !job?.diagnostic && typeof onLoadJobDiagnostics === 'function') {
      await onLoadJobDiagnostics(targetJobId);
    }
  }

  return (
    <div className="flex flex-col gap-6">
      <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
        <StatCard label={t('jobs.stats.total.label')} value={jobs.length} note={t('jobs.stats.total.note')} />
        <StatCard
          label={t('jobs.stats.queued.label')}
          value={queuedJobs}
          note={t('jobs.stats.queued.note')}
          accent={queuedJobs > 0}
        />
        <StatCard
          label={t('jobs.stats.failures.label')}
          value={failedJobs}
          note={t('jobs.stats.failures.note')}
          accent={failedJobs > 0}
        />
        <StatCard
          label={t('jobs.stats.repositories.label')}
          value={repoCount}
          note={t('jobs.stats.repositories.note')}
        />
      </div>

      <Card className="border bg-card/95">
        <CardHeader className="border-b">
          <div>
            <CardTitle>{t('jobs.title')}</CardTitle>
            <CardDescription>{t('jobs.description')}</CardDescription>
          </div>
        </CardHeader>
        <CardContent className="flex flex-col gap-4 pt-4">
          {viewState?.status === 'loading' ? (
            <LoadingBlock
              title={t('viewState.loading.title', { view: t('jobs.title') })}
              body={t('viewState.loading.body', { view: t('jobs.title') })}
            />
          ) : viewState?.status === 'error' ? (
            <ErrorBlock
              title={t('viewState.error.title', { view: t('jobs.title') })}
              body={viewState.error || t('viewState.error.body')}
            />
          ) : viewState?.status === 'empty' ? (
            <EmptyBlock title={t('jobs.empty.title')} body={t('jobs.empty.body')} />
          ) : (
            <>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t('jobs.table.job')}</TableHead>
                    <TableHead>{t('jobs.table.status')}</TableHead>
                    <TableHead>{t('jobs.table.repository')}</TableHead>
                    <TableHead>{t('jobs.table.labels')}</TableHead>
                    <TableHead>{t('jobs.table.diagnostics')}</TableHead>
                    <TableHead>{t('jobs.table.error')}</TableHead>
                    <TableHead className="text-right">{t('jobs.table.actions')}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {jobs.map((job) => {
                    const jobId = String(job.id || '').trim();
                    const diagnostic = jobDiagnosticsByJobId?.[jobId] || job.diagnostic || null;
                    const diagnosticSummaryCode = diagnostic?.summaryCode || job.diagnosticSummaryCode;
                    const diagnosticBlockingStage = diagnostic?.blockingStage || job.diagnosticBlockingStage;
                    const diagnosticBlockingMessage = diagnostic?.blockingMessage || job.diagnosticBlockingMessage;
                    const diagnosticsKnown = Boolean(diagnosticSummaryCode || diagnosticBlockingStage);
                    const selected = selectedJobId === jobId;

                    return (
                      <TableRow key={job.id}>
                        <TableCell className="font-medium">{t('jobs.table.jobValue', { jobId: job.githubJobId })}</TableCell>
                        <TableCell><StatusBadge value={job.status} /></TableCell>
                        <TableCell>{job.repoOwner}/{job.repoName}</TableCell>
                        <TableCell className="max-w-72 whitespace-normal">
                          <div className="flex flex-wrap gap-1">
                            {(job.labels || []).map((label) => (
                              <Badge key={label} variant="outline">{label}</Badge>
                            ))}
                          </div>
                        </TableCell>
                        <TableCell className="max-w-72 whitespace-normal">
                          {diagnosticsKnown ? (
                            <div className="flex flex-col gap-2">
                              <span className="text-sm font-medium text-foreground">
                                {summarizeDiagnosticSummary(diagnosticSummaryCode, t)}
                              </span>
                              {diagnosticBlockingStage ? (
                                <Badge variant="outline" className="w-fit">
                                  {summarizeDiagnosticStage(diagnosticBlockingStage, t)}
                                </Badge>
                              ) : (
                                <span className="text-xs text-muted-foreground">{t('jobs.table.diagnosticsReady')}</span>
                              )}
                            </div>
                          ) : (
                            <span className="text-sm text-muted-foreground">{t('jobs.table.diagnosticsFallback')}</span>
                          )}
                        </TableCell>
                        <TableCell className="max-w-72 whitespace-normal text-sm text-muted-foreground">
                          {diagnosticBlockingMessage || job.errorMessage || t('jobs.table.errorFallback')}
                        </TableCell>
                        <TableCell className="text-right">
                          <Button
                            variant={selected ? 'secondary' : 'outline'}
                            size="sm"
                            onClick={() => void handleDiagnosticsClick(job)}
                            disabled={!jobId}
                            aria-busy={jobDiagnosticsLoadingId === jobId}
                          >
                            {selected ? t('jobs.actions.hideDiagnostics') : t('jobs.actions.viewDiagnostics')}
                          </Button>
                        </TableCell>
                      </TableRow>
                    );
                  })}
                </TableBody>
              </Table>

              {selectedJob ? (
                <div className="rounded-xl border bg-background/70">
                  <div className="border-b px-4 py-3">
                    <div className="flex flex-wrap items-start justify-between gap-3">
                      <div>
                        <p className="text-sm font-medium">
                          {t('jobs.diagnostics.title', { jobId: selectedJob.githubJobId })}
                        </p>
                        <p className="text-sm text-muted-foreground">
                          {selectedJob.repoOwner}/{selectedJob.repoName}
                        </p>
                      </div>
                      <StatusBadge value={selectedJob.status} />
                    </div>
                  </div>

                  <div className="flex flex-col gap-4 px-4 py-4">
                    {diagnosticsLoading ? (
                      <LoadingBlock
                        title={t('jobs.diagnostics.loading.title')}
                        body={t('jobs.diagnostics.loading.body')}
                      />
                    ) : null}

                    {!diagnosticsLoading && selectedDiagnosticError ? (
                      <ErrorBlock
                        title={t('jobs.diagnostics.error.title')}
                        body={selectedDiagnosticError}
                      />
                    ) : null}

                    {!diagnosticsLoading && !selectedDiagnosticError && !selectedDiagnostic ? (
                      <EmptyBlock
                        title={t('jobs.diagnostics.empty.title')}
                        body={t('jobs.diagnostics.empty.body')}
                      />
                    ) : null}

                    {!diagnosticsLoading && !selectedDiagnosticError && selectedDiagnostic ? (
                      <>
                        <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-4">
                          <div className="rounded-xl border bg-card/80 px-4 py-3">
                            <p className="text-sm font-medium">{t('jobs.diagnostics.summary.summaryCode')}</p>
                            <p className="mt-1 text-sm text-muted-foreground">
                              {summarizeDiagnosticSummary(selectedDiagnostic.summaryCode, t)}
                            </p>
                          </div>
                          <div className="rounded-xl border bg-card/80 px-4 py-3">
                            <p className="text-sm font-medium">{t('jobs.diagnostics.summary.blockingStage')}</p>
                            <p className="mt-1 text-sm text-muted-foreground">
                              {selectedDiagnostic.blockingStage
                                ? summarizeDiagnosticStage(selectedDiagnostic.blockingStage, t)
                                : t('jobs.diagnostics.summary.none')}
                            </p>
                          </div>
                          <div className="rounded-xl border bg-card/80 px-4 py-3">
                            <p className="text-sm font-medium">{t('jobs.diagnostics.summary.runner')}</p>
                            <p className="mt-1 text-sm text-muted-foreground">
                              {selectedDiagnostic.runnerId || t('common.notSet')}
                            </p>
                          </div>
                          <div className="rounded-xl border bg-card/80 px-4 py-3">
                            <p className="text-sm font-medium">{t('jobs.diagnostics.summary.instance')}</p>
                            <p className="mt-1 font-mono text-xs text-muted-foreground">
                              {compactValue(selectedDiagnostic.instanceOcid)}
                            </p>
                          </div>
                        </div>

                        <div className="grid gap-3 md:grid-cols-2">
                          {DIAGNOSTIC_STAGE_ORDER
                            .filter((stageName) => selectedDiagnostic.stageStatuses?.[stageName])
                            .map((stageName) => {
                              const stage = selectedDiagnostic.stageStatuses[stageName];

                              return (
                                <div key={stageName} className="rounded-xl border bg-card/80 px-4 py-4">
                                  <div className="flex items-center justify-between gap-3">
                                    <p className="text-sm font-medium">{summarizeDiagnosticStage(stageName, t)}</p>
                                    <Badge variant={diagnosticStateVariant(stage.state)}>
                                      {normalizeOperatorText(stage.state, { keyPrefixes: ['formatter.status'] })}
                                    </Badge>
                                  </div>
                                  <p className="mt-2 text-sm text-muted-foreground">{stage.message || t('common.notSet')}</p>
                                  {stage.code ? (
                                    <p className="mt-2 text-xs text-muted-foreground">
                                      {summarizeDiagnosticCode(stage.code) || stage.code}
                                    </p>
                                  ) : null}
                                  <p className="mt-2 text-xs text-muted-foreground">{formatDateTime(stage.updatedAt)}</p>
                                </div>
                              );
                            })}
                        </div>
                      </>
                    ) : null}
                  </div>
                </div>
              ) : null}
            </>
          )}
        </CardContent>
        <CardFooter className="justify-between gap-3 text-sm text-muted-foreground">
          <p>{t('jobs.footer.primary')}</p>
          <p>
            {failedJobs > 0
              ? t('jobs.footer.failures', { count: failedJobs, suffix: failedJobs === 1 ? '' : 's' })
              : t('jobs.footer.noFailures')}
          </p>
        </CardFooter>
      </Card>
    </div>
  );
}
