import { startTransition, useDeferredValue, useEffect, useMemo, useRef, useState } from 'react';

import { toastError } from '@/components/ui/toaster';
import { api } from '@/lib/api';
import { useI18n } from '@/i18n';
import { normalizeOperatorErrorText, normalizeOperatorList, normalizeOperatorText } from '@/lib/operator-text';
import {
  createBlankBillingReportState,
  createBlankBillingGuardrailsState,
  createBlankGitHubConfigView,
  createBlankGitHubDriftState,
  createBlankGitHubManifestState,
  createBlankRunnerImageRecipeForm,
  createBlankRunnerImagesState,
  ALL_NAV_ITEMS,
  GITHUB_MANIFEST_INSTALLATION_ID_QUERY_KEY,
  GITHUB_MANIFEST_OWNER_TARGET_ORGANIZATION,
  GITHUB_MANIFEST_QUERY_KEY,
  GITHUB_SETUP_MODE_CREATE,
  GITHUB_SETUP_MODE_EXISTING,
  createBlankGitHubConfigStatus,
  createBlankOCICatalogState,
  createBlankOCIAuthStatus,
  createBlankOCIRuntimeStatus,
  createBlankSetupStatus,
  SETUP_STEP_ORDER
} from '@/lib/workspace-constants';
import {
  applyGitHubInstallationLookup,
  buildGitHubConfigFormFromStatus,
  buildGitHubManifestStartPayload,
  blankGitHubConfigForm,
  blankOCIAuthForm,
  mergeGitHubManifestIntoConfigForm,
  normalizeGitHubManifestOwnerTarget,
  normalizeGitHubSetupMode,
  blankOCIRuntimeForm,
  normalizeGitHubInstallationLookup,
  normalizeGitHubManifestPending,
  parseGitHubRepoSelection,
  parseGitHubTagList,
  normalizeGitHubConfigPayload,
  normalizeOCIRuntimePayload,
  normalizePolicyPayload,
  parsePolicyForm
} from '@/lib/workspace-forms';

const DEFAULT_LOGIN_FORM = {
  username: 'admin',
  password: 'admin'
};

const DEFAULT_PASSWORD_FORM = {
  currentPassword: '',
  newPassword: ''
};

function resolveAsyncViewState({ loading, loaded, error, itemCount }) {
  if (loading && !loaded) {
    return { status: 'loading', error: '' };
  }
  if (error && !loaded) {
    return { status: 'error', error };
  }
  if (itemCount === 0) {
    return { status: 'empty', error: '' };
  }
  return { status: 'loaded', error: '' };
}

function buildRunnerOverviewItems(runners) {
  return runners.slice(0, 5).map((runner) => ({
    key: runner.id,
    title: runner.runnerName,
    subtitle: `${runner.repoOwner}/${runner.repoName}`,
    status: runner.status,
    timestamp: runner.expiresAt
  }));
}

function buildJobOverviewItems(jobs, t) {
  return jobs.slice(0, 5).map((job) => ({
    key: job.id,
    title: t('overview.activity.jobTitle', { jobId: job.githubJobId }),
    subtitle: `${job.repoOwner}/${job.repoName}`,
    status: job.status,
    timestamp: job.updatedAt
  }));
}

function buildLogOverviewItems(logs, t) {
  return logs.slice(0, 5).map((log) => ({
    key: log.id,
    title: log.message,
    subtitle: log.deliveryId || t('overview.activity.systemEvent'),
    status: log.level,
    timestamp: log.createdAt
  }));
}

function normalizeBillingBucket(item) {
  if (!item || typeof item !== 'object') {
    return null;
  }

  return {
    timeStart: item.timeStart || '',
    timeEnd: item.timeEnd || '',
    totalCost: numericOrNull(item.totalCost) ?? 0,
    totalUsageQuantity: numericOrNull(item.totalUsageQuantity) ?? 0,
    usageUnits: ensureArray(item.usageUnits)
  };
}

function normalizeBillingItem(item) {
  if (!item || typeof item !== 'object') {
    return null;
  }

  return {
    policyId: numericOrNull(item.policyId) ?? 0,
    policyLabel: String(item.policyLabel || '').trim(),
    repoOwner: String(item.repoOwner || '').trim(),
    repoName: String(item.repoName || '').trim(),
    currency: String(item.currency || '').trim(),
    totalCost: numericOrNull(item.totalCost) ?? 0,
    totalUsageQuantity: numericOrNull(item.totalUsageQuantity) ?? 0,
    usageUnits: ensureArray(item.usageUnits),
    resourceCount: numericOrNull(item.resourceCount) ?? 0,
    verifiedResourceCount: numericOrNull(item.verifiedResourceCount) ?? 0,
    fallbackResourceCount: numericOrNull(item.fallbackResourceCount) ?? 0,
    tagOnlyResourceCount: numericOrNull(item.tagOnlyResourceCount) ?? 0,
    attributionStatus: String(item.attributionStatus || '').trim(),
    timeSeries: Array.isArray(item.timeSeries) ? item.timeSeries.map(normalizeBillingBucket).filter(Boolean) : []
  };
}

function normalizeBillingIssue(item) {
  if (!item || typeof item !== 'object') {
    return null;
  }

  return {
    resourceId: String(item.resourceId || '').trim(),
    policyId: numericOrNull(item.policyId),
    policyLabel: String(item.policyLabel || '').trim(),
    repoOwner: String(item.repoOwner || '').trim(),
    repoName: String(item.repoName || '').trim(),
    tagPolicyId: String(item.tagPolicyId || '').trim(),
    currency: String(item.currency || '').trim(),
    cost: numericOrNull(item.cost) ?? 0,
    timeStart: item.timeStart || '',
    timeEnd: item.timeEnd || '',
    reason: normalizeOperatorText(item.reason, { keyPrefixes: ['operator.billing.reason'] })
  };
}

function normalizeBillingReport(payload = {}, days = 7) {
  return {
    ...createBlankBillingReportState(),
    windowStart: payload.windowStart || '',
    windowEnd: payload.windowEnd || '',
    granularity: payload.granularity || 'DAILY',
    generatedAt: payload.generatedAt || '',
    sourceRegion: payload.sourceRegion || '',
    tagNamespace: payload.tagNamespace || '',
    tagKey: payload.tagKey || '',
    tagAttributionReady: Boolean(payload.tagAttributionReady),
    currency: payload.currency || '',
    ociBilledCost: numericOrNull(payload.ociBilledCost) ?? 0,
    totalCost: numericOrNull(payload.totalCost) ?? 0,
    mappedCost: numericOrNull(payload.mappedCost) ?? 0,
    tagVerifiedCost: numericOrNull(payload.tagVerifiedCost) ?? 0,
    resourceFallbackCost: numericOrNull(payload.resourceFallbackCost) ?? 0,
    tagOnlyCost: numericOrNull(payload.tagOnlyCost) ?? 0,
    unmappedCost: numericOrNull(payload.unmappedCost) ?? 0,
    lagNotice: normalizeOperatorText(payload.lagNotice, { keyPrefixes: ['operator.billing.lagNotice'] }),
    scopeNote: normalizeOperatorText(payload.scopeNote, { keyPrefixes: ['operator.billing.scopeNote'] }),
    items: Array.isArray(payload.items) ? payload.items.map(normalizeBillingItem).filter(Boolean) : [],
    issues: Array.isArray(payload.issues) ? payload.issues.map(normalizeBillingIssue).filter(Boolean) : [],
    loaded: true,
    days
  };
}

function createBillingLoadingState(days = 7) {
  return {
    ...createBlankBillingReportState(),
    loading: true,
    days
  };
}

function createBillingErrorState(days = 7, error = '') {
  return {
    ...createBlankBillingReportState(),
    error: normalizeOperatorErrorText(error),
    days
  };
}

function ensureArray(value) {
  if (Array.isArray(value)) {
    return value.filter(Boolean);
  }
  if (typeof value === 'string') {
    return value
      .split(/[\n,]/)
      .map((entry) => entry.trim())
      .filter(Boolean);
  }
  return [];
}

function coerceString(value) {
  return value == null ? '' : String(value).trim();
}

function normalizeRepoTarget(value) {
  const normalized = coerceString(value);
  if (!normalized) {
    return null;
  }

  const parts = normalized.split('/').map((entry) => entry.trim()).filter(Boolean);
  if (parts.length !== 2) {
    return null;
  }

  const [repoOwner, repoName] = parts;
  return {
    repoOwner,
    repoName,
    repoFullName: `${repoOwner}/${repoName}`
  };
}

function buildWarmTargetKey(policyId, repoOwner, repoName) {
  return `${numericOrNull(policyId) ?? 0}:${String(repoOwner || '').trim().toLowerCase()}/${String(repoName || '').trim().toLowerCase()}`;
}

function normalizeGitHubConfigId(value) {
  return coerceString(value);
}

function normalizeGitHubDeletePath(value) {
  if (value && typeof value === 'object') {
    return coerceString(value.href || value.path || value.url || value.endpoint);
  }
  return coerceString(value);
}

function buildGitHubConfigKey(config, index = 0) {
  const configId = normalizeGitHubConfigId(config?.id);
  if (configId) {
    return `id:${configId}`;
  }

  const appId = numericOrNull(config?.appId) ?? 0;
  const installationId = numericOrNull(config?.installationId) ?? 0;
  const accountLogin = coerceString(config?.accountLogin);
  const name = coerceString(config?.name);
  const signature = `app:${appId}|install:${installationId}|account:${accountLogin}|name:${name}`;
  return signature === 'app:0|install:0|account:|name:' ? `index:${index}` : signature;
}

function dedupeGitHubConfigs(configs = []) {
  const seen = new Set();

  return configs.filter((config, index) => {
    if (!config) {
      return false;
    }

    const key = buildGitHubConfigKey(config, index);
    if (seen.has(key)) {
      return false;
    }

    seen.add(key);
    return true;
  });
}

function collectGitHubSelectedRepos(configs = [], fallback = []) {
  const merged = configs.flatMap((config) => ensureArray(config?.selectedRepos));
  if (merged.length) {
    return Array.from(new Set(merged.map((repo) => String(repo).trim()).filter(Boolean)));
  }

  return Array.from(new Set(ensureArray(fallback).map((repo) => String(repo).trim()).filter(Boolean)));
}

function resolveGitHubConfigDeletePath(payload = {}) {
  return normalizeGitHubDeletePath(
    payload.deletePath
    || payload.deleteUrl
    || payload.removePath
    || payload.deleteEndpoint
    || payload.actions?.delete
    || payload.actions?.remove
    || payload.links?.delete
    || payload.links?.remove
  );
}

function resolveGitHubActiveAppDeletePathTemplate(status = {}) {
  const candidates = [
    status.activeAppDeletePathTemplate,
    status.activeAppsDeletePathTemplate,
    status.deleteAppPathTemplate,
    status.deleteByIdPathTemplate,
    status.deleteEndpointTemplate,
    status.endpoints?.activeAppDelete,
    status.endpoints?.activeAppDeleteById,
    status.endpoints?.deleteActiveApp,
    status.links?.activeAppDelete,
    status.links?.activeAppDeleteById,
    status.actions?.activeAppDelete,
    status.actions?.activeAppDeleteById
  ];

  return candidates.map(normalizeGitHubDeletePath).find(Boolean) || '';
}

function buildGitHubActiveAppDeletePath(status = {}, config = {}) {
  const configId = normalizeGitHubConfigId(config?.id);
  if (!configId) {
    return '';
  }

  const configDeletePath = resolveGitHubConfigDeletePath(config);
  if (configDeletePath) {
    return configDeletePath;
  }

  const template = normalizeGitHubDeletePath(status.activeAppDeletePathTemplate);
  if (template) {
    if (template.includes(':id')) {
      return template.replace(':id', encodeURIComponent(configId));
    }
    if (template.includes('{id}')) {
      return template.replace('{id}', encodeURIComponent(configId));
    }
    return template.endsWith('/')
      ? `${template}${encodeURIComponent(configId)}`
      : `${template}/${encodeURIComponent(configId)}`;
  }

  if (status.activeAppDeleteSupported) {
    return `/api/v1/github/config/apps/${encodeURIComponent(configId)}`;
  }

  return '';
}

function findMatchingGitHubConfig(configs = [], candidate = null) {
  if (!candidate || !configs.length) {
    return null;
  }

  const candidateId = normalizeGitHubConfigId(candidate.id);
  if (candidateId) {
    return configs.find((config) => normalizeGitHubConfigId(config.id) === candidateId) || null;
  }

  const candidateAppId = numericOrNull(candidate.appId) ?? 0;
  const candidateInstallationId = numericOrNull(candidate.installationId) ?? 0;
  return (
    configs.find((config) =>
      (numericOrNull(config.appId) ?? 0) === candidateAppId
      && (numericOrNull(config.installationId) ?? 0) === candidateInstallationId
    )
    || null
  );
}

function deriveCurrentSetupStep(steps) {
  return SETUP_STEP_ORDER.find((stepId) => !steps[stepId]?.completed) || SETUP_STEP_ORDER[SETUP_STEP_ORDER.length - 1];
}

function normalizeSetupStatus(payload = {}, sessionData = null) {
  const base = createBlankSetupStatus(sessionData);
  const rawSteps = payload.steps || {};
  const rawPassword = rawSteps.password || payload.password || {};
  const rawGitHub = rawSteps.github || payload.github || {};
  const rawOCI = rawSteps.oci || payload.oci || payload.ociRuntime || {};

  const steps = {
    password: {
      completed: Boolean(rawPassword.completed ?? rawPassword.ready ?? !sessionData?.mustChangePassword),
      missing: ensureArray(rawPassword.missing || (sessionData?.mustChangePassword ? ['setup.missing.newPassword'] : []))
    },
    github: {
      completed: Boolean(rawGitHub.completed ?? rawGitHub.ready ?? payload.githubReady ?? false),
      missing: ensureArray(rawGitHub.missing)
    },
    oci: {
      completed: Boolean(rawOCI.completed ?? rawOCI.ready ?? rawOCI.runtimeReady ?? payload.ociReady ?? false),
      missing: ensureArray(rawOCI.missing || rawOCI.runtimeConfigMissing || payload.runtimeConfigMissing)
    }
  };

  const completed = Boolean(payload.completed ?? payload.ready ?? SETUP_STEP_ORDER.every((stepId) => steps[stepId].completed));

  return {
    ...base,
    completed,
    currentStep: completed ? SETUP_STEP_ORDER[SETUP_STEP_ORDER.length - 1] : deriveCurrentSetupStep(steps),
    updatedAt: payload.updatedAt || new Date().toISOString(),
    steps
  };
}

function normalizeGitHubConfigStatus(payload = {}) {
  const status = payload.status || payload;
  const explicitActiveConfig = status.activeConfig ? normalizeGitHubConfigView(status.activeConfig) : null;
  const stagedConfig = status.stagedConfig ? normalizeGitHubConfigView(status.stagedConfig) : null;
  const activeConfigs = dedupeGitHubConfigs([
    explicitActiveConfig,
    ...(Array.isArray(status.activeConfigs) ? status.activeConfigs.map(normalizeGitHubConfigView) : [])
  ]);
  const effectiveConfig = normalizeGitHubConfigView(
    status.effectiveConfig || status.config || status.current || explicitActiveConfig || activeConfigs[0] || stagedConfig || {}
  );
  const activeConfig = explicitActiveConfig || findMatchingGitHubConfig(activeConfigs, effectiveConfig) || activeConfigs[0] || null;
  const combined = { ...effectiveConfig, ...status };
  const activeAppDeletePathTemplate = resolveGitHubActiveAppDeletePathTemplate(status);
  const activeAppDeleteSupported = Boolean(
    activeAppDeletePathTemplate
    || combined.activeAppDeleteSupported
    || combined.supportsActiveAppDelete
    || combined.supportsDeleteById
    || combined.deleteByIdSupported
  );

  return {
    source: combined.source || (combined.configured ? 'cms' : 'env'),
    ready: Boolean(combined.ready ?? combined.completed ?? combined.valid ?? false),
    stagedReady: Boolean(combined.stagedReady),
    configured: Boolean((combined.configured ?? (combined.source === 'cms')) || activeConfig || activeConfigs.length || stagedConfig),
    hasWebhookSecret: Boolean(
      combined.hasWebhookSecret ?? combined.webhookSecretStored ?? combined.webhookSecretConfigured ?? combined.webhookSecret
    ),
    hasAppCredentials: Boolean(
      combined.hasAppCredentials
      ?? combined.appCredentialsStored
      ?? combined.privateKeyStored
      ?? combined.appId
    ),
    lastTestedAt: combined.lastTestedAt || combined.validatedAt || effectiveConfig.lastTestedAt || '',
    webhookUrl: combined.webhookUrl || combined.publicWebhookUrl || combined.webhookEndpoint || '',
    accountLogin: combined.accountLogin || effectiveConfig.accountLogin || '',
    accountType: combined.accountType || effectiveConfig.accountType || '',
    selectedRepos: collectGitHubSelectedRepos(
      activeConfigs,
      combined.selectedRepos || combined.allowedRepos || effectiveConfig.selectedRepos
    ),
    missing: ensureArray(combined.missing),
    stagedMissing: ensureArray(combined.stagedMissing),
    stagedError: combined.stagedError || '',
    activeConfigs,
    activeAppDeleteSupported,
    activeAppDeletePathTemplate,
    activeConfig,
    stagedConfig,
    effectiveConfig
  };
}

function normalizeGitHubConfigView(payload = {}) {
  const combined = { ...createBlankGitHubConfigView(), ...payload };

  return {
    ...combined,
    id: normalizeGitHubConfigId(combined.id || combined.configId),
    name: coerceString(combined.name || combined.appName),
    tags: parseGitHubTagList(combined.tags || combined.auditTags),
    apiBaseUrl: combined.apiBaseUrl || '',
    authMode: combined.authMode || '',
    appId: numericOrNull(combined.appId) ?? 0,
    installationId: numericOrNull(combined.installationId) ?? 0,
    accountLogin: combined.accountLogin || '',
    accountType: combined.accountType || '',
    deletePath: resolveGitHubConfigDeletePath(combined),
    selectedRepos: ensureArray(combined.selectedRepos || combined.allowedRepos),
    installationState: combined.installationState || '',
    installationRepositorySelection: combined.installationRepositorySelection || '',
    installationRepositories: ensureArray(combined.installationRepositories),
    installationReady: Boolean(combined.installationReady),
    installationMissing: ensureArray(combined.installationMissing),
    installationError: combined.installationError || '',
    isActive: Boolean(combined.isActive),
    isStaged: Boolean(combined.isStaged),
    lastTestedAt: combined.lastTestedAt || '',
    createdAt: combined.createdAt || '',
    updatedAt: combined.updatedAt || ''
  };
}

function normalizeOCIInspectResult(payload = {}, configText = '') {
  const selectedProfile = payload.selectedProfile || payload.profile || payload.credential || {};
  const selectedProfileName = typeof selectedProfile === 'string' ? selectedProfile : '';
  const explicitProfiles = ensureArray(payload.availableProfiles || payload.profileNames);
  const derivedProfiles = Array.isArray(payload.profiles)
    ? payload.profiles
        .map((profile) => (typeof profile === 'string' ? profile : profile?.profileName || profile?.name || ''))
        .filter(Boolean)
    : [];
  const availableProfiles = explicitProfiles.length ? explicitProfiles : derivedProfiles;
  const profileName =
    payload.profileName ||
    selectedProfile.profileName ||
    selectedProfile.name ||
    selectedProfileName ||
    availableProfiles[0] ||
    'DEFAULT';

  return {
    profileName,
    suggestedName:
      payload.credentialName ||
      payload.name ||
      payload.suggestedName ||
      selectedProfile.displayName ||
      selectedProfile.label ||
      selectedProfileName ||
      profileName,
    configText: payload.configText || payload.normalizedConfigText || configText,
    region: payload.region || selectedProfile.region || '',
    tenancyOcid: payload.tenancyOcid || selectedProfile.tenancyOcid || selectedProfile.tenancy || '',
    userOcid: payload.userOcid || selectedProfile.userOcid || selectedProfile.user || '',
    fingerprint: payload.fingerprint || selectedProfile.fingerprint || '',
    keyFile: payload.keyFile || selectedProfile.keyFile || selectedProfile.keyPath || '',
    availableProfiles
  };
}

function numericOrNull(value) {
  const number = Number(value);
  return Number.isFinite(number) ? number : null;
}

function normalizeCatalogSubnet(item) {
  if (typeof item === 'string') {
    return {
      id: item,
      displayName: normalizeOperatorText(item, { keyPrefixes: ['operator.subnet'] }),
      availabilityDomain: '',
      cidrBlock: ''
    };
  }

  if (!item || typeof item !== 'object') {
    return null;
  }

  const id = item.id || item.subnetOcid || item.ocid || '';
  if (!id) {
    return null;
  }

  return {
    ...item,
    id,
    displayName: normalizeOperatorText(item.displayName || item.name || id, {
      keyPrefixes: ['operator.subnet']
    }),
    availabilityDomain: item.availabilityDomain || item.ad || '',
    cidrBlock: item.cidrBlock || item.cidr || ''
  };
}

function normalizeCatalogImage(item) {
  if (typeof item === 'string') {
    return {
      id: item,
      displayName: item,
      operatingSystem: '',
      operatingSystemVersion: '',
      timeCreated: ''
    };
  }

  if (!item || typeof item !== 'object') {
    return null;
  }

  const id = item.id || item.imageOcid || item.ocid || '';
  if (!id) {
    return null;
  }

  return {
    ...item,
    id,
    displayName: item.displayName || item.name || id,
    operatingSystem: item.operatingSystem || '',
    operatingSystemVersion: item.operatingSystemVersion || '',
    timeCreated: item.timeCreated || ''
  };
}

function normalizeCatalogShape(item) {
  if (typeof item === 'string') {
    return {
      shape: item,
      isFlexible: false,
      defaultOcpu: null,
      defaultMemoryGb: null,
      ocpuMin: null,
      ocpuMax: null,
      memoryMinGb: null,
      memoryMaxGb: null,
      memoryDefaultPerOcpuGb: null,
      memoryMinPerOcpuGb: null,
      memoryMaxPerOcpuGb: null
    };
  }

  if (!item || typeof item !== 'object' || !item.shape) {
    return null;
  }

  return {
    ...item,
    shape: item.shape,
    isFlexible: Boolean(item.isFlexible),
    defaultOcpu: numericOrNull(item.defaultOcpu),
    defaultMemoryGb: numericOrNull(item.defaultMemoryGb),
    ocpuMin: numericOrNull(item.ocpuMin),
    ocpuMax: numericOrNull(item.ocpuMax),
    memoryMinGb: numericOrNull(item.memoryMinGb),
    memoryMaxGb: numericOrNull(item.memoryMaxGb),
    memoryDefaultPerOcpuGb: numericOrNull(item.memoryDefaultPerOcpuGb),
    memoryMinPerOcpuGb: numericOrNull(item.memoryMinPerOcpuGb),
    memoryMaxPerOcpuGb: numericOrNull(item.memoryMaxPerOcpuGb)
  };
}

function buildCatalogParams(source = {}) {
  return {
    compartmentOcid: String(source.compartmentOcid || '').trim(),
    availabilityDomain: String(source.availabilityDomain || '').trim(),
    imageOcid: String(source.imageOcid || '').trim(),
    subnetOcid: String(source.subnetOcid || '').trim()
  };
}

function looksLikeOCIOcid(value) {
  const normalized = String(value || '').trim();
  return /^ocid1\./i.test(normalized) && normalized.includes('..');
}

function normalizeCatalog(payload = {}, params = {}) {
  return {
    ...createBlankOCICatalogState(),
    availabilityDomains: ensureArray(payload.availabilityDomains),
    subnets: Array.isArray(payload.subnets) ? payload.subnets.map(normalizeCatalogSubnet).filter(Boolean) : [],
    images: Array.isArray(payload.images) ? payload.images.map(normalizeCatalogImage).filter(Boolean) : [],
    shapes: Array.isArray(payload.shapes) ? payload.shapes.map(normalizeCatalogShape).filter(Boolean) : [],
    sourceRegion: payload.sourceRegion || '',
    validatedAt: payload.validatedAt || '',
    loaded: true,
    params: buildCatalogParams(params)
  };
}

function createCatalogLoadingState(params = {}) {
  return {
    ...createBlankOCICatalogState(),
    loading: true,
    params: buildCatalogParams(params)
  };
}

function createCatalogErrorState(params = {}, error = '') {
  return {
    ...createBlankOCICatalogState(),
    error,
    params: buildCatalogParams(params)
  };
}

function buildRuntimeCatalogValidation(form, catalog, t) {
  const fieldErrors = {};
  const params = buildCatalogParams(form);
  let catalogMessage = '';

  if (!params.compartmentOcid) {
    fieldErrors.compartmentOcid = t('validation.runtime.enterCompartment');
    catalogMessage = t('validation.runtime.enterCompartment');
  } else if (!looksLikeOCIOcid(params.compartmentOcid) && !catalog.loading && !catalog.loaded && !catalog.error) {
    fieldErrors.compartmentOcid = t('validation.runtime.enterFullCompartment');
    catalogMessage = t('validation.runtime.enterFullCompartment');
  } else if (catalog.loading && !catalog.loaded) {
    catalogMessage = t('validation.runtime.loadingCatalog');
  } else if (catalog.error) {
    catalogMessage = t('validation.runtime.catalogFailed', { error: catalog.error });
  } else if (catalog.loaded) {
    if (!catalog.availabilityDomains.length) {
      fieldErrors.availabilityDomain = t('validation.runtime.noAvailabilityDomains');
    } else if (!params.availabilityDomain) {
      fieldErrors.availabilityDomain = t('validation.runtime.chooseAvailabilityDomain');
    } else if (!catalog.availabilityDomains.includes(params.availabilityDomain)) {
      fieldErrors.availabilityDomain = t('validation.runtime.staleAvailabilityDomain');
    }

    const subnetIds = new Set(catalog.subnets.map((item) => item.id));
    if (!catalog.subnets.length) {
      fieldErrors.subnetOcid = t('validation.runtime.noSubnets');
    } else if (!params.subnetOcid) {
      fieldErrors.subnetOcid = t('validation.runtime.chooseSubnet');
    } else if (!subnetIds.has(params.subnetOcid)) {
      fieldErrors.subnetOcid = t('validation.runtime.staleSubnet');
    }

    const imageIds = new Set(catalog.images.map((item) => item.id));
    if (!catalog.images.length) {
      fieldErrors.imageOcid = t('validation.runtime.noImages');
    } else if (!params.imageOcid) {
      fieldErrors.imageOcid = t('validation.runtime.chooseImage');
    } else if (!imageIds.has(params.imageOcid)) {
      fieldErrors.imageOcid = t('validation.runtime.staleImage');
    }
  }

  if (form.cacheCompatEnabled) {
    if (!String(form.cacheBucketName || '').trim()) {
      fieldErrors.cacheBucketName = t('validation.runtime.cacheBucketRequired');
    }

    const cacheRetentionDays = Number(form.cacheRetentionDays);
    if (!Number.isFinite(cacheRetentionDays) || cacheRetentionDays <= 0) {
      fieldErrors.cacheRetentionDays = t('validation.runtime.cacheRetentionPositive');
    }
  }

  return {
    canSave: !catalogMessage && Object.keys(fieldErrors).length === 0,
    catalogMessage,
    fieldErrors
  };
}

function buildPolicyValidation(form, runtimeStatus, catalog, t) {
  const fieldErrors = {};
  let settingsMessage = '';
  let capacityMessage = '';
  const shapeName = String(form.shape || '').trim();
  const shapeByName = new Map((catalog.shapes || []).map((item) => [item.shape, item]));
  const selectedShape = shapeByName.get(shapeName) || null;

  if (!runtimeStatus.ready) {
    settingsMessage = t('validation.policy.settings.runtimeRequired');
  } else if (catalog.error) {
    settingsMessage = t('validation.policy.settings.catalogFailed', { error: catalog.error });
  } else if (catalog.loading && !catalog.loaded) {
    settingsMessage = t('validation.policy.settings.catalogLoading');
  } else if (!catalog.loaded) {
    settingsMessage = t('validation.policy.settings.catalogRequired');
  } else if (!catalog.shapes.length) {
    settingsMessage = t('validation.policy.settings.noShapes');
  }

  if (!settingsMessage) {
    if (!shapeName) {
      fieldErrors.shape = t('validation.policy.shape.required');
    } else if (!selectedShape) {
      fieldErrors.shape = t('validation.policy.shape.stale');
    } else if (!selectedShape.isFlexible && (selectedShape.defaultOcpu == null || selectedShape.defaultMemoryGb == null)) {
      fieldErrors.shape = t('validation.policy.shape.fixedDefaultsMissing');
    } else if (!selectedShape.isFlexible) {
      capacityMessage = t('validation.policy.capacity.fixed', {
        ocpu: selectedShape.defaultOcpu ?? t('validation.policy.capacity.defaultValue'),
        memoryGb: selectedShape.defaultMemoryGb ?? t('validation.policy.capacity.defaultValue')
      });
    } else {
      const ocpu = Number(form.ocpu);
      const memoryGb = Number(form.memoryGb);

      if (!Number.isFinite(ocpu)) {
        fieldErrors.ocpu = t('validation.policy.ocpu.required');
      } else if (selectedShape.ocpuMin != null && ocpu < selectedShape.ocpuMin) {
        fieldErrors.ocpu = t('validation.policy.ocpu.min', {
          min: selectedShape.ocpuMin,
          shape: selectedShape.shape
        });
      } else if (selectedShape.ocpuMax != null && ocpu > selectedShape.ocpuMax) {
        fieldErrors.ocpu = t('validation.policy.ocpu.max', {
          max: selectedShape.ocpuMax,
          shape: selectedShape.shape
        });
      }

      if (!Number.isFinite(memoryGb)) {
        fieldErrors.memoryGb = t('validation.policy.memory.required');
      } else if (selectedShape.memoryMinGb != null && memoryGb < selectedShape.memoryMinGb) {
        fieldErrors.memoryGb = t('validation.policy.memory.min', {
          min: selectedShape.memoryMinGb,
          shape: selectedShape.shape
        });
      } else if (selectedShape.memoryMaxGb != null && memoryGb > selectedShape.memoryMaxGb) {
        fieldErrors.memoryGb = t('validation.policy.memory.max', {
          max: selectedShape.memoryMaxGb,
          shape: selectedShape.shape
        });
      }

      if (!fieldErrors.ocpu && !fieldErrors.memoryGb && ocpu > 0) {
        const memoryPerOcpu = memoryGb / ocpu;
        if (selectedShape.memoryMinPerOcpuGb != null && memoryPerOcpu < selectedShape.memoryMinPerOcpuGb) {
          fieldErrors.memoryGb = t('validation.policy.memory.perOcpuMin', {
            min: selectedShape.memoryMinPerOcpuGb,
            shape: selectedShape.shape
          });
        } else if (selectedShape.memoryMaxPerOcpuGb != null && memoryPerOcpu > selectedShape.memoryMaxPerOcpuGb) {
          fieldErrors.memoryGb = t('validation.policy.memory.perOcpuMax', {
            max: selectedShape.memoryMaxPerOcpuGb,
            shape: selectedShape.shape
          });
        }
      }

      if (!fieldErrors.ocpu && !fieldErrors.memoryGb) {
        const bounds = [];
        if (selectedShape.ocpuMin != null && selectedShape.ocpuMax != null) {
          bounds.push(
            t('validation.policy.capacity.bound.ocpu', {
              min: selectedShape.ocpuMin,
              max: selectedShape.ocpuMax
            })
          );
        }
        if (selectedShape.memoryMinGb != null && selectedShape.memoryMaxGb != null) {
          bounds.push(
            t('validation.policy.capacity.bound.memory', {
              min: selectedShape.memoryMinGb,
              max: selectedShape.memoryMaxGb
            })
          );
        }
        if (selectedShape.memoryMinPerOcpuGb != null && selectedShape.memoryMaxPerOcpuGb != null) {
          bounds.push(
            t('validation.policy.capacity.bound.memoryPerOcpu', {
              min: selectedShape.memoryMinPerOcpuGb,
              max: selectedShape.memoryMaxPerOcpuGb
            })
          );
        }
        capacityMessage = bounds.length
          ? t('validation.policy.capacity.flexibleBounds', { bounds: bounds.join(' · ') })
          : t('validation.policy.capacity.flexibleFallback');
      }
    }
  }

  if (form.warmEnabled) {
    const warmMinIdle = Number(form.warmMinIdle);
    const warmTtlMinutes = Number(form.warmTtlMinutes);
    const warmRepoAllowlist = parseGitHubRepoSelection(form.warmRepoAllowlistText);

    if (!Number.isFinite(warmMinIdle) || warmMinIdle < 0 || warmMinIdle > 1) {
      fieldErrors.warmMinIdle = t('validation.policy.warm.minIdleBinary');
    } else if (warmMinIdle > 0 && warmRepoAllowlist.length === 0) {
      fieldErrors.warmRepoAllowlistText = t('validation.policy.warm.repoAllowlistRequired');
    }

    if (!Number.isFinite(warmTtlMinutes) || warmTtlMinutes <= 0) {
      fieldErrors.warmTtlMinutes = t('validation.policy.warm.ttlPositive');
    }
  }

  if (form.budgetEnabled) {
    const budgetCapAmount = Number(form.budgetCapAmount);
    const budgetWindowDays = Number(form.budgetWindowDays);

    if (!Number.isFinite(budgetCapAmount) || budgetCapAmount <= 0) {
      fieldErrors.budgetCapAmount = t('validation.policy.budget.capPositive');
    }

    if (!Number.isFinite(budgetWindowDays) || budgetWindowDays !== 7) {
      fieldErrors.budgetWindowDays = t('validation.policy.budget.windowFixed');
    }
  }

  return {
    canSave: !settingsMessage && Object.keys(fieldErrors).length === 0,
    settingsMessage,
    capacityMessage,
    fieldErrors,
    selectedShape
  };
}

function ensureRecord(value) {
  return value && typeof value === 'object' && !Array.isArray(value) ? value : null;
}

function normalizePolicy(item) {
  const source = ensureRecord(item) || {};

  return {
    ...source,
    labels: ensureArray(source.labels),
    subnetOcid: String(source.subnetOcid || '').trim(),
    shape: String(source.shape || '').trim(),
    ocpu: numericOrNull(source.ocpu) ?? 0,
    memoryGb: numericOrNull(source.memoryGb) ?? 0,
    maxRunners: numericOrNull(source.maxRunners) ?? 0,
    ttlMinutes: numericOrNull(source.ttlMinutes) ?? 0,
    spot: Boolean(source.spot),
    enabled: source.enabled == null ? false : Boolean(source.enabled),
    warmEnabled: Boolean(source.warmEnabled),
    warmMinIdle: numericOrNull(source.warmMinIdle) ?? 0,
    warmTtlMinutes: numericOrNull(source.warmTtlMinutes) ?? 0,
    warmRepoAllowlist: ensureArray(source.warmRepoAllowlist),
    budgetEnabled: Boolean(source.budgetEnabled),
    budgetCapAmount: numericOrNull(source.budgetCapAmount) ?? 0,
    budgetWindowDays: numericOrNull(source.budgetWindowDays) ?? 7
  };
}

function normalizeDiagnosticStage(item) {
  const source = ensureRecord(item) || {};

  return {
    state: String(source.state || '').trim(),
    code: String(source.code || '').trim(),
    message: normalizeOperatorText(source.message, {
      keyPrefixes: ['operator.diagnostic.message', 'operator.billing.reason', 'operator.error']
    }),
    details: ensureRecord(source.details) || {},
    updatedAt: source.updatedAt || ''
  };
}

function normalizeDiagnosticStageMap(value) {
  const source = ensureRecord(value) || {};
  return Object.fromEntries(
    Object.entries(source)
      .map(([key, item]) => [key, normalizeDiagnosticStage(item)])
      .filter(([, item]) => item.state || item.code || item.message || Object.keys(item.details).length || item.updatedAt)
  );
}

function resolveDiagnosticBlockingMessage(source, stageStatuses = {}) {
  const explicitMessage = normalizeOperatorText(source?.blockingMessage, {
    keyPrefixes: ['operator.diagnostic.message', 'operator.billing.reason', 'operator.job', 'operator.error']
  });
  if (explicitMessage) {
    return explicitMessage;
  }

  const blockingStage = coerceString(source?.blockingStage);
  if (blockingStage && stageStatuses[blockingStage]?.message) {
    return stageStatuses[blockingStage].message;
  }

  for (const stageName of [
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
  ]) {
    const stage = stageStatuses[stageName];
    if (!stage) {
      continue;
    }
    if (String(stage.state || '').trim().toLowerCase() === 'blocked' && stage.message) {
      return stage.message;
    }
  }

  return '';
}

function normalizePolicyCheck(item) {
  const source = ensureRecord(item) || {};

  return {
    policyId: numericOrNull(source.policyId) ?? 0,
    policyLabel: String(source.policyLabel || '').trim(),
    policyLabels: ensureArray(source.policyLabels || source.labels),
    matched: Boolean(source.matched),
    reasons: ensureArray(source.reasons).map((value) =>
      normalizeOperatorText(value, {
        keyPrefixes: ['operator.diagnostic.message', 'operator.billing.reason', 'operator.job', 'operator.error']
      })
    ),
    missingLabels: ensureArray(source.missingLabels),
    extraLabels: ensureArray(source.extraLabels),
    capacityBlocked: Boolean(source.capacityBlocked),
    activeRunners: numericOrNull(source.activeRunners) ?? 0,
    maxRunners: numericOrNull(source.maxRunners) ?? 0,
    budgetBlocked: Boolean(source.budgetBlocked),
    budgetDegraded: Boolean(source.budgetDegraded),
    budgetMessage: normalizeOperatorText(source.budgetMessage, {
      keyPrefixes: ['operator.billing.reason', 'operator.diagnostic.message', 'operator.error']
    }),
    warmConfigured: Boolean(source.warmConfigured),
    warmRepoEligible: Boolean(source.warmRepoEligible)
  };
}

function normalizeCompatibilityResult(payload = {}) {
  const source = ensureRecord(payload) || {};
  const stageStatuses = normalizeDiagnosticStageMap(source.stageStatuses);

  return {
    requestedLabels: ensureArray(source.normalizedLabels || source.requestedLabels),
    normalizedLabels: ensureArray(source.normalizedLabels || source.requestedLabels),
    blockingStage: String(source.blockingStage || '').trim(),
    summaryCode: String(source.summaryCode || '').trim(),
    blockingMessage: resolveDiagnosticBlockingMessage(source, stageStatuses),
    matchedPolicy: source.matchedPolicy ? normalizePolicy(source.matchedPolicy) : null,
    launchRequired: source.launchRequired == null ? true : Boolean(source.launchRequired),
    warmCandidate: ensureRecord(source.warmCandidate) || null,
    stageStatuses,
    policyChecks: Array.isArray(source.policyChecks) ? source.policyChecks.map(normalizePolicyCheck).filter(Boolean) : []
  };
}

function normalizeJobDiagnostic(item) {
  const envelope = ensureRecord(item) || {};
  const source = ensureRecord(envelope.diagnostic) || envelope;
  const jobSource = ensureRecord(envelope.job) || {};
  const stageStatuses = normalizeDiagnosticStageMap(source.stageStatuses);

  return {
    jobId: numericOrNull(source.jobId ?? jobSource.id) ?? 0,
    deliveryId: String(source.deliveryId || jobSource.deliveryId || '').trim(),
    summaryCode: String(source.summaryCode || '').trim(),
    blockingStage: String(source.blockingStage || '').trim(),
    blockingMessage: resolveDiagnosticBlockingMessage(source, stageStatuses),
    matchedPolicyId: numericOrNull(source.matchedPolicyId),
    runnerId: numericOrNull(source.runnerId),
    instanceOcid: String(source.instanceOcid || '').trim(),
    stageStatuses,
    createdAt: source.createdAt || '',
    updatedAt: source.updatedAt || ''
  };
}

function normalizeGuardrailItem(item) {
  const source = ensureRecord(item) || {};

  return {
    policyId: numericOrNull(source.policyId) ?? 0,
    policyLabel: String(source.policyLabel || '').trim(),
    budgetEnabled: Boolean(source.budgetEnabled),
    budgetCapAmount: numericOrNull(source.budgetCapAmount) ?? 0,
    budgetWindowDays: numericOrNull(source.budgetWindowDays) ?? 7,
    currency: String(source.currency || '').trim(),
    totalCost: numericOrNull(source.totalCost) ?? 0,
    blocked: Boolean(source.blocked),
    degraded: Boolean(source.degraded),
    reasonCode: String(source.reasonCode || '').trim(),
    message: normalizeOperatorText(source.message, {
      keyPrefixes: ['operator.billing.reason', 'operator.diagnostic.message', 'operator.error']
    }),
    snapshotGeneratedAt: source.snapshotGeneratedAt || ''
  };
}

function normalizeBillingGuardrailsReport(payload = {}) {
  const source = ensureRecord(payload) || {};

  return {
    ...createBlankBillingGuardrailsState(),
    generatedAt: source.generatedAt || '',
    windowDays: numericOrNull(source.windowDays) ?? 0,
    items: Array.isArray(source.items) ? source.items.map(normalizeGuardrailItem).filter(Boolean) : [],
    loaded: true,
    available: true
  };
}

function normalizeGitHubDriftIssue(item) {
  const source = ensureRecord(item) || {};

  return {
    code: String(source.code || '').trim(),
    severity: String(source.severity || '').trim(),
    configId: numericOrNull(source.configId),
    source: String(source.source || '').trim(),
    apiBaseUrl: String(source.apiBaseUrl || '').trim(),
    appId: numericOrNull(source.appId) ?? 0,
    installationId: numericOrNull(source.installationId) ?? 0,
    installationState: String(source.installationState || '').trim(),
    missingSelectedRepos: ensureArray(source.missingSelectedRepos),
    newlyVisibleRepos: ensureArray(source.newlyVisibleRepos),
    message: normalizeOperatorText(source.message, {
      keyPrefixes: ['operator.githubDrift.message', 'operator.error']
    })
  };
}

function normalizeGitHubDriftStatus(payload = {}) {
  const source = ensureRecord(payload) || {};

  return {
    ...createBlankGitHubDriftState(),
    generatedAt: source.generatedAt || '',
    severity: String(source.severity || 'ok').trim() || 'ok',
    activeConfigs: Array.isArray(source.activeConfigs) ? source.activeConfigs.map(normalizeGitHubConfigView).filter(Boolean) : [],
    stagedConfig: source.stagedConfig ? normalizeGitHubConfigView(source.stagedConfig) : null,
    issues: Array.isArray(source.issues) ? source.issues.map(normalizeGitHubDriftIssue).filter(Boolean) : [],
    loaded: true,
    available: true
  };
}

function normalizeCommandList(value) {
  if (Array.isArray(value)) {
    return value
      .map((entry) => {
        if (typeof entry === 'string') {
          return entry.trim();
        }
        if (!entry || typeof entry !== 'object') {
          return '';
        }
        return String(
          entry.command
          || entry.cmd
          || entry.value
          || entry.text
          || entry.line
          || entry.label
          || entry.title
          || entry.name
          || ''
        ).trim();
      })
      .filter(Boolean);
  }

  if (typeof value === 'string') {
    return value
      .split(/\r?\n/)
      .map((entry) => entry.trim())
      .filter(Boolean);
  }

  return [];
}

function normalizeJob(item) {
  const source = ensureRecord(item) || {};
  const diagnostic = source.diagnostic ? normalizeJobDiagnostic(source.diagnostic) : null;
  const diagnosticSummaryCode = String(source.summaryCode || source.diagnosticSummaryCode || diagnostic?.summaryCode || '').trim();
  const diagnosticBlockingStage = String(source.blockingStage || source.diagnosticBlockingStage || diagnostic?.blockingStage || '').trim();
  const diagnosticBlockingMessage = normalizeOperatorText(source.blockingMessage || source.diagnosticBlockingMessage, {
    keyPrefixes: ['operator.diagnostic.message', 'operator.billing.reason', 'operator.job', 'operator.error']
  }) || diagnostic?.blockingMessage || '';

  return {
    ...source,
    status: String(source.status || '').trim(),
    repoOwner: String(source.repoOwner || '').trim(),
    repoName: String(source.repoName || '').trim(),
    labels: ensureArray(source.labels),
    summaryCode: diagnosticSummaryCode,
    blockingStage: diagnosticBlockingStage,
    blockingMessage: diagnosticBlockingMessage,
    diagnosticSummaryCode,
    diagnosticBlockingStage,
    diagnosticBlockingMessage,
    diagnostic,
    errorMessage: normalizeOperatorText(source.errorMessage, { keyPrefixes: ['operator.job', 'operator.error'] })
  };
}

function normalizeRunner(item) {
  const source = ensureRecord(item) || {};

  return {
    ...source,
    status: String(source.status || '').trim(),
    repoOwner: String(source.repoOwner || '').trim(),
    repoName: String(source.repoName || '').trim(),
    runnerName: String(source.runnerName || source.name || '').trim(),
    labels: ensureArray(source.labels),
    source: String(source.source || '').trim(),
    warmState: String(source.warmState || '').trim(),
    warmPolicyId: numericOrNull(source.warmPolicyId),
    warmRepoOwner: String(source.warmRepoOwner || '').trim(),
    warmRepoName: String(source.warmRepoName || '').trim()
  };
}

function normalizeEvent(item) {
  const source = ensureRecord(item) || {};

  return {
    ...source,
    action: String(source.action || '').trim(),
    repoOwner: String(source.repoOwner || '').trim(),
    repoName: String(source.repoName || '').trim(),
    deliveryId: String(source.deliveryId || '').trim()
  };
}

function normalizeLog(item) {
  const source = ensureRecord(item) || {};

  return {
    ...source,
    level: String(source.level || '').trim(),
    message: normalizeOperatorText(source.message, { keyPrefixes: ['operator.event', 'operator.job', 'operator.runnerImages'] }),
    deliveryId: String(source.deliveryId || '').trim(),
    detailsJson: String(source.detailsJson || '').trim()
  };
}

function normalizeSubnetCandidate(item) {
  const source = ensureRecord(item) || {};

  return {
    ...source,
    displayName: normalizeOperatorText(source.displayName || source.name || source.id, {
      keyPrefixes: ['operator.subnet']
    }),
    recommendation: normalizeOperatorText(source.recommendation, { keyPrefixes: ['operator.subnet'] })
  };
}

function normalizeRunnerImageTags(value) {
  if (Array.isArray(value)) {
    return value
      .map((entry) => {
        if (typeof entry === 'string') {
          const [key, ...rest] = entry.split('=');
          return {
            key: String(key || '').trim(),
            value: rest.join('=').trim()
          };
        }
        if (!entry || typeof entry !== 'object') {
          return null;
        }
        const key = String(entry.key || entry.name || '').trim();
        if (!key) {
          return null;
        }
        return {
          key,
          value: String(entry.value || entry.displayValue || '').trim()
        };
      })
      .filter(Boolean);
  }

  if (value && typeof value === 'object') {
    return Object.entries(value)
      .map(([key, rawValue]) => ({
        key: String(key || '').trim(),
        value: String(rawValue || '').trim()
      }))
      .filter((entry) => entry.key);
  }

  return [];
}

function normalizeRunnerImageCheck(item) {
  if (typeof item === 'string') {
    return {
      name: normalizeOperatorText(item, { keyPrefixes: ['operator.runnerImages.preflight'] }),
      status: '',
      detail: ''
    };
  }

  if (!item || typeof item !== 'object') {
    return null;
  }

  const name = String(item.name || item.label || item.title || item.id || '').trim();
  if (!name) {
    return null;
  }

  const rawDetail = String(item.detail || item.summary || item.message || '').trim();
  const detailParts = rawDetail
    .split(',')
    .map((entry) => entry.trim())
    .filter(Boolean);

  return {
    name: normalizeOperatorText(name, { keyPrefixes: ['operator.runnerImages.preflight'] }),
    status: String(item.status || item.state || item.result || '').trim(),
    detail: detailParts.length > 1
      ? normalizeOperatorList(detailParts)
      : normalizeOperatorText(rawDetail, {
        keyPrefixes: ['operator.runnerImages.preflight', 'operator.error']
      })
  };
}

function normalizeRunnerImagePreflight(payload = {}, runtimeReady = false) {
  const source = ensureRecord(payload) || {};
  const checks = Array.isArray(source.checks)
    ? source.checks.map(normalizeRunnerImageCheck).filter(Boolean)
    : Array.isArray(source.items)
      ? source.items.map(normalizeRunnerImageCheck).filter(Boolean)
      : [];
  const ready = Boolean(source.ready ?? source.completed ?? source.passed ?? false);
  const blocked = Boolean(source.blocked ?? source.requiresSetup ?? !runtimeReady);

  return {
    loaded: Boolean(Object.keys(source).length),
    ready,
    blocked,
    status: String(source.status || (blocked ? 'blocked' : ready ? 'ready' : '')).trim(),
    summary: normalizeOperatorText(source.summary || source.description || source.message, {
      keyPrefixes: ['operator.runnerImages.preflight']
    }),
    resultSummary: normalizeOperatorText(source.resultSummary || source.result || source.note, {
      keyPrefixes: ['operator.runnerImages.preflight']
    }),
    updatedAt: source.updatedAt || source.checkedAt || source.lastVerifiedAt || '',
    missing: ensureArray(source.missing || source.missingItems || source.blockers).map((value) => normalizeOperatorText(value)),
    notes: ensureArray(source.notes || source.hints).map((value) =>
      normalizeOperatorText(value, { keyPrefixes: ['operator.runnerImages.preflight'] })
    ),
    setupCommands: normalizeCommandList(source.setupCommands || source.prepareCommands || source.preflightCommands),
    verifyCommands: normalizeCommandList(source.verifyCommands || source.validationCommands || source.checkCommands),
    checks
  };
}

function normalizeRunnerImageRecipe(item, index = 0) {
  const source = ensureRecord(item) || {};
  const id = String(source.id || source.name || source.displayName || `recipe-${index + 1}`).trim();

  return {
    id,
    name: String(source.name || source.slug || id).trim(),
    displayName: String(source.imageDisplayName || source.displayName || source.title || source.name || id).trim(),
    description: String(source.description || source.summary || '').trim(),
    baseImage: String(source.baseImageOcid || source.baseImage || source.baseImageName || source.sourceImage || '').trim(),
    subnetOcid: String(source.subnetOcid || '').trim(),
    shape: String(source.shape || '').trim(),
    ocpu: numericOrNull(source.ocpu) ?? 0,
    memoryGb: numericOrNull(source.memoryGb) ?? 0,
    promotedBuildId: String(source.promotedBuildId || '').trim(),
    promotedImage: String(source.promotedImageOcid || '').trim(),
    updatedAt: source.updatedAt || source.modifiedAt || source.createdAt || '',
    latestBuildStatus: String(source.latestBuildStatus || source.buildStatus || '').trim(),
    setupCommands: normalizeCommandList(source.setupCommands || source.prepareCommands),
    verifyCommands: normalizeCommandList(source.verifyCommands || source.validationCommands)
  };
}

function normalizeRunnerImageBuild(item, index = 0) {
  const source = ensureRecord(item) || {};
  const id = String(source.id || source.buildId || source.name || `build-${index + 1}`).trim();

  return {
    id,
    name: String(source.name || source.displayName || id).trim(),
    recipeId: String(source.recipeId || '').trim(),
    recipeName: String(source.recipeName || source.recipe || '').trim(),
    status: String(source.status || source.state || '').trim(),
    imageReference: String(source.imageOcid || source.imageReference || source.image || source.ocirReference || '').trim(),
    instanceOcid: String(source.instanceOcid || '').trim(),
    statusMessage: normalizeOperatorText(source.statusMessage, { keyPrefixes: ['operator.runnerImages'] }),
    errorMessage: normalizeOperatorText(source.errorMessage, { keyPrefixes: ['operator.runnerImages', 'operator.error'] }),
    summary: normalizeOperatorText(source.summary || source.errorMessage || source.statusMessage || source.resultSummary || source.message, {
      keyPrefixes: ['operator.runnerImages', 'operator.error']
    }),
    logExcerpt: normalizeOperatorText(source.logExcerpt || source.errorMessage || source.statusMessage || source.logTail || source.excerpt, {
      keyPrefixes: ['operator.runnerImages', 'operator.error']
    }),
    startedAt: source.launchedAt || source.startedAt || source.createdAt || '',
    finishedAt: source.completedAt || source.finishedAt || source.updatedAt || '',
    setupCommands: normalizeCommandList(source.setupCommands || source.prepareCommands),
    verifyCommands: normalizeCommandList(source.verifyCommands || source.validationCommands),
    canPromote: Boolean(source.canPromote ?? source.promotable ?? String(source.status || '').toLowerCase() === 'available'),
    promoted: Boolean(source.promoted ?? source.isPromoted ?? source.promotedAt)
  };
}

function normalizeRunnerImageResource(item, index = 0) {
  const source = ensureRecord(item) || {};
  const id = String(source.id || source.resourceId || source.imageId || `resource-${index + 1}`).trim();

  return {
    id,
    name: String(source.name || source.displayName || id).trim(),
    kind: normalizeOperatorText(source.kind || source.type || 'image', {
      keyPrefixes: ['operator.runnerImages.resourceKind']
    }),
    imageReference: String(source.imageReference || source.image || source.ocirReference || source.digest || source.id || '').trim(),
    status: String(source.status || source.state || '').trim(),
    compartment: String(source.compartment || source.compartmentName || '').trim(),
    sourceBuildId: String(source.sourceBuildId || source.buildId || '').trim(),
    discoveredAt: source.discoveredAt || source.createdAt || source.updatedAt || '',
    tags: normalizeRunnerImageTags(source.tags || source.definedTags || source.freeformTags),
    tracked: Boolean(source.tracked),
    canPromote: Boolean(source.canPromote ?? source.promotable ?? false),
    promoted: Boolean(source.promoted ?? source.isPromoted ?? false)
  };
}

function normalizeRunnerImageSelection(item) {
  const source = ensureRecord(item);
  if (!source) {
    return null;
  }

  return {
    name: normalizeOperatorText(source.name || source.displayName || source.imageReference || source.image, {
      keyPrefixes: ['operator.runnerImages']
    }),
    imageReference: String(source.imageReference || source.image || source.ocirReference || '').trim(),
    recipeName: String(source.recipeName || source.recipe || '').trim(),
    updatedAt: source.updatedAt || source.promotedAt || source.selectedAt || ''
  };
}

function extractRunnerImageItems(source, key) {
  if (Array.isArray(source?.[key])) {
    return source[key];
  }
  if (Array.isArray(source?.[key]?.items)) {
    return source[key].items;
  }
  return [];
}

function normalizeRunnerImagesPayload(payload = {}, runtimeReady = false) {
  const source = ensureRecord(payload) || {};
  const recipes = extractRunnerImageItems(source, 'recipes').map(normalizeRunnerImageRecipe).filter(Boolean);
  const builds = extractRunnerImageItems(source, 'builds').map(normalizeRunnerImageBuild).filter(Boolean);
  const resources = (
    extractRunnerImageItems(source, 'resources').length
      ? extractRunnerImageItems(source, 'resources')
      : extractRunnerImageItems(source, 'discoveredResources')
  )
    .map(normalizeRunnerImageResource)
    .filter(Boolean);

  return {
    ...createBlankRunnerImagesState(),
    loaded: true,
    preflight: normalizeRunnerImagePreflight(source.preflight || source.readiness || {}, runtimeReady),
    recipes,
    builds,
    resources,
    defaultImage: normalizeRunnerImageSelection(source.defaultImage || source.currentDefaultImage),
    promotedImage: normalizeRunnerImageSelection(source.promotedImage || source.latestPromotedImage)
  };
}

function buildRunnerImageRecipeForm(recipe = {}) {
  return {
    ...createBlankRunnerImageRecipeForm(),
    name: String(recipe.name || '').trim(),
    displayName: String(recipe.displayName || recipe.imageDisplayName || '').trim(),
    baseImage: String(recipe.baseImage || recipe.baseImageOcid || '').trim(),
    subnetOcid: String(recipe.subnetOcid || '').trim(),
    shape: String(recipe.shape || '').trim(),
    ocpu: numericOrNull(recipe.ocpu) ?? 1,
    memoryGb: numericOrNull(recipe.memoryGb) ?? 16,
    description: String(recipe.description || '').trim(),
    setupCommandsText: normalizeCommandList(recipe.setupCommands).join('\n'),
    verifyCommandsText: normalizeCommandList(recipe.verifyCommands).join('\n')
  };
}

function normalizeRunnerImageRecipePayload(form = {}) {
  return {
    name: String(form.name || '').trim(),
    imageDisplayName: String(form.displayName || '').trim(),
    baseImageOcid: String(form.baseImage || '').trim(),
    subnetOcid: String(form.subnetOcid || '').trim(),
    shape: String(form.shape || '').trim(),
    ocpu: numericOrNull(form.ocpu) ?? 0,
    memoryGb: numericOrNull(form.memoryGb) ?? 0,
    description: String(form.description || '').trim(),
    setupCommands: normalizeCommandList(form.setupCommandsText),
    verifyCommands: normalizeCommandList(form.verifyCommandsText)
  };
}

function validateRunnerImageRecipeForm(form, t) {
  if (!String(form.name || '').trim()) {
    return t('validation.runnerImages.recipe.nameRequired');
  }
  if (!String(form.baseImage || '').trim()) {
    return t('validation.runnerImages.recipe.baseImageRequired');
  }
  if (!String(form.shape || '').trim()) {
    return t('validation.runnerImages.recipe.shapeRequired');
  }
  if ((numericOrNull(form.ocpu) ?? 0) <= 0) {
    return t('validation.runnerImages.recipe.ocpuPositive');
  }
  if ((numericOrNull(form.memoryGb) ?? 0) <= 0) {
    return t('validation.runnerImages.recipe.memoryPositive');
  }
  if (!normalizeCommandList(form.setupCommandsText).length) {
    return t('validation.runnerImages.recipe.setupCommandsRequired');
  }
  if (!normalizeCommandList(form.verifyCommandsText).length) {
    return t('validation.runnerImages.recipe.verifyCommandsRequired');
  }
  return '';
}

export function useWorkspaceApp() {
  const { t } = useI18n();
  const [view, setView] = useState('overview');
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [session, setSession] = useState(null);
  const [setupStatus, setSetupStatus] = useState(() => createBlankSetupStatus());
  const [activeOnboardingStep, setActiveOnboardingStep] = useState('password');
  const [policies, setPolicies] = useState([]);
  const [jobs, setJobs] = useState([]);
  const [runners, setRunners] = useState([]);
  const [events, setEvents] = useState([]);
  const [logs, setLogs] = useState([]);
  const [subnetCandidates, setSubnetCandidates] = useState([]);
  const [defaultSubnetId, setDefaultSubnetId] = useState('');
  const [subnetError, setSubnetError] = useState('');
  const [githubConfigStatus, setGithubConfigStatus] = useState(createBlankGitHubConfigStatus);
  const [githubConfigForm, setGithubConfigForm] = useState(blankGitHubConfigForm);
  const [githubConfigMode, setGithubConfigMode] = useState(GITHUB_SETUP_MODE_CREATE);
  const [githubConfigResult, setGithubConfigResult] = useState(null);
  const [githubManifestState, setGithubManifestState] = useState(createBlankGitHubManifestState);
  const [githubConfigTesting, setGithubConfigTesting] = useState(false);
  const [githubConfigSaving, setGithubConfigSaving] = useState(false);
  const [githubConfigClearing, setGithubConfigClearing] = useState(false);
  const [githubConfigPromoting, setGithubConfigPromoting] = useState(false);
  const [githubActiveAppDeletingId, setGithubActiveAppDeletingId] = useState('');
  const [ociAuthStatus, setOciAuthStatus] = useState(createBlankOCIAuthStatus);
  const [ociAuthForm, setOciAuthForm] = useState(blankOCIAuthForm);
  const [ociAuthResult, setOciAuthResult] = useState(null);
  const [ociAuthInspecting, setOciAuthInspecting] = useState(false);
  const [ociAuthInspectResult, setOciAuthInspectResult] = useState(null);
  const [ociAuthTesting, setOciAuthTesting] = useState(false);
  const [ociAuthSaving, setOciAuthSaving] = useState(false);
  const [ociAuthClearing, setOciAuthClearing] = useState(false);
  const [ociRuntimeStatus, setOciRuntimeStatus] = useState(createBlankOCIRuntimeStatus);
  const [ociRuntimeForm, setOciRuntimeForm] = useState(blankOCIRuntimeForm);
  const [ociRuntimeSaving, setOciRuntimeSaving] = useState(false);
  const [ociRuntimeClearing, setOciRuntimeClearing] = useState(false);
  const [runtimeCatalog, setRuntimeCatalog] = useState(createBlankOCICatalogState);
  const [policyCatalog, setPolicyCatalog] = useState(createBlankOCICatalogState);
  const [billingReport, setBillingReport] = useState(createBlankBillingReportState);
  const [billingGuardrails, setBillingGuardrails] = useState(createBlankBillingGuardrailsState);
  const [githubDriftStatus, setGithubDriftStatus] = useState(createBlankGitHubDriftState);
  const [githubDriftReconciling, setGithubDriftReconciling] = useState(false);
  const [policyCompatibilityForm, setPolicyCompatibilityForm] = useState({
    repoOwner: '',
    repoName: '',
    labelsText: ''
  });
  const [policyCompatibilityResult, setPolicyCompatibilityResult] = useState(null);
  const [policyCompatibilityLoading, setPolicyCompatibilityLoading] = useState(false);
  const [policyCompatibilityError, setPolicyCompatibilityError] = useState('');
  const [jobDiagnosticsByJobId, setJobDiagnosticsByJobId] = useState({});
  const [jobDiagnosticsErrorsByJobId, setJobDiagnosticsErrorsByJobId] = useState({});
  const [jobDiagnosticsLoadingId, setJobDiagnosticsLoadingId] = useState('');
  const [runnerImages, setRunnerImages] = useState(createBlankRunnerImagesState);
  const [runnerImageRecipeForm, setRunnerImageRecipeForm] = useState(createBlankRunnerImageRecipeForm);
  const [editingRunnerImageRecipeId, setEditingRunnerImageRecipeId] = useState(null);
  const [runnerImageRecipeSaving, setRunnerImageRecipeSaving] = useState(false);
  const [runnerImageRecipeDeletingId, setRunnerImageRecipeDeletingId] = useState('');
  const [runnerImageBuildingId, setRunnerImageBuildingId] = useState('');
  const [runnerImagePromotingId, setRunnerImagePromotingId] = useState('');
  const [runnerImageReconciling, setRunnerImageReconciling] = useState(false);
  const [workspaceDataLoaded, setWorkspaceDataLoaded] = useState(false);
  const [workspaceDataError, setWorkspaceDataError] = useState('');
  const [eventSearch, setEventSearch] = useState('');
  const [loginForm, setLoginForm] = useState(DEFAULT_LOGIN_FORM);
  const [passwordForm, setPasswordForm] = useState(DEFAULT_PASSWORD_FORM);
  const [passwordChanging, setPasswordChanging] = useState(false);
  const [policyForm, setPolicyForm] = useState(parsePolicyForm);
  const [editingPolicyId, setEditingPolicyId] = useState(null);
  const runtimeCatalogRequestIdRef = useRef(0);
  const policyCatalogRequestIdRef = useRef(0);
  const deferredEventSearch = useDeferredValue(eventSearch);
  const deferredRuntimeCompartmentOcid = useDeferredValue(String(ociRuntimeForm.compartmentOcid || '').trim());
  const deferredRuntimeAvailabilityDomain = useDeferredValue(String(ociRuntimeForm.availabilityDomain || '').trim());

  const subnetById = useMemo(() => {
    return Object.fromEntries((subnetCandidates || []).map((item) => [item.id, item]));
  }, [subnetCandidates]);

  const filteredLogs = useMemo(() => {
    const query = deferredEventSearch.trim().toLowerCase();
    if (!query) {
      return logs;
    }
    return logs.filter((entry) =>
      [entry.message, entry.level, entry.deliveryId, entry.detailsJson].some((value) =>
        String(value || '').toLowerCase().includes(query)
      )
    );
  }, [deferredEventSearch, logs]);

  const currentView = useMemo(() => {
    return ALL_NAV_ITEMS.find((item) => item.id === view) || ALL_NAV_ITEMS[0];
  }, [view]);

  const runtimeCatalogParams = useMemo(() => {
    const catalogMatchesCompartment = runtimeCatalog.params.compartmentOcid === deferredRuntimeCompartmentOcid;
    const availabilityDomain = catalogMatchesCompartment
      && runtimeCatalog.availabilityDomains.includes(deferredRuntimeAvailabilityDomain)
      ? deferredRuntimeAvailabilityDomain
      : '';

    return buildCatalogParams({
      compartmentOcid: deferredRuntimeCompartmentOcid,
      availabilityDomain
    });
  }, [
    deferredRuntimeCompartmentOcid,
    deferredRuntimeAvailabilityDomain,
    runtimeCatalog.availabilityDomains,
    runtimeCatalog.params.compartmentOcid
  ]);

  const policyCatalogParams = useMemo(() => {
    const effectiveSettings = ociRuntimeStatus.effectiveSettings || {};
    return buildCatalogParams({
      compartmentOcid: effectiveSettings.compartmentOcid,
      availabilityDomain: effectiveSettings.availabilityDomain,
      imageOcid: effectiveSettings.imageOcid
    });
  }, [ociRuntimeStatus.effectiveSettings]);

  const currentOnboardingStep = useMemo(() => {
    if (session?.mustChangePassword) {
      return 'password';
    }
    return setupStatus.currentStep || deriveCurrentSetupStep(setupStatus.steps || {});
  }, [session?.mustChangePassword, setupStatus]);

  const needsOnboarding = useMemo(() => {
    return Boolean(session?.authenticated) && (session?.mustChangePassword || !setupStatus.completed);
  }, [session, setupStatus.completed]);

  const recommendedSubnets = useMemo(() => {
    return subnetCandidates.filter((item) => item.isRecommended);
  }, [subnetCandidates]);

  const liveRunners = useMemo(() => {
    return runners.filter((runner) => !runner.terminatedAt && runner.status !== 'terminated');
  }, [runners]);

  const enabledPolicies = useMemo(() => {
    return policies.filter((policy) => policy.enabled);
  }, [policies]);

  const queuedJobs = useMemo(() => {
    return jobs.filter((job) => String(job.status).toLowerCase() === 'queued');
  }, [jobs]);

  const errorLogs = useMemo(() => {
    return logs.filter((entry) => String(entry.level).toLowerCase() === 'error');
  }, [logs]);

  const blockedGuardrailItems = useMemo(() => {
    return billingGuardrails.items.filter((item) => item.blocked);
  }, [billingGuardrails.items]);

  const degradedGuardrailItems = useMemo(() => {
    return billingGuardrails.items.filter((item) => item.degraded && !item.blocked);
  }, [billingGuardrails.items]);

  const warmPoolStatus = useMemo(() => {
    const configuredPolicies = enabledPolicies.filter((policy) => policy.warmEnabled && policy.warmMinIdle > 0);
    if (!configuredPolicies.length) {
      return {
        configuredCount: 0,
        configuredTargetCount: 0,
        degradedTargets: [],
        degradedPolicies: [],
        healthy: true
      };
    }

    const configuredTargets = configuredPolicies.flatMap((policy) =>
      ensureArray(policy.warmRepoAllowlist)
        .map(normalizeRepoTarget)
        .filter(Boolean)
        .map((target) => ({
          policyId: policy.id,
          labels: policy.labels,
          warmMinIdle: policy.warmMinIdle,
          ...target
        }))
    );

    if (!configuredTargets.length) {
      return {
        configuredCount: configuredPolicies.length,
        configuredTargetCount: 0,
        degradedTargets: [],
        degradedPolicies: [],
        healthy: true
      };
    }

    const warmCountsByTarget = new Map();
    for (const runner of liveRunners) {
      const policyId = numericOrNull(runner.warmPolicyId);
      if (!policyId || String(runner.source || '').toLowerCase() !== 'warm') {
        continue;
      }

      const repoOwner = coerceString(runner.warmRepoOwner || runner.repoOwner);
      const repoName = coerceString(runner.warmRepoName || runner.repoName);
      if (!repoOwner || !repoName) {
        continue;
      }

      const key = buildWarmTargetKey(policyId, repoOwner, repoName);
      const current = warmCountsByTarget.get(key) || {
        idle: 0,
        reserved: 0,
        warming: 0,
        total: 0
      };
      const warmState = String(runner.warmState || '').trim().toLowerCase();
      current.total += 1;
      if (warmState === 'warm_idle') {
        current.idle += 1;
      } else if (warmState === 'reserved') {
        current.reserved += 1;
      } else if (warmState === 'warming') {
        current.warming += 1;
      }
      warmCountsByTarget.set(key, current);
    }

    const degradedTargets = configuredTargets
      .map((target) => {
        const counts = warmCountsByTarget.get(buildWarmTargetKey(target.policyId, target.repoOwner, target.repoName)) || {
          idle: 0,
          reserved: 0,
          warming: 0,
          total: 0
        };
        const missingIdle = Math.max(target.warmMinIdle - counts.idle, 0);
        if (missingIdle === 0) {
          return null;
        }
        return {
          policyId: target.policyId,
          labels: target.labels,
          repoOwner: target.repoOwner,
          repoName: target.repoName,
          repoFullName: target.repoFullName,
          warmMinIdle: target.warmMinIdle,
          warmIdleCount: counts.idle,
          warmReservedCount: counts.reserved,
          warmWarmingCount: counts.warming,
          missingIdle
        };
      })
      .filter(Boolean);

    const degradedPolicies = Array.from(
      degradedTargets.reduce((accumulator, target) => {
        const current = accumulator.get(target.policyId) || {
          policyId: target.policyId,
          labels: target.labels,
          affectedTargets: [],
          missingIdle: 0,
          warmIdleCount: 0,
          warmReservedCount: 0,
          warmWarmingCount: 0
        };

        current.affectedTargets.push(target.repoFullName);
        current.missingIdle += target.missingIdle;
        current.warmIdleCount += target.warmIdleCount;
        current.warmReservedCount += target.warmReservedCount;
        current.warmWarmingCount += target.warmWarmingCount;
        accumulator.set(target.policyId, current);
        return accumulator;
      }, new Map()).values()
    );

    return {
      configuredCount: configuredPolicies.length,
      configuredTargetCount: configuredTargets.length,
      degradedTargets,
      degradedPolicies,
      healthy: degradedTargets.length === 0
    };
  }, [enabledPolicies, liveRunners]);

  const cacheCompatStatus = useMemo(() => {
    const effectiveSettings = ociRuntimeStatus.effectiveSettings || {};
    const enabled = Boolean(effectiveSettings.cacheCompatEnabled);
    const bucketName = String(effectiveSettings.cacheBucketName || '').trim();
    const objectPrefix = String(effectiveSettings.cacheObjectPrefix || '').trim();
    const retentionDays = numericOrNull(effectiveSettings.cacheRetentionDays) ?? 0;
    const missing = [];

    if (enabled) {
      if (!bucketName) {
        missing.push('cacheBucketName');
      }
      if (retentionDays <= 0) {
        missing.push('cacheRetentionDays');
      }
    }

    return {
      enabled,
      bucketName,
      objectPrefix,
      retentionDays,
      missing,
      ready: enabled && missing.length === 0,
      incident: enabled && missing.length > 0
    };
  }, [ociRuntimeStatus.effectiveSettings]);

  const overviewRunnerItems = useMemo(() => buildRunnerOverviewItems(runners), [runners]);
  const overviewJobItems = useMemo(() => buildJobOverviewItems(jobs, t), [jobs, t]);
  const overviewLogItems = useMemo(() => buildLogOverviewItems(logs, t), [logs, t]);
  const majorViewStates = useMemo(() => {
    const baseState = {
      loading,
      loaded: workspaceDataLoaded,
      error: workspaceDataError
    };

    return {
      overview: resolveAsyncViewState({
        ...baseState,
        itemCount:
          enabledPolicies.length
          + liveRunners.length
          + queuedJobs.length
          + errorLogs.length
          + recommendedSubnets.length
          + billingReport.items.length
          + blockedGuardrailItems.length
          + degradedGuardrailItems.length
          + githubDriftStatus.issues.length
          + warmPoolStatus.degradedTargets.length
          + (cacheCompatStatus.incident ? 1 : 0)
      }),
      jobs: resolveAsyncViewState({
        ...baseState,
        itemCount: jobs.length
      }),
      runners: resolveAsyncViewState({
        ...baseState,
        itemCount: runners.length
      }),
      policies: resolveAsyncViewState({
        ...baseState,
        itemCount: policies.length
      }),
      events: resolveAsyncViewState({
        ...baseState,
        itemCount: events.length + logs.length
      }),
      runnerImages: resolveAsyncViewState({
        loading: runnerImages.loading,
        loaded: runnerImages.loaded,
        error: runnerImages.error,
        itemCount:
          runnerImages.recipes.length
          + runnerImages.builds.length
          + runnerImages.resources.length
      })
    };
  }, [
    billingReport.items.length,
    blockedGuardrailItems.length,
    cacheCompatStatus.incident,
    degradedGuardrailItems.length,
    enabledPolicies.length,
    errorLogs.length,
    events.length,
    githubDriftStatus.issues.length,
    jobs.length,
    loading,
    logs.length,
    policies.length,
    queuedJobs.length,
    recommendedSubnets.length,
    runnerImages.builds.length,
    runnerImages.error,
    runnerImages.loaded,
    runnerImages.loading,
    runnerImages.recipes.length,
    runnerImages.resources.length,
    runners.length,
    liveRunners.length,
    workspaceDataError,
    workspaceDataLoaded,
    warmPoolStatus.degradedTargets.length
  ]);
  const policyValidation = useMemo(() => buildPolicyValidation(policyForm, ociRuntimeStatus, policyCatalog, t), [
    policyCatalog,
    policyForm,
    ociRuntimeStatus,
    t
  ]);
  const runtimeCatalogValidation = useMemo(() => buildRuntimeCatalogValidation(ociRuntimeForm, runtimeCatalog, t), [
    ociRuntimeForm,
    runtimeCatalog,
    t
  ]);

  useEffect(() => {
    if (!session?.authenticated || !needsOnboarding) {
      setActiveOnboardingStep('password');
      return;
    }

    setActiveOnboardingStep(currentOnboardingStep);
  }, [session?.authenticated, needsOnboarding, currentOnboardingStep, setupStatus.updatedAt]);

  useEffect(() => {
    const selectedShape = policyValidation.selectedShape;
    if (!selectedShape || selectedShape.isFlexible) {
      return;
    }

    setPolicyForm((current) => {
      if (String(current.shape || '').trim() !== selectedShape.shape) {
        return current;
      }

      const nextOcpu = selectedShape.defaultOcpu ?? current.ocpu;
      const nextMemoryGb = selectedShape.defaultMemoryGb ?? current.memoryGb;

      if (current.ocpu === nextOcpu && current.memoryGb === nextMemoryGb) {
        return current;
      }

      return {
        ...current,
        ocpu: nextOcpu,
        memoryGb: nextMemoryGb
      };
    });
  }, [policyValidation.selectedShape]);

  function resetWorkspaceData() {
    setPolicies([]);
    setJobs([]);
    setRunners([]);
    setEvents([]);
    setLogs([]);
    setWorkspaceDataLoaded(false);
    setWorkspaceDataError('');
    setSubnetCandidates([]);
    setDefaultSubnetId('');
    setSubnetError('');
    setBillingReport(createBlankBillingReportState());
    setBillingGuardrails(createBlankBillingGuardrailsState());
    setJobDiagnosticsByJobId({});
    setJobDiagnosticsErrorsByJobId({});
    setJobDiagnosticsLoadingId('');
    setPolicyCompatibilityResult(null);
    setPolicyCompatibilityError('');
    setPolicyCompatibilityLoading(false);
    setRunnerImages(createBlankRunnerImagesState());
    setRunnerImagePromotingId('');
    setRunnerImageRecipeDeletingId('');
    setRunnerImageBuildingId('');
    setRunnerImageReconciling(false);
    setRunnerImageRecipeSaving(false);
    setEditingRunnerImageRecipeId(null);
    setRunnerImageRecipeForm(createBlankRunnerImageRecipeForm());
  }

  function resetSetupState(sessionData = null) {
    runtimeCatalogRequestIdRef.current += 1;
    policyCatalogRequestIdRef.current += 1;
    setSetupStatus(createBlankSetupStatus(sessionData));
    setGithubConfigStatus(createBlankGitHubConfigStatus());
    setGithubConfigForm(blankGitHubConfigForm());
    setGithubConfigMode(GITHUB_SETUP_MODE_CREATE);
    setGithubConfigResult(null);
    setGithubManifestState(createBlankGitHubManifestState());
    setGithubDriftStatus(createBlankGitHubDriftState());
    setGithubDriftReconciling(false);
    setGithubConfigPromoting(false);
    setGithubActiveAppDeletingId('');
    setOciAuthStatus(createBlankOCIAuthStatus());
    setOciAuthForm(blankOCIAuthForm());
    setOciAuthResult(null);
    setOciAuthInspectResult(null);
    setOciAuthInspecting(false);
    setOciRuntimeStatus(createBlankOCIRuntimeStatus());
    setOciRuntimeForm(blankOCIRuntimeForm());
    setRuntimeCatalog(createBlankOCICatalogState());
    setPolicyCatalog(createBlankOCICatalogState());
    setPolicyCompatibilityForm({
      repoOwner: '',
      repoName: '',
      labelsText: ''
    });
  }

  function resetProtectedState(sessionData = null) {
    resetWorkspaceData();
    resetSetupState(sessionData);
  }

  async function loadSession() {
    try {
      const sessionData = await api('/api/v1/auth/session');
      return sessionData.session || null;
    } catch (err) {
      if (err.status === 401) {
        return null;
      }
      throw err;
    }
  }

  function isAuthRelatedError(err) {
    return err?.status === 401 || err?.status === 403;
  }

  function isOptionalFeatureUnavailableError(err) {
    const message = String(err?.message || '').trim().toLowerCase();
    return err?.status === 404
      || err?.status === 501
      || message.includes('not found')
      || message.includes('not implemented')
      || message.includes('is not configured');
  }

  function reportError(err, options = {}) {
    const description = normalizeOperatorErrorText(err?.message || t('toast.somethingWentWrong'));
    toastError({
      title: options.title || t('toast.requestFailed'),
      description,
      dedupeKey: options.dedupeKey
    });
  }

  function readGitHubManifestQueryState() {
    const searchParams = new URLSearchParams(window.location.search);
    const marker = searchParams.get(GITHUB_MANIFEST_QUERY_KEY);
    const installationId = Number(searchParams.get(GITHUB_MANIFEST_INSTALLATION_ID_QUERY_KEY));

    return {
      status: marker === 'created' || marker === 'installed' || marker === 'failed' ? marker : '',
      installationId: Number.isFinite(installationId) && installationId > 0 ? String(installationId) : ''
    };
  }

  function clearGitHubManifestQueryStatus() {
    const current = new URL(window.location.href);
    if (
      !current.searchParams.has(GITHUB_MANIFEST_QUERY_KEY)
      && !current.searchParams.has(GITHUB_MANIFEST_INSTALLATION_ID_QUERY_KEY)
    ) {
      return;
    }
    current.searchParams.delete(GITHUB_MANIFEST_QUERY_KEY);
    current.searchParams.delete(GITHUB_MANIFEST_INSTALLATION_ID_QUERY_KEY);
    const nextSearch = current.searchParams.toString();
    window.history.replaceState(null, '', `${current.pathname}${nextSearch ? `?${nextSearch}` : ''}${current.hash}`);
  }

  async function loadOCICatalog(params, options = {}) {
    const target = options.target === 'policy' ? 'policy' : 'runtime';
    const normalizedParams = buildCatalogParams(params);
    const requestIdRef = target === 'policy' ? policyCatalogRequestIdRef : runtimeCatalogRequestIdRef;
    const setCatalogState = target === 'policy' ? setPolicyCatalog : setRuntimeCatalog;
    const requiresSavedRuntime = target === 'policy';
    const hasMinimumParams = Boolean(normalizedParams.compartmentOcid)
      && (options.force || looksLikeOCIOcid(normalizedParams.compartmentOcid))
      && (!requiresSavedRuntime || (normalizedParams.availabilityDomain && normalizedParams.imageOcid));

    requestIdRef.current += 1;
    const requestId = requestIdRef.current;

    if (!hasMinimumParams) {
      startTransition(() => {
        setCatalogState(createBlankOCICatalogState());
      });
      return null;
    }

    startTransition(() => {
      setCatalogState(createCatalogLoadingState(normalizedParams));
    });

    try {
      const payload = await api('/api/v1/oci/catalog', {
        method: 'POST',
        body: JSON.stringify(normalizedParams)
      });

      if (requestIdRef.current !== requestId) {
        return null;
      }

      const normalizedCatalog = normalizeCatalog(payload, normalizedParams);
      startTransition(() => {
        setCatalogState(normalizedCatalog);
      });
      return normalizedCatalog;
    } catch (err) {
      if (requestIdRef.current !== requestId) {
        return null;
      }

      startTransition(() => {
        setCatalogState(createCatalogErrorState(normalizedParams, normalizeOperatorErrorText(err?.message || t('toast.loadOciCatalogFailed'))));
      });

      if (!options.silent) {
        reportError(err, {
          title: target === 'policy' ? t('toast.loadPolicyCatalogFailed') : t('toast.loadOciCatalogFailed'),
          dedupeKey: `${target}-catalog:${err?.message || 'unknown'}`
        });
      }
      return null;
    }
  }

  useEffect(() => {
    if (!session?.authenticated) {
      setRuntimeCatalog(createBlankOCICatalogState());
      return;
    }

    if (!runtimeCatalogParams.compartmentOcid || !looksLikeOCIOcid(runtimeCatalogParams.compartmentOcid)) {
      runtimeCatalogRequestIdRef.current += 1;
      setRuntimeCatalog(createBlankOCICatalogState());
      return;
    }

    void loadOCICatalog(runtimeCatalogParams, { target: 'runtime', silent: true });
  }, [
    runtimeCatalogParams.availabilityDomain,
    runtimeCatalogParams.compartmentOcid,
    runtimeCatalogParams.imageOcid,
    runtimeCatalogParams.subnetOcid,
    session?.authenticated
  ]);

  useEffect(() => {
    if (
      !session?.authenticated
      || !ociRuntimeStatus.ready
      || !policyCatalogParams.compartmentOcid
      || !policyCatalogParams.availabilityDomain
      || !policyCatalogParams.imageOcid
    ) {
      policyCatalogRequestIdRef.current += 1;
      setPolicyCatalog(createBlankOCICatalogState());
      return;
    }

    void loadOCICatalog(policyCatalogParams, { target: 'policy', silent: true });
  }, [
    ociRuntimeStatus.ready,
    policyCatalogParams.availabilityDomain,
    policyCatalogParams.compartmentOcid,
    policyCatalogParams.imageOcid,
    session?.authenticated
  ]);

  async function loadSetupStatus(sessionData, options = {}) {
    try {
      const status = await api('/api/v1/setup/status');
      const normalized = normalizeSetupStatus(status, sessionData);
      startTransition(() => {
        setSetupStatus(normalized);
      });
      return normalized;
    } catch (err) {
      const fallback = createBlankSetupStatus(sessionData);
      startTransition(() => {
        setSetupStatus(fallback);
      });
      if (!options.silent && !sessionData?.mustChangePassword) {
        reportError(err, {
          title: t('toast.loadSetupStatusFailed'),
          dedupeKey: `setup-status:${err?.message || 'unknown'}`
        });
      }
      return fallback;
    }
  }

  async function loadSubnetCandidates(options = {}) {
    try {
      const subnetData = await api('/api/v1/oci/subnets');
      startTransition(() => {
        setSubnetCandidates((subnetData.items || []).map(normalizeSubnetCandidate));
        setDefaultSubnetId(subnetData.defaultSubnetId || '');
        setSubnetError('');
      });
    } catch (err) {
      startTransition(() => {
        setSubnetCandidates([]);
        setSubnetError(normalizeOperatorErrorText(err?.message || t('toast.somethingWentWrong')));
      });
      if (!options.silent) {
        reportError(err, {
          title: t('toast.loadSubnetsFailed'),
          dedupeKey: `subnets:${err?.message || 'unknown'}`
        });
      }
    }
  }

  async function loadGitHubConfig(options = {}) {
    const pendingManifest = Object.prototype.hasOwnProperty.call(options, 'pendingManifest')
      ? options.pendingManifest
      : githubManifestState.pending;

    try {
      const configData = await api('/api/v1/github/config');
      const status = normalizeGitHubConfigStatus(configData);
      const form = buildGitHubConfigFormFromStatus(configData);
      startTransition(() => {
        setGithubConfigStatus(status);
        setGithubConfigForm(form);
        setGithubConfigMode((currentMode) => normalizeGitHubSetupMode({
          mode: currentMode,
          apiBaseUrl: form.apiBaseUrl,
          pendingManifest
        }));
      });
      return status;
    } catch (err) {
      startTransition(() => {
        setGithubConfigStatus(createBlankGitHubConfigStatus());
        setGithubConfigForm(blankGitHubConfigForm());
        setGithubConfigMode((currentMode) => normalizeGitHubSetupMode({
          mode: currentMode,
          apiBaseUrl: '',
          pendingManifest
        }));
      });
      if (!options.silent && !isAuthRelatedError(err)) {
        reportError(err, {
          title: t('toast.loadGitHubSetupFailed'),
          dedupeKey: `github-config:${err?.message || 'unknown'}`
        });
      }
      return createBlankGitHubConfigStatus();
    }
  }

  async function loadGitHubManifestPending(options = {}) {
    const queryState = readGitHubManifestQueryState();

    startTransition(() => {
      setGithubManifestState((current) => ({
        ...current,
        loading: true,
        status: queryState.status || current.status
      }));
    });

    try {
      const payload = await api('/api/v1/github/config/manifest/pending');
      const pending = normalizeGitHubManifestPending(payload.pending);
      const returnedInstallationId = queryState.installationId;

      startTransition(() => {
        setGithubManifestState((current) => ({
          ...current,
          pending,
          loading: false,
          status: queryState.status || current.status,
          discoveryError: '',
          installations: [],
          autoInstallationId: returnedInstallationId ? Number(returnedInstallationId) : 0
        }));
        if (pending) {
          setGithubConfigForm((current) => mergeGitHubManifestIntoConfigForm(current, pending, {}, {
            installationId: returnedInstallationId
          }));
          setGithubConfigMode((currentMode) => normalizeGitHubSetupMode({
            mode: currentMode,
            apiBaseUrl: '',
            pendingManifest: pending
          }));
        }
      });

      const shouldDiscoverPendingInstallation = pending
        && options.discover !== false
        && (
          pending.ownerTarget !== GITHUB_MANIFEST_OWNER_TARGET_ORGANIZATION
          || Boolean(returnedInstallationId)
          || queryState.status === 'installed'
        );

      if (shouldDiscoverPendingInstallation) {
        await handleGitHubInstallationDiscovery({
          pending,
          installationId: returnedInstallationId,
          silent: true
        });
      }

      return pending;
    } catch (err) {
      startTransition(() => {
        setGithubManifestState((current) => ({
          ...current,
          pending: null,
          loading: false,
          status: queryState.status || current.status,
          installations: [],
          autoInstallationId: 0,
          discoveryError: ''
        }));
        setGithubConfigMode((currentMode) => normalizeGitHubSetupMode({
          mode: currentMode,
          apiBaseUrl: githubConfigForm.apiBaseUrl,
          pendingManifest: null
        }));
      });
      if (!options.silent && !isAuthRelatedError(err)) {
        reportError(err, {
          title: t('toast.loadGitHubManifestFailed'),
          dedupeKey: `github-manifest:${err?.message || 'unknown'}`
        });
      }
      return null;
    } finally {
      clearGitHubManifestQueryStatus();
    }
  }

  async function loadOCIAuthStatus() {
    try {
      const status = await api('/api/v1/oci/auth');
      startTransition(() => {
        setOciAuthStatus({
          effectiveMode: status.effectiveMode || '',
          defaultMode: status.defaultMode || '',
          activeCredential: status.activeCredential || null,
          runtimeConfigReady: Boolean(status.runtimeConfigReady),
          runtimeConfigMissing: status.runtimeConfigMissing || []
        });
      });
    } catch {
      startTransition(() => {
        setOciAuthStatus(createBlankOCIAuthStatus());
      });
    }
  }

  async function loadOCIRuntimeStatus() {
    try {
      const status = await api('/api/v1/oci/runtime');
      startTransition(() => {
        setOciRuntimeStatus({
          source: status.source || 'env',
          overrideSettings: status.overrideSettings || null,
          effectiveSettings: status.effectiveSettings || createBlankOCIRuntimeStatus().effectiveSettings,
          ready: Boolean(status.ready),
          missing: status.missing || []
        });
        setOciRuntimeForm(blankOCIRuntimeForm(status.overrideSettings || status.effectiveSettings || {}));
      });
    } catch {
      startTransition(() => {
        setOciRuntimeStatus(createBlankOCIRuntimeStatus());
        setOciRuntimeForm(blankOCIRuntimeForm());
      });
    }
  }

  async function loadBillingReport(options = {}) {
    const days = Number.isFinite(Number(options.days)) ? Number(options.days) : 7;
    startTransition(() => {
      setBillingReport(createBillingLoadingState(days));
    });

    try {
      const report = await api(`/api/v1/billing/policies?days=${days}`);
      const normalized = normalizeBillingReport(report, days);
      startTransition(() => {
        setBillingReport(normalized);
      });
      return normalized;
    } catch (err) {
      const errorState = createBillingErrorState(days, err?.message || t('toast.loadBillingFailed'));
      startTransition(() => {
        setBillingReport(errorState);
      });
      if (!options.silent && !isAuthRelatedError(err)) {
        reportError(err, {
          title: t('toast.loadBillingFailed'),
          dedupeKey: `billing:${err?.message || 'unknown'}`
        });
      }
      return errorState;
    }
  }

  async function loadBillingGuardrails(options = {}) {
    startTransition(() => {
      setBillingGuardrails((current) => ({
        ...(current?.loaded ? current : createBlankBillingGuardrailsState()),
        loading: true,
        error: ''
      }));
    });

    try {
      const payload = await api('/api/v1/billing/guardrails');
      const normalized = normalizeBillingGuardrailsReport(payload);
      startTransition(() => {
        setBillingGuardrails({
          ...normalized,
          loading: false,
          error: ''
        });
      });
      return normalized;
    } catch (err) {
      const unavailable = isOptionalFeatureUnavailableError(err);
      startTransition(() => {
        setBillingGuardrails({
          ...createBlankBillingGuardrailsState(),
          loaded: true,
          available: !unavailable,
          loading: false,
          error: normalizeOperatorErrorText(err?.message || t('toast.loadBillingGuardrailsFailed'))
        });
      });
      if (!options.silent && !unavailable && !isAuthRelatedError(err)) {
        reportError(err, {
          title: t('toast.loadBillingGuardrailsFailed'),
          dedupeKey: `billing-guardrails:${err?.message || 'unknown'}`
        });
      }
      return null;
    }
  }

  async function loadGitHubDrift(options = {}) {
    startTransition(() => {
      setGithubDriftStatus((current) => ({
        ...(current?.loaded ? current : createBlankGitHubDriftState()),
        loading: true,
        error: ''
      }));
    });

    try {
      const payload = await api('/api/v1/github/drift');
      const normalized = normalizeGitHubDriftStatus(payload);
      startTransition(() => {
        setGithubDriftStatus({
          ...normalized,
          loading: false,
          error: ''
        });
      });
      return normalized;
    } catch (err) {
      const unavailable = isOptionalFeatureUnavailableError(err);
      startTransition(() => {
        setGithubDriftStatus({
          ...createBlankGitHubDriftState(),
          loaded: true,
          available: !unavailable,
          loading: false,
          error: normalizeOperatorErrorText(err?.message || t('toast.loadGitHubDriftFailed'))
        });
      });
      if (!options.silent && !unavailable && !isAuthRelatedError(err)) {
        reportError(err, {
          title: t('toast.loadGitHubDriftFailed'),
          dedupeKey: `github-drift:${err?.message || 'unknown'}`
        });
      }
      return null;
    }
  }

  async function loadRunnerImages(options = {}) {
    startTransition(() => {
      setRunnerImages((current) => ({
        ...(current?.loaded ? current : createBlankRunnerImagesState()),
        loading: true,
        error: ''
      }));
    });

    try {
      const payload = await api('/api/v1/runner-images');
      const normalized = normalizeRunnerImagesPayload(
        payload,
        Boolean(ociRuntimeStatus.ready || ociAuthStatus.runtimeConfigReady)
      );
      startTransition(() => {
        setRunnerImages({
          ...normalized,
          loading: false,
          error: ''
        });
      });
      return normalized;
    } catch (err) {
      const errorMessage = err?.message || t('toast.loadRunnerImagesFailed');
      startTransition(() => {
        setRunnerImages((current) => ({
          ...(current?.loaded ? current : createBlankRunnerImagesState()),
          loading: false,
          error: normalizeOperatorErrorText(errorMessage)
        }));
      });
      if (!options.silent && !isAuthRelatedError(err)) {
        reportError(err, {
          title: t('toast.loadRunnerImagesFailed'),
          dedupeKey: `runner-images:${err?.message || 'unknown'}`
        });
      }
      return null;
    }
  }

  async function refreshAll() {
    setRefreshing(true);
    let sessionData = null;

    try {
      sessionData = await loadSession();

      if (!sessionData?.authenticated) {
        startTransition(() => {
          setSession(null);
          setView('overview');
          resetProtectedState();
        });
        return;
      }

      startTransition(() => {
        setSession(sessionData);
      });

      if (sessionData.mustChangePassword) {
        startTransition(() => {
          resetProtectedState(sessionData);
        });
        return;
      }

      const nextSetupStatus = await loadSetupStatus(sessionData);
      const setupPromises = [
        loadGitHubConfig({ silent: true }),
        loadGitHubDrift({ silent: true }),
        loadOCIAuthStatus(),
        loadOCIRuntimeStatus()
      ];

      if (!nextSetupStatus.completed) {
        startTransition(() => {
          resetWorkspaceData();
        });
        await Promise.all(setupPromises);
        await loadGitHubManifestPending({ silent: true });
        return;
      }

      startTransition(() => {
        setWorkspaceDataError('');
      });

      const [policyData, runnerData, jobData, eventData] = await Promise.all([
        api('/api/v1/policies'),
        api('/api/v1/runners'),
        api('/api/v1/jobs'),
        api('/api/v1/events'),
        ...setupPromises,
        loadSubnetCandidates({ silent: true }),
        loadBillingReport({ silent: true }),
        loadBillingGuardrails({ silent: true }),
        loadRunnerImages({ silent: true })
      ]);

      startTransition(() => {
        setPolicies((policyData.items || []).map(normalizePolicy));
        setRunners((runnerData.items || []).map(normalizeRunner));
        setJobs((jobData.items || []).map(normalizeJob));
        setEvents((eventData.events || []).map(normalizeEvent));
        setLogs((eventData.logs || []).map(normalizeLog));
        setWorkspaceDataLoaded(true);
        setWorkspaceDataError('');
      });
      await loadGitHubManifestPending({ silent: true });
    } catch (err) {
      if (!workspaceDataLoaded) {
        startTransition(() => {
          setWorkspaceDataError(normalizeOperatorErrorText(err?.message || t('toast.somethingWentWrong')));
        });
      }
      if (!isAuthRelatedError(err)) {
        startTransition(() => {
          if (sessionData?.authenticated) {
            setSession(sessionData);
          }
        });
        reportError(err, { dedupeKey: `refresh:${err?.message || 'unknown'}` });
        return;
      }

      const gatedSession = await loadSession().catch(() => null);
      startTransition(() => {
        setSession(gatedSession?.authenticated ? gatedSession : null);
        setView('overview');
        resetProtectedState(gatedSession?.authenticated ? gatedSession : null);
      });
      if (!gatedSession?.mustChangePassword && gatedSession?.authenticated) {
        reportError(err, { dedupeKey: `refresh:${err?.message || 'unknown'}` });
      }
    } finally {
      setLoading(false);
      setRefreshing(false);
    }
  }

  useEffect(() => {
    void refreshAll();
  }, []);

  async function handleLogin(event) {
    event.preventDefault();
    try {
      await api('/api/v1/auth/login', {
        method: 'POST',
        body: JSON.stringify(loginForm)
      });
    } catch (err) {
      reportError(err, { title: t('toast.signInFailed') });
      return;
    }

    await refreshAll();
    setPasswordForm((current) => ({ ...current, currentPassword: loginForm.password }));
  }

  async function handleLogout() {
    try {
      await api('/api/v1/auth/logout', { method: 'POST' });
      startTransition(() => {
        setSession(null);
        setView('overview');
        resetProtectedState();
      });
    } catch (err) {
      reportError(err, { title: t('toast.signOutFailed') });
    }
  }

  async function handlePasswordChange(event) {
    event.preventDefault();
    setPasswordChanging(true);
    try {
      await api('/api/v1/auth/change-password', {
        method: 'POST',
        body: JSON.stringify(passwordForm)
      });
    } catch (err) {
      reportError(err, { title: t('toast.passwordChangeFailed') });
      setPasswordChanging(false);
      return;
    }

    setPasswordForm({ currentPassword: '', newPassword: '' });
    await refreshAll();
    setPasswordChanging(false);
  }

  async function handlePolicySubmit(event) {
    event.preventDefault();

    if (!policyValidation.canSave) {
      const firstIssue = policyValidation.settingsMessage
        || policyValidation.fieldErrors.shape
        || policyValidation.fieldErrors.ocpu
        || policyValidation.fieldErrors.memoryGb
        || policyValidation.fieldErrors.warmMinIdle
        || policyValidation.fieldErrors.warmTtlMinutes
        || policyValidation.fieldErrors.budgetCapAmount
        || policyValidation.fieldErrors.budgetWindowDays
        || t('validation.policy.fixBeforeSaving');
      reportError(new Error(firstIssue), { title: t('toast.policySaveBlocked') });
      return;
    }

    try {
      const payload = normalizePolicyPayload(policyForm);
      if (policyValidation.selectedShape && !policyValidation.selectedShape.isFlexible) {
        payload.ocpu = policyValidation.selectedShape.defaultOcpu ?? payload.ocpu;
        payload.memoryGb = policyValidation.selectedShape.defaultMemoryGb ?? payload.memoryGb;
      }
      if (editingPolicyId) {
        await api(`/api/v1/policies/${editingPolicyId}`, {
          method: 'PUT',
          body: JSON.stringify(payload)
        });
      } else {
        await api('/api/v1/policies', {
          method: 'POST',
          body: JSON.stringify(payload)
        });
      }
      setEditingPolicyId(null);
      setPolicyForm(parsePolicyForm());
      await refreshAll();
      setView('policies');
    } catch (err) {
      reportError(err);
    }
  }

  async function handlePolicyDelete(id) {
    try {
      await api(`/api/v1/policies/${id}`, { method: 'DELETE' });
      await refreshAll();
    } catch (err) {
      reportError(err);
    }
  }

  function handlePolicyEdit(policy) {
    setEditingPolicyId(policy.id);
    setPolicyForm(parsePolicyForm(policy));
    setView('policies');
  }

  function handleCancelPolicyEdit() {
    setEditingPolicyId(null);
    setPolicyForm(parsePolicyForm());
  }

  function handlePolicyCompatibilityUseCurrentLabels() {
    setPolicyCompatibilityForm((current) => ({
      ...current,
      labelsText: policyForm.labels
    }));
  }

  async function handlePolicyCompatibilityCheck(event) {
    event?.preventDefault?.();

    const repoOwner = String(policyCompatibilityForm.repoOwner || '').trim();
    const repoName = String(policyCompatibilityForm.repoName || '').trim();
    const labels = ensureArray(policyCompatibilityForm.labelsText);

    if (!repoOwner) {
      setPolicyCompatibilityError(t('validation.policy.compatibility.repoOwnerRequired'));
      setPolicyCompatibilityResult(null);
      return;
    }

    if (!repoName) {
      setPolicyCompatibilityError(t('validation.policy.compatibility.repoNameRequired'));
      setPolicyCompatibilityResult(null);
      return;
    }

    if (!labels.length) {
      setPolicyCompatibilityError(t('validation.policy.compatibility.labelsRequired'));
      setPolicyCompatibilityResult(null);
      return;
    }

    setPolicyCompatibilityLoading(true);
    setPolicyCompatibilityError('');

    try {
      const payload = await api('/api/v1/policies/compatibility-check', {
        method: 'POST',
        body: JSON.stringify({
          repoOwner,
          repoName,
          labels
        })
      });
      startTransition(() => {
        setPolicyCompatibilityResult(normalizeCompatibilityResult(payload));
      });
    } catch (err) {
      startTransition(() => {
        setPolicyCompatibilityResult(null);
        setPolicyCompatibilityError(normalizeOperatorErrorText(err?.message || t('toast.policyCompatibilityFailed')));
      });
    } finally {
      setPolicyCompatibilityLoading(false);
    }
  }

  function handlePolicyShapeChange(shapeName) {
    const selectedShape = (policyCatalog.shapes || []).find((item) => item.shape === shapeName) || null;
    setPolicyForm((current) => ({
      ...current,
      shape: shapeName,
      ocpu: selectedShape?.defaultOcpu ?? current.ocpu,
      memoryGb: selectedShape?.defaultMemoryGb ?? current.memoryGb
    }));
  }

  async function handleTerminateRunner(id) {
    try {
      await api(`/api/v1/runners/${id}/terminate`, { method: 'POST' });
      await refreshAll();
    } catch (err) {
      reportError(err);
    }
  }

  async function handleCleanup() {
    try {
      await api('/api/v1/system/cleanup', { method: 'POST' });
      await refreshAll();
    } catch (err) {
      reportError(err);
    }
  }

  async function handleJobDiagnosticsLoad(jobId, options = {}) {
    const targetJobId = String(jobId || '').trim();
    if (!targetJobId) {
      return null;
    }

    if (jobDiagnosticsByJobId[targetJobId] && !options.force) {
      return jobDiagnosticsByJobId[targetJobId];
    }

    setJobDiagnosticsLoadingId(targetJobId);
    startTransition(() => {
      setJobDiagnosticsErrorsByJobId((current) => {
        const next = { ...current };
        delete next[targetJobId];
        return next;
      });
    });

    try {
      const payload = await api(`/api/v1/jobs/${encodeURIComponent(targetJobId)}/diagnostics`);
      const normalized = normalizeJobDiagnostic(payload);
      startTransition(() => {
        setJobDiagnosticsByJobId((current) => ({
          ...current,
          [targetJobId]: normalized
        }));
      });
      return normalized;
    } catch (err) {
      startTransition(() => {
        setJobDiagnosticsErrorsByJobId((current) => ({
          ...current,
          [targetJobId]: normalizeOperatorErrorText(err?.message || t('toast.jobDiagnosticsFailed'))
        }));
      });
      return null;
    } finally {
      setJobDiagnosticsLoadingId('');
    }
  }

  async function clearGitHubManifestPending(options = {}) {
    startTransition(() => {
      setGithubManifestState((current) => ({
        ...current,
        loading: true
      }));
    });

    try {
      await api('/api/v1/github/config/manifest/pending', { method: 'DELETE' });
    } catch (err) {
      startTransition(() => {
        setGithubManifestState((current) => ({
          ...current,
          loading: false
        }));
      });
      if (!options.silent && !isAuthRelatedError(err)) {
        reportError(err, {
          title: t('toast.clearGitHubManifestFailed'),
          dedupeKey: `github-manifest-clear:${err?.message || 'unknown'}`
        });
      }
      return;
    }

    startTransition(() => {
      setGithubManifestState(createBlankGitHubManifestState());
      if (options.resetDraftCredentials) {
        setGithubConfigForm((current) => ({
          ...current,
          appId: '',
          installationId: '',
          privateKeyPem: '',
          webhookSecret: ''
        }));
        setGithubConfigResult(null);
      }
      setGithubConfigMode((currentMode) => normalizeGitHubSetupMode({
        mode: options.nextMode || currentMode,
        apiBaseUrl: githubConfigForm.apiBaseUrl
      }));
    });
  }

  async function handleGitHubManifestCreate() {
    const manifestStartPayload = buildGitHubManifestStartPayload(githubConfigForm);

    startTransition(() => {
      setGithubConfigForm((current) => ({
        ...current,
        ownerTarget: normalizeGitHubManifestOwnerTarget(current.ownerTarget)
      }));
    });

    setGithubManifestState((current) => ({
      ...current,
      creating: true
    }));

    try {
      const result = await api('/api/v1/github/config/manifest/start', {
        method: 'POST',
        body: JSON.stringify(manifestStartPayload)
      });
      window.location.assign(result.redirectUrl);
    } catch (err) {
      startTransition(() => {
        setGithubManifestState((current) => ({
          ...current,
          creating: false
        }));
      });
      reportError(err, { title: t('toast.startGitHubManifestFailed') });
    }
  }

  async function handleGitHubInstallationDiscovery(options = {}) {
    const pending = normalizeGitHubManifestPending(options.pending || githubManifestState.pending);
    const sourceForm = pending
      ? mergeGitHubManifestIntoConfigForm(githubConfigForm, pending, {}, {
          installationId: options.installationId
        })
      : githubConfigForm;

    setGithubManifestState((current) => ({
      ...current,
      discovering: true,
      discoveryError: ''
    }));

    try {
      const result = await api('/api/v1/github/config/installations/discover', {
        method: 'POST',
        body: JSON.stringify(normalizeGitHubConfigPayload(sourceForm))
      });
      const lookup = normalizeGitHubInstallationLookup(result);

      startTransition(() => {
        setGithubManifestState((current) => ({
          ...current,
          discovering: false,
          discoveryError: '',
          installations: lookup.installations,
          autoInstallationId: lookup.autoInstallationId || current.autoInstallationId
        }));
        setGithubConfigForm((current) => applyGitHubInstallationLookup(current, lookup));
      });
      return lookup;
    } catch (err) {
      startTransition(() => {
        setGithubManifestState((current) => ({
          ...current,
          discovering: false,
          discoveryError: err?.message || t('toast.githubInstallationDiscoveryFailed'),
          installations: [],
          autoInstallationId: current.autoInstallationId
        }));
      });
      if (!options.silent && !isAuthRelatedError(err)) {
        reportError(err, { title: t('toast.githubInstallationDiscoveryFailed') });
      }
      return normalizeGitHubInstallationLookup();
    }
  }

  async function handleGitHubTest(event) {
    event.preventDefault();
    setGithubConfigTesting(true);
    try {
      const result = await api('/api/v1/github/config/test', {
        method: 'POST',
        body: JSON.stringify(normalizeGitHubConfigPayload(githubConfigForm))
      });
      const selectableRepos = ensureArray(result.repositories)
        .map((repository) => {
          if (typeof repository === 'string') {
            return repository.trim();
          }
          if (!repository || typeof repository !== 'object') {
            return '';
          }
          if (repository.admin === false) {
            return '';
          }
          return repository.fullName || [repository.owner, repository.name].filter(Boolean).join('/');
        })
        .filter(Boolean);
      startTransition(() => {
        setGithubConfigResult(result);
        setGithubConfigForm((current) => {
          const available = new Set(selectableRepos.map((repoName) => repoName.toLowerCase()));
          const retained = ensureArray(current.selectedRepos).filter((repoName) =>
            available.has(String(repoName).toLowerCase())
          );
          return {
            ...current,
            selectedRepos: retained.length ? retained : selectableRepos
          };
        });
      });
      await loadSetupStatus(session, { silent: true });
    } catch (err) {
      reportError(err, { title: t('toast.githubTestFailed') });
    } finally {
      setGithubConfigTesting(false);
    }
  }

  async function handleGitHubSave() {
    setGithubConfigSaving(true);
    try {
      const result = await api('/api/v1/github/config/staged', {
        method: 'POST',
        body: JSON.stringify(normalizeGitHubConfigPayload(githubConfigForm))
      });
      startTransition(() => {
        setGithubConfigResult(result);
      });
      if (githubManifestState.pending) {
        await clearGitHubManifestPending({ silent: true });
      }
      await Promise.all([
        loadGitHubConfig({ silent: true, pendingManifest: null }),
        loadGitHubDrift({ silent: true }),
        loadSetupStatus(session, { silent: true })
      ]);
    } catch (err) {
      reportError(err, { title: t('toast.githubSaveFailed') });
    } finally {
      setGithubConfigSaving(false);
    }
  }

  async function handleGitHubClear() {
    setGithubConfigClearing(true);
    try {
      await api('/api/v1/github/config/staged', {
        method: 'DELETE'
      });
      startTransition(() => {
        setGithubConfigResult(null);
      });
      await Promise.all([
        loadGitHubConfig({ silent: true }),
        loadGitHubDrift({ silent: true }),
        loadSetupStatus(session, { silent: true })
      ]);
    } catch (err) {
      reportError(err, { title: t('toast.githubClearFailed') });
    } finally {
      setGithubConfigClearing(false);
    }
  }

  async function handleGitHubPromote() {
    setGithubConfigPromoting(true);
    try {
      await api('/api/v1/github/config/staged/promote', {
        method: 'POST'
      });
      startTransition(() => {
        setGithubConfigResult(null);
      });
      await Promise.all([
        loadGitHubConfig({ silent: true }),
        loadGitHubDrift({ silent: true }),
        loadSetupStatus(session, { silent: true })
      ]);
    } catch (err) {
      reportError(err, { title: t('toast.githubPromoteFailed') });
    } finally {
      setGithubConfigPromoting(false);
    }
  }

  async function handleGitHubActiveAppRemove(configId) {
    const targetId = normalizeGitHubConfigId(configId);
    if (!targetId) {
      return;
    }

    const targetConfig = Array.isArray(githubConfigStatus.activeConfigs)
      ? githubConfigStatus.activeConfigs.find((config) => normalizeGitHubConfigId(config.id) === targetId)
      : null;
    const deletePath = buildGitHubActiveAppDeletePath(githubConfigStatus, targetConfig || { id: targetId });
    if (!deletePath) {
      return;
    }

    setGithubActiveAppDeletingId(targetId);
    try {
      await api(deletePath, {
        method: 'DELETE'
      });
      startTransition(() => {
        setGithubConfigResult(null);
      });
      await Promise.all([
        loadGitHubConfig({ silent: true }),
        loadGitHubDrift({ silent: true }),
        loadSetupStatus(session, { silent: true })
      ]);
    } catch (err) {
      reportError(err, { title: t('toast.githubRemoveActiveAppFailed') });
    } finally {
      setGithubActiveAppDeletingId('');
    }
  }

  async function handleGitHubDriftReconcile() {
    setGithubDriftReconciling(true);
    try {
      const payload = await api('/api/v1/github/drift/reconcile', {
        method: 'POST'
      });
      startTransition(() => {
        setGithubDriftStatus({
          ...normalizeGitHubDriftStatus(payload),
          loading: false,
          error: ''
        });
      });
      return payload;
    } catch (err) {
      const unavailable = isOptionalFeatureUnavailableError(err);
      startTransition(() => {
        setGithubDriftStatus({
          ...createBlankGitHubDriftState(),
          loaded: true,
          available: !unavailable,
          loading: false,
          error: normalizeOperatorErrorText(err?.message || t('toast.githubDriftReconcileFailed'))
        });
      });
      if (!unavailable && !isAuthRelatedError(err)) {
        reportError(err, { title: t('toast.githubDriftReconcileFailed') });
      }
      return null;
    } finally {
      setGithubDriftReconciling(false);
    }
  }

  function handleGitHubConfigModeChange(nextModeOrOptions) {
    const requestedMode =
      typeof nextModeOrOptions === 'string'
        ? nextModeOrOptions
        : nextModeOrOptions?.mode;
    const nextMode = requestedMode === GITHUB_SETUP_MODE_EXISTING
      ? GITHUB_SETUP_MODE_EXISTING
      : GITHUB_SETUP_MODE_CREATE;
    const discardManifest = Boolean(nextModeOrOptions?.discardManifest);
    const normalizedMode = normalizeGitHubSetupMode({
      mode: nextMode,
      apiBaseUrl: githubConfigForm.apiBaseUrl
    });

    setGithubConfigMode(normalizedMode);

    if (discardManifest && githubManifestState.pending) {
      void clearGitHubManifestPending({
        nextMode: normalizedMode,
        resetDraftCredentials: true
      });
    }
  }

  async function handleOCIAuthFile(field, file) {
    if (!file) {
      return;
    }

    const text = await file.text();
    setOciAuthResult(null);

    if (field !== 'configText') {
      setOciAuthForm((current) => ({ ...current, [field]: text }));
      return;
    }

    setOciAuthInspecting(true);
    try {
      const inspectResult = await api('/api/v1/oci/auth/inspect', {
        method: 'POST',
        body: JSON.stringify({
          configText: text,
          profileName: ociAuthForm.profileName || 'DEFAULT'
        })
      });
      const normalizedInspect = normalizeOCIInspectResult(inspectResult, text);
      startTransition(() => {
        setOciAuthInspectResult(normalizedInspect);
        setOciAuthForm((current) => ({
          ...current,
          configText: normalizedInspect.configText || text,
          profileName: normalizedInspect.profileName || current.profileName,
          name: current.name || normalizedInspect.suggestedName || normalizedInspect.profileName || ''
        }));
      });
    } catch (err) {
      startTransition(() => {
        setOciAuthInspectResult(null);
        setOciAuthForm((current) => ({ ...current, configText: text }));
      });
      reportError(err, { title: t('toast.inspectOciConfigFailed') });
    } finally {
      setOciAuthInspecting(false);
    }
  }

  async function handleOCIAuthTest(event) {
    event.preventDefault();
    setOciAuthTesting(true);
    try {
      const result = await api('/api/v1/oci/auth/test', {
        method: 'POST',
        body: JSON.stringify(ociAuthForm)
      });
      startTransition(() => {
        setOciAuthResult(result);
      });
      await Promise.all([loadOCIAuthStatus(), loadSetupStatus(session, { silent: true })]);
    } catch (err) {
      reportError(err);
    } finally {
      setOciAuthTesting(false);
    }
  }

  async function handleOCIAuthSave() {
    setOciAuthSaving(true);
    try {
      const result = await api('/api/v1/oci/auth', {
        method: 'POST',
        body: JSON.stringify(ociAuthForm)
      });
      startTransition(() => {
        setOciAuthResult(result);
        setOciAuthInspectResult(null);
        setOciAuthForm(blankOCIAuthForm());
      });
      await Promise.all([
        loadOCIAuthStatus(),
        loadSetupStatus(session, { silent: true }),
        loadSubnetCandidates({ silent: true })
      ]);
    } catch (err) {
      reportError(err);
    } finally {
      setOciAuthSaving(false);
    }
  }

  async function handleOCIAuthClear() {
    setOciAuthClearing(true);
    try {
      const result = await api('/api/v1/oci/auth', {
        method: 'DELETE'
      });
      startTransition(() => {
        setOciAuthInspectResult(null);
        setOciAuthResult(null);
        setOciAuthForm(blankOCIAuthForm());
        setOciAuthStatus(result.status || createBlankOCIAuthStatus());
      });
      await Promise.all([
        loadSetupStatus(session, { silent: true }),
        loadOCIRuntimeStatus(),
        loadSubnetCandidates({ silent: true })
      ]);
    } catch (err) {
      reportError(err);
    } finally {
      setOciAuthClearing(false);
    }
  }

  async function handleOCIRuntimeSave() {
    if (!runtimeCatalogValidation.canSave) {
      const firstIssue = runtimeCatalogValidation.catalogMessage
        || runtimeCatalogValidation.fieldErrors.compartmentOcid
        || runtimeCatalogValidation.fieldErrors.availabilityDomain
        || runtimeCatalogValidation.fieldErrors.subnetOcid
        || runtimeCatalogValidation.fieldErrors.imageOcid
        || runtimeCatalogValidation.fieldErrors.cacheBucketName
        || runtimeCatalogValidation.fieldErrors.cacheRetentionDays
        || t('validation.runtime.fixBeforeSaving');
      reportError(new Error(firstIssue), { title: t('toast.runtimeSaveBlocked') });
      return;
    }

    setOciRuntimeSaving(true);
    try {
      const status = await api('/api/v1/oci/runtime', {
        method: 'PUT',
        body: JSON.stringify(normalizeOCIRuntimePayload(ociRuntimeForm))
      });
      startTransition(() => {
        setOciRuntimeStatus(status);
        setOciRuntimeForm(blankOCIRuntimeForm(status.overrideSettings || status.effectiveSettings || {}));
      });
      await Promise.all([
        loadSubnetCandidates({ silent: true }),
        loadOCIAuthStatus(),
        loadSetupStatus(session, { silent: true })
      ]);
    } catch (err) {
      reportError(err);
    } finally {
      setOciRuntimeSaving(false);
    }
  }

  async function handleRuntimeCatalogRefresh() {
    await loadOCICatalog(runtimeCatalogParams, {
      target: 'runtime',
      force: true
    });
  }

  async function handleOCIRuntimeClear() {
    setOciRuntimeClearing(true);
    try {
      const result = await api('/api/v1/oci/runtime', { method: 'DELETE' });
      startTransition(() => {
        const status = result.status || createBlankOCIRuntimeStatus();
        setOciRuntimeStatus(status);
        setOciRuntimeForm(blankOCIRuntimeForm(status.effectiveSettings || {}));
      });
      await Promise.all([
        loadSubnetCandidates({ silent: true }),
        loadOCIAuthStatus(),
        loadSetupStatus(session, { silent: true })
      ]);
    } catch (err) {
      reportError(err);
    } finally {
      setOciRuntimeClearing(false);
    }
  }

  function handleRunnerImageRecipeEdit(recipe) {
    setEditingRunnerImageRecipeId(recipe.id);
    setRunnerImageRecipeForm(buildRunnerImageRecipeForm(recipe));
    setView('runner-images');
  }

  function handleRunnerImageRecipeCancel() {
    setEditingRunnerImageRecipeId(null);
    setRunnerImageRecipeForm(createBlankRunnerImageRecipeForm());
  }

  async function handleRunnerImageRecipeSubmit(event) {
    event.preventDefault();

    const validationError = validateRunnerImageRecipeForm(runnerImageRecipeForm, t);
    if (validationError) {
      reportError(new Error(validationError), { title: t('toast.runnerImageRecipeSaveBlocked') });
      return;
    }

    setRunnerImageRecipeSaving(true);
    try {
      const payload = normalizeRunnerImageRecipePayload(runnerImageRecipeForm);
      if (editingRunnerImageRecipeId) {
        await api(`/api/v1/runner-images/recipes/${editingRunnerImageRecipeId}`, {
          method: 'PUT',
          body: JSON.stringify(payload)
        });
      } else {
        await api('/api/v1/runner-images/recipes', {
          method: 'POST',
          body: JSON.stringify(payload)
        });
      }
      startTransition(() => {
        setEditingRunnerImageRecipeId(null);
        setRunnerImageRecipeForm(createBlankRunnerImageRecipeForm());
      });
      await loadRunnerImages({ silent: true });
      setView('runner-images');
    } catch (err) {
      reportError(err, { title: t('toast.runnerImageRecipeSaveFailed') });
    } finally {
      setRunnerImageRecipeSaving(false);
    }
  }

  async function handleRunnerImageRecipeDelete(recipeId) {
    const targetId = String(recipeId || '').trim();
    if (!targetId) {
      return;
    }

    setRunnerImageRecipeDeletingId(targetId);
    try {
      await api(`/api/v1/runner-images/recipes/${targetId}`, {
        method: 'DELETE'
      });
      startTransition(() => {
        if (editingRunnerImageRecipeId === targetId) {
          setEditingRunnerImageRecipeId(null);
          setRunnerImageRecipeForm(createBlankRunnerImageRecipeForm());
        }
      });
      await loadRunnerImages({ silent: true });
    } catch (err) {
      reportError(err, { title: t('toast.runnerImageRecipeDeleteFailed') });
    } finally {
      setRunnerImageRecipeDeletingId('');
    }
  }

  async function handleRunnerImageBuildCreate(recipeId) {
    const numericRecipeId = Number.parseInt(String(recipeId || '').trim(), 10);
    if (!Number.isFinite(numericRecipeId) || numericRecipeId <= 0) {
      reportError(new Error(t('validation.runnerImages.recipe.idRequired')), {
        title: t('toast.runnerImageBuildStartFailed')
      });
      return;
    }

    setRunnerImageBuildingId(String(numericRecipeId));
    try {
      await api('/api/v1/runner-images/builds', {
        method: 'POST',
        body: JSON.stringify({ recipeId: numericRecipeId })
      });
      await loadRunnerImages({ silent: true });
    } catch (err) {
      reportError(err, { title: t('toast.runnerImageBuildStartFailed') });
    } finally {
      setRunnerImageBuildingId('');
    }
  }

  async function handleRunnerImagePromote(buildId) {
    const targetId = String(buildId || '').trim();
    if (!targetId) {
      return;
    }

    setRunnerImagePromotingId(targetId);
    try {
      await api(`/api/v1/runner-images/builds/${targetId}/promote`, {
        method: 'POST'
      });
      await Promise.all([
        loadRunnerImages({ silent: true }),
        loadOCIAuthStatus(),
        loadOCIRuntimeStatus(),
        loadSetupStatus(session, { silent: true })
      ]);
    } catch (err) {
      reportError(err, { title: t('toast.runnerImagePromoteFailed') });
    } finally {
      setRunnerImagePromotingId('');
    }
  }

  async function handleRunnerImageReconcile() {
    setRunnerImageReconciling(true);
    try {
      await api('/api/v1/runner-images/reconcile', {
        method: 'POST'
      });
      await loadRunnerImages({ silent: true });
    } catch (err) {
      reportError(err, { title: t('toast.runnerImageReconcileFailed') });
    } finally {
      setRunnerImageReconciling(false);
    }
  }

  function handleSelectOnboardingStep(nextStep) {
    if (!needsOnboarding || !SETUP_STEP_ORDER.includes(nextStep)) {
      return;
    }

    if (nextStep !== currentOnboardingStep && !setupStatus.steps?.[nextStep]?.completed) {
      return;
    }

    setActiveOnboardingStep(nextStep);
  }

  return {
    view,
    setView,
    loading,
    refreshing,
    session,
    setupStatus,
    needsOnboarding,
    currentOnboardingStep,
    activeOnboardingStep,
    selectOnboardingStep: handleSelectOnboardingStep,
    policies,
    jobs,
    runners,
    events,
    logs,
    subnetCandidates,
    defaultSubnetId,
    subnetError,
    subnetById,
    githubConfigStatus,
    githubConfigForm,
    setGithubConfigForm,
    githubConfigMode,
    setGithubConfigMode: handleGitHubConfigModeChange,
    githubConfigResult,
    githubManifestState,
    githubConfigTesting,
    githubConfigSaving,
    githubConfigClearing,
    githubConfigPromoting,
    githubActiveAppDeletingId,
    githubDriftStatus,
    githubDriftReconciling,
    ociAuthStatus,
    ociAuthForm,
    setOciAuthForm,
    ociAuthResult,
    ociAuthInspecting,
    ociAuthInspectResult,
    ociAuthTesting,
    ociAuthSaving,
    ociAuthClearing,
    ociRuntimeStatus,
    ociRuntimeForm,
    setOciRuntimeForm,
    ociRuntimeSaving,
    ociRuntimeClearing,
    eventSearch,
    setEventSearch,
    loginForm,
    setLoginForm,
    passwordForm,
    setPasswordForm,
    passwordChanging,
    policyForm,
    setPolicyForm,
    editingPolicyId,
    policyCompatibilityForm,
    setPolicyCompatibilityForm,
    policyCompatibilityResult,
    policyCompatibilityLoading,
    policyCompatibilityError,
    filteredLogs,
    currentView,
    recommendedSubnets,
    liveRunners,
    enabledPolicies,
    queuedJobs,
    errorLogs,
    overviewRunnerItems,
    overviewJobItems,
    overviewLogItems,
    warmPoolStatus,
    cacheCompatStatus,
    runtimeCatalog,
    runtimeCatalogValidation,
    policyCatalog,
    policyValidation,
    majorViewStates,
    billingReport,
    billingGuardrails,
    blockedGuardrailItems,
    degradedGuardrailItems,
    jobDiagnosticsByJobId,
    jobDiagnosticsErrorsByJobId,
    jobDiagnosticsLoadingId,
    runnerImages,
    runnerImageRecipeForm,
    setRunnerImageRecipeForm,
    editingRunnerImageRecipeId,
    runnerImageRecipeSaving,
    runnerImageRecipeDeletingId,
    runnerImageBuildingId,
    runnerImagePromotingId,
    runnerImageReconciling,
    refreshAll,
    loadRunnerImages,
    handleLogin,
    handleLogout,
    handlePasswordChange,
    handlePolicySubmit,
    handlePolicyDelete,
    handlePolicyEdit,
    handleCancelPolicyEdit,
    handlePolicyCompatibilityCheck,
    handlePolicyCompatibilityUseCurrentLabels,
    handlePolicyShapeChange,
    handleTerminateRunner,
    handleCleanup,
    handleJobDiagnosticsLoad,
    handleGitHubTest,
    handleGitHubSave,
    handleGitHubClear,
    handleGitHubPromote,
    handleGitHubActiveAppRemove,
    handleGitHubDriftReconcile,
    handleGitHubManifestCreate,
    handleGitHubInstallationDiscovery,
    handleOCIAuthFile,
    handleOCIAuthTest,
    handleOCIAuthSave,
    handleOCIAuthClear,
    handleOCIRuntimeSave,
    handleOCIRuntimeClear,
    handleRuntimeCatalogRefresh,
    loadGitHubDrift,
    loadSubnetCandidates,
    handleRunnerImageRecipeEdit,
    handleRunnerImageRecipeCancel,
    handleRunnerImageRecipeSubmit,
    handleRunnerImageRecipeDelete,
    handleRunnerImageBuildCreate,
    handleRunnerImagePromote,
    handleRunnerImageReconcile
  };
}
