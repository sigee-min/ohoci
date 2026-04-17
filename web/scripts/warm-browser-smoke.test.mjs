import test from 'node:test';
import assert from 'node:assert/strict';
import path from 'node:path';

import { chromium } from 'playwright';

import { startBrowserTestServer } from './browser-test-server.mjs';

const WEB_ROOT = path.resolve(import.meta.dirname, '..');
const FIXTURE_TIMESTAMP = '2026-04-17T08:00:00.000Z';
const FIXTURE_EXPIRY = '2026-04-17T09:00:00.000Z';
const FIXTURE_POLICY_ID = 17;
const FIXTURE_TARGET = {
  owner: 'seeded-org',
  repo: 'warm-target',
  fullName: 'seeded-org/warm-target'
};
const FIXTURE_LABEL = 'warm-smoke';
const FIXTURE_RUNNER_NAME = 'warm-reserved-runner';
const FIXTURE_INSTANCE_OCID = 'ocid1.instance.oc1..seededwarmrunner';
const FIXTURE_COMPARTMENT_OCID = 'ocid1.compartment.oc1..seededwarm';
const FIXTURE_SUBNET_OCID = 'ocid1.subnet.oc1..seededwarm';
const FIXTURE_IMAGE_OCID = 'ocid1.image.oc1..seededwarm';

function fulfillJson(route, payload, status = 200) {
  return route.fulfill({
    status,
    headers: {
      'Content-Type': 'application/json'
    },
    body: JSON.stringify(payload)
  });
}

function createApiFixture() {
  const unexpectedRequests = [];
  const requestCounts = new Map();

  const githubConfig = {
    status: {
      source: 'cms',
      ready: true,
      configured: true,
      hasWebhookSecret: true,
      hasAppCredentials: true,
      accountLogin: FIXTURE_TARGET.owner,
      accountType: 'Organization',
      selectedRepos: [FIXTURE_TARGET.fullName],
      activeConfig: {
        id: 'seeded-active-github-config',
        name: 'Seeded active config',
        apiBaseUrl: 'https://api.github.com',
        authMode: 'app',
        appId: 123,
        installationId: 456,
        accountLogin: FIXTURE_TARGET.owner,
        accountType: 'Organization',
        selectedRepos: [FIXTURE_TARGET.fullName],
        installationState: 'installed',
        installationRepositorySelection: 'selected',
        installationRepositories: [FIXTURE_TARGET.fullName],
        installationReady: true,
        isActive: true,
        lastTestedAt: FIXTURE_TIMESTAMP,
        createdAt: FIXTURE_TIMESTAMP,
        updatedAt: FIXTURE_TIMESTAMP
      },
      activeConfigs: [
        {
          id: 'seeded-active-github-config',
          name: 'Seeded active config',
          apiBaseUrl: 'https://api.github.com',
          authMode: 'app',
          appId: 123,
          installationId: 456,
          accountLogin: FIXTURE_TARGET.owner,
          accountType: 'Organization',
          selectedRepos: [FIXTURE_TARGET.fullName],
          installationState: 'installed',
          installationRepositorySelection: 'selected',
          installationRepositories: [FIXTURE_TARGET.fullName],
          installationReady: true,
          isActive: true,
          lastTestedAt: FIXTURE_TIMESTAMP,
          createdAt: FIXTURE_TIMESTAMP,
          updatedAt: FIXTURE_TIMESTAMP
        }
      ]
    }
  };

  const ociRuntime = {
    source: 'cms',
    overrideSettings: null,
    effectiveSettings: {
      compartmentOcid: FIXTURE_COMPARTMENT_OCID,
      availabilityDomain: 'kIdk:AP-SEOUL-1-AD-1',
      subnetOcid: FIXTURE_SUBNET_OCID,
      nsgOcids: [],
      imageOcid: FIXTURE_IMAGE_OCID,
      assignPublicIp: false,
      cacheCompatEnabled: false,
      cacheBucketName: '',
      cacheObjectPrefix: '',
      cacheRetentionDays: 0
    },
    ready: true,
    missing: []
  };

  async function handle(route) {
    const request = route.request();
    const url = new URL(request.url());
    const key = `${request.method()} ${url.pathname}`;
    requestCounts.set(key, (requestCounts.get(key) || 0) + 1);

    switch (key) {
      case 'GET /api/v1/auth/session':
        return fulfillJson(route, {
          session: {
            authenticated: true,
            mustChangePassword: false,
            username: 'seeded-admin'
          }
        });
      case 'GET /api/v1/setup/status':
        return fulfillJson(route, {
          completed: true,
          updatedAt: FIXTURE_TIMESTAMP,
          steps: {
            password: { completed: true, missing: [] },
            github: { completed: true, missing: [] },
            oci: { completed: true, missing: [] }
          }
        });
      case 'GET /api/v1/github/config':
        return fulfillJson(route, githubConfig);
      case 'GET /api/v1/github/drift':
        return fulfillJson(route, {
          generatedAt: FIXTURE_TIMESTAMP,
          severity: 'ok',
          issues: []
        });
      case 'GET /api/v1/github/config/manifest/pending':
        return fulfillJson(route, { pending: null });
      case 'GET /api/v1/oci/auth':
        return fulfillJson(route, {
          effectiveMode: 'api_key',
          defaultMode: 'api_key',
          activeCredential: {
            id: 'seeded-credential',
            profile: 'DEFAULT'
          },
          runtimeConfigReady: true,
          runtimeConfigMissing: []
        });
      case 'GET /api/v1/oci/runtime':
        return fulfillJson(route, ociRuntime);
      case 'POST /api/v1/oci/catalog':
        return fulfillJson(route, {
          availabilityDomains: ['kIdk:AP-SEOUL-1-AD-1'],
          subnets: [
            {
              id: FIXTURE_SUBNET_OCID,
              displayName: 'Seeded subnet'
            }
          ],
          images: [
            {
              id: FIXTURE_IMAGE_OCID,
              displayName: 'Seeded warm image'
            }
          ],
          shapes: [
            {
              shape: 'VM.Standard.E4.Flex',
              isFlexible: true,
              ocpuMin: 1,
              ocpuMax: 4,
              memoryMinGb: 16,
              memoryMaxGb: 64,
              memoryMinPerOcpuGb: 16,
              memoryMaxPerOcpuGb: 64
            }
          ],
          sourceRegion: 'ap-seoul-1',
          validatedAt: FIXTURE_TIMESTAMP
        });
      case 'GET /api/v1/oci/subnets':
        return fulfillJson(route, {
          items: [],
          defaultSubnetId: ''
        });
      case 'GET /api/v1/policies':
        return fulfillJson(route, {
          items: [
            {
              id: FIXTURE_POLICY_ID,
              labels: [FIXTURE_LABEL],
              subnetOcid: FIXTURE_SUBNET_OCID,
              shape: 'VM.Standard.E4.Flex',
              ocpu: 1,
              memoryGb: 16,
              maxRunners: 2,
              ttlMinutes: 30,
              spot: false,
              enabled: true,
              warmEnabled: true,
              warmMinIdle: 1,
              warmTtlMinutes: 30,
              warmRepoAllowlist: [FIXTURE_TARGET.fullName]
            }
          ]
        });
      case 'GET /api/v1/runners':
        return fulfillJson(route, {
          items: [
            {
              id: 501,
              runnerName: FIXTURE_RUNNER_NAME,
              repoOwner: FIXTURE_TARGET.owner,
              repoName: FIXTURE_TARGET.repo,
              status: 'running',
              expiresAt: FIXTURE_EXPIRY,
              instanceOcid: FIXTURE_INSTANCE_OCID,
              source: 'warm',
              warmState: 'reserved',
              warmPolicyId: FIXTURE_POLICY_ID,
              warmRepoOwner: FIXTURE_TARGET.owner,
              warmRepoName: FIXTURE_TARGET.repo
            }
          ]
        });
      case 'GET /api/v1/jobs':
        return fulfillJson(route, { items: [] });
      case 'GET /api/v1/events':
        return fulfillJson(route, {
          events: [],
          logs: []
        });
      case 'GET /api/v1/billing/policies':
        return fulfillJson(route, {
          generatedAt: FIXTURE_TIMESTAMP,
          currency: 'USD',
          ociBilledCost: 0,
          totalCost: 0,
          mappedCost: 0,
          tagVerifiedCost: 0,
          resourceFallbackCost: 0,
          tagOnlyCost: 0,
          unmappedCost: 0,
          items: [],
          issues: []
        });
      case 'GET /api/v1/billing/guardrails':
        return fulfillJson(route, {
          generatedAt: FIXTURE_TIMESTAMP,
          windowDays: 7,
          items: []
        });
      case 'GET /api/v1/runner-images':
        return fulfillJson(route, {
          recipes: [],
          builds: [],
          resources: []
        });
      default:
        unexpectedRequests.push(`${key}${url.search}`);
        return fulfillJson(route, {
          error: `Unexpected seeded smoke request: ${key}${url.search}`
        }, 500);
    }
  }

  return {
    unexpectedRequests,
    requestCounts,
    handle
  };
}

async function startFixtureServer() {
  return startBrowserTestServer({
    root: WEB_ROOT
  });
}

test('seeded warm-degraded workspace renders overview and runner warm surfaces', async () => {
  const { server, baseUrl } = await startFixtureServer();
  const browser = await chromium.launch();
  const page = await browser.newPage({
    viewport: {
      width: 1440,
      height: 1100
    }
  });
  const pageErrors = [];
  const requestFailures = [];
  const apiFixture = createApiFixture();

  page.on('pageerror', (error) => {
    pageErrors.push(error?.message || String(error));
  });
  page.on('requestfailed', (request) => {
    const url = request.url();
    const resourceType = request.resourceType();
    if (resourceType !== 'document' && !url.includes('/api/v1/')) {
      return;
    }
    requestFailures.push(`${request.method()} ${request.url()} ${request.failure()?.errorText || ''}`.trim());
  });

  await page.route('**/api/v1/**', (route) => apiFixture.handle(route));

  try {
    await page.goto(baseUrl, { waitUntil: 'domcontentloaded' });

    await page.getByRole('button', { name: 'Overview', exact: true }).waitFor();
    await page.getByText('Warm degraded').first().waitFor();
    await page.getByText('Warm capacity is below target.').first().waitFor();
    await page.getByText('1 warm targets are short by 1 idle warm runners.').first().waitFor();
    await page.getByText('Warm target gaps').waitFor();
    await page.getByText(FIXTURE_TARGET.fullName).first().waitFor();
    await page.getByText(`Policy #${FIXTURE_POLICY_ID}`).waitFor();
    await page.getByText(`Labels: ${FIXTURE_LABEL}`).waitFor();
    await page.getByText(/Missing 1 .* Idle 0 .* Reserved 1 .* Warming 0/).waitFor();

    await page.getByRole('button', { name: 'Runners', exact: true }).click();
    await page.getByText(FIXTURE_RUNNER_NAME).waitFor();
    await page.getByText('Source: warm').waitFor();
    await page.getByText('Warm state: reserved').waitFor();
    await page.getByText(`Warm target: ${FIXTURE_TARGET.fullName}`).waitFor();
    await page.getByText(`Warm policy: ${FIXTURE_POLICY_ID}`).waitFor();

    assert.deepEqual(pageErrors, []);
    assert.deepEqual(requestFailures, []);
    assert.deepEqual(apiFixture.unexpectedRequests, []);
    assert.ok((apiFixture.requestCounts.get('GET /api/v1/auth/session') || 0) >= 1, 'session fixture should be requested');
    assert.ok((apiFixture.requestCounts.get('GET /api/v1/policies') || 0) >= 1, 'policy fixture should be requested');
    assert.ok((apiFixture.requestCounts.get('GET /api/v1/runners') || 0) >= 1, 'runner fixture should be requested');
  } finally {
    await page.close();
    await browser.close();
    await server.close();
  }
});
