import fs from 'node:fs/promises';
import path from 'node:path';

import { chromium } from 'playwright';

const TARGET_URL = process.env.SETUP_QA_URL || 'http://127.0.0.1:5174';
const OUTPUT_DIR = process.env.SETUP_QA_OUTPUT_DIR
  || path.resolve('artifacts/setup-qa/manual');
const BOOTSTRAP_PASSWORD = process.env.SETUP_QA_BOOTSTRAP_PASSWORD || 'admin';
const NEXT_PASSWORD = process.env.SETUP_QA_NEXT_PASSWORD || 'Admin12345!';

async function ensureSettingsView(page, mobile = false) {
  if (mobile) {
    const toggle = page.locator('[data-slot="sidebar-trigger"]').first();
    if (await toggle.count()) {
      await toggle.dispatchEvent('click');
      await page.locator('[data-mobile="true"]').waitFor({ state: 'visible' });
    }
  }

  const settingsSurface = mobile ? page.locator('[data-mobile="true"]') : page;
  const settingsButton = settingsSurface
    .getByRole('button', { name: /^Settings$/i })
    .or(settingsSurface.getByRole('link', { name: /^Settings$/i }));

  await settingsButton.first().click();
  if (mobile) {
    await page.locator('[data-mobile="true"]').waitFor({ state: 'hidden' });
  }
  await page.getByText('Setup overview').waitFor({ state: 'visible' });
}

async function loginAndPrepare(page, passwordValue) {
  await page.goto(TARGET_URL, { waitUntil: 'networkidle' });
  const usernameField = page.getByLabel('Username');
  const passwordField = page.getByLabel('Password');
  const submitButton = page.getByRole('button', { name: /Enter workspace/i });
  await usernameField.fill('admin');
  await passwordField.fill(passwordValue);
  await submitButton.click();
  await page.waitForLoadState('networkidle');
  await page.waitForTimeout(800);

  const currentPasswordField = page.getByLabel('Current password');
  if (await currentPasswordField.count()) {
    await currentPasswordField.fill(BOOTSTRAP_PASSWORD);
    await page.getByLabel('New password').fill(NEXT_PASSWORD);
    await page.getByRole('button', { name: /Save password/i }).click();
    await page.waitForLoadState('networkidle');
    await page.waitForTimeout(1200);
    return NEXT_PASSWORD;
  }

  return passwordValue;
}

async function captureScenario(name, currentPassword, contextOptions = {}) {
  const browser = await chromium.launch({ headless: true });
  const context = await browser.newContext(contextOptions);
  const page = await context.newPage();

  try {
    const nextPassword = await loginAndPrepare(page, currentPassword);
    await ensureSettingsView(page, Boolean(contextOptions.isMobile));

    const accordionTriggers = page.locator('[data-slot="accordion-trigger"]');
    const snapshot = {
      name,
      url: page.url(),
      overviewVisible: await page.getByText('Setup overview').first().isVisible(),
      triggerCount: await accordionTriggers.count(),
      firstExpanded: await accordionTriggers.nth(0).getAttribute('aria-expanded'),
      secondExpanded: await accordionTriggers.nth(1).getAttribute('aria-expanded')
    };

    console.log(JSON.stringify(snapshot, null, 2));
    await page.evaluate(() => window.scrollTo({ top: 0, behavior: 'instant' }));
    await page.waitForTimeout(200);
    await page.screenshot({
      path: path.join(OUTPUT_DIR, `${name}.png`),
      fullPage: true
    });
    return nextPassword;
  } catch (error) {
    console.error(`capture failed for ${name}:`, error);
    console.error((await page.locator('body').innerText()).slice(0, 2500));
    await page.screenshot({
      path: path.join(OUTPUT_DIR, `${name}-failure.png`),
      fullPage: true
    });
    throw error;
  } finally {
    await browser.close();
  }
}

async function main() {
  await fs.mkdir(OUTPUT_DIR, { recursive: true });
  let currentPassword = BOOTSTRAP_PASSWORD;
  currentPassword = await captureScenario('setup-desktop', currentPassword, {
    viewport: { width: 1512, height: 982 }
  });
  await captureScenario('setup-mobile', currentPassword, {
    viewport: { width: 390, height: 844 },
    isMobile: true,
    hasTouch: true
  });
}

main().catch((error) => {
  console.error(error);
  process.exit(1);
});
