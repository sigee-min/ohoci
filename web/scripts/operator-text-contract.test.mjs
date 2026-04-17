import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';

import { LOCALE_MESSAGES } from '../src/i18n/messages.js';

const WEB_ROOT = path.resolve(import.meta.dirname, '..');
const BILLING_SERVICE_PATH = path.resolve(WEB_ROOT, '../internal/ocibilling/service.go');
const OPERATOR_TEXT_PATH = path.resolve(WEB_ROOT, 'src/lib/operator-text.js');
const BILLING_SCOPE_NOTE_KEY = 'operator.billing.scopeNote.default';

function normalizeLookupValue(value) {
  return String(value || '')
    .trim()
    .toLowerCase()
    .replace(/\s+/g, ' ');
}

function escapeForRegExp(value) {
  return String(value || '').replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}

test('billing scope note backend literal stays mapped to the localized frontend key', async () => {
  const [billingServiceSource, operatorTextSource] = await Promise.all([
    fs.readFile(BILLING_SERVICE_PATH, 'utf8'),
    fs.readFile(OPERATOR_TEXT_PATH, 'utf8')
  ]);

  const defaultScopeNoteMatch = billingServiceSource.match(/defaultScopeNote\s*=\s*"([^"]+)"/);
  assert.ok(defaultScopeNoteMatch, 'Could not find defaultScopeNote literal in billing service.');

  const lookupValue = normalizeLookupValue(defaultScopeNoteMatch[1]);
  const mappingPattern = new RegExp(
    `['"]${escapeForRegExp(lookupValue)}['"]\\s*:\\s*['"]${escapeForRegExp(BILLING_SCOPE_NOTE_KEY)}['"]`
  );

  assert.match(
    operatorTextSource,
    mappingPattern,
    'Billing scope note literal is not mapped to the localized frontend key.'
  );
  assert.equal(typeof LOCALE_MESSAGES.en[BILLING_SCOPE_NOTE_KEY], 'string');
  assert.notEqual(LOCALE_MESSAGES.en[BILLING_SCOPE_NOTE_KEY].trim(), '');
  assert.equal(typeof LOCALE_MESSAGES.ko[BILLING_SCOPE_NOTE_KEY], 'string');
  assert.notEqual(LOCALE_MESSAGES.ko[BILLING_SCOPE_NOTE_KEY].trim(), '');
});
