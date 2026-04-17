import { getCurrentLocale, hasTranslation, translate, translateMaybeKey } from '@/i18n';

const EXACT_MESSAGE_KEYS = {
  'method not allowed': 'operator.error.methodNotAllowed',
  'invalid json payload': 'operator.error.invalidJsonPayload',
  'invalid body': 'operator.error.invalidBody',
  'invalid signature': 'operator.error.invalidSignature',
  'days must be between 1 and 90': 'operator.error.daysOutOfRange',
  'invalid runner id': 'operator.error.invalidRunnerId',
  'runner not found': 'operator.error.runnerNotFound',
  'compartmentocid is required': 'operator.error.compartmentRequired',
  'setup service is not configured': 'operator.error.setupServiceUnavailable',
  'runner image service is not configured': 'operator.error.runnerImageServiceUnavailable',
  'oci billing service is not configured': 'operator.error.billingServiceUnavailable',
  'oci runtime service is not configured': 'operator.error.ociRuntimeServiceUnavailable',
  'oci credential service is not configured': 'operator.error.ociCredentialServiceUnavailable',
  'github config service is not configured': 'operator.error.githubConfigServiceUnavailable',
  'github client is required for workflow_job processing': 'operator.error.githubClientRequired',
  'cache bucket name is required when cache compatibility is enabled': 'operator.error.cacheBucketRequired',
  'cache retention days must be greater than zero': 'operator.error.cacheRetentionPositive',
  'too many requests': 'operator.error.tooManyRequests',
  'rate limited': 'operator.error.tooManyRequests',
  'rate limit exceeded': 'operator.error.tooManyRequests',
  'connect: connection refused': 'operator.error.connectionRefused',
  'connection refused': 'operator.error.connectionRefused',
  'service unavailable': 'operator.error.serviceUnavailable',
  'context deadline exceeded': 'operator.error.requestTimedOut',
  'deadline exceeded': 'operator.error.requestTimedOut',
  'i/o timeout': 'operator.error.requestTimedOut',
  'request timeout': 'operator.error.requestTimedOut',
  'request timed out': 'operator.error.requestTimedOut',
  'timed out': 'operator.error.requestTimedOut',
  'client.timeout exceeded while awaiting headers': 'operator.error.requestTimedOut',
  'visual qa runner lookup unavailable': 'operator.error.visualQaRunnerLookupUnavailable',
  'repository not allowed': 'operator.job.repositoryNotAllowed',
  'no matching policy': 'operator.job.noMatchingPolicy',
  'policy max_runners reached': 'operator.job.policyMaxRunnersReached',
  'policy max runners reached': 'operator.job.policyMaxRunnersReached',
  'workflow_job queued without matching policy': 'operator.event.workflowJobQueuedWithoutMatchingPolicy',
  'terminate requested': 'operator.event.terminateRequested',
  'no tracked runner': 'operator.event.noTrackedRunner',
  'build queued.': 'operator.runnerImages.buildQueued',
  'bake instance launched.': 'operator.runnerImages.bakeInstanceLaunched',
  'bake phase updated.': 'operator.runnerImages.phaseUpdated',
  'bake verification failed.': 'operator.runnerImages.verificationFailed',
  'runner image is ready.': 'operator.runnerImages.imageReady',
  'image promoted to the default runtime.': 'operator.runnerImages.imagePromoted',
  'bake instance stopped before a success marker was recorded.': 'operator.runnerImages.instanceStoppedBeforeSuccess',
  'runner image builds can launch.': 'operator.runnerImages.preflight.readyToLaunch',
  'complete oci runtime setup before baking images.': 'operator.runnerImages.preflight.runtimeRequired',
  'setup and verify commands passed': 'operator.runnerImages.setupAndVerifyPassed',
  'setup commands failed': 'operator.runnerImages.setupCommandsFailed',
  'verify commands failed': 'operator.runnerImages.verifyCommandsFailed',
  'current default image': 'operator.runnerImages.currentDefaultImage',
  'fake default subnet': 'operator.subnet.fakeDefaultName',
  'fake oci mode default subnet': 'operator.subnet.fakeModeRecommendation',
  'oci usage api data is delayed and the current day can remain incomplete until oci finishes aggregation.':
    'operator.billing.lagNotice.default',
  'this report shows tenancy-scope oci billed cost for the same window alongside ohoci-tracked runner attribution. the detailed breakdown and gap review remain limited to tracked runner resources, so some non-runner oci charges can stay outside attribution.':
    'operator.billing.scopeNote.default',
  'this report covers tracked runner instances. oci charges billed only to separate attached resources may remain outside this view until they can be correlated safely.':
    'operator.billing.scopeNote.default'
};

const MESSAGE_RULES = [
  {
    pattern: /database is locked|sqlite_busy/i,
    key: 'operator.error.databaseBusy',
    params: () => ({})
  },
  {
    pattern: /\b(?:429\b[\s:.-]*)?(?:too many requests|rate limited|rate limit exceeded)\b/i,
    key: 'operator.error.tooManyRequests',
    params: () => ({})
  },
  {
    pattern: /^github api\s+([a-z]+)\s+(\S+)\s+failed:\s+(.+)$/i,
    key: 'operator.error.githubApiRequestFailed',
    params: (match, locale) => ({
      method: normalizeRequestMethod(match[1]),
      path: match[2],
      reason: normalizeOperatorReason(match[3], locale)
    })
  },
  {
    pattern: /^github app metadata request failed:\s+(.+)$/i,
    key: 'operator.error.githubAppMetadataRequestFailed',
    params: (match, locale) => ({
      reason: normalizeOperatorReason(match[1], locale)
    })
  },
  {
    pattern: /^request failed:\s*(\d+)$/i,
    key: 'operator.error.requestFailedStatus',
    params: (match) => ({ status: match[1] })
  },
  {
    pattern: /^(get|post|put|patch|delete|head|options)\s+"([^"]+)":\s+(.+)$/i,
    key: 'operator.error.httpRequestFailed',
    params: (match, locale) => ({
      method: normalizeRequestMethod(match[1]),
      target: match[2],
      reason: normalizeOperatorReason(match[3], locale)
    })
  },
  {
    pattern: /^dial tcp\s+(.+):\s+connect:\s+connection refused$/i,
    key: 'operator.error.transportDialTcpConnectionRefused',
    params: (match) => ({ address: match[1] })
  },
  {
    pattern: /^dial tcp\s+(.+):\s+i\/o timeout$/i,
    key: 'operator.error.transportDialTcpTimeout',
    params: (match) => ({ address: match[1] })
  },
  {
    pattern: /\b(?:context deadline exceeded|deadline exceeded|i\/o timeout|timed out|timeout exceeded while awaiting headers)\b/i,
    key: 'operator.error.requestTimedOut',
    params: () => ({})
  },
  {
    pattern: /\bservice unavailable\b/i,
    key: 'operator.error.serviceUnavailable',
    params: () => ({})
  },
  {
    pattern: /^repository\s+(.+?)\s+is not allowed$/i,
    key: 'operator.event.repositoryNotAllowed',
    params: (match) => ({ repository: match[1] })
  },
  {
    pattern: /^runner\s+(.+?)\s+launch requested$/i,
    key: 'operator.event.runnerLaunchRequested',
    params: (match) => ({ runnerName: match[1] })
  },
  {
    pattern: /^job\s+(\d+)\s+already has tracked runner\s+(.+)$/i,
    key: 'operator.event.jobAlreadyHasTrackedRunner',
    params: (match) => ({ jobId: match[1], runnerName: match[2] })
  },
  {
    pattern: /^workflow_job action\s+(.+?)\s+ignored$/i,
    key: 'operator.event.workflowJobActionIgnored',
    params: (match) => ({ action: match[1] })
  },
  {
    pattern: /^runner lookup for\s+(.+?)\s+failed:\s+(.+)$/i,
    key: 'operator.event.runnerLookupFailed',
    params: (match, locale) => ({
      runnerName: match[1],
      reason: normalizeOperatorText(match[2], { locale, keyPrefixes: ['operator.error'] })
    })
  },
  {
    pattern: /^sync github runner for\s+(.+?)\s+failed:\s+(.+)$/i,
    key: 'operator.event.githubRunnerSyncFailed',
    params: (match, locale) => ({
      runnerName: match[1],
      reason: normalizeOperatorText(match[2], { locale, keyPrefixes: ['operator.error'] })
    })
  },
  {
    pattern: /^job\s+(\d+)\s+updated to\s+([a-z0-9_]+)\s+\((.+)\)$/i,
    key: 'operator.event.jobUpdated.withRunnerMessage',
    params: (match, locale) => ({
      jobId: match[1],
      status: normalizeOperatorText(match[2], { locale, keyPrefixes: ['formatter.status'] }),
      runnerMessage: formatList(
        match[3]
          .split(',')
          .map((part) => normalizeOperatorText(part, { locale, keyPrefixes: ['operator.event', 'operator.error'] }))
          .filter(Boolean),
        locale
      )
    })
  },
  {
    pattern: /^job\s+(\d+)\s+updated to\s+([a-z0-9_]+)$/i,
    key: 'operator.event.jobUpdated',
    params: (match, locale) => ({
      jobId: match[1],
      status: normalizeOperatorText(match[2], { locale, keyPrefixes: ['formatter.status'] })
    })
  },
  {
    pattern: /^runner\s+(.+?)\s+synced as\s+(\d+)$/i,
    key: 'operator.event.runnerSynced',
    params: (match) => ({ runnerName: match[1], runnerId: match[2] })
  },
  {
    pattern: /^runner\s+(.+?)\s+updated without github id$/i,
    key: 'operator.event.runnerUpdatedWithoutGitHubId',
    params: (match) => ({ runnerName: match[1] })
  },
  {
    pattern: /^terminal workflow cleanup failed for\s+(.+?):\s+(.+)$/i,
    key: 'operator.event.terminalWorkflowCleanupFailed',
    params: (match, locale) => ({
      runnerName: match[1],
      reason: normalizeOperatorText(match[2], { locale, keyPrefixes: ['operator.error'] })
    })
  },
  {
    pattern: /^delete github runner\s+(.+?)\s+failed after oci terminal state:\s+(.+)$/i,
    key: 'operator.event.githubRunnerDeleteAfterOciTerminalStateFailed',
    params: (match, locale) => ({
      runnerName: match[1],
      reason: normalizeOperatorText(match[2], { locale, keyPrefixes: ['operator.error'] })
    })
  },
  {
    pattern: /^github runner delete failed for\s+(.+?):\s+(.+)$/i,
    key: 'operator.event.githubRunnerDeleteFailed',
    params: (match, locale) => ({
      runnerName: match[1],
      reason: normalizeOperatorText(match[2], { locale, keyPrefixes: ['operator.error'] })
    })
  },
  {
    pattern: /^runner\s+(.+?)\s+already in terminal oci state\s+([a-z0-9_]+)$/i,
    key: 'operator.event.runnerAlreadyInTerminalOciState',
    params: (match, locale) => ({
      runnerName: match[1],
      state: normalizeOperatorText(match[2], { locale, keyPrefixes: ['formatter.status'] })
    })
  },
  {
    pattern: /^terminate runner\s+(.+?)\s+failed:\s+(.+)$/i,
    key: 'operator.event.terminateRunnerFailed',
    params: (match, locale) => ({
      runnerName: match[1],
      reason: normalizeOperatorText(match[2], { locale, keyPrefixes: ['operator.error'] })
    })
  },
  {
    pattern: /^terminate requested for runner\s+(.+)$/i,
    key: 'operator.event.terminateRequestedForRunner',
    params: (match) => ({ runnerName: match[1] })
  },
  {
    pattern: /^image state:\s+(.+)$/i,
    key: 'operator.runnerImages.imageState',
    params: (match, locale) => ({
      state: normalizeOperatorText(match[1], { locale, keyPrefixes: ['formatter.status'] })
    })
  }
];

function resolveLocale(locale) {
  return locale || getCurrentLocale();
}

function normalizeLookupValue(value) {
  return String(value || '')
    .trim()
    .toLowerCase()
    .replace(/\s+/g, ' ');
}

function normalizeRequestMethod(value) {
  return String(value || '').trim().toUpperCase();
}

function normalizeOperatorReason(value, locale) {
  return normalizeOperatorText(value, {
    locale,
    keyPrefixes: ['operator.error']
  });
}

function translateByPrefixes(value, keyPrefixes = [], locale, params = {}) {
  if (!keyPrefixes.length) {
    return '';
  }

  const candidates = new Set([value, value.replace(/[\s-]+/g, '_')]);
  for (const candidate of candidates) {
    for (const prefix of keyPrefixes) {
      const key = `${prefix}.${candidate}`;
      if (hasTranslation(key, locale)) {
        return translate(key, params, locale);
      }
    }
  }

  return '';
}

function formatList(values, locale) {
  try {
    return new Intl.ListFormat(locale, {
      style: 'long',
      type: 'conjunction'
    }).format(values);
  } catch {
    return values.join(', ');
  }
}

export function normalizeOperatorText(value, options = {}) {
  const normalizedValue = String(value || '').trim();
  if (!normalizedValue) {
    return '';
  }

  const locale = resolveLocale(options.locale);
  const params = options.params || {};
  const translatedMaybeKey = translateMaybeKey(normalizedValue, params, locale);
  if (translatedMaybeKey !== normalizedValue) {
    return translatedMaybeKey;
  }

  const lookupValue = normalizeLookupValue(normalizedValue);
  const exactKey = EXACT_MESSAGE_KEYS[lookupValue];
  if (exactKey) {
    return translate(exactKey, params, locale);
  }

  const prefixed = translateByPrefixes(lookupValue, options.keyPrefixes, locale, params);
  if (prefixed) {
    return prefixed;
  }

  for (const rule of MESSAGE_RULES) {
    const match = normalizedValue.match(rule.pattern);
    if (!match) {
      continue;
    }
    return translate(rule.key, rule.params(match, locale), locale);
  }

  return normalizedValue;
}

export function normalizeOperatorList(values, options = {}) {
  const items = (Array.isArray(values) ? values : [])
    .map((value) => normalizeOperatorText(value, options))
    .filter(Boolean);

  if (!items.length) {
    return '';
  }

  return formatList(items, resolveLocale(options.locale));
}

export function normalizeOperatorErrorText(value, options = {}) {
  return normalizeOperatorText(value, {
    ...options,
    keyPrefixes: [...(options.keyPrefixes || []), 'operator.error']
  });
}
