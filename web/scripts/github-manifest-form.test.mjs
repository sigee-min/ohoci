import test from 'node:test';
import assert from 'node:assert/strict';

import {
  applyGitHubInstallationLookup,
  buildGitHubConfigFormFromStatus,
  buildGitHubManifestStartPayload,
  blankGitHubConfigForm,
  getGitHubRepositorySectionState,
  hasGitHubStagedConfigState,
  isGitHubManifestHelperSupported,
  mergeGitHubManifestIntoConfigForm,
  normalizeGitHubConfigPayload,
  normalizeGitHubManifestPending,
  normalizeGitHubSetupMode,
  normalizeGitHubInstallationLookup,
  parseGitHubTagList,
  resolveGitHubActiveConfigs,
  resolveGitHubRepositoryChoicesSource
} from '../src/lib/workspace-forms.js';
import {
  GITHUB_MANIFEST_OWNER_TARGET_ORGANIZATION,
  GITHUB_MANIFEST_OWNER_TARGET_PERSONAL
} from '../src/lib/workspace-constants.js';

test('isGitHubManifestHelperSupported only allows github.com defaults', () => {
  assert.equal(isGitHubManifestHelperSupported(''), true);
  assert.equal(isGitHubManifestHelperSupported('https://api.github.com'), true);
  assert.equal(isGitHubManifestHelperSupported('https://api.github.com/'), true);
  assert.equal(isGitHubManifestHelperSupported('https://ghe.example.test/api/v3'), false);
});

test('mergeGitHubManifestIntoConfigForm applies generated credentials and auto-filled installation id', () => {
  const form = blankGitHubConfigForm({
    apiBaseUrl: '',
    selectedRepos: ['example/repo']
  });

  const merged = mergeGitHubManifestIntoConfigForm(
    form,
    {
      appId: 123,
      privateKeyPem: '-----BEGIN PRIVATE KEY-----\nmanifest\n-----END PRIVATE KEY-----',
      webhookSecret: 'manifest-secret'
    },
    {
      autoInstallationId: 456
    }
  );

  assert.deepEqual(merged, {
    ...form,
    appId: '123',
    installationId: '456',
    privateKeyPem: '-----BEGIN PRIVATE KEY-----\nmanifest\n-----END PRIVATE KEY-----',
    webhookSecret: 'manifest-secret'
  });
});

test('blankGitHubConfigForm defaults manifest owner target to personal', () => {
  const form = blankGitHubConfigForm();

  assert.equal(form.ownerTarget, GITHUB_MANIFEST_OWNER_TARGET_PERSONAL);
  assert.equal(form.organizationSlug, '');
  assert.equal(form.name, '');
  assert.equal(form.tagsText, '');
});

test('blankGitHubConfigForm normalizes name and tags from existing config data', () => {
  const form = blankGitHubConfigForm({
    id: 42,
    name: 'Primary app',
    tags: ['production', 'payments', 'production']
  });

  assert.equal(form.id, '42');
  assert.equal(form.name, 'Primary app');
  assert.equal(form.tagsText, 'production\npayments');
});

test('buildGitHubConfigFormFromStatus keeps the staging form blank when multiple active apps exist without a staged config', () => {
  const form = buildGitHubConfigFormFromStatus({
    status: {
      activeConfig: {
        authMode: 'app',
        appId: 111,
        installationId: 222,
        name: 'Payments',
        selectedRepos: ['acme/payments']
      },
      activeConfigs: [
        {
          id: 'cfg-payments',
          authMode: 'app',
          appId: 111,
          installationId: 222,
          name: 'Payments',
          tags: ['prod'],
          selectedRepos: ['acme/payments']
        },
        {
          id: 'cfg-ops',
          authMode: 'app',
          appId: 333,
          installationId: 444,
          name: 'Ops',
          tags: ['ops'],
          selectedRepos: ['acme/ops']
        }
      ],
      effectiveConfig: {
        authMode: 'app',
        appId: 111,
        installationId: 222,
        name: 'Payments'
      }
    }
  });

  assert.deepEqual(form, blankGitHubConfigForm());
});

test('normalizeGitHubManifestPending defaults owner target to personal for legacy drafts', () => {
  const pending = normalizeGitHubManifestPending({
    appId: 123,
    privateKeyPem: '-----BEGIN PRIVATE KEY-----\nmanifest\n-----END PRIVATE KEY-----',
    webhookSecret: 'manifest-secret'
  });

  assert.equal(pending.ownerTarget, GITHUB_MANIFEST_OWNER_TARGET_PERSONAL);
  assert.equal(pending.transferUrl, '');
});

test('mergeGitHubManifestIntoConfigForm clears a stale installation id until manifest installation discovery resolves it', () => {
  const form = {
    ...blankGitHubConfigForm({
      apiBaseUrl: '',
      selectedRepos: ['example/repo']
    }),
    installationId: '777'
  };

  const merged = mergeGitHubManifestIntoConfigForm(form, {
    appId: 123,
    privateKeyPem: '-----BEGIN PRIVATE KEY-----\nmanifest\n-----END PRIVATE KEY-----',
    webhookSecret: 'manifest-secret'
  });

  assert.deepEqual(merged, {
    ...form,
    appId: '123',
    installationId: '',
    privateKeyPem: '-----BEGIN PRIVATE KEY-----\nmanifest\n-----END PRIVATE KEY-----',
    webhookSecret: 'manifest-secret'
  });
});

test('mergeGitHubManifestIntoConfigForm keeps a returned installation id on the manifest draft path before discovery resolves', () => {
  const form = {
    ...blankGitHubConfigForm({
      apiBaseUrl: '',
      selectedRepos: ['example/repo']
    }),
    installationId: '777'
  };

  const merged = mergeGitHubManifestIntoConfigForm(
    form,
    {
      appId: 123,
      privateKeyPem: '-----BEGIN PRIVATE KEY-----\nmanifest\n-----END PRIVATE KEY-----',
      webhookSecret: 'manifest-secret'
    },
    {},
    {
      installationId: '654321'
    }
  );

  assert.deepEqual(merged, {
    ...form,
    appId: '123',
    installationId: '654321',
    privateKeyPem: '-----BEGIN PRIVATE KEY-----\nmanifest\n-----END PRIVATE KEY-----',
    webhookSecret: 'manifest-secret'
  });
});

test('mergeGitHubManifestIntoConfigForm preserves organization owner context from the pending draft', () => {
  const form = blankGitHubConfigForm({
    ownerTarget: GITHUB_MANIFEST_OWNER_TARGET_PERSONAL,
    organizationSlug: 'example-org'
  });

  const merged = mergeGitHubManifestIntoConfigForm(form, {
    appId: 123,
    ownerTarget: GITHUB_MANIFEST_OWNER_TARGET_ORGANIZATION,
    transferUrl: 'https://github.com/settings/apps/example/advanced',
    privateKeyPem: '-----BEGIN PRIVATE KEY-----\nmanifest\n-----END PRIVATE KEY-----',
    webhookSecret: 'manifest-secret'
  });

  assert.equal(merged.ownerTarget, GITHUB_MANIFEST_OWNER_TARGET_ORGANIZATION);
  assert.equal(merged.organizationSlug, 'example-org');
});

test('mergeGitHubManifestIntoConfigForm resets a stale GitHub API URL to github.com helper defaults', () => {
  const form = {
    ...blankGitHubConfigForm({
      apiBaseUrl: 'https://ghe.example.test/api/v3',
      selectedRepos: ['example/repo']
    }),
    installationId: '777'
  };

  const merged = mergeGitHubManifestIntoConfigForm(form, {
    appId: 123,
    privateKeyPem: '-----BEGIN PRIVATE KEY-----\nmanifest\n-----END PRIVATE KEY-----',
    webhookSecret: 'manifest-secret'
  });

  assert.deepEqual(merged, {
    ...form,
    apiBaseUrl: '',
    appId: '123',
    installationId: '',
    privateKeyPem: '-----BEGIN PRIVATE KEY-----\nmanifest\n-----END PRIVATE KEY-----',
    webhookSecret: 'manifest-secret'
  });
});

test('applyGitHubInstallationLookup preserves installation id when no automatic choice exists', () => {
  const current = {
    ...blankGitHubConfigForm(),
    installationId: '777'
  };

  const updated = applyGitHubInstallationLookup(current, {
    installations: [
      { id: 111, accountLogin: 'one' },
      { id: 222, accountLogin: 'two' }
    ]
  });

  assert.equal(updated.installationId, '777');
});

test('normalizeGitHubInstallationLookup filters invalid installations and keeps auto selection', () => {
  const normalized = normalizeGitHubInstallationLookup({
    autoInstallationId: 222,
    installations: [
      null,
      { id: 111, accountLogin: 'one', repositorySelection: 'selected' },
      { id: 0, accountLogin: 'ignored' },
      { id: 222, accountLogin: 'two', accountType: 'Organization', htmlUrl: 'https://example.test/install/222' }
    ]
  });

  assert.deepEqual(normalized, {
    autoInstallationId: 222,
    installations: [
      {
        id: 111,
        accountLogin: 'one',
        accountType: '',
        repositorySelection: 'selected',
        htmlUrl: '',
        appSlug: ''
      },
      {
        id: 222,
        accountLogin: 'two',
        accountType: 'Organization',
        repositorySelection: '',
        htmlUrl: 'https://example.test/install/222',
        appSlug: ''
      }
    ]
  });
});

test('normalizeGitHubSetupMode keeps an explicit existing-app choice even when a manifest draft is pending', () => {
  assert.equal(
    normalizeGitHubSetupMode({
      mode: 'existing',
      apiBaseUrl: '',
      pendingManifest: { appId: 123 }
    }),
    'existing'
  );

  assert.equal(
    normalizeGitHubSetupMode({
      mode: 'create',
      apiBaseUrl: '',
      pendingManifest: { appId: 123 }
    }),
    'create'
  );
});

test('buildGitHubManifestStartPayload includes organization slug for organization manifests', () => {
  const payload = buildGitHubManifestStartPayload({
    apiBaseUrl: '',
    ownerTarget: GITHUB_MANIFEST_OWNER_TARGET_ORGANIZATION,
    organizationSlug: 'example-org'
  });

  assert.deepEqual(payload, {
    apiBaseUrl: '',
    ownerTarget: GITHUB_MANIFEST_OWNER_TARGET_ORGANIZATION,
    organizationSlug: 'example-org'
  });
});

test('buildGitHubManifestStartPayload omits organization slug for personal manifests', () => {
  const payload = buildGitHubManifestStartPayload({
    apiBaseUrl: '',
    ownerTarget: GITHUB_MANIFEST_OWNER_TARGET_PERSONAL,
    organizationSlug: 'example-org'
  });

  assert.deepEqual(payload, {
    apiBaseUrl: '',
    ownerTarget: GITHUB_MANIFEST_OWNER_TARGET_PERSONAL
  });
});

test('parseGitHubTagList supports comma and newline delimiters with dedupe', () => {
  assert.deepEqual(parseGitHubTagList('production, payments\ncritical,production'), [
    'production',
    'payments',
    'critical'
  ]);
});

test('normalizeGitHubConfigPayload includes optional name and tags', () => {
  const payload = normalizeGitHubConfigPayload({
    id: 'staged-42',
    name: 'Primary app',
    tagsText: 'production\npayments',
    apiBaseUrl: '',
    appId: '123',
    installationId: '456',
    privateKeyPem: 'pem',
    webhookSecret: 'secret',
    selectedRepos: ['owner/repo']
  });

  assert.deepEqual(payload, {
    id: 'staged-42',
    name: 'Primary app',
    tags: ['production', 'payments'],
    authMode: 'app',
    apiBaseUrl: undefined,
    appId: 123,
    installationId: 456,
    privateKeyPem: 'pem',
    webhookSecret: 'secret',
    selectedRepos: ['owner/repo']
  });
});

test('resolveGitHubRepositoryChoicesSource ignores live app repository fallbacks when multiple active apps exist', () => {
  const repositories = resolveGitHubRepositoryChoicesSource(null, {
    activeConfig: {
      installationRepositories: ['acme/payments']
    },
    activeConfigs: [
      {
        installationRepositories: ['acme/payments']
      },
      {
        installationRepositories: ['acme/ops']
      }
    ],
    effectiveConfig: {
      installationRepositories: ['acme/effective']
    }
  });

  assert.deepEqual(repositories, []);
});

test('resolveGitHubRepositoryChoicesSource falls back to staged draft installation repositories', () => {
  const repositories = resolveGitHubRepositoryChoicesSource(null, {
    stagedConfig: {
      installationRepositories: [
        { owner: 'acme', name: 'staged-one' },
        { owner: 'acme', name: 'staged-two' }
      ]
    },
    activeConfigs: [
      {
        installationRepositories: ['acme/live-one']
      },
      {
        installationRepositories: ['acme/live-two']
      }
    ]
  });

  assert.deepEqual(repositories, [
    { owner: 'acme', name: 'staged-one' },
    { owner: 'acme', name: 'staged-two' }
  ]);
});

test('resolveGitHubRepositoryChoicesSource preserves an explicit empty test result', () => {
  const repositories = resolveGitHubRepositoryChoicesSource(
    {
      repositories: []
    },
    {
      stagedConfig: {
        installationRepositories: ['acme/staged-one']
      }
    }
  );

  assert.deepEqual(repositories, []);
});

test('resolveGitHubActiveConfigs ignores effectiveConfig when there are no real active apps', () => {
  assert.deepEqual(
    resolveGitHubActiveConfigs({
      effectiveConfig: {
        appId: 123,
        installationId: 456,
        name: 'Env config'
      }
    }),
    []
  );

  assert.deepEqual(
    resolveGitHubActiveConfigs({
      activeConfig: {
        id: 'cfg-live',
        name: 'Live app'
      },
      effectiveConfig: {
        id: 'cfg-env',
        name: 'Env config'
      }
    }),
    [
      {
        id: 'cfg-live',
        name: 'Live app'
      }
    ]
  );
});

test('getGitHubRepositorySectionState uses multi-active-safe repository messaging', () => {
  assert.deepEqual(
    getGitHubRepositorySectionState({
      activeConfigs: [{ id: 'one' }, { id: 'two' }],
      stagedConfig: null,
      githubConfigResult: null,
      repositoryChoices: [],
      currentSelectedRepos: ['acme/payments'],
      ready: true
    }),
    {
      show: false,
      emptyState: ''
    }
  );

  assert.deepEqual(
    getGitHubRepositorySectionState({
      activeConfigs: [{ id: 'one' }, { id: 'two' }],
      stagedConfig: null,
      githubConfigResult: null,
      repositoryChoices: [],
      currentSelectedRepos: [],
      ready: true
    }),
    {
      show: true,
      emptyState: 'multiActive'
    }
  );
});

test('hasGitHubStagedConfigState only returns true when real staged review state exists', () => {
  assert.equal(hasGitHubStagedConfigState({}), false);
  assert.equal(hasGitHubStagedConfigState({ stagedReady: false, stagedMissing: [], stagedError: '' }), false);
  assert.equal(hasGitHubStagedConfigState({ stagedConfig: { appId: 123 } }), true);
  assert.equal(hasGitHubStagedConfigState({ stagedMissing: ['github.contract.appId'] }), true);
  assert.equal(hasGitHubStagedConfigState({ stagedError: 'staged validation failed' }), true);
});
