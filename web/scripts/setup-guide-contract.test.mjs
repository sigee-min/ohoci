import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';

const WEB_ROOT = path.resolve(import.meta.dirname, '..');
const README_PATH = path.resolve(WEB_ROOT, '..', 'README.md');
const SETUP_GUIDE_PATH = path.resolve(WEB_ROOT, '..', 'docs', 'setup-guide.md');

function escapeForRegExp(value) {
  return String(value || '').replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}

test('setup guide documents the five-task setup flow and keeps settings-only concepts out of first run', async () => {
  const guide = await fs.readFile(SETUP_GUIDE_PATH, 'utf8');

  const requiredSections = [
    '## Task 1: Change the admin password',
    '## Task 2: Connect the GitHub App',
    '## Task 3: Save the OCI credential',
    '## Task 4: Choose repositories',
    '## Task 5: Save the OCI launch target',
    '[Revisit Settings after setup](./getting-started.md#revisit-settings-after-setup)'
  ];
  for (const section of requiredSections) {
    assert.match(guide, new RegExp(escapeForRegExp(section)));
  }

  const bannedPhrases = [
    'Active apps',
    'GitHub drift',
    'cache compatibility',
    'NSG'
  ];
  for (const phrase of bannedPhrases) {
    assert.doesNotMatch(guide, new RegExp(escapeForRegExp(phrase), 'i'));
  }
});

test('README setup summary matches the same five-task first-run contract', async () => {
  const readme = await fs.readFile(README_PATH, 'utf8');

  const requiredPhrases = [
    '1. Change the bootstrap admin password.',
    '2. Verify and save the GitHub App route.',
    '3. Save one OCI credential.',
    '4. Choose at least one repository.',
    '5. Save the OCI launch target.',
    'Advanced GitHub and OCI operations stay in Settings after setup is complete.'
  ];
  for (const phrase of requiredPhrases) {
    assert.match(readme, new RegExp(escapeForRegExp(phrase)));
  }
});
