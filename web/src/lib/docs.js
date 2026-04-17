export const DOCS_BASE_PATH = '/docs';

export function normalizeSearchText(value) {
  return String(value || '')
    .toLowerCase()
    .replace(/[`*_>#~[\]()]/g, ' ')
    .replace(/\s+/g, ' ')
    .trim();
}

export function slugifyHeading(value) {
  const normalized = String(value || '')
    .normalize('NFKD')
    .replace(/[\u0300-\u036f]/g, '')
    .toLowerCase()
    .replace(/[^a-z0-9\s-]/g, ' ')
    .replace(/\s+/g, ' ')
    .trim()
    .replace(/\s+/g, '-')
    .replace(/-+/g, '-');

  return normalized || 'section';
}

export function createHeadingIdGenerator() {
  const seen = new Map();

  return (value) => {
    const base = slugifyHeading(value);
    const current = seen.get(base) || 0;
    seen.set(base, current + 1);
    return current === 0 ? base : `${base}-${current + 1}`;
  };
}

export function buildDocsPath(slug = '') {
  const normalized = String(slug || '').trim().replace(/^\/+|\/+$/g, '');
  return normalized ? `${DOCS_BASE_PATH}/${normalized}` : DOCS_BASE_PATH;
}

export function buildDocsHref(slug = '', headingId = '') {
  const path = buildDocsPath(slug);
  const normalizedHeadingId = String(headingId || '').trim().replace(/^#+/, '');
  return normalizedHeadingId ? `${path}#${normalizedHeadingId}` : path;
}

export function parseDocsPath(pathname) {
  const normalized = String(pathname || '').trim();
  if (!normalized.startsWith(DOCS_BASE_PATH)) {
    return { isDocsRoute: false, slug: '' };
  }

  const suffix = normalized.slice(DOCS_BASE_PATH.length).replace(/^\/+|\/+$/g, '');
  return {
    isDocsRoute: true,
    slug: suffix || ''
  };
}

export function parseDocsHref(href) {
  const normalized = String(href || '').trim();
  if (!normalized.startsWith(DOCS_BASE_PATH)) {
    return { slug: '', headingId: '' };
  }

  const [pathname, hash = ''] = normalized.split('#');
  const route = parseDocsPath(pathname);
  return {
    slug: route.slug,
    headingId: hash.trim()
  };
}
