import { ActivityList, EmptyBlock, ErrorBlock, LoadingBlock, StatusBadge } from '@/components/app/display-primitives';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Separator } from '@/components/ui/separator';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { formatTranslatedList, useI18n } from '@/i18n';
import { normalizeOperatorList, normalizeOperatorText } from '@/lib/operator-text';
import {
  compactValue,
  describeSubnet,
  formatCurrency,
  formatDateTime,
  formatNumber,
  summarizeOCIAuthMode
} from '@/lib/workspace-formatters';

function getOverviewState({ enabledPolicies, liveRunners, queuedJobs, errorLogs, ociAuthStatus }, t) {
  if (!ociAuthStatus.runtimeConfigReady) {
    return {
      title: t('overview.state.setup.title'),
      description: t('overview.state.setup.description')
    };
  }

  if (!enabledPolicies.length) {
    return {
      title: t('overview.state.policies.title'),
      description: t('overview.state.policies.description')
    };
  }

  if (errorLogs.length) {
    return {
      title: t('overview.state.errors.title'),
      description: t('overview.state.errors.description')
    };
  }

  if (queuedJobs.length > liveRunners.length && queuedJobs.length > 0) {
    return {
      title: t('overview.state.queued.title'),
      description: t('overview.state.queued.description')
    };
  }

  if (liveRunners.length) {
    return {
      title: t('overview.state.runners.title'),
      description: t('overview.state.runners.description')
    };
  }

  return {
    title: t('overview.state.ready.title'),
    description: t('overview.state.ready.description')
  };
}

function buildOverviewDecisionModel({ enabledPolicies, liveRunners, queuedJobs, errorLogs, ociAuthStatus }, t) {
  const runtimeMissing = formatTranslatedList((ociAuthStatus.runtimeConfigMissing || []).filter(Boolean));

  if (!ociAuthStatus.runtimeConfigReady) {
    return {
      primaryAction: {
        view: 'settings',
        label: t('overview.actions.settings'),
        note: t('overview.decision.primary.settings')
      },
      secondaryAction: {
        view: 'policies',
        label: t('overview.actions.policies'),
        note: t('overview.decision.secondary.policies')
      },
      incident: {
        tone: 'warning',
        label: t('overview.incident.label.blocked'),
        title: t('overview.incident.setup.title'),
        body: runtimeMissing
          ? t('overview.incident.setup.body.withList', { items: runtimeMissing })
          : t('overview.incident.setup.body')
      }
    };
  }

  if (!enabledPolicies.length) {
    return {
      primaryAction: {
        view: 'policies',
        label: t('overview.actions.policies'),
        note: t('overview.decision.primary.policies')
      },
      secondaryAction: {
        view: 'settings',
        label: t('overview.actions.settings'),
        note: t('overview.decision.secondary.settings')
      },
      incident: {
        tone: 'warning',
        label: t('overview.incident.label.policyGap'),
        title: t('overview.incident.policies.title'),
        body: t('overview.incident.policies.body')
      }
    };
  }

  if (errorLogs.length) {
    const latestError = errorLogs[0]?.message || '';
    return {
      primaryAction: {
        view: 'events',
        label: t('overview.actions.events'),
        note: t('overview.decision.primary.events')
      },
      secondaryAction: {
        view: 'jobs',
        label: t('overview.actions.jobs'),
        note: t('overview.decision.secondary.jobs')
      },
      incident: {
        tone: 'critical',
        label: t('overview.incident.label.risk'),
        title: t('overview.incident.errors.title'),
        body: latestError
          ? t('overview.incident.errors.body.withMessage', {
            count: errorLogs.length,
            message: latestError
          })
          : t('overview.incident.errors.body', { count: errorLogs.length })
      }
    };
  }

  if (queuedJobs.length > liveRunners.length && queuedJobs.length > 0) {
    return {
      primaryAction: {
        view: 'jobs',
        label: t('overview.actions.jobs'),
        note: t('overview.decision.primary.jobs')
      },
      secondaryAction: {
        view: 'runners',
        label: t('overview.actions.runners'),
        note: t('overview.decision.secondary.runners')
      },
      incident: {
        tone: 'warning',
        label: t('overview.incident.label.backlog'),
        title: t('overview.incident.queued.title'),
        body: t('overview.incident.queued.body', {
          queuedCount: queuedJobs.length,
          runnerCount: liveRunners.length
        })
      }
    };
  }

  if (liveRunners.length) {
    return {
      primaryAction: {
        view: 'runners',
        label: t('overview.actions.runners'),
        note: t('overview.decision.primary.runners')
      },
      secondaryAction: {
        view: 'jobs',
        label: t('overview.actions.jobs'),
        note: t('overview.decision.secondary.jobsWatch')
      },
      incident: {
        tone: 'success',
        label: t('overview.incident.label.active'),
        title: t('overview.incident.runners.title'),
        body: t('overview.incident.runners.body', { count: liveRunners.length })
      }
    };
  }

  return {
    primaryAction: {
      view: 'jobs',
      label: t('overview.actions.jobs'),
      note: t('overview.decision.primary.jobsWatch')
    },
    secondaryAction: {
      view: 'policies',
      label: t('overview.actions.policies'),
      note: t('overview.decision.secondary.policiesTuning')
    },
    incident: {
      tone: 'success',
      label: t('overview.incident.label.ready'),
      title: t('overview.incident.ready.title'),
      body: t('overview.incident.ready.body')
    }
  };
}

function incidentToneClassName(tone) {
  switch (tone) {
    case 'critical':
      return 'border-destructive/25 bg-destructive/6';
    case 'warning':
      return 'border-amber-500/20 bg-amber-500/8';
    case 'success':
      return 'border-emerald-500/20 bg-emerald-500/8';
    default:
      return 'border-border/70 bg-muted/15';
  }
}

function buildOperationalIncidents({
  blockedGuardrailItems,
  githubDriftStatus,
  warmPoolStatus,
  cacheCompatStatus
}, t) {
  const incidents = [];

  if (blockedGuardrailItems.length) {
    incidents.push({
      key: 'budget_blocked',
      tone: 'critical',
      label: t('overview.incident.label.budgetBlocked'),
      title: t('overview.incident.budgetBlocked.title'),
      body: t('overview.incident.budgetBlocked.body', {
        count: blockedGuardrailItems.length,
        policy: blockedGuardrailItems[0]?.policyLabel || t('common.notSet')
      })
    });
  }

  if (githubDriftStatus?.available !== false && githubDriftStatus?.issues?.length) {
    const firstIssue = githubDriftStatus.issues[0];
    incidents.push({
      key: 'drift_detected',
      tone: githubDriftStatus.severity === 'critical' ? 'critical' : 'warning',
      label: t('overview.incident.label.driftDetected'),
      title: t('overview.incident.driftDetected.title'),
      body: t('overview.incident.driftDetected.body', {
        count: githubDriftStatus.issues.length,
        issue: normalizeOperatorText(firstIssue?.code, { keyPrefixes: ['operator.githubDrift.issue'] }) || t('common.notSet')
      })
    });
  }

  if (warmPoolStatus?.degradedTargets?.length) {
    incidents.push({
      key: 'warm_degraded',
      tone: 'warning',
      label: t('overview.incident.label.warmDegraded'),
      title: t('overview.incident.warmDegraded.title'),
      body: t('overview.incident.warmDegraded.body', {
        count: warmPoolStatus.degradedTargets.length,
        missing: warmPoolStatus.degradedTargets.reduce((total, item) => total + item.missingIdle, 0)
      })
    });
  }

  if (cacheCompatStatus?.incident) {
    incidents.push({
      key: 'cache_unavailable',
      tone: 'warning',
      label: t('overview.incident.label.cacheUnavailable'),
      title: t('overview.incident.cacheUnavailable.title'),
      body: t('overview.incident.cacheUnavailable.body.incomplete')
    });
  }

  return incidents;
}

function buildOperationalHeadline(incident, t) {
  switch (incident?.key) {
    case 'budget_blocked':
      return {
        title: t('overview.state.budget.title'),
        description: t('overview.state.budget.description')
      };
    case 'drift_detected':
      return {
        title: t('overview.state.drift.title'),
        description: t('overview.state.drift.description')
      };
    case 'warm_degraded':
      return {
        title: t('overview.state.warm.title'),
        description: t('overview.state.warm.description')
      };
    case 'cache_unavailable':
      return {
        title: t('overview.state.cache.title'),
        description: t('overview.state.cache.description')
      };
    default:
      return null;
  }
}

function buildOperationalDecisionModel(incident, t) {
  switch (incident?.key) {
    case 'budget_blocked':
      return {
        primaryAction: {
          view: 'policies',
          label: t('overview.actions.policies'),
          note: t('overview.decision.primary.budgetPolicies')
        },
        secondaryAction: {
          view: 'jobs',
          label: t('overview.actions.jobs'),
          note: t('overview.decision.secondary.budgetJobs')
        },
        incident
      };
    case 'drift_detected':
      return {
        primaryAction: {
          view: 'settings',
          label: t('overview.actions.settings'),
          note: t('overview.decision.primary.githubSettings')
        },
        secondaryAction: {
          view: 'jobs',
          label: t('overview.actions.jobs'),
          note: t('overview.decision.secondary.jobs')
        },
        incident
      };
    case 'warm_degraded':
      return {
        primaryAction: {
          view: 'policies',
          label: t('overview.actions.policies'),
          note: t('overview.decision.primary.warmPolicies')
        },
        secondaryAction: {
          view: 'runners',
          label: t('overview.actions.runners'),
          note: t('overview.decision.secondary.runners')
        },
        incident
      };
    case 'cache_unavailable':
      return {
        primaryAction: {
          view: 'settings',
          label: t('overview.actions.settings'),
          note: t('overview.decision.primary.cacheSettings')
        },
        secondaryAction: {
          view: 'jobs',
          label: t('overview.actions.jobs'),
          note: t('overview.decision.secondary.jobsWatch')
        },
        incident
      };
    default:
      return null;
  }
}

function DecisionActionCard({ emphasis = 'secondary', action, onNavigate, title }) {
  return (
    <div className="rounded-xl border bg-background/75 p-4">
      <p className="text-[11px] font-medium uppercase tracking-[0.16em] text-muted-foreground">{title}</p>
      <p className="mt-2 text-base font-semibold tracking-tight">{action.label}</p>
      <p className="mt-2 text-sm leading-6 text-muted-foreground">{action.note}</p>
      <Button
        className="mt-4 w-full"
        variant={emphasis === 'primary' ? 'default' : 'outline'}
        onClick={() => onNavigate(action.view)}
      >
        {action.label}
      </Button>
    </div>
  );
}

export function OverviewView({
  viewState,
  enabledPolicies,
  liveRunners,
  queuedJobs,
  errorLogs,
  billingReport,
  billingGuardrails,
  blockedGuardrailItems,
  overviewRunnerItems,
  overviewJobItems,
  overviewLogItems,
  githubDriftStatus,
  ociAuthStatus,
  warmPoolStatus,
  cacheCompatStatus,
  recommendedSubnets,
  subnetError,
  subnetById,
  defaultSubnetId,
  onNavigate
}) {
  const { t } = useI18n();
  const overviewState = getOverviewState(
    {
      enabledPolicies,
      liveRunners,
      queuedJobs,
      errorLogs,
      ociAuthStatus
    },
    t
  );
  const decisionModel = buildOverviewDecisionModel(
    {
      enabledPolicies,
      liveRunners,
      queuedJobs,
      errorLogs,
      ociAuthStatus
    },
    t
  );
  const operationalIncidents = buildOperationalIncidents(
    {
      blockedGuardrailItems,
      githubDriftStatus,
      warmPoolStatus,
      cacheCompatStatus
    },
    t
  );
  const operationalHeadline = buildOperationalHeadline(operationalIncidents[0], t);
  const operationalDecisionModel = buildOperationalDecisionModel(operationalIncidents[0], t);
  const displayedOverviewState = operationalHeadline || overviewState;
  const displayedDecisionModel = operationalDecisionModel || decisionModel;

  const snapshotItems = [
    {
      label: t('overview.snapshot.enabledPolicies.label'),
      value: enabledPolicies.length,
      note: enabledPolicies.length ? t('overview.snapshot.enabledPolicies.note.ready') : t('overview.snapshot.enabledPolicies.note.empty')
    },
    {
      label: t('overview.snapshot.liveRunners.label'),
      value: liveRunners.length,
      note: liveRunners.length ? t('overview.snapshot.liveRunners.note.active') : t('overview.snapshot.liveRunners.note.idle')
    },
    {
      label: t('overview.snapshot.queuedJobs.label'),
      value: queuedJobs.length,
      note: queuedJobs.length ? t('overview.snapshot.queuedJobs.note.waiting') : t('overview.snapshot.queuedJobs.note.clear')
    },
    {
      label: t('overview.snapshot.errorLogs.label'),
      value: errorLogs.length,
      note: errorLogs.length ? t('overview.snapshot.errorLogs.note.checkSoon') : t('overview.snapshot.errorLogs.note.quiet')
    }
  ];

  const billingCurrency = billingReport.currency || 'USD';
  const trackedCost = billingReport.totalCost;
  const ociBilledCost = billingReport.ociBilledCost;
  const nonTrackedCost = Math.max(ociBilledCost - trackedCost, 0);
  const coverageRatio = ociBilledCost > 0 ? trackedCost / ociBilledCost : null;
  const billingPrimaryStats = [
    {
      label: t('overview.billing.stats.ociBilledCost.label'),
      value: formatCurrency(ociBilledCost, billingCurrency),
      note: billingReport.loaded
        ? t('overview.billing.stats.ociBilledCost.note.window', { days: billingReport.days })
        : t('overview.billing.stats.ociBilledCost.note.pending')
    },
    {
      label: t('overview.billing.stats.trackedCost.label'),
      value: formatCurrency(trackedCost, billingCurrency),
      note: billingReport.loaded
        ? t('overview.billing.stats.trackedCost.note.window', { days: billingReport.days })
        : t('overview.billing.stats.trackedCost.note.pending')
    }
  ];
  const billingSecondaryStats = [
    {
      label: t('overview.billing.stats.nonTrackedCost.label'),
      value: formatCurrency(nonTrackedCost, billingCurrency),
      note: nonTrackedCost > 0
        ? t('overview.billing.stats.nonTrackedCost.note.active')
        : t('overview.billing.stats.nonTrackedCost.note.clear')
    },
    {
      label: t('overview.billing.stats.coverage.label'),
      value: coverageRatio == null ? '—' : `${formatNumber(coverageRatio * 100, 1)}%`,
      note: coverageRatio == null
        ? t('overview.billing.stats.coverage.note.empty')
        : t('overview.billing.stats.coverage.note.ready')
    }
  ];
  const billingAttributionStats = [
    {
      label: t('overview.billing.stats.tagVerified.label'),
      value: formatCurrency(billingReport.tagVerifiedCost, billingCurrency),
      note: billingReport.tagAttributionReady
        ? t('overview.billing.stats.tagVerified.note.ready')
        : t('overview.billing.stats.tagVerified.note.pending')
    },
    {
      label: t('overview.billing.stats.resourceFallback.label'),
      value: formatCurrency(billingReport.resourceFallbackCost, billingCurrency),
      note: t('overview.billing.stats.resourceFallback.note')
    },
    {
      label: t('overview.billing.stats.unmapped.label'),
      value: formatCurrency(billingReport.unmappedCost, billingCurrency),
      note: billingReport.issues.length
        ? t('overview.billing.stats.unmapped.note.issues', {
          count: billingReport.issues.length,
          suffix: billingReport.issues.length === 1 ? '' : 's'
        })
        : t('overview.billing.stats.unmapped.note.clear')
    }
  ];

  const tagBasisValue = billingReport.tagAttributionReady
    ? `${billingReport.tagNamespace}/${billingReport.tagKey}`
    : t('overview.billing.notes.tagBasis.fallback');
  const billingOperationalNotes = [
    billingReport.lagNotice || t('overview.billing.notes.lagFallback'),
    billingReport.scopeNote || t('overview.billing.notes.scopeFallback'),
    t('overview.billing.notes.scopeComparison'),
    t('overview.billing.notes.tagBasis', { value: tagBasisValue }),
    billingReport.sourceRegion ? t('overview.billing.notes.sourceRegion', { value: billingReport.sourceRegion }) : null
  ].filter(Boolean);

  return (
    <div className="flex flex-col gap-6">
      <div className="grid gap-6 xl:grid-cols-[minmax(0,1.25fr)_minmax(320px,0.75fr)]">
        <Card className="border bg-card/95">
          <CardHeader className="border-b">
            <div className="flex flex-col gap-1">
              <CardTitle>{t('overview.title')}</CardTitle>
              <CardDescription>{t('overview.description')}</CardDescription>
            </div>
          </CardHeader>
          <CardContent className="flex flex-col gap-6 pt-5">
            <div className="grid gap-6 lg:grid-cols-[minmax(0,1.15fr)_minmax(260px,0.85fr)]">
              {viewState?.status === 'loading' ? (
                <LoadingBlock
                  title={t('viewState.loading.title', { view: t('overview.title') })}
                  body={t('viewState.loading.body', { view: t('overview.title') })}
                  className="lg:col-span-2"
                />
              ) : viewState?.status === 'error' ? (
                <ErrorBlock
                  title={t('viewState.error.title', { view: t('overview.title') })}
                  body={viewState.error || t('viewState.error.body')}
                  className="lg:col-span-2"
                />
              ) : viewState?.status === 'empty' ? (
                <EmptyBlock
                  title={t('overview.state.ready.title')}
                  body={t('overview.state.ready.description')}
                  className="lg:col-span-2"
                />
              ) : (
                <>
                  <div className="flex flex-col gap-5">
                    <div className="flex flex-col gap-4">
                      <div className="flex flex-col gap-2">
                        <p className="text-3xl font-semibold tracking-tight text-balance">{displayedOverviewState.title}</p>
                        <p className="max-w-2xl text-sm leading-6 text-muted-foreground">{displayedOverviewState.description}</p>
                      </div>

                      <div className={`rounded-xl border px-4 py-4 ${incidentToneClassName(displayedDecisionModel.incident.tone)}`}>
                        <p className="text-[11px] font-medium uppercase tracking-[0.16em] text-muted-foreground">
                          {displayedDecisionModel.incident.label}
                        </p>
                        <p className="mt-2 text-base font-semibold tracking-tight">{displayedDecisionModel.incident.title}</p>
                        <p className="mt-2 text-sm leading-6 text-muted-foreground">{displayedDecisionModel.incident.body}</p>
                      </div>
                    </div>

                    <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
                      {snapshotItems.map((item) => (
                        <div key={item.label} className="rounded-xl bg-muted/35 px-4 py-4">
                          <p className="text-sm text-muted-foreground">{item.label}</p>
                          <p className="mt-2 text-2xl font-semibold tracking-tight">{item.value}</p>
                          <p className="mt-1 text-sm text-muted-foreground">{item.note}</p>
                        </div>
                      ))}
                    </div>

                    {operationalIncidents.length ? (
                      <div className="grid gap-3 md:grid-cols-2">
                        {operationalIncidents.map((incident) => (
                          <div key={incident.key} className={`rounded-xl border px-4 py-4 ${incidentToneClassName(incident.tone)}`}>
                            <p className="text-[11px] font-medium uppercase tracking-[0.16em] text-muted-foreground">
                              {incident.label}
                            </p>
                            <p className="mt-2 text-base font-semibold tracking-tight">{incident.title}</p>
                            <p className="mt-2 text-sm leading-6 text-muted-foreground">{incident.body}</p>
                          </div>
                        ))}
                      </div>
                    ) : null}

                    {warmPoolStatus?.degradedTargets?.length ? (
                      <div className="rounded-xl border bg-muted/15 p-4">
                        <div className="flex flex-col gap-1">
                          <p className="text-sm font-medium">{t('overview.warmTargets.title')}</p>
                          <p className="text-sm text-muted-foreground">
                            {t('overview.warmTargets.description', { count: warmPoolStatus.degradedTargets.length })}
                          </p>
                        </div>
                        <div className="mt-4 grid gap-3 md:grid-cols-2">
                          {warmPoolStatus.degradedTargets.map((target) => (
                            <div key={`${target.policyId}:${target.repoFullName}`} className="rounded-xl border bg-background/70 px-4 py-4">
                              <div className="flex flex-col gap-1">
                                <p className="text-sm font-medium">{target.repoFullName}</p>
                                <p className="text-xs text-muted-foreground">
                                  {t('overview.warmTargets.policy', { id: target.policyId })}
                                </p>
                                <p className="text-xs text-muted-foreground">
                                  {t('overview.warmTargets.labels', {
                                    labels: normalizeOperatorList(target.labels) || t('common.notSet')
                                  })}
                                </p>
                              </div>
                              <p className="mt-3 text-sm text-muted-foreground">
                                {t('overview.warmTargets.counts', {
                                  missing: target.missingIdle,
                                  idle: target.warmIdleCount,
                                  reserved: target.warmReservedCount,
                                  warming: target.warmWarmingCount
                                })}
                              </p>
                            </div>
                          ))}
                        </div>
                      </div>
                    ) : null}
                  </div>

                  <div className="flex flex-col gap-4 rounded-xl border bg-muted/15 p-4">
                    <div className="flex flex-col gap-1">
                      <p className="text-sm font-medium">{t('overview.decision.title')}</p>
                      <p className="text-sm text-muted-foreground">{t('overview.decision.description')}</p>
                    </div>
                    <DecisionActionCard
                      emphasis="primary"
                      title={t('overview.decision.primaryLabel')}
                      action={displayedDecisionModel.primaryAction}
                      onNavigate={onNavigate}
                    />
                    <DecisionActionCard
                      title={t('overview.decision.secondaryLabel')}
                      action={displayedDecisionModel.secondaryAction}
                      onNavigate={onNavigate}
                    />
                  </div>
                </>
              )}
            </div>
          </CardContent>
        </Card>

        <Card className="border bg-card/95">
          <CardHeader className="border-b">
            <CardTitle>{t('overview.setup.title')}</CardTitle>
            <CardDescription>{t('overview.setup.description')}</CardDescription>
          </CardHeader>
          <CardContent className="flex flex-col gap-4 pt-4">
            <div className="flex items-start justify-between gap-3 rounded-xl bg-muted/20 px-4 py-3">
              <div className="flex flex-col gap-1">
                <p className="text-sm font-medium">{t('overview.setup.access.label')}</p>
                <p className="text-sm text-muted-foreground">{t('overview.setup.access.note')}</p>
              </div>
              <span className="text-sm font-medium">
                {summarizeOCIAuthMode(ociAuthStatus.effectiveMode || ociAuthStatus.defaultMode, t)}
              </span>
            </div>
            <div className="flex flex-col gap-3 text-sm">
              <div className="flex items-center justify-between gap-4">
                <span className="text-muted-foreground">{t('overview.setup.runtimeConfig.label')}</span>
                <span className="font-medium">
                  {ociAuthStatus.runtimeConfigReady ? t('common.ready') : t('common.needsSetup')}
                </span>
              </div>
              <Separator />
              <div className="flex items-center justify-between gap-4">
                <span className="text-muted-foreground">{t('overview.setup.defaultAccess.label')}</span>
                <span className="font-medium">{summarizeOCIAuthMode(ociAuthStatus.defaultMode, t)}</span>
              </div>
              <Separator />
              <div className="flex items-center justify-between gap-4">
                <span className="text-muted-foreground">{t('overview.setup.defaultSubnet.label')}</span>
                <span className="truncate text-right font-medium">{describeSubnet('', subnetById, defaultSubnetId)}</span>
              </div>
              <Separator />
              <div className="flex items-center justify-between gap-4">
                <span className="text-muted-foreground">{t('overview.setup.suggestedSubnets.label')}</span>
                <span className="font-medium">{recommendedSubnets.length}</span>
              </div>
              <Separator />
              <div className="flex items-center justify-between gap-4">
                <span className="text-muted-foreground">{t('overview.setup.cacheCompat.label')}</span>
                <span className="font-medium">
                  {cacheCompatStatus.ready
                    ? t('overview.setup.cacheCompat.ready')
                    : cacheCompatStatus.enabled
                      ? t('overview.setup.cacheCompat.incomplete')
                      : t('overview.setup.cacheCompat.disabled')}
                </span>
              </div>
            </div>
            {!ociAuthStatus.runtimeConfigReady && (ociAuthStatus.runtimeConfigMissing || []).length ? (
              <div className="rounded-xl bg-muted/30 px-4 py-3 text-sm text-muted-foreground">
                {t('overview.setup.runtimeConfig.missing', {
                  items: formatTranslatedList((ociAuthStatus.runtimeConfigMissing || []).filter(Boolean))
                })}
              </div>
            ) : null}
          </CardContent>
        </Card>
      </div>

      <Card className="border bg-card/95">
        <CardHeader className="border-b">
          <div className="flex flex-col gap-1">
            <CardTitle>{t('overview.billing.title')}</CardTitle>
            <CardDescription>{t('overview.billing.description')}</CardDescription>
          </div>
        </CardHeader>
        <CardContent className="flex flex-col gap-5 pt-5">
          {billingReport.loading ? (
            <LoadingBlock
              title={t('viewState.loading.title', { view: t('overview.billing.title') })}
              body={t('overview.billing.loading')}
            />
          ) : null}

          {!billingReport.loading && billingReport.error ? (
            <ErrorBlock title={t('overview.billing.unavailable.title')} body={billingReport.error} />
          ) : null}

          {!billingReport.loading && !billingReport.error ? (
            <>
              <div className="overflow-hidden rounded-2xl border bg-muted/10">
                <div className="grid gap-px bg-border/70 xl:grid-cols-[minmax(0,1.35fr)_minmax(280px,0.65fr)]">
                  <div className="grid gap-px bg-border/70 md:grid-cols-2">
                    {billingPrimaryStats.map((item, index) => (
                      <div
                        key={item.label}
                        className={`px-5 py-5 ${index === 0 ? 'bg-background/95' : 'bg-accent/20'}`}
                      >
                        <p className="text-[11px] font-medium uppercase tracking-[0.16em] text-muted-foreground">{item.label}</p>
                        <p className="mt-3 text-3xl font-semibold tracking-tight sm:text-[2rem]">{item.value}</p>
                        <p className="mt-2 max-w-md text-sm leading-6 text-muted-foreground">{item.note}</p>
                      </div>
                    ))}
                  </div>

                  <div className="grid gap-px bg-border/70 sm:grid-cols-2 xl:grid-cols-1">
                    {billingSecondaryStats.map((item) => (
                      <div key={item.label} className="flex flex-col gap-2 bg-background/90 px-5 py-4">
                        <div className="flex items-end justify-between gap-4">
                          <p className="text-sm font-medium text-muted-foreground">{item.label}</p>
                          <p className="text-xl font-semibold tracking-tight">{item.value}</p>
                        </div>
                        <p className="text-sm leading-6 text-muted-foreground">{item.note}</p>
                      </div>
                    ))}
                  </div>
                </div>
              </div>

              <div className="grid gap-6 xl:grid-cols-[minmax(0,1.1fr)_minmax(320px,0.9fr)]">
                <div className="flex flex-col gap-6">
                  <div className="flex flex-col gap-3">
                    <div className="flex flex-col gap-1">
                      <p className="text-sm font-medium">{t('overview.billing.attribution.title')}</p>
                      <p className="text-sm text-muted-foreground">{t('overview.billing.attribution.description')}</p>
                    </div>
                    <div className="grid gap-px overflow-hidden rounded-2xl border bg-border/60">
                      {billingAttributionStats.map((item, index) => (
                        <div
                          key={item.label}
                          className={`grid gap-3 px-4 py-4 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-center ${index === 0 ? 'bg-accent/25' : 'bg-background/90'}`}
                        >
                          <div className="min-w-0">
                            <p className="text-sm font-medium">{item.label}</p>
                            <p className="mt-1 text-sm leading-6 text-muted-foreground">{item.note}</p>
                          </div>
                          <p className="shrink-0 text-left text-lg font-semibold tracking-tight sm:text-right">{item.value}</p>
                        </div>
                      ))}
                    </div>
                  </div>

                  <div className="flex flex-col gap-3">
                    <div className="flex items-center justify-between gap-3">
                      <p className="text-sm font-medium">{t('overview.billing.breakdown.title')}</p>
                      <p className="text-xs text-muted-foreground">{formatDateTime(billingReport.generatedAt)}</p>
                    </div>
                    {!billingReport.items.length ? (
                      <EmptyBlock
                        title={t('overview.billing.breakdown.emptyTitle')}
                        body={t('overview.billing.breakdown.emptyBody')}
                      />
                    ) : (
                      <div className="flex flex-col divide-y rounded-2xl border bg-muted/10">
                        {billingReport.items.slice(0, 5).map((item) => (
                          <div
                            key={`${item.policyId}:${item.repoOwner}/${item.repoName}:${item.attributionStatus}`}
                            className="flex items-start justify-between gap-4 px-4 py-4"
                          >
                            <div className="min-w-0 flex flex-col gap-1">
                              <p className="truncate text-sm font-medium">
                                {item.policyLabel || t('overview.billing.breakdown.policyFallback', { policyId: item.policyId })}
                                {item.repoOwner && item.repoName ? ` · ${item.repoOwner}/${item.repoName}` : ''}
                              </p>
                              <p className="text-sm text-muted-foreground">
                                {t('overview.billing.breakdown.resourceSummary', {
                                  count: item.resourceCount,
                                  suffix: item.resourceCount === 1 ? '' : 's',
                                  usage: formatNumber(item.totalUsageQuantity),
                                  unit: item.usageUnits[0] || t('overview.billing.breakdown.usageUnitsFallback')
                                })}
                              </p>
                            </div>
                            <div className="flex shrink-0 flex-col items-end gap-2 text-right">
                              <StatusBadge value={item.attributionStatus} />
                              <p className="text-sm font-medium">{formatCurrency(item.totalCost, item.currency || billingCurrency)}</p>
                            </div>
                          </div>
                        ))}
                      </div>
                    )}
                  </div>
                </div>

                <div className="flex flex-col gap-4 rounded-2xl border bg-muted/10 p-5">
                  <div className="flex flex-col gap-1">
                    <p className="text-sm font-medium">{t('overview.billing.notes.title')}</p>
                    <p className="text-sm text-muted-foreground">
                      {billingReport.windowStart && billingReport.windowEnd
                        ? t('overview.billing.notes.window', {
                          start: formatDateTime(billingReport.windowStart),
                          end: formatDateTime(billingReport.windowEnd)
                        })
                        : t('overview.billing.notes.windowPending')}
                    </p>
                  </div>

                  <div className="flex flex-col divide-y rounded-xl border bg-background/70">
                    {billingOperationalNotes.map((note, index) => (
                      <p key={`${index}:${note}`} className="px-4 py-3 text-sm leading-6 text-muted-foreground">
                        {note}
                      </p>
                    ))}
                  </div>

                  <Separator />

                  <div className="flex flex-col gap-3">
                    <div className="flex items-center justify-between gap-3">
                      <p className="text-sm font-medium">{t('overview.billing.notes.gapsTitle')}</p>
                      <p className="text-[11px] font-medium uppercase tracking-[0.16em] text-muted-foreground">
                        {billingReport.issues.length}
                      </p>
                    </div>
                    {billingReport.issues.length ? (
                      <div className="flex flex-col gap-2">
                        {billingReport.issues.slice(0, 3).map((issue) => (
                          <div
                            key={`${issue.resourceId}:${issue.reason}:${issue.timeStart}`}
                            className="rounded-xl border bg-background/80 px-4 py-3"
                          >
                            <div className="flex items-start justify-between gap-3">
                              <div className="min-w-0">
                                <p className="truncate text-sm font-medium">{issue.policyLabel || compactValue(issue.resourceId)}</p>
                                <p className="mt-1 text-sm leading-6 text-muted-foreground">{issue.reason}</p>
                              </div>
                              <p className="shrink-0 text-sm font-medium">{formatCurrency(issue.cost, issue.currency || billingCurrency)}</p>
                            </div>
                          </div>
                        ))}
                      </div>
                    ) : (
                      <div className="rounded-xl border bg-background/70 px-4 py-4 text-sm text-muted-foreground">
                        {t('overview.billing.stats.unmapped.note.clear')}
                      </div>
                    )}
                  </div>
                </div>
              </div>
            </>
          ) : null}
        </CardContent>
      </Card>

      <div className="grid gap-6 xl:grid-cols-[minmax(0,1.2fr)_minmax(320px,0.8fr)]">
        <Card className="border bg-card/95">
          <Tabs defaultValue="runners" className="gap-0">
            <CardHeader className="border-b">
              <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
                <div className="flex flex-col gap-1">
                  <CardTitle>{t('overview.activity.title')}</CardTitle>
                  <CardDescription>{t('overview.activity.description')}</CardDescription>
                </div>
                <TabsList variant="line">
                  <TabsTrigger value="runners">{t('overview.activity.tab.runners')}</TabsTrigger>
                  <TabsTrigger value="jobs">{t('overview.activity.tab.jobs')}</TabsTrigger>
                  <TabsTrigger value="logs">{t('overview.activity.tab.logs')}</TabsTrigger>
                </TabsList>
              </div>
            </CardHeader>
            <CardContent className="pt-4">
              <TabsContent value="runners">
                <ActivityList
                  items={overviewRunnerItems}
                  emptyTitle={t('overview.activity.runners.emptyTitle')}
                  emptyBody={t('overview.activity.runners.emptyBody')}
                  renderMeta={(item) => (
                    <div className="flex flex-col gap-1">
                      <StatusBadge value={item.status} />
                      <p>{formatDateTime(item.timestamp)}</p>
                    </div>
                  )}
                />
              </TabsContent>
              <TabsContent value="jobs">
                <ActivityList
                  items={overviewJobItems}
                  emptyTitle={t('overview.activity.jobs.emptyTitle')}
                  emptyBody={t('overview.activity.jobs.emptyBody')}
                  renderMeta={(item) => (
                    <div className="flex flex-col gap-1">
                      <StatusBadge value={item.status} />
                      <p>{formatDateTime(item.timestamp)}</p>
                    </div>
                  )}
                />
              </TabsContent>
              <TabsContent value="logs">
                <ActivityList
                  items={overviewLogItems}
                  emptyTitle={t('overview.activity.logs.emptyTitle')}
                  emptyBody={t('overview.activity.logs.emptyBody')}
                  renderMeta={(item) => (
                    <div className="flex flex-col gap-1">
                      <StatusBadge value={item.status} />
                      <p>{formatDateTime(item.timestamp)}</p>
                    </div>
                  )}
                />
              </TabsContent>
            </CardContent>
          </Tabs>
        </Card>

        <Card className="border bg-card/95">
          <CardHeader className="border-b">
            <CardTitle>{t('overview.subnets.title')}</CardTitle>
            <CardDescription>{t('overview.subnets.description')}</CardDescription>
          </CardHeader>
          <CardContent className="pt-4">
            {recommendedSubnets.length === 0 ? (
              <EmptyBlock
                title={t('overview.subnets.emptyTitle')}
                body={subnetError || t('overview.subnets.emptyBody')}
              />
            ) : (
              <div className="flex flex-col gap-3">
                {recommendedSubnets.slice(0, 3).map((item) => (
                  <div key={item.id} className="rounded-xl bg-muted/20 px-4 py-3">
                    <div className="flex items-center justify-between gap-3">
                      <div className="min-w-0 flex flex-col gap-1">
                        <p className="truncate text-sm font-medium">{item.displayName || item.id}</p>
                        <p className="text-sm text-muted-foreground">{item.cidrBlock}</p>
                      </div>
                    </div>
                    <p className="mt-2 text-sm text-muted-foreground">
                      {normalizeOperatorList([item.recommendation])}
                    </p>
                  </div>
                ))}
              </div>
            )}
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
