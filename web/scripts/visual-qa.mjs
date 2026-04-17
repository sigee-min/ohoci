import assert from 'node:assert/strict';
import { createHmac } from 'node:crypto';
import { createServer } from 'node:http';
import { mkdir } from 'node:fs/promises';
import path from 'node:path';

import { chromium } from 'playwright';

const BASE_URL = process.env.GRUNNER_QA_BASE_URL || 'http://127.0.0.1:18080';
const API_BASE_URL = process.env.GRUNNER_QA_API_BASE_URL || BASE_URL;
const SCREENSHOT_ROOT = path.resolve(process.cwd(), 'artifacts', 'visual-qa');
const TARGET_PASSWORD = process.env.GRUNNER_QA_PASSWORD || 'Admin12345!';
const HEADLESS = process.env.GRUNNER_QA_HEADLESS === 'true';
const CREATE_SAMPLE_POLICY = process.env.GRUNNER_QA_CREATE_POLICY === 'true';
const LOGIN_CANDIDATES = ['admin', TARGET_PASSWORD, 'adminadmin'];
const POLICY_LABELS = ['visual-qa', 'runner'];
const EXPIRING_POLICY_LABELS = ['visual-qa-expire', 'runner'];
const RUNNER_IMAGE_RECIPE_NAME = 'visual-qa-node22';
const GITHUB_APP_ID = 123;
const GITHUB_INSTALLATION_ID = 456;
const GITHUB_OWNER = 'visual-qa-org';
const GITHUB_REPO = 'runner-smoke';
const QA_WEBHOOK_SECRET = 'visual-qa-webhook-secret';
const QA_STAGED_WEBHOOK_SECRET = 'visual-qa-webhook-secret-staged';
const LOCALE_STORAGE_KEY = 'ohoci.locale';
const QA_SELECTED_REPOS = [`${GITHUB_OWNER}/${GITHUB_REPO}`];
const POLICY_RATE_LIMIT_ERROR_TEXT_KO = '요청이 너무 많아 잠시 제한되었습니다. 잠시 후 다시 시도하세요.';
const KOREAN_RUNNER_IMAGE_KIND_LABELS = {
  image: '이미지',
  instance: '인스턴스',
  github_runner_instance: 'GitHub 러너 인스턴스',
  runner_image: '러너 이미지',
  runner_image_bake_instance: '베이크 인스턴스',
  console_capture: '콘솔 캡처'
};
const RAW_RUNNER_IMAGE_ENUM_RESIDUE_KINDS = new Set([
  'github_runner_instance',
  'runner_image',
  'runner_image_bake_instance',
  'console_capture'
]);
const KOREAN_RUNTIME_RESIDUE_PATTERNS = [
  /github api\s+[A-Z]+\s+\S+\s+failed:/i,
  /github app metadata request failed:/i,
  /\b(?:Get|Post|Put|Patch|Delete|Head|Options)\s+"/,
  /\bdial tcp\b/i,
  /\bconnect:\s+connection refused\b/i,
  /\bconnection refused\b/i,
  /\bservice unavailable\b/i,
  /\bcontext deadline exceeded\b/i,
  /\bi\/o timeout\b/i,
  /\btoo many requests\b/i,
  /\brate limit exceeded\b/i,
  /\brate limited\b/i,
  /\brequest timed out\b/i,
  /\btimed out\b/i,
  /\bgithub_runner_instance\b/i,
  /\brunner_image\b/i,
  /\brunner_image_bake_instance\b/i,
  /\bconsole_capture\b/i,
  /\bvisual qa\b/i,
  /visual qa runner lookup unavailable/i
];
const VIEW_LABELS = {
  en: {
    overview: 'Overview',
    settings: 'Settings',
    policies: 'Policies',
    runners: 'Runners',
    runnerImages: 'Runner images',
    jobs: 'Jobs',
    events: 'Events'
  },
  ko: {
    overview: '개요',
    settings: '설정',
    policies: '정책',
    runners: '러너',
    runnerImages: '러너 이미지',
    jobs: '작업',
    events: '이벤트'
  }
};
const TEST_PRIVATE_KEY = `-----BEGIN PRIVATE KEY-----
MIICdgIBADANBgkqhkiG9w0BAQEFAASCAmAwggJcAgEAAoGBAOW0ED5HhOi+am89
+A8Gs84lcTxj95fyY/m4El01AaOMwB6Ufnx8lIIY7abn71exSaKDzsFNEM+uBkdH
W8mG+Lna3TGmRS52G46DnulBiREnpRV+NIQwMjZHpQ5WvW9nzePZ4navmdnhyrcE
pYA3vKJKND/p8+8mlD0G8CfD0Ko3AgMBAAECgYA1HvMys+90s7SBjV80emRSpC4P
vT6hERk1wu/cRknevMohSE4IE/d0LrenBbRAH2vb/YdvBJeCr8gb69C6RlB2mo25
gMv8A+zggDGyIJEq5JCIGsFWa463bd8P/Y+tZ6ZsCULVuksWl+suvhoJvr3zBeeM
eQMF3rd8hzhYa5iqYQJBAPakEcZAcMAQWcjzBQKmdZoP+zXvExMOrDlFKeqsbeWP
VHFrpcZ+t/A3SwKKOmX5Ie50rPtCBi+2NfLYYebGnv0CQQDua3Vvomv1zyJmuEi+
Hr+rqHtzjjA8vVUCK8Tb9UEqWLZ3JQNcoGvgHUZrw3Euq1nqvOYYHsZGTLXSIrlu
waZDAkBa+tSvq++reZyVGsgbXSn+ZazGDWWc3wm6qn+22FpFluSQXiQtn2rcipj5
2+GE4iyZGKMCoC1GBlHKPfWHOndFAkAwso44EQrQGFDEfluNSaaIn08n2SENJvbY
DKyW6M84oQoT5+F55+Jg0lnx5OeXSrSA97hfsNl6vmxc0W7iqncVAkEArERxQtrn
d/fYemHb9Wv5ibLOZWoPCNy2WACMGyHQ7+3+pB/IxI9ueUrnrRaCAQLkDuhF82sW
nApG0TpVWHyZUQ==
-----END PRIVATE KEY-----`;

let githubStub;

async function ensureDirectory() {
  const stamp = new Date().toISOString().replaceAll(':', '-');
  const targetDir = path.join(SCREENSHOT_ROOT, stamp);
  await mkdir(targetDir, { recursive: true });
  return targetDir;
}

async function capture(page, outputDir, name, options = {}) {
  const transientAlert = page.getByText('Request failed');
  if (await transientAlert.isVisible().catch(() => false)) {
    await page.waitForTimeout(1800);
  }
  await page.waitForTimeout(400);
  const filePath = path.join(outputDir, `${name}.png`);
  await page.screenshot({
    path: filePath,
    fullPage: Boolean(options.fullPage),
    animations: 'disabled'
  });
  return filePath;
}

function writeJSON(response, payload) {
  response.statusCode = 200;
  response.setHeader('Content-Type', 'application/json');
  response.end(JSON.stringify(payload));
}

async function ensureGitHubStub() {
  if (githubStub) {
    return githubStub.baseUrl;
  }

  const server = createServer((request, response) => {
    const { pathname } = new URL(request.url || '/', 'http://127.0.0.1');

    if (request.method === 'GET' && pathname === '/app') {
      writeJSON(response, { id: GITHUB_APP_ID });
      return;
    }
    if (request.method === 'POST' && pathname === `/app/installations/${GITHUB_INSTALLATION_ID}/access_tokens`) {
      writeJSON(response, { token: 'visual-qa-installation-token' });
      return;
    }
    if (request.method === 'GET' && pathname === `/app/installations/${GITHUB_INSTALLATION_ID}`) {
      writeJSON(response, {
        account: {
          login: GITHUB_OWNER,
          type: 'Organization'
        },
        repository_selection: 'selected'
      });
      return;
    }
    if (request.method === 'GET' && pathname === '/installation/repositories') {
      writeJSON(response, {
        repositories: [
          {
            full_name: `${GITHUB_OWNER}/${GITHUB_REPO}`,
            name: GITHUB_REPO,
            private: true,
            owner: { login: GITHUB_OWNER },
            permissions: { admin: true }
          }
        ]
      });
      return;
    }
    if (request.method === 'POST' && /^\/repos\/[^/]+\/[^/]+\/actions\/runners\/registration-token$/.test(pathname)) {
      writeJSON(response, {
        token: 'visual-qa-runner-token',
        expires_at: new Date(Date.now() + 60 * 60 * 1000).toISOString()
      });
      return;
    }
    if (request.method === 'GET' && /^\/repos\/[^/]+\/[^/]+\/actions\/runners$/.test(pathname)) {
      response.statusCode = 502;
      response.setHeader('Content-Type', 'text/plain');
      response.end('visual qa runner lookup unavailable');
      return;
    }
    if (request.method === 'DELETE' && /^\/repos\/[^/]+\/[^/]+\/actions\/runners\/\d+$/.test(pathname)) {
      response.statusCode = 204;
      response.end();
      return;
    }

    response.statusCode = 404;
    response.end('not found');
  });

  await new Promise((resolve, reject) => {
    server.once('error', reject);
    server.listen(0, '127.0.0.1', () => resolve());
  });

  const address = server.address();
  githubStub = {
    server,
    baseUrl: `http://127.0.0.1:${address.port}`
  };
  return githubStub.baseUrl;
}

function normalizeLabels(values) {
  return Array.from(new Set((Array.isArray(values) ? values : []).map((value) => String(value || '').trim()).filter(Boolean))).sort();
}

function sameLabels(left, right) {
  const leftNormalized = normalizeLabels(left);
  const rightNormalized = normalizeLabels(right);
  if (leftNormalized.length !== rightNormalized.length) {
    return false;
  }
  return leftNormalized.every((value, index) => value === rightNormalized[index]);
}

function buildRunnerName(owner, repo, jobId) {
  const base = `${owner}-${repo}`.replaceAll('/', '-').toLowerCase();
  return `ohoci-${base}-${jobId}`;
}

function buildGitHubConfigPayload(apiBaseUrl, webhookSecret = QA_WEBHOOK_SECRET) {
  return {
    apiBaseUrl,
    appId: GITHUB_APP_ID,
    installationId: GITHUB_INSTALLATION_ID,
    privateKeyPem: TEST_PRIVATE_KEY,
    webhookSecret,
    selectedRepos: QA_SELECTED_REPOS
  };
}

function resolveGitHubStatusPayload(payload = {}) {
  return payload?.status || payload || {};
}

function resolveActiveGitHubConfig(payload = {}) {
  const status = resolveGitHubStatusPayload(payload);
  return status.activeConfig || status.effectiveConfig || status.config || status.current || null;
}

function activeGitHubConfigMatchesStub(payload = {}, apiBaseUrl) {
  const status = resolveGitHubStatusPayload(payload);
  const config = resolveActiveGitHubConfig(payload) || {};

  return String(config.apiBaseUrl || '').trim() === apiBaseUrl
    && Number(config.appId || 0) === GITHUB_APP_ID
    && Number(config.installationId || 0) === GITHUB_INSTALLATION_ID
    && sameLabels(config.selectedRepos || status.selectedRepos, QA_SELECTED_REPOS);
}

function normalizeVisibleText(value) {
  return String(value || '')
    .replace(/\s+/g, ' ')
    .trim();
}

function assertNoEnglishResidue(text, description) {
  const normalized = normalizeVisibleText(text);
  const matchedPattern = KOREAN_RUNTIME_RESIDUE_PATTERNS.find((pattern) => pattern.test(normalized));
  assert.equal(
    matchedPattern,
    undefined,
    `Unexpected English runtime residue in ${description}: ${matchedPattern} matched "${normalized}"`
  );
}

async function assertNoEnglishResidueOnPage(page, description) {
  let text = '';
  try {
    text = await page.locator('main').textContent();
  } catch {
    text = await page.locator('body').textContent();
  }
  assertNoEnglishResidue(text, description);
}

async function readTableTextByHeader(page, headerLabel) {
  const table = page.locator('table').filter({
    has: page.getByRole('columnheader', { name: headerLabel, exact: true })
  }).first();
  await table.waitFor({ state: 'visible', timeout: 15000 });
  return normalizeVisibleText(await table.textContent());
}

async function assertKoreanRunnerImageKindsLocalized(page, resourceKinds = []) {
  const discoveryTableText = await readTableTextByHeader(page, '종류');
  for (const kind of resourceKinds) {
    const localizedLabel = KOREAN_RUNNER_IMAGE_KIND_LABELS[kind];
    if (localizedLabel) {
      assert.ok(
        discoveryTableText.includes(localizedLabel),
        `Expected Korean runner image resource kind "${localizedLabel}" for raw kind "${kind}".`
      );
    }
    if (RAW_RUNNER_IMAGE_ENUM_RESIDUE_KINDS.has(kind)) {
      assert.ok(
        !discoveryTableText.includes(kind),
        `Unexpected raw runner image resource kind "${kind}" in the Korean discovery table: "${discoveryTableText}"`
      );
    }
  }
}

async function captureKoreanPolicyCatalogRateLimit(page, outputDir) {
  const routePattern = '**/api/v1/oci/catalog';
  const rateLimitHandler = async (route) => {
    if (route.request().method() !== 'POST') {
      await route.continue();
      return;
    }

    await route.fulfill({
      status: 429,
      headers: {
        'Content-Type': 'application/json',
        'Retry-After': '1'
      },
      body: JSON.stringify({ error: 'too many requests' })
    });
  };

  await page.route(routePattern, rateLimitHandler);
  try {
    await page.reload({ waitUntil: 'domcontentloaded' });
    await waitForWorkspaceShell(page, 'ko');
    await gotoWorkspaceView(page, VIEW_LABELS.ko.policies);
    await assertVisibleText(page, POLICY_RATE_LIMIT_ERROR_TEXT_KO, 'the translated Korean policy catalog rate-limit error');
    await assertNoEnglishResidueOnPage(page, 'the Korean policies rate-limit workspace');
    await capture(page, outputDir, 'policies-loaded-ko-rate-limit-desktop');
  } finally {
    await page.unroute(routePattern, rateLimitHandler);
  }

  await page.reload({ waitUntil: 'domcontentloaded' });
  await waitForWorkspaceShell(page, 'ko');
}

function signGitHubWebhook(body, secret = QA_WEBHOOK_SECRET) {
  return `sha256=${createHmac('sha256', secret).update(body).digest('hex')}`;
}

function buildInstallationPayload(action) {
  return {
    action,
    installation: {
      id: GITHUB_INSTALLATION_ID,
      account: {
        login: GITHUB_OWNER,
        type: 'Organization'
      },
      repository_selection: 'selected'
    }
  };
}

function buildWorkflowJobPayload(action, jobId, labels, options = {}) {
  return {
    action,
    repository: {
      name: GITHUB_REPO,
      owner: {
        login: GITHUB_OWNER
      }
    },
    installation: {
      id: GITHUB_INSTALLATION_ID
    },
    workflow_job: {
      id: jobId,
      run_id: options.runId ?? jobId,
      run_attempt: options.runAttempt ?? 1,
      status: options.status ?? action,
      conclusion: options.conclusion ?? '',
      runner_id: options.runnerId ?? 0,
      runner_name: options.runnerName ?? '',
      labels: ['self-hosted', ...labels]
    }
  };
}

async function postGitHubWebhook(page, eventType, deliveryId, payload, options = {}) {
  const body = JSON.stringify(payload);
  const response = await page.context().request.fetch(`${API_BASE_URL}/api/v1/github/webhook`, {
    method: 'POST',
    data: body,
    headers: {
      'Content-Type': 'application/json',
      'X-GitHub-Delivery': deliveryId,
      'X-GitHub-Event': eventType,
      'X-Hub-Signature-256': signGitHubWebhook(body, options.secret)
    }
  });
  if (!response.ok()) {
    throw new Error(`POST /api/v1/github/webhook (${eventType}) failed: ${response.status()} ${await response.text()}`);
  }
  const result = await response.json().catch(() => ({}));
  if (result?.processed === false) {
    throw new Error(`POST /api/v1/github/webhook (${eventType}) was accepted but not processed: ${result.error || 'unknown error'}`);
  }
  return result;
}

async function waitForCondition(page, description, check, options = {}) {
  const timeoutMs = Number.isFinite(options.timeoutMs) ? options.timeoutMs : 15000;
  const intervalMs = Number.isFinite(options.intervalMs) ? options.intervalMs : 300;
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    const result = await check();
    if (result) {
      return result;
    }
    await page.waitForTimeout(intervalMs);
  }
  throw new Error(`Timed out waiting for ${description}.`);
}

async function assertVisibleText(page, text, description, options = {}) {
  const locator = page.getByText(text, { exact: false }).first();
  await locator.waitFor({ state: 'visible', timeout: Number.isFinite(options.timeoutMs) ? options.timeoutMs : 15000 });
  assert.ok(await locator.isVisible(), `Expected ${description}: ${text}`);
}

async function setLocale(page, locale) {
  await page.evaluate(
    ([storageKey, nextLocale]) => window.localStorage.setItem(storageKey, nextLocale),
    [LOCALE_STORAGE_KEY, locale]
  );
  await page.reload({ waitUntil: 'domcontentloaded' });
  await waitForWorkspaceShell(page, locale);
}

async function gotoWorkspaceView(page, label) {
  const targetButton = page.getByRole('button', { name: label, exact: true });
  const visible = await targetButton.isVisible().catch(() => false);
  if (!visible) {
    const sidebarTrigger = page.getByRole('button', { name: 'Toggle Sidebar' });
    if (await sidebarTrigger.isVisible().catch(() => false)) {
      await sidebarTrigger.click();
    }
  }
  await targetButton.click();
  await page.waitForTimeout(500);
}

async function isPasswordChangeStep(page) {
  if (await page.getByRole('heading', { name: 'Set the admin password' }).isVisible().catch(() => false)) {
    return true;
  }
  return page.getByLabel('Current password').isVisible().catch(() => false);
}

async function attemptLogin(page, password) {
  await page.goto(BASE_URL, { waitUntil: 'domcontentloaded' });
  await page.waitForTimeout(1200);

  if (await isPasswordChangeStep(page)) {
    return { kind: 'password-change', password };
  }
  if (await page.getByRole('heading', { name: 'Overview' }).isVisible().catch(() => false)) {
    return { kind: 'workspace', password };
  }

  const usernameField = page.locator('#username');
  const passwordField = page.locator('#password');
  if (!(await usernameField.isVisible().catch(() => false)) || !(await passwordField.isVisible().catch(() => false))) {
    return { kind: 'unknown', password };
  }

  const response = await page.context().request.fetch(`${API_BASE_URL}/api/v1/auth/login`, {
    method: 'POST',
    data: {
      username: 'admin',
      password
    },
    headers: {
      'Content-Type': 'application/json'
    }
  });

  if (response.status() === 401) {
    return { kind: 'login-failed', password };
  }
  if (!response.ok()) {
    return { kind: 'unknown', password };
  }

  await page.goto(BASE_URL, { waitUntil: 'domcontentloaded' });
  await page.waitForTimeout(1200);

  if (await isPasswordChangeStep(page)) {
    return { kind: 'password-change', password };
  }

  const sessionResponse = await page.context().request.fetch(`${API_BASE_URL}/api/v1/auth/session`);
  if (sessionResponse.ok()) {
    return { kind: 'workspace', password };
  }

  return { kind: 'unknown', password };
}

async function waitForWorkspaceShell(page, locale = 'en', timeoutMs = 30000) {
  const labels = VIEW_LABELS[locale] || VIEW_LABELS.en;
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    for (const locator of [
      page.getByRole('heading', { name: labels.overview }),
      page.getByRole('button', { name: labels.overview, exact: true }),
      page.getByRole('button', { name: labels.policies, exact: true }),
      page.getByRole('button', { name: labels.runnerImages, exact: true })
    ]) {
      if (await locator.isVisible().catch(() => false)) {
        return;
      }
    }
    await page.waitForTimeout(500);
  }
  throw new Error('Workspace shell did not appear after setup bootstrap.');
}

async function ensureWorkspaceSetup(page) {
  const setup = await requestJSON(page, 'GET', '/api/v1/setup');
  if (setup?.ready) {
    await ensureActiveGitHubConfig(page);
    return;
  }

  await ensureActiveGitHubConfig(page, { force: !setup?.github?.ready });

  if (!setup?.ociRuntime?.ready) {
    await requestJSON(page, 'PUT', '/api/v1/oci/runtime', {
      compartmentOcid: 'ocid1.compartment.oc1..visualqa',
      availabilityDomain: 'AD-1',
      subnetOcid: 'ocid1.subnet.oc1..ad1',
      imageOcid: 'ocid1.image.oc1..ubuntu',
      nsgOcids: [],
      assignPublicIp: false
    });
  }

  for (let attempt = 0; attempt < 8; attempt += 1) {
    const next = await requestJSON(page, 'GET', '/api/v1/setup');
    if (next?.ready) {
      return;
    }
    await page.waitForTimeout(300);
  }
  throw new Error('Workspace setup did not reach ready state during QA bootstrap.');
}

async function configureStagedGitHubWebhookRoute(page, secret) {
  const apiBaseUrl = await ensureGitHubStub();
  await requestJSON(page, 'DELETE', '/api/v1/github/config/staged');
  await requestJSON(page, 'POST', '/api/v1/github/config/staged', buildGitHubConfigPayload(apiBaseUrl, secret));
}

async function ensureAuthenticated(page, outputDir) {
  await page.goto(BASE_URL, { waitUntil: 'domcontentloaded' });
  await page.waitForTimeout(800);

  if (await page.getByRole('button', { name: VIEW_LABELS.en.overview, exact: true }).isVisible().catch(() => false)) {
    await ensureWorkspaceSetup(page);
    return TARGET_PASSWORD;
  }

  if (await isPasswordChangeStep(page)) {
    await capture(page, outputDir, 'password-change-before');
    await page.locator('#current-password').fill('admin');
    await page.locator('#new-password').fill(TARGET_PASSWORD);
    await page.getByRole('button', { name: 'Save password' }).click();
    await page.waitForTimeout(1200);
    await ensureWorkspaceSetup(page);
    await page.goto(BASE_URL, { waitUntil: 'domcontentloaded' });
    await waitForWorkspaceShell(page, 'en');
    return TARGET_PASSWORD;
  }

  await capture(page, outputDir, 'login-before');

  for (const password of LOGIN_CANDIDATES) {
    const result = await attemptLogin(page, password);
    if (result.kind === 'password-change') {
      await capture(page, outputDir, 'password-change-before');
      await page.getByLabel('Current password').fill(password);
      await page.getByLabel('New password').fill(TARGET_PASSWORD);
      await page.getByRole('button', { name: 'Save password' }).click();
      await page.waitForTimeout(1200);
      await ensureWorkspaceSetup(page);
      await page.goto(BASE_URL, { waitUntil: 'domcontentloaded' });
      await waitForWorkspaceShell(page, 'en');
      return TARGET_PASSWORD;
    }
    if (result.kind === 'workspace') {
      await ensureWorkspaceSetup(page);
      await page.goto(BASE_URL, { waitUntil: 'domcontentloaded' });
      await waitForWorkspaceShell(page, 'en');
      return password;
    }
  }

  throw new Error('Could not authenticate with the known local QA credentials.');
}

function buildPolicyPayload(shape, labels, overrides = {}) {
  const ocpu = Math.round(shape.defaultOcpu || shape.ocpuMin || 1);
  const memoryGb = Math.round(shape.defaultMemoryGb || shape.memoryDefaultPerOcpuGb || shape.memoryMinGb || 16);
  return {
    labels,
    subnetOcid: '',
    shape: shape.shape,
    ocpu,
    memoryGb,
    maxRunners: overrides.maxRunners ?? 2,
    ttlMinutes: overrides.ttlMinutes ?? 30,
    spot: false,
    enabled: true
  };
}

async function ensurePolicy(page, labels, overrides = {}) {
  const currentPolicies = await requestJSON(page, 'GET', '/api/v1/policies');
  const existing = (currentPolicies.items || []).find((item) => sameLabels(item.labels, labels));

  const runtimeStatus = await requestJSON(page, 'GET', '/api/v1/oci/runtime');
  if (!runtimeStatus?.ready) {
    throw new Error(`OCI runtime must be ready before creating the ${labels.join(', ')} QA policy.`);
  }

  const effectiveSettings = runtimeStatus.effectiveSettings || {};
  const catalog = await requestJSON(page, 'POST', '/api/v1/oci/catalog', {
    compartmentOcid: effectiveSettings.compartmentOcid,
    availabilityDomain: effectiveSettings.availabilityDomain,
    imageOcid: effectiveSettings.imageOcid,
    subnetOcid: effectiveSettings.subnetOcid
  });
  const shape = (catalog.shapes || []).find((item) => !item.isFlexible) || (catalog.shapes || [])[0];
  if (!shape?.shape) {
    throw new Error(`No OCI shape was available to create the ${labels.join(', ')} QA policy.`);
  }

  const payload = buildPolicyPayload(shape, labels, overrides);
  if (!existing) {
    return requestJSON(page, 'POST', '/api/v1/policies', payload);
  }

  return requestJSON(page, 'PUT', `/api/v1/policies/${existing.id}`, payload);
}

async function ensureSamplePolicy(page) {
  return ensurePolicy(page, POLICY_LABELS, { ttlMinutes: 30, maxRunners: 2 });
}

async function ensureExpiringPolicy(page) {
  return ensurePolicy(page, EXPIRING_POLICY_LABELS, { ttlMinutes: 0, maxRunners: 1 });
}

function resolveRateLimitDelayMs(response, attempt, baseDelayMs = 750) {
  const retryAfter = response.headers()['retry-after'];
  if (retryAfter) {
    const seconds = Number.parseFloat(retryAfter);
    if (Number.isFinite(seconds) && seconds >= 0) {
      return Math.ceil(seconds * 1000);
    }

    const retryAt = Date.parse(retryAfter);
    if (Number.isFinite(retryAt)) {
      return Math.max(retryAt - Date.now(), baseDelayMs);
    }
  }

  return baseDelayMs * (attempt + 1);
}

async function requestJSON(page, method, pathname, data, options = {}) {
  const attempts = Number.isFinite(options.attempts) ? options.attempts : 4;
  const rateLimitDelayMs = Number.isFinite(options.rateLimitDelayMs) ? options.rateLimitDelayMs : 750;
  let lastError;

  for (let attempt = 0; attempt < attempts; attempt += 1) {
    const response = await page.context().request.fetch(`${API_BASE_URL}${pathname}`, {
      method,
      data,
      headers: data ? { 'Content-Type': 'application/json' } : undefined
    });

    if (response.ok()) {
      return response.json().catch(() => ({}));
    }

    const body = await response.text();
    const error = new Error(`${method} ${pathname} failed: ${response.status()} ${body}`);
    error.status = response.status();
    error.responseBody = body;
    lastError = error;

    if (response.status() !== 429 || attempt === attempts - 1) {
      throw error;
    }

    await page.waitForTimeout(resolveRateLimitDelayMs(response, attempt, rateLimitDelayMs));
  }

  throw lastError;
}

async function ensureActiveGitHubConfig(page, options = {}) {
  const apiBaseUrl = await ensureGitHubStub();
  const status = await requestJSON(page, 'GET', '/api/v1/github/config').catch(() => ({}));
  if (!options.force && activeGitHubConfigMatchesStub(status, apiBaseUrl)) {
    return status;
  }

  await requestJSON(page, 'POST', '/api/v1/github/config', buildGitHubConfigPayload(apiBaseUrl));
  return requestJSON(page, 'GET', '/api/v1/github/config').catch(() => ({}));
}

function isBusyError(error) {
  return /SQLITE_BUSY|database is locked/i.test(String(error?.message || error || ''));
}

async function requestJSONWithBusyRetry(page, method, pathname, data, options = {}) {
  const attempts = Number.isFinite(options.attempts) ? options.attempts : 4;
  const delayMs = Number.isFinite(options.delayMs) ? options.delayMs : 400;
  let lastError;
  for (let attempt = 0; attempt < attempts; attempt += 1) {
    try {
      return await requestJSON(page, method, pathname, data);
    } catch (error) {
      lastError = error;
      if (!isBusyError(error) || attempt === attempts - 1) {
        throw error;
      }
      await page.waitForTimeout(delayMs);
    }
  }
  throw lastError;
}

function buildItemsForRecipe(snapshot, recipeId) {
  return (snapshot?.builds || [])
    .filter((item) => String(item.recipeId) === String(recipeId))
    .sort((left, right) => Number(right.id || 0) - Number(left.id || 0));
}

function terminalBuildStatus(status) {
  return ['available', 'failed', 'promoted'].includes(String(status || '').toLowerCase());
}

async function ensureSampleRunnerImage(page) {
  let snapshot;
  try {
    snapshot = await requestJSON(page, 'GET', '/api/v1/runner-images');
  } catch {
    return;
  }

  if (!snapshot?.preflight?.ready) {
    return;
  }

  let recipe = (snapshot.recipes || []).find((item) => item.name === RUNNER_IMAGE_RECIPE_NAME);
  if (!recipe) {
    try {
      recipe = await requestJSON(page, 'POST', '/api/v1/runner-images/recipes', {
        name: RUNNER_IMAGE_RECIPE_NAME,
        imageDisplayName: 'ohoci-visual-qa-node22',
        baseImageOcid: 'ocid1.image.oc1..ubuntu',
        shape: 'VM.Standard.E4.Flex',
        ocpu: 1,
        memoryGb: 16,
        description: 'Visual QA sample recipe',
        setupCommands: ['sudo apt-get update', 'sudo apt-get install -y nodejs'],
        verifyCommands: ['node --version']
      });
    } catch {
      return;
    }
  }

  let retriedBusyBuild = false;

  if (!buildItemsForRecipe(snapshot, recipe.id).length) {
    try {
      await requestJSONWithBusyRetry(page, 'POST', '/api/v1/runner-images/builds', {
        recipeId: Number(recipe.id)
      });
    } catch {
      // Keep the screenshot flow alive even when sample bake data cannot be prepared.
    }
  }

  try {
    for (let attempt = 0; attempt < 18; attempt += 1) {
      if (attempt < 6 || attempt % 3 === 0) {
        try {
          await requestJSONWithBusyRetry(page, 'POST', '/api/v1/runner-images/reconcile', undefined, {
            attempts: 3,
            delayMs: 300
          });
        } catch {
          // Keep polling even if reconcile hits a transient lock.
        }
      }

      const current = await requestJSON(page, 'GET', '/api/v1/runner-images');
      const currentResources = current?.resources || [];
      const builds = buildItemsForRecipe(current, recipe.id);
      const latest = builds[0];
      const latestStatus = String(latest?.status || '').toLowerCase();

      if (latestStatus === 'available' && latest?.canPromote) {
        await requestJSONWithBusyRetry(page, 'POST', `/api/v1/runner-images/builds/${latest.id}/promote`, undefined, {
          attempts: 3,
          delayMs: 300
        });
        await page.waitForTimeout(300);
        continue;
      }

      if (latestStatus === 'promoted' && currentResources.length > 0) {
        break;
      }

      if (latestStatus === 'failed' && isBusyError(latest?.summary || latest?.statusMessage || latest?.errorMessage)) {
        if (!retriedBusyBuild) {
          retriedBusyBuild = true;
          await requestJSONWithBusyRetry(page, 'POST', '/api/v1/runner-images/builds', {
            recipeId: Number(recipe.id)
          });
        } else {
          break;
        }
      }

      if (terminalBuildStatus(latestStatus) && currentResources.length > 0) {
        break;
      }
      await page.waitForTimeout(500);
    }
  } catch {
    // Keep the screenshot flow alive even when reconcile polling is unavailable.
  }
}

async function seedLoadedEventAndRunnerData(page) {
  await ensureSamplePolicy(page);
  await ensureExpiringPolicy(page);

  const seedBase = Date.now();
  const terminatedJobId = seedBase;
  const expiringJobId = seedBase + 1;
  const orphanJobId = seedBase + 2;
  const terminatedRunnerName = buildRunnerName(GITHUB_OWNER, GITHUB_REPO, terminatedJobId);
  const expiringRunnerName = buildRunnerName(GITHUB_OWNER, GITHUB_REPO, expiringJobId);

  await postGitHubWebhook(
    page,
    'installation',
    `visual-qa-installation-created-${seedBase}`,
    buildInstallationPayload('created')
  );
  await postGitHubWebhook(
    page,
    'installation_repositories',
    `visual-qa-installation-added-${seedBase}`,
    buildInstallationPayload('added')
  );

  await postGitHubWebhook(
    page,
    'workflow_job',
    `visual-qa-runner-queued-${terminatedJobId}`,
    buildWorkflowJobPayload('queued', terminatedJobId, POLICY_LABELS)
  );
  await waitForCondition(
    page,
    `runner ${terminatedRunnerName} to be created`,
    async () => {
      const snapshot = await requestJSON(page, 'GET', '/api/v1/runners');
      return (snapshot.items || []).find((runner) => runner.runnerName === terminatedRunnerName);
    }
  );
  await postGitHubWebhook(
    page,
    'workflow_job',
    `visual-qa-runner-completed-${terminatedJobId}`,
    buildWorkflowJobPayload('completed', terminatedJobId, POLICY_LABELS, {
      status: 'completed',
      conclusion: 'success',
      runnerName: terminatedRunnerName
    })
  );

  await postGitHubWebhook(
    page,
    'workflow_job',
    `visual-qa-runner-queued-${expiringJobId}`,
    buildWorkflowJobPayload('queued', expiringJobId, EXPIRING_POLICY_LABELS)
  );
  await waitForCondition(
    page,
    `runner ${expiringRunnerName} to be created`,
    async () => {
      const snapshot = await requestJSON(page, 'GET', '/api/v1/runners');
      return (snapshot.items || []).find((runner) => runner.runnerName === expiringRunnerName);
    }
  );

  await postGitHubWebhook(
    page,
    'workflow_job',
    `visual-qa-orphan-completed-${orphanJobId}`,
    buildWorkflowJobPayload('completed', orphanJobId, POLICY_LABELS, {
      status: 'completed',
      conclusion: 'success'
    })
  );

  await requestJSON(page, 'POST', '/api/v1/system/cleanup');

  await waitForCondition(
    page,
    `runner ${terminatedRunnerName} to reach terminated`,
    async () => {
      const snapshot = await requestJSON(page, 'GET', '/api/v1/runners');
      return (snapshot.items || []).find(
        (runner) => runner.runnerName === terminatedRunnerName && String(runner.status || '').toLowerCase() === 'terminated'
      );
    }
  );
  await waitForCondition(
    page,
    `runner ${expiringRunnerName} to reach terminating`,
    async () => {
      const snapshot = await requestJSON(page, 'GET', '/api/v1/runners');
      return (snapshot.items || []).find(
        (runner) => runner.runnerName === expiringRunnerName && String(runner.status || '').toLowerCase() === 'terminating'
      );
    }
  );

  try {
    await configureStagedGitHubWebhookRoute(page, QA_STAGED_WEBHOOK_SECRET);
    await postGitHubWebhook(
      page,
      'installation',
      `visual-qa-installation-suspend-${seedBase}`,
      buildInstallationPayload('suspend'),
      { secret: QA_STAGED_WEBHOOK_SECRET }
    );
  } finally {
    await requestJSON(page, 'DELETE', '/api/v1/github/config/staged');
  }

  await waitForCondition(
    page,
    'seeded event deliveries and logs',
    async () => {
      const snapshot = await requestJSON(page, 'GET', '/api/v1/events');
      const actions = new Set((snapshot.events || []).map((event) => String(event.action || '').trim().toLowerCase()));
      const messages = (snapshot.logs || []).map((entry) => String(entry.message || ''));
      const hasRequiredActions = actions.has('created') && actions.has('added') && actions.has('suspend');
      const hasRequiredLogs = messages.some((message) => message.includes('no tracked runner'))
        && messages.some((message) => message.includes(`sync GitHub runner for ${terminatedRunnerName} failed:`))
        && messages.some((message) => message.includes(`runner ${terminatedRunnerName} already in terminal OCI state TERMINATED`))
        && messages.some((message) => message.includes(`terminate requested for runner ${expiringRunnerName}`));
      return hasRequiredActions && hasRequiredLogs ? snapshot : null;
    }
  );

  return {
    terminatedRunnerName,
    expiringRunnerName
  };
}

async function captureKoreanLoadedData(page, outputDir) {
  const seeded = await seedLoadedEventAndRunnerData(page);

  await setLocale(page, 'ko');

  await gotoWorkspaceView(page, VIEW_LABELS.ko.overview);
  await assertVisibleText(page, '오류 로그', 'the translated overview snapshot label');
  await assertNoEnglishResidueOnPage(page, 'the Korean overview workspace');
  await capture(page, outputDir, 'overview-loaded-ko-desktop');

  await gotoWorkspaceView(page, VIEW_LABELS.ko.runners);
  await assertVisibleText(page, seeded.terminatedRunnerName, 'the seeded terminated runner name');
  await assertVisibleText(page, seeded.expiringRunnerName, 'the seeded expiring runner name');
  await assertVisibleText(page, '종료됨', 'the translated terminated runner status');
  await assertVisibleText(page, '종료 중', 'the translated terminating runner status');
  await capture(page, outputDir, 'runners-loaded-ko-desktop');

  await gotoWorkspaceView(page, VIEW_LABELS.ko.events);
  await assertVisibleText(page, '생성됨', 'the translated created event action');
  await assertVisibleText(page, '추가됨', 'the translated added event action');
  await assertVisibleText(page, '일시 중지', 'the translated suspend event action');
  await capture(page, outputDir, 'events-loaded-ko-deliveries-desktop');

  await page.getByRole('tab', { name: '로그 줄', exact: true }).click();
  await assertVisibleText(page, '추적 중인 러너가 없습니다', 'the translated no-tracked-runner log');
  await assertVisibleText(page, 'GitHub 러너 동기화에 실패했습니다', 'the translated runner sync failure log');
  await assertVisibleText(page, '러너 조회를 사용할 수 없습니다', 'the translated nested runner lookup reason');
  await assertVisibleText(page, '이미 OCI 종료 상태', 'the translated terminal OCI state log');
  await assertNoEnglishResidueOnPage(page, 'the Korean events log workspace');
  await capture(page, outputDir, 'events-loaded-ko-logs-desktop');

  await gotoWorkspaceView(page, VIEW_LABELS.ko.policies);
  await assertVisibleText(page, '정책 만들기', 'the Korean policies form title');
  await assertVisibleText(page, '정확한 라벨 매칭', 'the Korean policies match alert');
  await assertVisibleText(page, '매칭', 'the Korean policies tab label');
  await assertNoEnglishResidueOnPage(page, 'the Korean policies workspace');
  await capture(page, outputDir, 'policies-loaded-ko-desktop');

  await captureKoreanPolicyCatalogRateLimit(page, outputDir);

  await ensureSampleRunnerImage(page);
  let reconcileAttempt = 0;
  const runnerImagesSnapshot = await waitForCondition(
    page,
    'runner image discovery to include a GitHub runner instance',
    async () => {
      if (reconcileAttempt % 3 === 0) {
        await requestJSONWithBusyRetry(page, 'POST', '/api/v1/runner-images/reconcile', undefined, {
          attempts: 3,
          delayMs: 300
        }).catch(() => {});
      }
      reconcileAttempt += 1;

      const snapshot = await requestJSON(page, 'GET', '/api/v1/runner-images').catch(() => null);
      const resourceKinds = Array.from(new Set(
        (snapshot?.resources || [])
          .map((resource) => String(resource.kind || resource.type || '').trim())
          .filter(Boolean)
      ));
      return resourceKinds.includes('github_runner_instance')
        ? { snapshot, resourceKinds }
        : null;
    },
    { timeoutMs: 20000, intervalMs: 500 }
  );

  await gotoWorkspaceView(page, VIEW_LABELS.ko.runnerImages);
  await assertVisibleText(page, '발견된 OCI 리소스', 'the Korean runner images discovery section');
  await assertKoreanRunnerImageKindsLocalized(page, runnerImagesSnapshot.resourceKinds);
  await assertNoEnglishResidueOnPage(page, 'the Korean runner images workspace');
  await capture(page, outputDir, 'runner-images-loaded-ko-desktop', { fullPage: true });
}

async function captureDesktop(outputDir) {
  const labels = VIEW_LABELS.en;
  const browser = await chromium.launch({ headless: HEADLESS });
  const context = await browser.newContext({
    viewport: { width: 1600, height: 960 }
  });
  const page = await context.newPage();

  await ensureAuthenticated(page, outputDir);
  await page.reload({ waitUntil: 'domcontentloaded' });
  await page.waitForTimeout(1000);
  await capture(page, outputDir, 'overview-desktop');
  if (CREATE_SAMPLE_POLICY) {
    await ensureSamplePolicy(page);
    await page.goto(BASE_URL, { waitUntil: 'domcontentloaded' });
    await waitForWorkspaceShell(page, 'en');
    await gotoWorkspaceView(page, labels.policies);
  } else {
    await gotoWorkspaceView(page, labels.policies);
  }
  await capture(page, outputDir, 'policies-desktop');

  await ensureSampleRunnerImage(page);
  await page.goto(BASE_URL, { waitUntil: 'domcontentloaded' });
  await waitForWorkspaceShell(page, 'en');

  for (const label of [labels.settings, labels.runners, labels.runnerImages, labels.jobs, labels.events]) {
    await gotoWorkspaceView(page, label);
    const slug = label.toLowerCase().replaceAll(' ', '-');
    await capture(page, outputDir, `${slug}-desktop`, { fullPage: slug === 'runner-images' });
  }

  await captureKoreanLoadedData(page, outputDir);
  await browser.close();
}

async function captureMobile(outputDir) {
  const labels = VIEW_LABELS.en;
  const browser = await chromium.launch({ headless: HEADLESS });
  const context = await browser.newContext({
    viewport: { width: 393, height: 852 },
    isMobile: true,
    hasTouch: true
  });
  const page = await context.newPage();

  await ensureAuthenticated(page, outputDir);
  await page.reload({ waitUntil: 'domcontentloaded' });
  await page.waitForTimeout(1000);
  await capture(page, outputDir, 'overview-mobile');
  await gotoWorkspaceView(page, labels.policies);
  await capture(page, outputDir, 'policies-mobile');
  await ensureSampleRunnerImage(page);
  await page.goto(BASE_URL, { waitUntil: 'domcontentloaded' });
  await waitForWorkspaceShell(page, 'en');
  await gotoWorkspaceView(page, labels.runnerImages);
  await capture(page, outputDir, 'runner-images-mobile', { fullPage: true });

  await browser.close();
}

async function main() {
  const outputDir = await ensureDirectory();
  try {
    await captureDesktop(outputDir);
    await captureMobile(outputDir);
    console.log(outputDir);
  } finally {
    if (githubStub?.server) {
      await new Promise((resolve, reject) => githubStub.server.close((error) => (error ? reject(error) : resolve())));
      githubStub = null;
    }
  }
}

main().catch((error) => {
  console.error(error);
  process.exit(1);
});
