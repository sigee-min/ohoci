import { CloudIcon, GitBranchIcon, RocketIcon } from 'lucide-react';

import { Badge } from '@/components/ui/badge';
import { Accordion, AccordionContent, AccordionItem, AccordionTrigger } from '@/components/ui/accordion';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { useI18n } from '@/i18n';
import { compactValue, summarizeOCIAuthMode } from '@/lib/workspace-formatters';
import { resolveGitHubActiveConfigs } from '@/lib/workspace-forms';
import { GitHubConfigView } from '@/views/github-config-view';
import { OCIAuthView } from '@/views/oci-auth-view';

function summarizeSource(source, t) {
  switch (source) {
    case 'cms':
      return t('setup.summary.source.cms');
    case 'env':
      return t('setup.summary.source.env');
    default:
      return t('setup.summary.source.unknown');
  }
}

function summarizeMode(mode, t) {
  if (!mode) {
    return t('setup.summary.mode.notDetected');
  }
  return summarizeOCIAuthMode(mode, t);
}

function statusBadgeVariant(ready) {
  return ready ? 'default' : 'secondary';
}

function summarizeGitHubAuthMode(mode, t) {
  return mode === 'app' ? t('github.mode.app') : t('github.mode.unconfigured');
}

function summarizeGitHubDisplayName(config = {}) {
  return config?.name || config?.accountLogin || '';
}

function summarizeGitHubDrift(driftStatus, t) {
  if (!driftStatus) {
    return '';
  }

  if (driftStatus.available === false) {
    return t('setup.summary.github.driftUnavailable');
  }

  if (Array.isArray(driftStatus.issues) && driftStatus.issues.length) {
    return t('setup.summary.github.driftIssues', { count: driftStatus.issues.length });
  }

  if (driftStatus.loaded) {
    return t('setup.summary.github.driftOk');
  }

  return '';
}

function buildGitHubSummary({
  sourceLabel,
  liveMode,
  activeAppCount,
  liveName,
  liveAccountLogin,
  selectedRepos,
  missing,
  ready,
  stagedConfig,
  stagedReady,
  stagedMissing,
  driftStatus,
  t
}) {
  const parts = [sourceLabel, summarizeGitHubAuthMode(liveMode, t)];

  if (activeAppCount > 1) {
    parts.push(t('github.summary.activeApps', { count: activeAppCount }));
  } else if (liveName) {
    parts.push(liveName);
  } else if (liveAccountLogin) {
    parts.push(liveAccountLogin);
  }

  if (selectedRepos.length) {
    parts.push(t('github.summary.liveRepos', { count: selectedRepos.length }));
  } else if (missing.length) {
    parts.push(t('github.summary.liveMissing', { count: missing.length }));
  } else if (ready) {
    parts.push(t('github.summary.liveReady'));
  }

  if (stagedConfig) {
    parts.push(
      stagedReady
        ? t('github.summary.stagedReady')
        : stagedMissing.length
          ? t('github.summary.stagedRemaining', { count: stagedMissing.length })
          : t('github.summary.stagedPresent')
    );
  } else if (!ready) {
    parts.push(t('github.summary.stageApp'));
  }

  const driftSummary = summarizeGitHubDrift(driftStatus, t);
  if (driftSummary) {
    parts.push(driftSummary);
  }

  return parts.join(' · ');
}

function buildOCISectionSummary({ sourceLabel, modeLabel, activeCredentialName, missing, ready, t }) {
  const parts = [sourceLabel, modeLabel];

  if (activeCredentialName) {
    parts.push(activeCredentialName);
  } else if (missing.length) {
    parts.push(t('setup.summary.oci.missingItems', { count: missing.length }));
  } else {
    parts.push(ready ? t('setup.summary.oci.launchTargetSaved') : t('setup.summary.oci.credentialNeeded'));
  }

  return parts.join(' · ');
}

function buildRuntimeSummary({ sourceLabel, effectiveSettings, missing, t }) {
  const parts = [sourceLabel];

  if (effectiveSettings.availabilityDomain) {
    parts.push(effectiveSettings.availabilityDomain);
  }

  if (effectiveSettings.subnetOcid) {
    parts.push(t('setup.summary.runtime.subnet', { subnet: compactValue(effectiveSettings.subnetOcid, 8, 6) }));
  }

  if (effectiveSettings.imageOcid) {
    parts.push(t('setup.summary.runtime.imageSelected'));
  }

  if (parts.length === 1) {
    parts.push(missing.length ? t('setup.summary.runtime.missingFields', { count: missing.length }) : t('setup.summary.runtime.noneSaved'));
  }

  return parts.join(' · ');
}

function SetupOverviewTile({ icon: Icon, label, value, note, ready }) {
  const { t } = useI18n();

  return (
    <div className="rounded-xl border bg-muted/20 px-4 py-4">
      <div className="flex items-start justify-between gap-3">
        <div className="flex items-center gap-2">
          <Icon className="size-4 text-muted-foreground" />
          <p className="text-sm font-medium">{label}</p>
        </div>
        <Badge variant={statusBadgeVariant(ready)}>{ready ? t('common.ready') : t('common.needsSetup')}</Badge>
      </div>
      <p className="mt-3 text-lg font-semibold tracking-tight">{value}</p>
      <p className="mt-1 text-sm text-muted-foreground">{note}</p>
    </div>
  );
}

function SetupSectionTrigger({ icon: Icon, title, ready, summary }) {
  const { t } = useI18n();

  return (
    <div className="flex min-w-0 flex-1 flex-col gap-3 pr-4 md:flex-row md:items-center md:justify-between">
      <div className="min-w-0">
        <div className="flex items-center gap-2">
          <Icon className="size-4 text-muted-foreground" />
          <span className="text-base font-semibold tracking-tight">{title}</span>
        </div>
      </div>
      <div className="flex flex-wrap items-center gap-2 md:justify-end">
        <Badge variant={statusBadgeVariant(ready)}>{ready ? t('common.ready') : t('common.needsSetup')}</Badge>
        <span className="text-sm text-muted-foreground">{summary}</span>
      </div>
    </div>
  );
}

export function SetupView({
  setupStatus,
  githubSetupProps,
  ociAuthForm,
  setOciAuthForm,
  onOCIFileUpload,
  onOCITest,
  onOCISave,
  onOCIClear,
  ociAuthInspecting,
  ociAuthInspectResult,
  ociAuthTesting,
  ociAuthSaving,
  ociAuthClearing,
  ociAuthStatus,
  ociAuthResult,
  ociRuntimeStatus,
  ociRuntimeForm,
  setOciRuntimeForm,
  runtimeCatalog,
  runtimeCatalogValidation,
  onRuntimeCatalogRefresh,
  onOCIRuntimeSave,
  onOCIRuntimeClear,
  ociRuntimeSaving,
  ociRuntimeClearing
}) {
  const { t } = useI18n();
  const githubConfigStatus = githubSetupProps.githubConfigStatus;
  const githubDriftStatus = githubSetupProps.githubDriftStatus;
  const githubReady = Boolean(setupStatus.steps?.github?.completed || githubConfigStatus.ready);
  const activeGitHubApps = resolveGitHubActiveConfigs(githubConfigStatus);
  const singleGitHubApp = activeGitHubApps.length === 1 ? activeGitHubApps[0] : null;
  const liveGitHubConfig = githubConfigStatus.activeConfig || githubConfigStatus.effectiveConfig || null;
  const githubSelectedRepos = Array.isArray(githubConfigStatus.selectedRepos) ? githubConfigStatus.selectedRepos : [];
  const githubMissing = Array.isArray(githubConfigStatus.missing) ? githubConfigStatus.missing : [];
  const githubStagedMissing = Array.isArray(githubConfigStatus.stagedMissing) ? githubConfigStatus.stagedMissing : [];
  const githubSourceLabel = summarizeSource(githubConfigStatus.source, t);
  const githubSummary = buildGitHubSummary({
    sourceLabel: githubSourceLabel,
    liveMode: liveGitHubConfig?.authMode,
    activeAppCount: activeGitHubApps.length,
    liveName: singleGitHubApp ? summarizeGitHubDisplayName(singleGitHubApp) : '',
    liveAccountLogin: singleGitHubApp?.accountLogin || '',
    selectedRepos: githubSelectedRepos,
    missing: githubMissing,
    ready: githubReady,
    stagedConfig: githubConfigStatus.stagedConfig,
    stagedReady: githubConfigStatus.stagedReady,
    stagedMissing: githubStagedMissing,
    driftStatus: githubDriftStatus,
    t
  });

  const ociStepReady = Boolean(setupStatus.steps?.oci?.completed);
  const ociStepMissing = Array.isArray(setupStatus.steps?.oci?.missing) ? setupStatus.steps.oci.missing : [];
  const ociModeLabel = summarizeMode(ociAuthStatus.effectiveMode || ociAuthStatus.defaultMode, t);
  const runtimeSourceLabel = summarizeSource(ociRuntimeStatus.source, t);
  const runtimeMissing = Array.isArray(ociRuntimeStatus.missing) ? ociRuntimeStatus.missing : [];
  const effectiveSettings = ociRuntimeStatus.effectiveSettings || {};
  const runtimeSummary = buildRuntimeSummary({
    sourceLabel: runtimeSourceLabel,
    effectiveSettings,
    missing: runtimeMissing,
    t
  });
  const ociSummary = buildOCISectionSummary({
    sourceLabel: runtimeSourceLabel,
    modeLabel: ociModeLabel,
    activeCredentialName: ociAuthStatus.activeCredential?.name,
    missing: ociStepMissing,
    ready: ociStepReady,
    t
  });

  return (
    <div className="flex flex-col gap-6">
      <Card className="border bg-card/95">
        <CardHeader className="border-b">
          <div>
            <CardTitle>{t('setup.overview.title')}</CardTitle>
          </div>
        </CardHeader>
        <CardContent className="grid gap-4 pt-5 md:grid-cols-2 xl:grid-cols-3">
          <SetupOverviewTile
            icon={GitBranchIcon}
            label={t('setup.tile.github.label')}
            value={
              activeGitHubApps.length > 1
                ? t('setup.tile.github.value.activeApps', { count: activeGitHubApps.length })
                : summarizeGitHubDisplayName(singleGitHubApp)
              || (
                liveGitHubConfig?.authMode
                  ? summarizeGitHubAuthMode(liveGitHubConfig.authMode, t)
                  : githubReady
                    ? t('setup.tile.github.value.ready')
                    : t('github.summary.awaitingStaging')
              )
            }
            note={githubSummary}
            ready={githubReady}
          />
          <SetupOverviewTile
            icon={CloudIcon}
            label={t('setup.tile.ociAccess.label')}
            value={ociModeLabel}
            note={
              ociAuthStatus.activeCredential
                ? t('setup.tile.ociAccess.note.withCredential', {
                    name: ociAuthStatus.activeCredential.name,
                    profile: ociAuthStatus.activeCredential.profileName || t('common.default')
                  })
                : t('setup.tile.ociAccess.note.default')
            }
            ready={ociStepReady}
          />
          <SetupOverviewTile
            icon={RocketIcon}
            label={t('setup.tile.launchTarget.label')}
            value={ociRuntimeStatus.ready ? t('setup.tile.launchTarget.value.ready') : t('setup.tile.launchTarget.value.pending')}
            note={runtimeSummary}
            ready={ociRuntimeStatus.ready}
          />
        </CardContent>
      </Card>

      <Accordion type="single" collapsible className="gap-4">
        <AccordionItem value="github" className="rounded-xl border bg-card/95 px-4 not-last:border-b-0">
          <AccordionTrigger className="gap-4 py-4 hover:no-underline">
            <SetupSectionTrigger
              icon={GitBranchIcon}
              title={t('setup.section.github.title')}
              ready={githubReady}
              summary={githubSummary}
            />
          </AccordionTrigger>
          <AccordionContent className="pb-4">
            <div className="border-t pt-4">
              <GitHubConfigView
                {...githubSetupProps}
                githubReady={githubReady}
                description={null}
                mode="settings"
              />
            </div>
          </AccordionContent>
        </AccordionItem>

        <AccordionItem value="oci" className="rounded-xl border bg-card/95 px-4 not-last:border-b-0">
          <AccordionTrigger className="gap-4 py-4 hover:no-underline">
            <SetupSectionTrigger
              icon={CloudIcon}
              title={t('setup.section.oci.title')}
              ready={ociStepReady}
              summary={ociSummary}
            />
          </AccordionTrigger>
          <AccordionContent className="pb-4">
            <div className="border-t pt-4">
              <OCIAuthView
                title={t('oci.title')}
                description={null}
                mode="settings"
                ociAuthForm={ociAuthForm}
                setOciAuthForm={setOciAuthForm}
                onFileUpload={onOCIFileUpload}
                onTest={onOCITest}
                onSave={onOCISave}
                onClear={onOCIClear}
                ociAuthInspecting={ociAuthInspecting}
                ociAuthInspectResult={ociAuthInspectResult}
                ociAuthTesting={ociAuthTesting}
                ociAuthSaving={ociAuthSaving}
                ociAuthClearing={ociAuthClearing}
                ociAuthStatus={ociAuthStatus}
                ociAuthResult={ociAuthResult}
                ociRuntimeStatus={ociRuntimeStatus}
                ociRuntimeForm={ociRuntimeForm}
                setOciRuntimeForm={setOciRuntimeForm}
                runtimeCatalog={runtimeCatalog}
                runtimeCatalogValidation={runtimeCatalogValidation}
                onRuntimeCatalogRefresh={onRuntimeCatalogRefresh}
                onRuntimeSave={onOCIRuntimeSave}
                onRuntimeClear={onOCIRuntimeClear}
                ociRuntimeSaving={ociRuntimeSaving}
                ociRuntimeClearing={ociRuntimeClearing}
              />
            </div>
          </AccordionContent>
        </AccordionItem>
      </Accordion>
    </div>
  );
}
