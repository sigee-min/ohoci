import { useEffect } from 'react';
import {
  ArrowRightIcon,
  ExternalLinkIcon,
  GitBranchIcon,
  KeyRoundIcon,
  RefreshCwIcon,
  ShieldCheckIcon,
  Trash2Icon
} from 'lucide-react';

import { BusyButtonContent } from '@/components/app/busy-button-content';
import { EmptyBlock } from '@/components/app/display-primitives';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle
} from '@/components/ui/card';
import { Checkbox } from '@/components/ui/checkbox';
import { Field, FieldDescription, FieldGroup, FieldLabel } from '@/components/ui/field';
import { Input } from '@/components/ui/input';
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Separator } from '@/components/ui/separator';
import { Textarea } from '@/components/ui/textarea';
import { translateMaybeKey, useI18n } from '@/i18n';
import { normalizeOperatorText } from '@/lib/operator-text';
import { cn } from '@/lib/utils';
import { formatDateTime } from '@/lib/workspace-formatters';
import {
  GITHUB_MANIFEST_OWNER_TARGET_ORGANIZATION,
  GITHUB_MANIFEST_OWNER_TARGET_PERSONAL,
  GITHUB_ORGANIZATION_SLUG_MAX_LENGTH,
  GITHUB_SETUP_MODE_CREATE,
  GITHUB_SETUP_MODE_EXISTING
} from '@/lib/workspace-constants';
import {
  getGitHubRepositorySectionState,
  resolveGitHubActiveConfigs,
  resolveGitHubRepositoryChoicesSource,
  hasGitHubStagedConfigState,
  isGitHubManifestHelperSupported,
  normalizeGitHubManifestOwnerTarget,
  normalizeGitHubSetupMode
} from '@/lib/workspace-forms';

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

function summarizeAuthMode(mode, t) {
  return mode === 'app' ? t('github.mode.app') : t('github.mode.unconfigured');
}

function summarizeInstallationSelection(selection, t) {
  const normalizedSelection = String(selection || '').trim().toLowerCase();

  if (!normalizedSelection) {
    return t('common.notSet');
  }

  switch (normalizedSelection) {
    case 'all':
      return t('github.installation.selection.all');
    case 'selected':
      return t('github.installation.selection.selected');
    default:
      return t('common.unknown');
  }
}

function buildGitHubConfigKey(config, index = 0) {
  if (config?.id) {
    return `id:${config.id}`;
  }

  const signature = `app:${config?.appId || 0}|install:${config?.installationId || 0}|account:${config?.accountLogin || ''}|name:${config?.name || ''}`;
  return signature === 'app:0|install:0|account:|name:' ? `index:${index}` : signature;
}

function collectSelectedRepos(configs = [], fallback = []) {
  const merged = configs.flatMap((config) => (Array.isArray(config?.selectedRepos) ? config.selectedRepos : []));
  if (merged.length) {
    return Array.from(new Set(merged.map((repoName) => String(repoName).trim()).filter(Boolean)));
  }

  return Array.from(new Set((Array.isArray(fallback) ? fallback : []).map((repoName) => String(repoName).trim()).filter(Boolean)));
}

function summarizeGitHubAppName(config, t) {
  if (config?.name) {
    return config.name;
  }

  if (config?.appId) {
    return t('github.activeApps.fallbackNameWithId', { appId: config.appId });
  }

  return t('github.activeApps.fallbackName');
}

function summarizeGitHubAppTarget(config, t) {
  const parts = [];

  if (config?.accountLogin) {
    parts.push(config.accountLogin);
  }
  if (config?.accountType) {
    parts.push(config.accountType);
  }
  if (config?.installationId) {
    parts.push(t('github.activeApps.installationTarget', { id: config.installationId }));
  }

  return parts.join(' · ') || t('common.notSet');
}

function summarizeGitHubAppRepoCount(config, t) {
  const selectedRepoCount = Array.isArray(config?.selectedRepos) ? config.selectedRepos.length : 0;
  const installationRepoCount = Array.isArray(config?.installationRepositories) ? config.installationRepositories.length : 0;
  return t('github.activeApps.repoCount', { count: selectedRepoCount || installationRepoCount });
}

function repositoryDisplayName(repository) {
  return repository?.fullName || [repository?.owner, repository?.name].filter(Boolean).join('/');
}

function normalizeRepository(repository) {
  if (typeof repository === 'string') {
    return {
      fullName: repository,
      owner: repository.split('/')[0] || '',
      private: false
    };
  }

  if (!repository || typeof repository !== 'object') {
    return null;
  }

  return {
    fullName: repositoryDisplayName(repository),
    owner: repository.owner || repository.ownerLogin || repository.accountLogin || '',
    private: Boolean(repository.private)
  };
}

function buildRepositoryChoices(result, status) {
  return resolveGitHubRepositoryChoicesSource(result, status)
    .map(normalizeRepository)
    .filter(Boolean);
}

function summarizeResult(result, t) {
  if (!result) {
    return '';
  }

  const details = [];
  const repositoryCount = Array.isArray(result.repositories) ? result.repositories.length : 0;
  const selectedRepoCount = Array.isArray(result.config?.selectedRepos) ? result.config.selectedRepos.length : 0;

  if (result.accountLogin) {
    details.push(t('github.result.account', { login: result.accountLogin }));
  }
  if (Array.isArray(result.owners) && result.owners.length) {
    details.push(t('github.result.owners', { owners: result.owners.join(', ') }));
  }
  if (repositoryCount > 0) {
    details.push(
      selectedRepoCount > 0
        ? t('github.result.repositories.someAdmin', {
            adminCount: selectedRepoCount,
            totalCount: repositoryCount
          })
        : t('github.result.repositories.allAdmin', { count: repositoryCount })
    );
  } else if (
    result.accountLogin
    || (Array.isArray(result.owners) && result.owners.length)
    || Object.prototype.hasOwnProperty.call(result, 'repositories')
  ) {
    details.push(t('github.result.repositories.noneAdmin'));
  }

  return details.join(' ') || t('github.result.default');
}

function summarizeVerificationTitle(result, t) {
  const repositoryCount = Array.isArray(result?.repositories) ? result.repositories.length : 0;
  return repositoryCount > 0 ? t('github.alert.verifiedTitle') : t('github.repositories.adminWarningTitle');
}

const GITHUB_CONTRACT_KEY_MAP = {
  appId: 'github.contract.appId',
  installationId: 'github.contract.installationId',
  name: 'github.contract.name',
  tags: 'github.contract.tags',
  privateKeyPem: 'github.contract.privateKeyPem',
  webhookSecret: 'github.contract.webhookSecret',
  selectedRepos: 'github.contract.selectedRepos',
  installationState: 'github.contract.installationState',
  installationRepositorySelection: 'github.contract.installationRepositorySelection',
  installationRepositories: 'github.contract.installationRepositories'
};

const GITHUB_INSTALLATION_STATE_KEY_MAP = {
  active: 'github.installation.state.active',
  suspended: 'github.installation.state.suspended',
  deleted: 'github.installation.state.deleted'
};

function translateGitHubContractValue(value, t, locale) {
  const normalizedValue = String(value || '').trim();
  if (!normalizedValue) {
    return '';
  }

  const installationStateKey = GITHUB_INSTALLATION_STATE_KEY_MAP[normalizedValue.toLowerCase()];
  if (installationStateKey) {
    return t(installationStateKey);
  }

  const contractKey = GITHUB_CONTRACT_KEY_MAP[normalizedValue];
  if (contractKey) {
    return t(contractKey);
  }

  return translateMaybeKey(normalizedValue, {}, locale);
}

function formatGitHubContractList(values, t, locale) {
  return values
    .map((value) => translateGitHubContractValue(value, t, locale))
    .filter(Boolean)
    .join(', ');
}

function StatusValue({ label, value }) {
  const { t } = useI18n();
  const displayValue = value == null || String(value).trim() === '' ? t('common.notSet') : value;

  return (
    <div className="rounded-xl border bg-background/70 px-4 py-3">
      <p className="text-sm font-medium">{label}</p>
      <p className="mt-1 text-sm text-muted-foreground">{displayValue}</p>
    </div>
  );
}

function GitHubCredentialsFields({ githubConfigForm, onFieldChange }) {
  const { t } = useI18n();

  return (
    <FieldGroup className="md:grid md:grid-cols-2">
      <Field>
        <FieldLabel htmlFor="github-app-id">{t('github.form.appId')}</FieldLabel>
        <Input
          id="github-app-id"
          inputMode="numeric"
          value={githubConfigForm.appId}
          onChange={(event) => onFieldChange('appId', event.target.value)}
          placeholder="123456"
        />
      </Field>

      <Field>
        <FieldLabel htmlFor="github-installation-id">{t('github.form.installationId')}</FieldLabel>
        <Input
          id="github-installation-id"
          inputMode="numeric"
          value={githubConfigForm.installationId}
          onChange={(event) => onFieldChange('installationId', event.target.value)}
          placeholder="7890123"
        />
      </Field>

      <Field className="md:col-span-2">
        <FieldLabel htmlFor="github-api-base-url">{t('github.form.apiUrl')}</FieldLabel>
        <Input
          id="github-api-base-url"
          value={githubConfigForm.apiBaseUrl}
          onChange={(event) => onFieldChange('apiBaseUrl', event.target.value)}
          placeholder={t('github.form.apiUrlPlaceholder')}
        />
        <FieldDescription>{t('github.form.apiUrlDescription')}</FieldDescription>
      </Field>

      <Field className="md:col-span-2">
        <FieldLabel htmlFor="github-private-key">{t('github.form.privateKeyPem')}</FieldLabel>
        <Textarea
          id="github-private-key"
          rows={10}
          value={githubConfigForm.privateKeyPem}
          onChange={(event) => onFieldChange('privateKeyPem', event.target.value)}
          placeholder="-----BEGIN RSA PRIVATE KEY-----"
        />
        <FieldDescription>{t('github.form.privateKeyDescription')}</FieldDescription>
      </Field>

      <Field className="md:col-span-2">
        <FieldLabel htmlFor="github-webhook-secret">{t('github.form.webhookSecret')}</FieldLabel>
        <Input
          id="github-webhook-secret"
          type="password"
          value={githubConfigForm.webhookSecret}
          onChange={(event) => onFieldChange('webhookSecret', event.target.value)}
          placeholder={t('github.form.webhookSecretPlaceholder')}
        />
        <FieldDescription>{t('github.form.webhookSecretDescription')}</FieldDescription>
      </Field>
    </FieldGroup>
  );
}

function GitHubMetadataFields({ githubConfigForm, onFieldChange }) {
  const { t } = useI18n();

  return (
    <FieldGroup className="md:grid md:grid-cols-2">
      <Field>
        <FieldLabel htmlFor="github-app-name">{t('github.form.name')}</FieldLabel>
        <Input
          id="github-app-name"
          value={githubConfigForm.name || ''}
          onChange={(event) => onFieldChange('name', event.target.value)}
          placeholder={t('github.form.namePlaceholder')}
        />
        <FieldDescription>{t('github.form.nameDescription')}</FieldDescription>
      </Field>

      <Field>
        <FieldLabel htmlFor="github-app-tags">{t('github.form.tags')}</FieldLabel>
        <Textarea
          id="github-app-tags"
          rows={4}
          value={githubConfigForm.tagsText || ''}
          onChange={(event) => onFieldChange('tagsText', event.target.value)}
          placeholder={t('github.form.tagsPlaceholder')}
        />
        <FieldDescription>{t('github.form.tagsDescription')}</FieldDescription>
      </Field>
    </FieldGroup>
  );
}

function GitHubActiveAppsPanel({
  activeConfigs,
  deletingId,
  onRemoveActiveApp,
  removeSupported,
  busy
}) {
  const { t, locale } = useI18n();

  if (!activeConfigs.length) {
    return null;
  }

  return (
    <div className="rounded-xl border bg-background/70 px-4 py-4">
      <div className="space-y-1">
        <p className="text-sm font-medium">{t('github.activeApps.title')}</p>
        <p className="text-sm text-muted-foreground">{t('github.activeApps.description')}</p>
      </div>

      <div className="mt-4 grid gap-3">
        {activeConfigs.map((config, index) => {
          const configId = config?.id ? String(config.id) : '';
          const canRemove = Boolean(onRemoveActiveApp && configId && (removeSupported || config.deletePath));

          return (
            <div key={buildGitHubConfigKey(config, index)} className="rounded-xl border bg-card/80 px-4 py-3">
              <div className="flex items-start justify-between gap-3">
                <div className="min-w-0 flex-1">
                  <p className="truncate text-sm font-medium">{summarizeGitHubAppName(config, t)}</p>
                  <p className="mt-1 text-sm text-muted-foreground">{summarizeGitHubAppTarget(config, t)}</p>
                </div>

                {canRemove ? (
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    onClick={() => void onRemoveActiveApp(configId)}
                    disabled={busy}
                    aria-busy={deletingId === configId}
                  >
                    <BusyButtonContent
                      busy={deletingId === configId}
                      label={t('github.button.removeApp')}
                      icon={Trash2Icon}
                    />
                  </Button>
                ) : null}
              </div>

              <div className="mt-3 flex flex-wrap gap-2">
                <Badge variant="secondary">{summarizeGitHubAppRepoCount(config, t)}</Badge>
                {Array.isArray(config?.tags) && config.tags.length ? (
                  config.tags.map((tag) => (
                    <Badge key={`${buildGitHubConfigKey(config, index)}:${tag}`} variant="outline">
                      {tag}
                    </Badge>
                  ))
                ) : (
                  <Badge variant="outline">{t('github.activeApps.noTags')}</Badge>
                )}
              </div>

              <div className="mt-3 grid gap-2 text-sm sm:grid-cols-2">
                <div className="flex items-center justify-between gap-3 rounded-lg bg-background/70 px-3 py-2">
                  <span className="text-muted-foreground">{t('github.installation.runtimeState')}</span>
                  <span className="text-right font-medium">
                    {translateGitHubContractValue(config?.installationState, t, locale) || t('common.notSet')}
                  </span>
                </div>
                <div className="flex items-center justify-between gap-3 rounded-lg bg-background/70 px-3 py-2">
                  <span className="text-muted-foreground">{t('github.installation.repositorySelection')}</span>
                  <span className="text-right font-medium">
                    {summarizeInstallationSelection(config?.installationRepositorySelection, t)}
                  </span>
                </div>
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}

function driftSeverityVariant(severity) {
  switch (String(severity || '').trim().toLowerCase()) {
    case 'critical':
      return 'destructive';
    case 'warning':
      return 'outline';
    case 'ok':
      return 'secondary';
    default:
      return 'outline';
  }
}

function summarizeDriftIssueCode(code) {
  return normalizeOperatorText(code, { keyPrefixes: ['operator.githubDrift.issue'] }) || code;
}

function GitHubDriftPanel({
  githubDriftStatus,
  onRefreshDrift,
  onReconcileDrift,
  githubDriftReconciling,
  busy
}) {
  const { t } = useI18n();

  if (!githubDriftStatus) {
    return null;
  }

  return (
    <div className="rounded-xl border bg-background/70 px-4 py-4">
      <div className="flex flex-col gap-4">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
          <div>
            <p className="text-sm font-medium">{t('github.drift.title')}</p>
            <p className="mt-1 text-sm text-muted-foreground">{t('github.drift.description')}</p>
          </div>
          <div className="flex flex-wrap gap-2">
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => void onRefreshDrift?.({ silent: false })}
              disabled={busy || githubDriftStatus.loading}
              aria-busy={githubDriftStatus.loading}
            >
              <BusyButtonContent
                busy={githubDriftStatus.loading}
                label={t('github.drift.actions.refresh')}
                icon={RefreshCwIcon}
                busyIcon={RefreshCwIcon}
                spin
              />
            </Button>
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => void onReconcileDrift?.()}
              disabled={busy || githubDriftReconciling}
              aria-busy={githubDriftReconciling}
            >
              <BusyButtonContent
                busy={githubDriftReconciling}
                label={t('github.drift.actions.reconcile')}
                icon={ArrowRightIcon}
              />
            </Button>
          </div>
        </div>

        {!githubDriftStatus.available ? (
          <Alert>
            <GitBranchIcon />
            <AlertTitle>{t('github.drift.unavailableTitle')}</AlertTitle>
            <AlertDescription>{githubDriftStatus.error || t('github.drift.unavailableBody')}</AlertDescription>
          </Alert>
        ) : null}

        {githubDriftStatus.available && githubDriftStatus.error ? (
          <Alert>
            <GitBranchIcon />
            <AlertTitle>{t('github.drift.errorTitle')}</AlertTitle>
            <AlertDescription>{githubDriftStatus.error}</AlertDescription>
          </Alert>
        ) : null}

        {githubDriftStatus.available && !githubDriftStatus.error && !githubDriftStatus.issues.length ? (
          <Alert>
            <ShieldCheckIcon />
            <AlertTitle>{t('github.drift.healthyTitle')}</AlertTitle>
            <AlertDescription>
              {githubDriftStatus.generatedAt
                ? t('github.drift.healthyBody.withTimestamp', { updatedAt: formatDateTime(githubDriftStatus.generatedAt) })
                : t('github.drift.healthyBody')}
            </AlertDescription>
          </Alert>
        ) : null}

        {githubDriftStatus.available && !githubDriftStatus.error && githubDriftStatus.issues.length ? (
          <>
            <div className="flex flex-wrap items-center gap-2">
              <Badge variant={driftSeverityVariant(githubDriftStatus.severity)}>
                {normalizeOperatorText(githubDriftStatus.severity, { keyPrefixes: ['formatter.status'] })}
              </Badge>
              <span className="text-sm text-muted-foreground">
                {t('github.drift.issueCount', { count: githubDriftStatus.issues.length })}
              </span>
            </div>

            <div className="grid gap-3">
              {githubDriftStatus.issues.map((issue, index) => (
                <div key={`${issue.code || 'issue'}:${issue.installationId || index}`} className="rounded-xl border bg-card/80 px-4 py-4">
                  <div className="flex flex-wrap items-center justify-between gap-3">
                    <div>
                      <p className="text-sm font-medium">{summarizeDriftIssueCode(issue.code)}</p>
                      <p className="mt-1 text-sm text-muted-foreground">{issue.message || t('common.notSet')}</p>
                    </div>
                    <Badge variant={driftSeverityVariant(issue.severity)}>
                      {normalizeOperatorText(issue.severity, { keyPrefixes: ['formatter.status'] })}
                    </Badge>
                  </div>

                  {(issue.missingSelectedRepos?.length || issue.newlyVisibleRepos?.length) ? (
                    <div className="mt-3 flex flex-col gap-2 text-sm text-muted-foreground">
                      {issue.missingSelectedRepos?.length ? (
                        <div className="flex flex-wrap gap-2">
                          <span>{t('github.drift.missingRepos')}</span>
                          {issue.missingSelectedRepos.map((repoName) => (
                            <Badge key={`${issue.code}:missing:${repoName}`} variant="outline">{repoName}</Badge>
                          ))}
                        </div>
                      ) : null}
                      {issue.newlyVisibleRepos?.length ? (
                        <div className="flex flex-wrap gap-2">
                          <span>{t('github.drift.newRepos')}</span>
                          {issue.newlyVisibleRepos.map((repoName) => (
                            <Badge key={`${issue.code}:new:${repoName}`} variant="outline">{repoName}</Badge>
                          ))}
                        </div>
                      ) : null}
                    </div>
                  ) : null}
                </div>
              ))}
            </div>
          </>
        ) : null}
      </div>
    </div>
  );
}

export function GitHubConfigView({
  githubConfigForm,
  setGithubConfigForm,
  githubConfigMode = GITHUB_SETUP_MODE_CREATE,
  setGithubConfigMode,
  githubManifestState,
  onTest,
  onSave,
  onClear,
  onPromote,
  onRemoveActiveApp,
  onManifestCreate,
  onDiscoverInstallations,
  githubConfigTesting,
  githubConfigSaving,
  githubConfigClearing,
  githubConfigPromoting = false,
  githubActiveAppDeletingId = '',
  githubConfigStatus,
  githubConfigResult,
  githubDriftStatus,
  githubDriftReconciling = false,
  onRefreshDrift,
  onReconcileDrift,
  githubReady = false,
  title,
  description,
  layout = 'default'
}) {
  const { locale, t } = useI18n();
  const onboardingMode = layout === 'onboarding';
  const testingBusy = githubConfigTesting;
  const savingBusy = githubConfigSaving;
  const clearingBusy = githubConfigClearing;
  const promotingBusy = githubConfigPromoting;
  const deletingBusy = Boolean(githubActiveAppDeletingId);
  const busy = githubConfigTesting || githubConfigSaving || githubConfigClearing || githubConfigPromoting || deletingBusy;
  const ready = Boolean(githubReady || githubConfigStatus.ready);
  const activeConfigs = resolveGitHubActiveConfigs(githubConfigStatus);
  const liveConfig = githubConfigStatus.activeConfig || githubConfigStatus.effectiveConfig || null;
  const stagedConfig = githubConfigStatus.stagedConfig || null;
  const selectedRepos = Array.isArray(githubConfigForm.selectedRepos) ? githubConfigForm.selectedRepos : [];
  const manifestPending = githubManifestState?.pending || null;
  const manifestInstallations = Array.isArray(githubManifestState?.installations) ? githubManifestState.installations : [];
  const manifestAutoInstallationId = Number(githubManifestState?.autoInstallationId) || 0;
  const manifestSupported = isGitHubManifestHelperSupported(githubConfigForm.apiBaseUrl);
  const manifestOwnerTarget = normalizeGitHubManifestOwnerTarget(githubConfigForm.ownerTarget);
  const manifestOrganizationSlug = String(githubConfigForm.organizationSlug || '').trim();
  const manifestRequiresOrganizationSlug = manifestOwnerTarget === GITHUB_MANIFEST_OWNER_TARGET_ORGANIZATION;
  const currentSelectedRepos = collectSelectedRepos(
    activeConfigs,
    Array.isArray(githubConfigStatus.selectedRepos) ? githubConfigStatus.selectedRepos : liveConfig?.selectedRepos || []
  );
  const repositoryChoices = buildRepositoryChoices(githubConfigResult, githubConfigStatus);
  const missing = Array.isArray(githubConfigStatus.missing) ? githubConfigStatus.missing : [];
  const stagedMissing = Array.isArray(githubConfigStatus.stagedMissing) ? githubConfigStatus.stagedMissing : [];
  const hasStagedState = hasGitHubStagedConfigState(githubConfigStatus);
  const sourceLabel = summarizeSource(githubConfigStatus.source, t);
  const resolvedTitle = title || t('github.title');
  const resolvedDescription = description || t('github.description');
  const manifestBusy = busy || githubManifestState?.creating || githubManifestState?.discovering;
  const manifestCreateDisabled = !manifestSupported || manifestBusy || (manifestRequiresOrganizationSlug && !manifestOrganizationSlug);
  const selectedMode = normalizeGitHubSetupMode({
    mode: githubConfigMode,
    apiBaseUrl: githubConfigForm.apiBaseUrl,
    pendingManifest: manifestPending
  });
  const repositorySectionState = getGitHubRepositorySectionState({
    activeConfigs,
    stagedConfig,
    githubConfigResult,
    repositoryChoices,
    currentSelectedRepos,
    ready
  });
  const showRepositorySection = repositorySectionState.show;
  const showManifestDiscardAlert = Boolean(manifestPending && selectedMode === GITHUB_SETUP_MODE_EXISTING);

  useEffect(() => {
    if (selectedMode !== githubConfigMode && typeof setGithubConfigMode === 'function') {
      setGithubConfigMode(selectedMode);
    }
  }, [githubConfigMode, selectedMode, setGithubConfigMode]);

  function updateGitHubConfigField(field, value) {
    setGithubConfigForm((current) => ({
      ...current,
      [field]: value
    }));
  }

  function handleGitHubConfigModeChange(nextMode) {
    if (typeof setGithubConfigMode === 'function') {
      setGithubConfigMode(nextMode);
    }
  }

  function updateSelectedRepos(nextSelectedRepos) {
    setGithubConfigForm((current) => ({
      ...current,
      selectedRepos: nextSelectedRepos
    }));
  }

  function toggleRepository(fullName, checked) {
    if (checked) {
      updateSelectedRepos(Array.from(new Set([...selectedRepos, fullName])));
      return;
    }

    updateSelectedRepos(selectedRepos.filter((repoName) => repoName !== fullName));
  }

  const modeOptions = [
    {
      value: GITHUB_SETUP_MODE_CREATE,
      label: t('github.setupMode.create.label'),
      description: t('github.setupMode.create.description')
    },
    {
      value: GITHUB_SETUP_MODE_EXISTING,
      label: t('github.setupMode.existing.label'),
      description: t('github.setupMode.existing.description')
    }
  ];
  const selectedModeDescription = modeOptions.find((option) => option.value === selectedMode)?.description || '';
  const manifestResolvedInstallationId = String(githubConfigForm.installationId || manifestAutoInstallationId || '').trim();
  const manifestHasMultipleInstallations = manifestInstallations.length > 1;
  const manifestInstallationReady = Boolean(manifestResolvedInstallationId);
  const showFooterVerifyAction = selectedMode !== GITHUB_SETUP_MODE_CREATE;
  const showManifestVerifyAction = Boolean(manifestPending && manifestInstallationReady);
  const showManifestInstallAction = Boolean(
    manifestPending?.installUrl
    && !manifestInstallationReady
    && !manifestHasMultipleInstallations
  );
  const showManifestDiscoveryAction = Boolean(
    manifestPending
    && onDiscoverInstallations
    && !manifestInstallationReady
    && !manifestHasMultipleInstallations
  );
  const showManifestRestartAction = Boolean(
    manifestPending
    && !manifestBusy
    && !manifestHasMultipleInstallations
    && !showManifestInstallAction
    && !showManifestVerifyAction
    && !showManifestDiscoveryAction
  );
  const canStageWithoutRepoSelection = onboardingMode && (
    showRepositorySection
    || Boolean(stagedConfig)
    || ready
    || Boolean(githubConfigResult)
  );
  const showStageAction = savingBusy || selectedRepos.length > 0 || canStageWithoutRepoSelection;
  const showPromoteAction = Boolean(onPromote && (promotingBusy || (stagedConfig && githubConfigStatus.stagedReady)));
  const showClearAction = clearingBusy || Boolean(stagedConfig);
  const showFooterActions = showFooterVerifyAction || showStageAction || showPromoteAction || showClearAction;
  const manifestAudienceHint = t(
    manifestOwnerTarget === GITHUB_MANIFEST_OWNER_TARGET_ORGANIZATION
      ? 'github.manifest.ownerTarget.organizationHint'
      : 'github.manifest.ownerTarget.personalHint'
  );
  const manifestAppName = manifestPending
    ? manifestPending.appName
      || (manifestPending.appId ? t('github.manifest.generatedAppFallback', { appId: manifestPending.appId }) : t('github.manifest.pendingTitle'))
    : '';
  const manifestStatusText = manifestPending
    ? manifestInstallationReady
      ? t('github.manifest.installationAutoFilled', { installationId: manifestResolvedInstallationId })
      : manifestHasMultipleInstallations
        ? t('github.manifest.installationMultiple', { count: manifestInstallations.length })
        : t(
            manifestPending.ownerTarget === GITHUB_MANIFEST_OWNER_TARGET_ORGANIZATION
              ? 'github.manifest.pendingBodyOrganization'
              : 'github.manifest.pendingBodyPersonal'
          )
    : '';
  const manifestIssues = [
    !manifestSupported ? t('github.manifest.unsupported') : '',
    githubManifestState?.status === 'failed' && !manifestPending ? t('github.manifest.failed') : '',
    githubManifestState?.discoveryError || ''
  ].filter(Boolean);
  const accountStatusLabel = activeConfigs.length > 1 ? t('github.status.activeApps') : t('github.status.account');
  const accountStatusValue = activeConfigs.length > 1
    ? t('github.summary.activeApps', { count: activeConfigs.length })
    : liveConfig?.accountLogin || githubConfigStatus.accountLogin;

  const manifestInstallationSelectField = manifestPending && manifestHasMultipleInstallations ? (
    <Field className="space-y-2">
      <FieldLabel htmlFor="github-manifest-installation">{t('github.manifest.installationSelect')}</FieldLabel>
      <Select
        value={githubConfigForm.installationId || ''}
        onValueChange={(value) => updateGitHubConfigField('installationId', value)}
      >
        <SelectTrigger id="github-manifest-installation" className="w-full">
          <SelectValue placeholder={t('github.manifest.installationSelectPlaceholder')} />
        </SelectTrigger>
        <SelectContent align="start">
          <SelectGroup>
            {manifestInstallations.map((installation) => (
              <SelectItem key={installation.id} value={String(installation.id)}>
                {installation.accountLogin || t('common.unknown')} · {summarizeInstallationSelection(installation.repositorySelection, t)}
              </SelectItem>
            ))}
          </SelectGroup>
        </SelectContent>
      </Select>
    </Field>
  ) : null;

  const manifestAudienceField = (
    <div className="space-y-2">
      <p className="text-sm font-medium">{t('github.manifest.ownerTarget')}</p>
      <div aria-label={t('github.manifest.ownerTarget')} className="inline-flex w-full rounded-xl border bg-muted/30 p-1">
        {[
          {
            value: GITHUB_MANIFEST_OWNER_TARGET_PERSONAL,
            label: t('github.manifest.ownerTarget.personal')
          },
          {
            value: GITHUB_MANIFEST_OWNER_TARGET_ORGANIZATION,
            label: t('github.manifest.ownerTarget.organization')
          }
        ].map((option) => {
          const isSelected = option.value === manifestOwnerTarget;

          return (
            <button
              key={option.value}
              type="button"
              onClick={() => updateGitHubConfigField('ownerTarget', option.value)}
              aria-pressed={isSelected}
              className={cn(
                'flex-1 rounded-lg px-3 py-2 text-sm font-medium transition-colors focus-visible:ring-[3px] focus-visible:ring-ring/50 focus-visible:outline-none',
                isSelected ? 'bg-background text-foreground shadow-sm' : 'text-muted-foreground hover:text-foreground'
              )}
            >
              {option.label}
            </button>
          );
        })}
      </div>
      {!manifestPending ? <p className="text-sm text-muted-foreground">{manifestAudienceHint}</p> : null}
    </div>
  );

  const manifestOrganizationField = manifestRequiresOrganizationSlug && !manifestPending ? (
    <Field>
      <FieldLabel htmlFor="github-manifest-organization-slug">{t('github.manifest.organizationSlug')}</FieldLabel>
      <Input
        id="github-manifest-organization-slug"
        value={githubConfigForm.organizationSlug || ''}
        onChange={(event) => updateGitHubConfigField('organizationSlug', event.target.value)}
        placeholder={t('github.manifest.organizationSlugPlaceholder')}
        autoCapitalize="none"
        autoCorrect="off"
        spellCheck={false}
        maxLength={GITHUB_ORGANIZATION_SLUG_MAX_LENGTH}
      />
      <FieldDescription>{t('github.manifest.organizationSlugHint')}</FieldDescription>
    </Field>
  ) : null;

  const verificationAlert = githubConfigResult ? (
    <Alert>
      <ShieldCheckIcon />
      <AlertTitle>{summarizeVerificationTitle(githubConfigResult, t)}</AlertTitle>
      <AlertDescription>{summarizeResult(githubConfigResult, t)}</AlertDescription>
    </Alert>
  ) : null;

  const migrationAlert = hasStagedState ? (
    <Alert>
      <GitBranchIcon />
      <AlertTitle>{t('github.alert.stagedTitle')}</AlertTitle>
      <AlertDescription>{t('github.alert.stagedBody')}</AlertDescription>
    </Alert>
  ) : null;

  const manifestHelperPanel = onManifestCreate ? (
    <div className="rounded-xl border bg-background/70 px-4 py-4">
      <div className="space-y-4">
        {manifestAudienceField}
        {manifestOrganizationField}

        {manifestPending ? (
          <div className="space-y-4 border-t pt-4">
            <div className="space-y-1">
              <p className="text-sm font-medium">{manifestAppName}</p>
              <p className="text-sm text-muted-foreground">{manifestStatusText}</p>
            </div>

            {manifestInstallationSelectField}

            {manifestIssues.length ? (
              <div className="space-y-2">
                {manifestIssues.map((issue) => (
                  <p key={issue} className="text-sm text-destructive">
                    {issue}
                  </p>
                ))}
              </div>
            ) : null}

            <div className="flex flex-wrap gap-2">
              {showManifestVerifyAction ? (
                <Button type="submit" disabled={busy} aria-busy={testingBusy}>
                  <BusyButtonContent
                    busy={testingBusy}
                    label={t('github.button.test')}
                    icon={ShieldCheckIcon}
                  />
                </Button>
              ) : null}

              {showManifestInstallAction ? (
                <Button type="button" asChild>
                  <a href={manifestPending.installUrl} target="_blank" rel="noreferrer">
                    <ExternalLinkIcon data-icon="inline-start" />
                    {t('github.button.installApp')}
                  </a>
                </Button>
              ) : null}

              {showManifestDiscoveryAction ? (
                <Button type="button" variant="outline" onClick={() => void onDiscoverInstallations()} disabled={manifestBusy}>
                  <BusyButtonContent
                    busy={Boolean(githubManifestState?.discovering)}
                    label={t('github.button.refreshInstallations')}
                    icon={RefreshCwIcon}
                    busyIcon={RefreshCwIcon}
                    spin
                  />
                </Button>
              ) : null}

              {showManifestRestartAction ? (
                <Button type="button" variant="ghost" onClick={() => void onManifestCreate()} disabled={manifestCreateDisabled}>
                  <BusyButtonContent
                    busy={Boolean(githubManifestState?.creating)}
                    label={t('github.button.recreateApp')}
                    icon={KeyRoundIcon}
                  />
                </Button>
              ) : null}
            </div>
          </div>
        ) : (
          <div className="space-y-4 border-t pt-4">
            {manifestIssues.length ? (
              <div className="space-y-2">
                {manifestIssues.map((issue) => (
                  <p key={issue} className="text-sm text-destructive">
                    {issue}
                  </p>
                ))}
              </div>
            ) : null}

            <div className="flex flex-wrap gap-2">
              <Button type="button" onClick={() => void onManifestCreate()} disabled={manifestCreateDisabled}>
                <BusyButtonContent
                  busy={Boolean(githubManifestState?.creating)}
                  label={t('github.button.createApp')}
                  icon={KeyRoundIcon}
                />
              </Button>
            </div>
          </div>
        )}
      </div>
    </div>
  ) : null;

  const manifestDiscardAlert = showManifestDiscardAlert ? (
    <div className="rounded-xl border bg-background/70 px-4 py-4">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div className="space-y-1">
          <p className="text-sm font-medium">{t('github.manifest.discardTitle')}</p>
          <p className="text-sm text-muted-foreground">{t('github.manifest.discardCompactBody')}</p>
        </div>
        <Button
          type="button"
          variant="outline"
          onClick={() => setGithubConfigMode?.({ mode: GITHUB_SETUP_MODE_EXISTING, discardManifest: true })}
          disabled={busy || githubManifestState?.loading}
        >
          <BusyButtonContent
            busy={Boolean(githubManifestState?.loading)}
            label={t('github.button.discardDraft')}
            icon={Trash2Icon}
          />
        </Button>
      </div>
    </div>
  ) : null;

  const repositoryChooser = repositoryChoices.length ? (
    <Card size="sm" className="border bg-background/70">
      <CardHeader className="border-b">
        <div>
          <CardTitle className="text-sm">{t('github.repositories.title')}</CardTitle>
          <CardDescription>{t('github.repositories.description')}</CardDescription>
        </div>
      </CardHeader>
      <CardContent className={cn('grid gap-3 pt-4', onboardingMode && repositoryChoices.length > 5 && 'lg:max-h-[360px] lg:overflow-y-auto lg:pr-1')}>
        {repositoryChoices.map((repository) => {
          const fullName = repository.fullName;
          const checked = selectedRepos.includes(fullName);

          return (
            <label
              key={fullName}
              className="flex items-start gap-3 rounded-xl border bg-card/80 px-4 py-3 transition-colors hover:border-foreground/20 hover:bg-background"
            >
              <Checkbox
                checked={checked}
                disabled={busy}
                onCheckedChange={(nextValue) => toggleRepository(fullName, Boolean(nextValue))}
                aria-label={t('github.repositories.checkboxAria', { repo: fullName })}
              />
              <div className="min-w-0 flex-1">
                <div className="flex flex-wrap items-center gap-2">
                  <p className="text-sm font-medium">{fullName}</p>
                  {repository.private ? <Badge variant="secondary">{t('common.private')}</Badge> : null}
                  {repository.owner ? <Badge variant="outline">{repository.owner}</Badge> : null}
                </div>
                <p className="mt-1 text-sm text-muted-foreground">
                  {checked ? t('github.repositories.selected') : t('github.repositories.available')}
                </p>
              </div>
            </label>
          );
        })}
      </CardContent>
      <CardFooter className="flex flex-col items-start gap-2 border-t pt-4 text-sm text-muted-foreground">
        <p>{t('github.repositories.footer', { total: repositoryChoices.length, selected: selectedRepos.length })}</p>
        {onboardingMode ? <p>{t('github.repositories.deferHint')}</p> : null}
      </CardFooter>
    </Card>
  ) : repositorySectionState.emptyState === 'multiActive' ? (
    <EmptyBlock
      title={t('github.repositories.multiActiveTitle')}
      body={t('github.repositories.multiActiveBody')}
    />
  ) : (
    <EmptyBlock
      title={t(repositorySectionState.emptyState === 'loaded' ? 'github.repositories.emptyLoadedTitle' : 'github.repositories.emptyIdleTitle')}
      body={t(repositorySectionState.emptyState === 'loaded' ? 'github.repositories.emptyLoadedBody' : 'github.repositories.emptyIdleBody')}
    />
  );

  const activeAppsPanel = (
    <GitHubActiveAppsPanel
      activeConfigs={activeConfigs}
      deletingId={githubActiveAppDeletingId}
      onRemoveActiveApp={onRemoveActiveApp}
      removeSupported={githubConfigStatus.activeAppDeleteSupported}
      busy={busy}
    />
  );

  const formBlock = (
    <form className="flex flex-col gap-5" onSubmit={onTest}>
      <div className="space-y-2">
        <p className="text-sm font-medium">{t('github.setupMode.label')}</p>
        <div aria-label={t('github.setupMode.label')} className="inline-flex w-full rounded-xl border bg-muted/30 p-1">
          {modeOptions.map((option) => {
            const isSelected = option.value === selectedMode;

            return (
              <button
                key={option.value}
                type="button"
                onClick={() => handleGitHubConfigModeChange(option.value)}
                aria-pressed={isSelected}
                className={cn(
                  'flex-1 rounded-lg px-3 py-2 text-sm font-medium transition-colors focus-visible:ring-[3px] focus-visible:ring-ring/50 focus-visible:outline-none',
                  isSelected ? 'bg-background text-foreground shadow-sm' : 'text-muted-foreground hover:text-foreground'
                )}
              >
                {option.label}
              </button>
            );
          })}
        </div>
        <p className="text-sm text-muted-foreground">{selectedModeDescription}</p>
      </div>

      {selectedMode === GITHUB_SETUP_MODE_CREATE ? (
        <div className="space-y-5">
          {manifestHelperPanel}
          <GitHubMetadataFields
            githubConfigForm={githubConfigForm}
            onFieldChange={updateGitHubConfigField}
          />
          {verificationAlert}
          {showRepositorySection ? repositoryChooser : null}
        </div>
      ) : (
        <div className="space-y-5">
          {manifestDiscardAlert}
          <GitHubCredentialsFields
            githubConfigForm={githubConfigForm}
            onFieldChange={updateGitHubConfigField}
          />
          <GitHubMetadataFields
            githubConfigForm={githubConfigForm}
            onFieldChange={updateGitHubConfigField}
          />

          {verificationAlert}
          {showRepositorySection ? repositoryChooser : null}
        </div>
      )}

      {showFooterActions ? (
        <div className="flex flex-wrap gap-2 border-t pt-5">
          {showFooterVerifyAction ? (
            <Button type="submit" disabled={busy} aria-busy={testingBusy}>
              <BusyButtonContent
                busy={testingBusy}
                label={t('github.button.test')}
                icon={ShieldCheckIcon}
              />
            </Button>
          ) : null}
          {showStageAction ? (
            <Button
              type="button"
              variant="outline"
              onClick={() => void onSave()}
              disabled={onboardingMode ? busy : busy || selectedRepos.length === 0}
              aria-busy={savingBusy}
            >
              <BusyButtonContent
                busy={savingBusy}
                label={onboardingMode ? t('common.saveAndContinue') : t('github.button.stage')}
                icon={KeyRoundIcon}
              />
            </Button>
          ) : null}
          {showPromoteAction ? (
            <Button
              type="button"
              variant="outline"
              onClick={() => void onPromote()}
              disabled={busy || !stagedConfig || !githubConfigStatus.stagedReady}
              aria-busy={promotingBusy}
            >
              <BusyButtonContent
                busy={promotingBusy}
                label={t('github.button.promote')}
                icon={ArrowRightIcon}
              />
            </Button>
          ) : null}
          {showClearAction ? (
            <Button type="button" variant="ghost" onClick={() => void onClear()} disabled={busy || !stagedConfig} aria-busy={clearingBusy}>
              <BusyButtonContent
                busy={clearingBusy}
                label={t('github.button.clear')}
                icon={Trash2Icon}
              />
            </Button>
          ) : null}
        </div>
      ) : null}
    </form>
  );

  const statePanel = (
    <Card className="border bg-card/95">
      <CardHeader className="border-b">
        <div>
          <CardTitle>{t('github.state.title')}</CardTitle>
          <CardDescription>{t('github.state.description')}</CardDescription>
        </div>
      </CardHeader>
      <CardContent className="flex flex-col gap-5 pt-4">
        {activeAppsPanel}
        <GitHubDriftPanel
          githubDriftStatus={githubDriftStatus}
          onRefreshDrift={onRefreshDrift}
          onReconcileDrift={onReconcileDrift}
          githubDriftReconciling={githubDriftReconciling}
          busy={busy}
        />

        <div className="grid gap-3 md:grid-cols-2">
          <StatusValue label={t('github.source.label')} value={sourceLabel} />
          <StatusValue label={t('github.state.runtimeMode')} value={summarizeAuthMode(liveConfig?.authMode, t)} />
          <StatusValue label={accountStatusLabel} value={accountStatusValue} />
          <StatusValue
            label={t('github.state.stagedStatus')}
            value={stagedConfig ? (githubConfigStatus.stagedReady ? t('github.summary.stagedReady') : t('github.summary.stagedPresent')) : t('github.summary.noneStaged')}
          />
        </div>

        {currentSelectedRepos.length ? (
          <div className="rounded-xl border bg-background/70 px-4 py-4">
            <p className="text-sm font-medium">{t('github.savedScope.title')}</p>
            <p className="mt-1 text-sm text-muted-foreground">{t('github.savedScope.description')}</p>
            <div className="mt-3 flex flex-wrap gap-2">
              {currentSelectedRepos.map((repoName) => (
                <Badge key={repoName} variant="secondary">
                  {repoName}
                </Badge>
              ))}
            </div>
          </div>
        ) : null}

        <div className="rounded-xl border bg-background/70 px-4 py-4">
          <p className="text-sm font-medium">{t('github.signals.title')}</p>
          <p className="mt-1 text-sm text-muted-foreground">{t('github.signals.description')}</p>
          <div className="mt-4 grid gap-3 text-sm">
            <div className="flex items-center justify-between gap-4">
              <span className="text-muted-foreground">{t('github.signals.webhookSecret')}</span>
              <span className="font-medium">{githubConfigStatus.hasWebhookSecret ? t('github.signals.tokenStored') : t('github.signals.tokenMissing')}</span>
            </div>
            {githubConfigStatus.lastTestedAt ? (
              <>
                <Separator />
                <div className="flex items-center justify-between gap-4">
                  <span className="text-muted-foreground">{t('github.signals.lastSaved')}</span>
                  <span className="font-medium">{formatDateTime(githubConfigStatus.lastTestedAt)}</span>
                </div>
              </>
            ) : null}
          </div>
        </div>

        {githubConfigStatus.webhookUrl ? (
          <div className="rounded-xl border bg-background/70 px-4 py-3">
            <p className="text-sm font-medium">{t('github.webhookUrl')}</p>
            <p className="mt-1 break-all text-sm text-muted-foreground">{githubConfigStatus.webhookUrl}</p>
          </div>
        ) : null}

        {missing.length ? (
          <Alert>
            <ShieldCheckIcon />
            <AlertTitle>{t('github.missing.title')}</AlertTitle>
            <AlertDescription>{formatGitHubContractList(missing, t, locale)}</AlertDescription>
          </Alert>
        ) : null}

        {stagedMissing.length ? (
          <Alert>
            <ShieldCheckIcon />
            <AlertTitle>{t('github.state.stagedMissingTitle')}</AlertTitle>
            <AlertDescription>{formatGitHubContractList(stagedMissing, t, locale)}</AlertDescription>
          </Alert>
        ) : null}
      </CardContent>
    </Card>
  );

  if (onboardingMode) {
    return (
      <div className="mx-auto w-full max-w-[820px]">
        <Card className="border bg-card/95">
          <CardHeader className="border-b">
            <div>
              <CardTitle>{resolvedTitle}</CardTitle>
              <CardDescription>{resolvedDescription}</CardDescription>
            </div>
          </CardHeader>
          <CardContent className="flex flex-col gap-5 pt-4">
            {migrationAlert}
            {activeAppsPanel}
            <GitHubDriftPanel
              githubDriftStatus={githubDriftStatus}
              onRefreshDrift={onRefreshDrift}
              onReconcileDrift={onReconcileDrift}
              githubDriftReconciling={githubDriftReconciling}
              busy={busy}
            />
            {formBlock}
          </CardContent>
        </Card>
      </div>
    );
  }

  return (
    <div className="grid gap-6 xl:grid-cols-[minmax(0,1.05fr)_minmax(320px,0.95fr)]">
      <Card className="border bg-card/95">
        <CardHeader className="border-b">
          <div>
            <CardTitle>{resolvedTitle}</CardTitle>
            <CardDescription>{resolvedDescription}</CardDescription>
          </div>
        </CardHeader>
        <CardContent className="flex flex-col gap-5 pt-4">
          {migrationAlert}
          {formBlock}
        </CardContent>
      </Card>

      {statePanel}
    </div>
  );
}
