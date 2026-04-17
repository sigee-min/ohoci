import fs from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

import { buildDocsPath, createHeadingIdGenerator, normalizeSearchText, slugifyHeading } from '../src/lib/docs.js';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const DEFAULT_DOCS_DIR = path.resolve(__dirname, '../../docs');
const DEFAULT_OUTPUT_FILE = path.resolve(__dirname, '../src/generated/docs-data.js');
const REQUIRED_FIELDS = ['app_docs', 'access', 'title', 'slug', 'order'];

function parseScalarValue(raw) {
  const value = String(raw || '').trim();
  if (value === 'true') {
    return true;
  }
  if (value === 'false') {
    return false;
  }
  if (/^-?\d+$/.test(value)) {
    return Number(value);
  }
  return value.replace(/^['"]|['"]$/g, '');
}

export function parseFrontmatterDocument(source, filePath = '') {
  const text = String(source || '');
  if (!text.startsWith('---\n')) {
    return { frontmatter: {}, content: text };
  }

  const endIndex = text.indexOf('\n---\n', 4);
  if (endIndex === -1) {
    throw new Error(`Invalid frontmatter in ${filePath || 'document'}`);
  }

  const rawFrontmatter = text.slice(4, endIndex);
  const content = text.slice(endIndex + 5);
  const frontmatter = {};

  for (const rawLine of rawFrontmatter.split('\n')) {
    const line = rawLine.trim();
    if (!line || line.startsWith('#')) {
      continue;
    }
    const separatorIndex = line.indexOf(':');
    if (separatorIndex === -1) {
      throw new Error(`Invalid frontmatter line "${rawLine}" in ${filePath || 'document'}`);
    }
    const key = line.slice(0, separatorIndex).trim();
    const value = line.slice(separatorIndex + 1);
    frontmatter[key] = parseScalarValue(value);
  }

  return { frontmatter, content };
}

function stripMarkdownFormatting(value) {
  return String(value || '')
    .replace(/!\[[^\]]*]\([^)]*\)/g, ' ')
    .replace(/\[([^\]]+)]\([^)]*\)/g, '$1')
    .replace(/`{1,3}([^`]*)`{1,3}/g, '$1')
    .replace(/[*_~>#-]/g, ' ')
    .replace(/\s+/g, ' ')
    .trim();
}

export function extractHeadings(markdown) {
  const headings = [];
  const nextHeadingId = createHeadingIdGenerator();
  let inFence = false;

  for (const rawLine of String(markdown || '').split('\n')) {
    const line = rawLine.trim();
    if (line.startsWith('```')) {
      inFence = !inFence;
      continue;
    }
    if (inFence) {
      continue;
    }
    const match = /^(#{1,6})\s+(.*)$/.exec(line);
    if (!match) {
      continue;
    }
    const text = stripMarkdownFormatting(match[2]);
    if (!text) {
      continue;
    }
    headings.push({
      level: match[1].length,
      text,
      id: nextHeadingId(text)
    });
  }

  return headings;
}

function resolveInternalMarkdownTarget(href, sourcePath) {
  const [pathname, hash = ''] = String(href || '').split('#');
  const normalizedPath = pathname.trim();
  if (!normalizedPath.endsWith('.md')) {
    return null;
  }

  const resolvedPath = path.resolve(path.dirname(sourcePath), normalizedPath);
  return {
    resolvedPath,
    hash: hash.trim()
  };
}

function rewriteInternalLinks(content, sourcePath, docByPath) {
  return String(content || '').replace(/\[([^\]]+)]\(([^)]+)\)/g, (match, label, rawHref) => {
    const href = String(rawHref || '').trim();
    if (!href || href.startsWith('#') || /^[a-z]+:/i.test(href)) {
      return match;
    }

    const target = resolveInternalMarkdownTarget(href, sourcePath);
    if (!target) {
      return match;
    }

    const targetDoc = docByPath.get(target.resolvedPath);
    if (!targetDoc) {
      throw new Error(`Doc link ${href} in ${path.basename(sourcePath)} points to an unflagged or missing markdown file`);
    }

    const rewrittenHash = resolveHeadingHash(targetDoc, target.hash);
    const rewrittenHref = `${buildDocsPath(targetDoc.slug)}${rewrittenHash ? `#${rewrittenHash}` : ''}`;
    return `[${label}](${rewrittenHref})`;
  });
}

function resolveHeadingHash(targetDoc, rawHash) {
  const normalizedHash = String(rawHash || '').trim().replace(/^#/, '');
  if (!normalizedHash) {
    return '';
  }

  const exactHeading = targetDoc.headings?.find((heading) => heading.id === normalizedHash);
  if (exactHeading) {
    return exactHeading.id;
  }

  const normalizedId = slugifyHeading(normalizedHash);
  const matchingHeading = targetDoc.headings?.find((heading) => heading.id === normalizedId);
  return matchingHeading?.id || normalizedId;
}

function resolveSectionMetadata(rawSection) {
  const section = String(rawSection || 'Guides').trim() || 'Guides';
  return {
    section,
    sectionKey: `docs.section.${slugifyHeading(section)}`
  };
}

function toDocRecord(filePath, frontmatter, content) {
  if (frontmatter.app_docs !== true) {
    return null;
  }

  for (const field of REQUIRED_FIELDS) {
    if (frontmatter[field] == null || frontmatter[field] === '') {
      throw new Error(`Missing required frontmatter "${field}" in ${path.basename(filePath)}`);
    }
  }
  if (frontmatter.access !== 'public') {
    throw new Error(`Unsupported access "${frontmatter.access}" in ${path.basename(filePath)}. Only "public" is supported in v1.`);
  }

  return {
    filePath,
    fileName: path.basename(filePath),
    access: 'public',
    title: String(frontmatter.title).trim(),
    slug: String(frontmatter.slug).trim(),
    order: Number(frontmatter.order),
    ...resolveSectionMetadata(frontmatter.section),
    summary: String(frontmatter.summary || '').trim(),
    content: String(content || '')
  };
}

export async function collectAppDocs({ docsDir = DEFAULT_DOCS_DIR } = {}) {
  const entries = await fs.readdir(docsDir, { withFileTypes: true });
  const rawDocs = [];

  for (const entry of entries) {
    if (!entry.isFile() || !entry.name.endsWith('.md')) {
      continue;
    }
    const filePath = path.join(docsDir, entry.name);
    const source = await fs.readFile(filePath, 'utf8');
    const { frontmatter, content } = parseFrontmatterDocument(source, filePath);
    const doc = toDocRecord(filePath, frontmatter, content);
    if (doc) {
      rawDocs.push(doc);
    }
  }

  if (!rawDocs.length) {
    throw new Error(`No app docs were found in ${docsDir}`);
  }

  const slugSet = new Set();
  const docByPath = new Map();
  for (const doc of rawDocs) {
    if (slugSet.has(doc.slug)) {
      throw new Error(`Duplicate doc slug "${doc.slug}"`);
    }
    slugSet.add(doc.slug);
    doc.headings = extractHeadings(doc.content);
    docByPath.set(doc.filePath, doc);
  }

  const docs = rawDocs.map((doc) => {
    const content = rewriteInternalLinks(doc.content, doc.filePath, docByPath);
    const headings = doc.headings;
    const searchText = normalizeSearchText([
      doc.title,
      doc.summary,
      headings.map((heading) => heading.text).join(' '),
      stripMarkdownFormatting(content)
    ].join(' '));

    return {
      slug: doc.slug,
      title: doc.title,
      order: doc.order,
      section: doc.section,
      sectionKey: doc.sectionKey,
      summary: doc.summary,
      access: doc.access,
      content,
      headings,
      searchText
    };
  });

  docs.sort((left, right) => {
    if (left.order !== right.order) {
      return left.order - right.order;
    }
    return left.title.localeCompare(right.title);
  });

  return docs;
}

export function buildGeneratedModuleSource(docs) {
  return `// This file is auto-generated by web/scripts/generate-docs.mjs\nexport const APP_DOCS = ${JSON.stringify(docs, null, 2)};\n`;
}

export async function generateDocsModule({ docsDir = DEFAULT_DOCS_DIR, outputFile = DEFAULT_OUTPUT_FILE } = {}) {
  const docs = await collectAppDocs({ docsDir });
  const moduleSource = buildGeneratedModuleSource(docs);
  await fs.mkdir(path.dirname(outputFile), { recursive: true });
  await fs.writeFile(outputFile, moduleSource, 'utf8');
  return docs;
}

async function main() {
  await generateDocsModule();
}

if (import.meta.url === `file://${__filename}`) {
  main().catch((error) => {
    console.error(error instanceof Error ? error.message : error);
    process.exitCode = 1;
  });
}
