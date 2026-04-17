import { createContext, useContext, useEffect, useState } from 'react';

import {
  DEFAULT_LOCALE,
  LOCALE_MESSAGES,
  LOCALE_OPTIONS,
  LOCALE_STORAGE_KEY,
  SUPPORTED_LOCALES
} from '@/i18n/messages';

let currentLocale = DEFAULT_LOCALE;

function normalizeLocale(value) {
  return SUPPORTED_LOCALES.includes(value) ? value : DEFAULT_LOCALE;
}

function readStoredLocale() {
  if (typeof window === 'undefined') {
    return DEFAULT_LOCALE;
  }

  try {
    return normalizeLocale(window.localStorage.getItem(LOCALE_STORAGE_KEY));
  } catch {
    return DEFAULT_LOCALE;
  }
}

function resolveLocale(locale = currentLocale) {
  return normalizeLocale(locale);
}

function resolveMessage(template, params = {}, locale) {
  if (typeof template === 'function') {
    return String(template(params, locale));
  }

  return String(template).replace(/\{(\w+)\}/g, (_, token) => {
    return params[token] == null ? '' : String(params[token]);
  });
}

function lookupMessage(key, locale) {
  const normalizedKey = String(key || '').trim();
  if (!normalizedKey) {
    return null;
  }

  const requestedLocale = normalizeLocale(locale);
  return LOCALE_MESSAGES[requestedLocale]?.[normalizedKey] ?? LOCALE_MESSAGES[DEFAULT_LOCALE]?.[normalizedKey] ?? null;
}

export function getCurrentLocale() {
  return currentLocale;
}

export function hasTranslation(key, locale) {
  return lookupMessage(key, resolveLocale(locale)) != null;
}

export function translate(key, params = {}, locale) {
  const resolvedLocale = resolveLocale(locale);
  const message = lookupMessage(key, resolvedLocale);
  if (message == null) {
    return String(key || '');
  }

  return resolveMessage(message, params, resolvedLocale);
}

export function translateMaybeKey(value, params = {}, locale) {
  const resolvedLocale = resolveLocale(locale);
  const normalizedValue = String(value || '').trim();
  if (!normalizedValue) {
    return '';
  }

  return hasTranslation(normalizedValue, resolvedLocale)
    ? translate(normalizedValue, params, resolvedLocale)
    : normalizedValue;
}

export function formatTranslatedList(values, locale) {
  const resolvedLocale = resolveLocale(locale);
  return values.map((value) => translateMaybeKey(value, {}, resolvedLocale)).filter(Boolean).join(', ');
}

const I18nContext = createContext({
  locale: DEFAULT_LOCALE,
  setLocale: () => {},
  t: (key, params) => translate(key, params, DEFAULT_LOCALE)
});

export function I18nProvider({ children }) {
  const [locale, setLocale] = useState(readStoredLocale);
  const normalizedLocale = normalizeLocale(locale);

  currentLocale = normalizedLocale;

  useEffect(() => {
    if (typeof document !== 'undefined') {
      document.documentElement.lang = normalizedLocale;
    }

    if (typeof window !== 'undefined') {
      try {
        window.localStorage.setItem(LOCALE_STORAGE_KEY, normalizedLocale);
      } catch {
        // Ignore storage write failures and keep the in-memory locale.
      }
    }
  }, [normalizedLocale]);

  return (
    <I18nContext.Provider
      value={{
        locale: normalizedLocale,
        setLocale: (nextLocale) => setLocale(normalizeLocale(nextLocale)),
        t: (key, params) => translate(key, params, normalizedLocale)
      }}
    >
      {children}
    </I18nContext.Provider>
  );
}

export function useI18n() {
  return useContext(I18nContext);
}

export { DEFAULT_LOCALE, LOCALE_OPTIONS, SUPPORTED_LOCALES };
