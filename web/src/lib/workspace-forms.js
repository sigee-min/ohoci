import {
  DEFAULT_GITHUB_API_BASE_URL,
  GITHUB_AUTH_MODE_APP,
  GITHUB_MANIFEST_OWNER_TARGET_ORGANIZATION,
  GITHUB_MANIFEST_OWNER_TARGET_PERSONAL,
  GITHUB_SETUP_MODE_CREATE,
  GITHUB_SETUP_MODE_EXISTING
} from './workspace-constants.js';

export function parsePolicyForm(source = {}) {
  return {
    labels: Array.isArray(source.labels) ? source.labels.join(', ') : '',
    subnetOcid: source.subnetOcid || '',
    shape: source.shape || '',
    ocpu: source.ocpu ?? '',
    memoryGb: source.memoryGb ?? '',
    maxRunners: source.maxRunners ?? 1,
    ttlMinutes: source.ttlMinutes ?? 30,
    spot: Boolean(source.spot),
    enabled: source.enabled == null ? true : Boolean(source.enabled),
    warmEnabled: Boolean(source.warmEnabled),
    warmMinIdle: source.warmMinIdle ?? 1,
    warmTtlMinutes: source.warmTtlMinutes ?? source.ttlMinutes ?? 30,
    warmRepoAllowlistText: Array.isArray(source.warmRepoAllowlist) ? source.warmRepoAllowlist.join('\n') : '',
    budgetEnabled: Boolean(source.budgetEnabled),
    budgetCapAmount: source.budgetCapAmount ?? 0,
    budgetWindowDays: source.budgetWindowDays ?? 7
  };
}

export function normalizePolicyPayload(form) {
  return {
    labels: form.labels.split(',').map((value) => value.trim()).filter(Boolean),
    subnetOcid: (form.subnetOcid || '').trim(),
    shape: form.shape.trim(),
    ocpu: Number(form.ocpu),
    memoryGb: Number(form.memoryGb),
    maxRunners: Number(form.maxRunners),
    ttlMinutes: Number(form.ttlMinutes),
    spot: Boolean(form.spot),
    enabled: Boolean(form.enabled),
    warmEnabled: Boolean(form.warmEnabled),
    warmMinIdle: form.warmEnabled ? Number(form.warmMinIdle) : 0,
    warmTtlMinutes: Number(form.warmTtlMinutes),
    warmRepoAllowlist: parseGitHubRepoSelection(form.warmRepoAllowlistText),
    budgetEnabled: Boolean(form.budgetEnabled),
    budgetCapAmount: Number(form.budgetCapAmount),
    budgetWindowDays: 7
  };
}

export function blankOCIAuthForm() {
  return {
    name: '',
    profileName: 'DEFAULT',
    configText: '',
    privateKeyPem: '',
    passphrase: ''
  };
}

export function parseGitHubRepoSelection(value) {
  if (Array.isArray(value)) {
    return Array.from(
      new Set(
        value
          .map((entry) => {
            if (typeof entry === 'string') {
              return entry.trim();
            }

            if (!entry || typeof entry !== 'object') {
              return '';
            }

            const owner = entry.owner || entry.login || '';
            const name = entry.name || '';
            return entry.fullName || [owner, name].filter(Boolean).join('/');
          })
          .filter(Boolean)
      )
    );
  }

  if (typeof value === 'string') {
    return Array.from(
      new Set(
        value
          .split(/[\n,]/)
          .map((entry) => entry.trim())
          .filter(Boolean)
      )
    );
  }

  return [];
}

export function parseGitHubTagList(value) {
  if (Array.isArray(value)) {
    return Array.from(
      new Set(
        value
          .map((entry) => String(entry || '').trim())
          .filter(Boolean)
      )
    );
  }

  if (typeof value === 'string') {
    return Array.from(
      new Set(
        value
          .split(/[\n,]/)
          .map((entry) => entry.trim())
          .filter(Boolean)
      )
    );
  }

  return [];
}

export function formatGitHubTagList(value) {
  return parseGitHubTagList(value).join('\n');
}

function normalizePositiveIntegerString(value) {
  const number = Number((value ?? '').toString().trim());
  return Number.isFinite(number) && number > 0 ? String(number) : '';
}

export function normalizeGitHubManifestOwnerTarget(value) {
  return String(value || '').trim().toLowerCase() === GITHUB_MANIFEST_OWNER_TARGET_ORGANIZATION
    ? GITHUB_MANIFEST_OWNER_TARGET_ORGANIZATION
    : GITHUB_MANIFEST_OWNER_TARGET_PERSONAL;
}

export function blankGitHubConfigForm(source = {}) {
  return {
    id: source.id == null ? '' : String(source.id).trim(),
    name: String(source.name || source.appName || '').trim(),
    tagsText: formatGitHubTagList(source.tags || source.tagsText),
    apiBaseUrl: source.apiBaseUrl || '',
    appId: source.appId ? String(source.appId) : '',
    installationId: source.installationId ? String(source.installationId) : '',
    privateKeyPem: '',
    webhookSecret: '',
    selectedRepos: parseGitHubRepoSelection(source.selectedRepos || source.allowedRepos),
    ownerTarget: normalizeGitHubManifestOwnerTarget(source.ownerTarget),
    organizationSlug: String(source.organizationSlug || '').trim()
  };
}

function isGitHubAppConfig(config = {}) {
  return String(config?.authMode || '').trim().toLowerCase() === GITHUB_AUTH_MODE_APP;
}

export function buildGitHubConfigFormFromStatus(payload = {}) {
  const status = payload.status || payload;
  const activeConfigs = resolveGitHubActiveConfigs(status);

  if (!status.stagedConfig && activeConfigs.length > 1) {
    return blankGitHubConfigForm();
  }

  const preferredConfig =
    status.stagedConfig
    || (isGitHubAppConfig(status.activeConfig) ? status.activeConfig : null)
    || (isGitHubAppConfig(status.effectiveConfig) ? status.effectiveConfig : null)
    || status.activeConfig
    || activeConfigs[0]
    || status.effectiveConfig
    || status.config
    || status.current
    || status
    || payload;

  return blankGitHubConfigForm({
    id: status.stagedConfig?.id,
    name: preferredConfig.name || preferredConfig.appName,
    tags: preferredConfig.tags,
    apiBaseUrl: preferredConfig.apiBaseUrl,
    appId: preferredConfig.appId,
    installationId: preferredConfig.installationId,
    selectedRepos: preferredConfig.selectedRepos || preferredConfig.allowedRepos || payload.selectedRepos
  });
}

export function resolveGitHubRepositoryChoicesSource(result = null, status = {}) {
  const activeConfigs = resolveGitHubActiveConfigs(status);
  const allowRuntimeFallback = activeConfigs.length <= 1;

  if (Array.isArray(result?.repositories)) {
    return result.repositories;
  }

  if (Array.isArray(result?.config?.installationRepositories)) {
    return result.config.installationRepositories;
  }

  if (Array.isArray(status?.stagedConfig?.installationRepositories)) {
    return status.stagedConfig.installationRepositories;
  }

  if (allowRuntimeFallback && Array.isArray(status?.activeConfig?.installationRepositories)) {
    return status.activeConfig.installationRepositories;
  }

  if (allowRuntimeFallback && Array.isArray(status?.effectiveConfig?.installationRepositories)) {
    return status.effectiveConfig.installationRepositories;
  }

  return [];
}

export function resolveGitHubActiveConfigs(status = {}) {
  if (Array.isArray(status.activeConfigs) && status.activeConfigs.length) {
    return status.activeConfigs.filter(Boolean);
  }

  return status.activeConfig ? [status.activeConfig] : [];
}

export function getGitHubRepositorySectionState({
  activeConfigs = [],
  stagedConfig = null,
  githubConfigResult = null,
  repositoryChoices = [],
  currentSelectedRepos = [],
  ready = false
} = {}) {
  const activeCount = Array.isArray(activeConfigs) ? activeConfigs.filter(Boolean).length : 0;
  const hasRepositoryChoices = Array.isArray(repositoryChoices) && repositoryChoices.length > 0;
  const hasCurrentSelectedRepos = Array.isArray(currentSelectedRepos) && currentSelectedRepos.length > 0;
  const hasMultiActiveRuntime = activeCount > 1 && !stagedConfig && !githubConfigResult;

  if (hasMultiActiveRuntime && !hasRepositoryChoices) {
    return hasCurrentSelectedRepos
      ? { show: false, emptyState: '' }
      : { show: true, emptyState: 'multiActive' };
  }

  const shouldShow = hasRepositoryChoices || Boolean(githubConfigResult) || hasCurrentSelectedRepos || ready || Boolean(stagedConfig);
  if (!shouldShow) {
    return { show: false, emptyState: '' };
  }

  if (hasRepositoryChoices) {
    return { show: true, emptyState: '' };
  }

  return {
    show: true,
    emptyState: githubConfigResult || ready || stagedConfig ? 'loaded' : 'idle'
  };
}

export function normalizeGitHubManifestPending(source = {}) {
  if (!source || typeof source !== 'object') {
    return null;
  }

  const appId = Number(source.appId);
  const normalized = {
    appId: Number.isFinite(appId) && appId > 0 ? appId : 0,
    appName: String(source.appName || '').trim(),
    appSlug: String(source.appSlug || '').trim(),
    appSettingsUrl: String(source.appSettingsUrl || '').trim(),
    transferUrl: String(source.transferUrl || '').trim(),
    installUrl: String(source.installUrl || '').trim(),
    privateKeyPem: String(source.privateKeyPem || '').trim(),
    webhookSecret: String(source.webhookSecret || '').trim(),
    ownerTarget: normalizeGitHubManifestOwnerTarget(source.ownerTarget),
    createdAt: String(source.createdAt || '').trim(),
    expiresAt: String(source.expiresAt || '').trim()
  };

  if (!normalized.appId || !normalized.privateKeyPem || !normalized.webhookSecret) {
    return null;
  }
  return normalized;
}

export function normalizeGitHubInstallationLookup(source = {}) {
  const installations = Array.isArray(source.installations)
    ? source.installations
        .map((item) => {
          if (!item || typeof item !== 'object') {
            return null;
          }
          const id = Number(item.id);
          if (!Number.isFinite(id) || id <= 0) {
            return null;
          }
          return {
            id,
            accountLogin: String(item.accountLogin || '').trim(),
            accountType: String(item.accountType || '').trim(),
            repositorySelection: String(item.repositorySelection || '').trim(),
            htmlUrl: String(item.htmlUrl || '').trim(),
            appSlug: String(item.appSlug || '').trim()
          };
        })
        .filter(Boolean)
    : [];

  const autoInstallationId = Number(source.autoInstallationId);

  return {
    installations,
    autoInstallationId: Number.isFinite(autoInstallationId) && autoInstallationId > 0 ? autoInstallationId : 0
  };
}

export function isGitHubManifestHelperSupported(apiBaseUrl) {
  const normalized = String(apiBaseUrl || '').trim().replace(/\/+$/, '');
  return !normalized || normalized === DEFAULT_GITHUB_API_BASE_URL;
}

export function normalizeGitHubSetupMode({ mode, apiBaseUrl } = {}) {
  if (!isGitHubManifestHelperSupported(apiBaseUrl)) {
    return GITHUB_SETUP_MODE_EXISTING;
  }

  return mode === GITHUB_SETUP_MODE_EXISTING ? GITHUB_SETUP_MODE_EXISTING : GITHUB_SETUP_MODE_CREATE;
}

export function resolveGitHubSetupMode({ apiBaseUrl, pendingManifest } = {}) {
  if (pendingManifest) {
    return GITHUB_SETUP_MODE_CREATE;
  }

  return normalizeGitHubSetupMode({
    mode: GITHUB_SETUP_MODE_CREATE,
    apiBaseUrl
  });
}

export function hasGitHubStagedConfigState(status = {}) {
  return Boolean(
    status.stagedConfig
    || status.stagedReady
    || (Array.isArray(status.stagedMissing) && status.stagedMissing.length)
    || String(status.stagedError || '').trim()
  );
}

export function applyGitHubInstallationLookup(form, lookup = {}) {
  const normalizedLookup = normalizeGitHubInstallationLookup(lookup);
  if (!normalizedLookup.autoInstallationId) {
    return {
      ...form,
      installationId: form.installationId || ''
    };
  }

  return {
    ...form,
    installationId: String(normalizedLookup.autoInstallationId)
  };
}

export function mergeGitHubManifestIntoConfigForm(form, pending, lookup = {}, options = {}) {
  const normalizedPending = normalizeGitHubManifestPending(pending);
  if (!normalizedPending) {
    return { ...form };
  }

  const returnedInstallationId = normalizePositiveIntegerString(options.installationId);

  return applyGitHubInstallationLookup(
    {
      ...form,
      name: form.name || normalizedPending.appName || '',
      apiBaseUrl: '',
      appId: String(normalizedPending.appId),
      installationId: returnedInstallationId,
      ownerTarget: normalizedPending.ownerTarget,
      privateKeyPem: normalizedPending.privateKeyPem,
      webhookSecret: normalizedPending.webhookSecret
    },
    lookup
  );
}

export function buildGitHubManifestStartPayload(form = {}) {
  const ownerTarget = normalizeGitHubManifestOwnerTarget(form.ownerTarget);
  const payload = {
    apiBaseUrl: String(form.apiBaseUrl || '').trim(),
    ownerTarget
  };

  if (ownerTarget === GITHUB_MANIFEST_OWNER_TARGET_ORGANIZATION) {
    payload.organizationSlug = String(form.organizationSlug || '').trim();
  }

  return payload;
}

export function blankOCIRuntimeForm(source = {}) {
  return {
    compartmentOcid: source.compartmentOcid || '',
    availabilityDomain: source.availabilityDomain || '',
    subnetOcid: source.subnetOcid || '',
    nsgOcidText: Array.isArray(source.nsgOcids) ? source.nsgOcids.join('\n') : '',
    imageOcid: source.imageOcid || '',
    assignPublicIp: Boolean(source.assignPublicIp),
    cacheCompatEnabled: Boolean(source.cacheCompatEnabled),
    cacheBucketName: source.cacheBucketName || '',
    cacheObjectPrefix: source.cacheObjectPrefix || '',
    cacheRetentionDays: source.cacheRetentionDays ?? 7
  };
}

export function normalizeOCIRuntimePayload(form) {
  return {
    compartmentOcid: (form.compartmentOcid || '').trim(),
    availabilityDomain: (form.availabilityDomain || '').trim(),
    subnetOcid: (form.subnetOcid || '').trim(),
    nsgOcids: (form.nsgOcidText || '')
      .split(/[\n,]/)
      .map((value) => value.trim())
      .filter(Boolean),
    imageOcid: (form.imageOcid || '').trim(),
    assignPublicIp: Boolean(form.assignPublicIp),
    cacheCompatEnabled: Boolean(form.cacheCompatEnabled),
    cacheBucketName: (form.cacheBucketName || '').trim(),
    cacheObjectPrefix: (form.cacheObjectPrefix || '').trim(),
    cacheRetentionDays: Number(form.cacheRetentionDays)
  };
}

export function normalizeGitHubConfigPayload(form) {
  const id = String(form.id || '').trim();
  const name = String(form.name || '').trim();
  const tags = parseGitHubTagList(form.tagsText || form.tags);
  const apiBaseUrl = (form.apiBaseUrl || '').trim();
  const appId = Number((form.appId || '').toString().trim());
  const installationId = Number((form.installationId || '').toString().trim());

  return {
    id: id || undefined,
    name,
    tags,
    authMode: 'app',
    apiBaseUrl: apiBaseUrl || undefined,
    appId: Number.isFinite(appId) && appId > 0 ? appId : undefined,
    installationId: Number.isFinite(installationId) && installationId > 0 ? installationId : undefined,
    privateKeyPem: (form.privateKeyPem || '').trim(),
    webhookSecret: (form.webhookSecret || '').trim(),
    selectedRepos: parseGitHubRepoSelection(form.selectedRepos)
  };
}
