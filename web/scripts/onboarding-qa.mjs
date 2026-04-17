import { mkdir } from 'node:fs/promises';
import path from 'node:path';

import { chromium } from 'playwright';

const BASE_URL = process.env.OHOCI_ONBOARDING_BASE_URL || 'http://127.0.0.1:5173';
const API_BASE_URL = process.env.OHOCI_ONBOARDING_API_BASE_URL || 'http://127.0.0.1:8080';
const SCREENSHOT_ROOT = path.resolve(process.cwd(), 'artifacts', 'onboarding-qa');
const TARGET_PASSWORD = process.env.OHOCI_ONBOARDING_PASSWORD || 'Admin12345';
const VIEW = process.env.OHOCI_ONBOARDING_VIEW || 'desktop';
const NEXT_STEP = process.env.OHOCI_ONBOARDING_NEXT_STEP || 'github';

async function ensureDirectory(label) {
  const stamp = new Date().toISOString().replaceAll(':', '-');
  const targetDir = path.join(SCREENSHOT_ROOT, `${stamp}-${label}`);
  await mkdir(targetDir, { recursive: true });
  return targetDir;
}

async function captureSetupScenario(outputDir, options) {
  const browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({
    viewport: options.viewport,
    isMobile: options.mobile,
    hasTouch: options.mobile
  });
  const page = await context.newPage();

  console.log(`[qa] loading ${options.name}`);
  const response = await context.request.post(`${API_BASE_URL}/api/v1/auth/login`, {
    data: {
      username: 'admin',
      password: 'admin'
    }
  });
  if (!response.ok()) {
    throw new Error(`[qa] login failed for ${options.name}: ${response.status()} ${await response.text()}`);
  }
  await page.goto(BASE_URL, { waitUntil: 'domcontentloaded' });
  console.log(`[qa] wait password step ${options.name}`);
  await page.getByLabel('Current password').waitFor({ state: 'visible' });
  await page.waitForTimeout(500);
  await page.screenshot({
    path: path.join(outputDir, `password-${options.name}.png`),
    fullPage: true,
    animations: 'disabled'
  });

  console.log(`[qa] submit password ${options.name}`);
  await page.getByLabel('Current password').fill('admin');
  await page.getByLabel('New password').fill(TARGET_PASSWORD);
  await page.getByRole('button', { name: 'Save password' }).click({ noWaitAfter: true });
  console.log(`[qa] wait ${NEXT_STEP} step ${options.name}`);
  if (NEXT_STEP === 'oci') {
    await page.getByRole('tab', { name: 'Credential' }).waitFor({ state: 'visible' });
  } else {
    await page.getByLabel('Personal access token').waitFor({ state: 'visible' });
  }
  await page.waitForTimeout(700);
  await page.screenshot({
    path: path.join(outputDir, `${NEXT_STEP}-${options.name}.png`),
    fullPage: true,
    animations: 'disabled'
  });

  await browser.close();
}

async function main() {
  const outputDir = await ensureDirectory(`scenario-a-${VIEW}`);
  const options = VIEW === 'mobile'
    ? {
        name: 'mobile',
        viewport: { width: 393, height: 852 },
        mobile: true
      }
    : {
        name: 'desktop',
        viewport: { width: 1440, height: 1200 },
        mobile: false
      };

  await captureSetupScenario(outputDir, options);
  console.log(outputDir);
}

main().catch((error) => {
  console.error(error);
  process.exit(1);
});
