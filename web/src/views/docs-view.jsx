import { useDeferredValue, useEffect, useMemo, useRef, useState } from 'react';
import { ArrowLeftIcon, ArrowRightIcon, BookOpenTextIcon, FileTextIcon, ListIcon, SearchIcon } from 'lucide-react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';

import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
  SheetTrigger
} from '@/components/ui/sheet';
import { cn } from '@/lib/utils';
import {
  buildDocsPath,
  createHeadingIdGenerator,
  normalizeSearchText,
  parseDocsHref,
  slugifyHeading
} from '@/lib/docs';
import { translateMaybeKey, useI18n } from '@/i18n';

const DOCS_SECTION_META = {
  'docs.section.start-here': {
    descriptionKey: 'docs.sectionDescription.start-here'
  },
  'docs.section.configure': {
    descriptionKey: 'docs.sectionDescription.configure'
  },
  'docs.section.operate': {
    descriptionKey: 'docs.sectionDescription.operate'
  },
  'docs.section.diagnose': {
    descriptionKey: 'docs.sectionDescription.diagnose'
  },
  'docs.section.guides': {
    descriptionKey: 'docs.sectionDescription.guides'
  }
};

function flattenReactText(children) {
  return Array.isArray(children)
    ? children.map(flattenReactText).join('')
    : typeof children === 'string' || typeof children === 'number'
      ? String(children)
      : children?.props?.children
        ? flattenReactText(children.props.children)
        : '';
}

function resolveDocSectionKey(doc) {
  const sectionKey = String(doc?.sectionKey || '').trim();
  if (sectionKey) {
    return sectionKey;
  }

  const section = String(doc?.section || '').trim();
  return section ? `docs.section.${slugifyHeading(section)}` : 'docs.section.guides';
}

function resolveDocSectionLabel(doc, t, locale) {
  const sectionKey = resolveDocSectionKey(doc);
  const translated = translateMaybeKey(sectionKey, {}, locale);
  if (translated !== sectionKey) {
    return translated;
  }

  const section = String(doc?.section || '').trim();
  return section || t('docs.section.guides');
}

function resolveDocSectionMeta(doc, t, locale) {
  const sectionKey = resolveDocSectionKey(doc);
  const metadata = DOCS_SECTION_META[sectionKey] || DOCS_SECTION_META['docs.section.guides'];
  return {
    key: sectionKey,
    label: resolveDocSectionLabel(doc, t, locale),
    description: t(metadata.descriptionKey)
  };
}

function groupDocsBySection(docs) {
  const sections = [];
  const sectionMap = new Map();

  for (const doc of docs) {
    const sectionKey = resolveDocSectionKey(doc);
    let section = sectionMap.get(sectionKey);
    if (!section) {
      section = {
        key: sectionKey,
        fallbackLabel: String(doc.section || '').trim(),
        items: []
      };
      sectionMap.set(sectionKey, section);
      sections.push(section);
    }
    section.items.push(doc);
  }

  return sections;
}

function buildDocSearchIndex(doc) {
  return normalizeSearchText([
    doc.title,
    doc.summary,
    doc.searchText,
    doc.headings.map((heading) => heading.text).join(' ')
  ].join(' '));
}

function DocsRail({
  docs,
  selectedDoc,
  searchQuery,
  setSearchQuery,
  onSelectDoc,
  closeMobile
}) {
  const { locale, t } = useI18n();
  const deferredSearchQuery = useDeferredValue(searchQuery);
  const filteredDocs = useMemo(() => {
    const query = normalizeSearchText(deferredSearchQuery);
    if (!query) {
      return docs;
    }

    return docs.filter((doc) => buildDocSearchIndex(doc).includes(query));
  }, [deferredSearchQuery, docs]);

  const groupedDocs = useMemo(() => groupDocsBySection(filteredDocs), [filteredDocs]);

  return (
    <div className="flex h-full min-h-0 flex-col gap-4">
      <div className="shrink-0 space-y-3">
        <div className="flex items-center gap-2 rounded-xl border bg-background/80 px-3 py-2.5">
          <SearchIcon className="size-4 text-muted-foreground" />
          <Input
            value={searchQuery}
            onChange={(event) => setSearchQuery(event.target.value)}
            placeholder={t('docs.searchPlaceholder')}
            className="h-auto border-0 bg-transparent px-0 py-0 shadow-none focus-visible:ring-0"
          />
        </div>
        <p className="text-xs text-muted-foreground">
          {t('docs.documentCount', { count: filteredDocs.length })}
        </p>
      </div>

      <div className="flex min-h-0 flex-1 flex-col gap-4 overflow-y-auto pr-1 overscroll-contain">
        <div className="flex flex-col gap-4">
          {groupedDocs.map((section) => (
            <section key={section.key} className="rounded-2xl border bg-background/55">
              <div className="border-b px-4 py-3">
                <div className="flex items-center justify-between gap-3">
                  <p className="text-xs font-semibold uppercase tracking-[0.14em] text-muted-foreground">
                    {resolveDocSectionMeta({ sectionKey: section.key, section: section.fallbackLabel }, t, locale).label}
                  </p>
                  <span className="text-xs text-muted-foreground">{t('docs.documentCount', { count: section.items.length })}</span>
                </div>
              </div>
              <div className="flex flex-col gap-1 p-2">
                {section.items.map((doc, index) => (
                  <button
                    key={doc.slug}
                    type="button"
                    className={cn(
                      'rounded-xl border px-3 py-3 text-left transition-colors',
                      selectedDoc?.slug === doc.slug
                        ? 'border-foreground/15 bg-accent/55 shadow-sm'
                        : 'border-transparent bg-transparent hover:bg-muted/35'
                    )}
                    onClick={() => {
                      onSelectDoc(doc.slug);
                      closeMobile?.();
                    }}
                  >
                    <div className="flex items-start gap-3">
                      <div className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-background text-xs font-semibold text-muted-foreground ring-1 ring-border/70">
                        {String(index + 1).padStart(2, '0')}
                      </div>
                      <div className="min-w-0 flex-1">
                        <p className="text-sm font-medium">{doc.title}</p>
                      </div>
                      {selectedDoc?.slug === doc.slug ? <Badge variant="secondary">{t('docs.openBadge')}</Badge> : null}
                    </div>
                  </button>
                ))}
              </div>
            </section>
          ))}
        </div>
      </div>
    </div>
  );
}

function DocsGuideTools({
  selectedDoc,
  selectedDocIndex,
  docCount,
  activeHeadingId,
  onSelectHeading,
  previousDoc,
  nextDoc,
  onSelectDoc
}) {
  const { locale, t } = useI18n();
  const sectionMeta = resolveDocSectionMeta(selectedDoc, t, locale);
  const guideContext = selectedDoc.summary || sectionMeta.description;

  return (
    <Card className="border bg-card/95">
      <CardHeader className="border-b pb-4">
        <div className="space-y-3">
          <div className="flex flex-wrap items-center gap-2">
            <Badge variant="secondary">{sectionMeta.label}</Badge>
            <span className="text-xs font-semibold uppercase tracking-[0.14em] text-muted-foreground">
              {t('docs.documentProgress', { current: selectedDocIndex + 1, total: docCount })}
            </span>
          </div>
          <div className="space-y-1">
            <CardTitle className="text-base">{t('docs.toolsTitle')}</CardTitle>
            <CardDescription>{t('docs.toolsDescription')}</CardDescription>
          </div>
        </div>
      </CardHeader>
      <CardContent className="flex flex-col gap-5 pt-5">
        <div className="rounded-2xl border bg-muted/20 px-4 py-3">
          <p className="text-xs font-semibold uppercase tracking-[0.14em] text-muted-foreground">{t('docs.currentGuide')}</p>
          <p className="mt-2 text-sm font-semibold text-foreground">{selectedDoc.title}</p>
          {guideContext ? <p className="mt-1 text-sm leading-5 text-muted-foreground">{guideContext}</p> : null}
        </div>

        {selectedDoc?.headings?.length ? (
          <div className="flex flex-col gap-2 border-t pt-5">
            <p className="text-xs font-semibold uppercase tracking-[0.14em] text-muted-foreground">{t('docs.onThisPage')}</p>
            <div className="flex flex-col gap-1">
              {selectedDoc.headings.map((heading) => (
                <button
                  key={heading.id}
                  type="button"
                  className={cn(
                    'rounded-xl px-3 py-2 text-left text-sm leading-5 transition-colors hover:bg-muted/40',
                    heading.level > 1 && 'ml-3',
                    heading.level > 2 && 'ml-6',
                    activeHeadingId === heading.id && 'bg-accent/55 font-medium text-foreground'
                  )}
                  onClick={() => onSelectHeading(heading.id)}
                >
                  {heading.text}
                </button>
              ))}
            </div>
          </div>
        ) : null}

        <div className={cn('flex flex-col gap-2', selectedDoc?.headings?.length && 'border-t pt-5')}>
          <div>
            <p className="text-xs font-semibold uppercase tracking-[0.14em] text-muted-foreground">{t('docs.readingPathTitle')}</p>
            <p className="mt-1 text-sm text-muted-foreground">{t('docs.readingPathDescription')}</p>
          </div>
          <div className="flex flex-col gap-2">
            {previousDoc ? (
              <Button
                type="button"
                variant="ghost"
                className="h-auto justify-start rounded-xl border px-3 py-3 text-left"
                onClick={() => onSelectDoc(previousDoc.slug, '')}
              >
                <ArrowLeftIcon data-icon="inline-start" />
                <span className="flex min-w-0 flex-col items-start gap-0.5">
                  <span className="text-xs uppercase tracking-[0.12em] text-muted-foreground">{t('docs.previousDoc')}</span>
                  <span className="truncate text-sm font-medium">{previousDoc.title}</span>
                </span>
              </Button>
            ) : null}
            {nextDoc ? (
              <Button
                type="button"
                variant="ghost"
                className="h-auto justify-start rounded-xl border px-3 py-3 text-left"
                onClick={() => onSelectDoc(nextDoc.slug, '')}
              >
                <ArrowRightIcon data-icon="inline-start" />
                <span className="flex min-w-0 flex-col items-start gap-0.5">
                  <span className="text-xs uppercase tracking-[0.12em] text-muted-foreground">{t('docs.nextDoc')}</span>
                  <span className="truncate text-sm font-medium">{nextDoc.title}</span>
                </span>
              </Button>
            ) : null}
          </div>
        </div>
      </CardContent>
    </Card>
  );
}

function DocsArticle({ doc, docs, onNavigateDoc, initialHeadingId, onHeadingChange }) {
  const { locale, t } = useI18n();
  const articleRef = useRef(null);
  const nextHeadingId = createHeadingIdGenerator();
  const sectionMeta = resolveDocSectionMeta(doc, t, locale);
  const docIndex = docs.findIndex((entry) => entry.slug === doc.slug);
  const leadText = doc.summary || sectionMeta.description;

  useEffect(() => {
    const articleElement = articleRef.current;
    if (!articleElement) {
      return undefined;
    }

    const headings = Array.from(articleElement.querySelectorAll('h1[id], h2[id], h3[id], h4[id], h5[id], h6[id]'));
    if (!headings.length) {
      onHeadingChange('');
      return undefined;
    }

    const observer = new IntersectionObserver(
      (entries) => {
        const visible = entries
          .filter((entry) => entry.isIntersecting)
          .sort((left, right) => left.boundingClientRect.top - right.boundingClientRect.top);
        if (visible[0]) {
          onHeadingChange(visible[0].target.id);
        }
      },
      {
        rootMargin: '-18% 0px -60% 0px',
        threshold: [0, 0.25, 0.5, 1]
      }
    );

    headings.forEach((heading) => observer.observe(heading));
    onHeadingChange(headings[0].id);

    return () => observer.disconnect();
  }, [doc.slug, onHeadingChange]);

  useEffect(() => {
    const scrollTargetId = String(initialHeadingId || '').trim();
    if (!scrollTargetId) {
      articleRef.current?.scrollIntoView({ block: 'start' });
      return;
    }

    const scrollToHeading = () => {
      const target = document.getElementById(scrollTargetId);
      if (target) {
        target.scrollIntoView({ block: 'start' });
        onHeadingChange(scrollTargetId);
      }
    };

    const frame = window.requestAnimationFrame(scrollToHeading);
    return () => window.cancelAnimationFrame(frame);
  }, [doc.slug, initialHeadingId, onHeadingChange]);

  function renderHeading(Tag, className) {
    return function HeadingComponent({ children, ...props }) {
      const text = flattenReactText(children);
      const id = nextHeadingId(text);
      return (
        <Tag id={id} className={className} {...props}>
          {children}
        </Tag>
      );
    };
  }

  return (
    <div className="flex flex-col gap-4">
      <div className="rounded-2xl border border-dashed bg-muted/18 px-5 py-4 sm:px-6">
        <div className="space-y-3">
          <div className="flex flex-wrap items-center gap-2">
            <Badge variant="secondary">{sectionMeta.label}</Badge>
            <span className="text-xs font-semibold uppercase tracking-[0.14em] text-muted-foreground">
              {t('docs.documentProgress', { current: docIndex + 1, total: docs.length })}
            </span>
          </div>
          {leadText ? <p className="max-w-3xl text-sm leading-6 text-muted-foreground">{leadText}</p> : null}
        </div>
      </div>

      <div className="rounded-[28px] border bg-card/98 px-5 py-6 shadow-sm sm:px-8">
        <article ref={articleRef} className="docs-prose">
          <ReactMarkdown
            remarkPlugins={[remarkGfm]}
            components={{
              h1: renderHeading('h1', 'docs-heading docs-heading-1'),
              h2: renderHeading('h2', 'docs-heading docs-heading-2'),
              h3: renderHeading('h3', 'docs-heading docs-heading-3'),
              h4: renderHeading('h4', 'docs-heading docs-heading-4'),
              h5: renderHeading('h5', 'docs-heading docs-heading-5'),
              h6: renderHeading('h6', 'docs-heading docs-heading-6'),
              a: ({ href, children, ...props }) => {
                const { slug, headingId } = parseDocsHref(href);
                if (slug) {
                  return (
                    <a
                      href={href}
                      {...props}
                      onClick={(event) => {
                        event.preventDefault();
                        onNavigateDoc(slug, headingId);
                      }}
                    >
                      {children}
                    </a>
                  );
                }

                const external = /^https?:\/\//i.test(String(href || ''));
                return (
                  <a href={href} {...props} target={external ? '_blank' : undefined} rel={external ? 'noreferrer' : undefined}>
                    {children}
                  </a>
                );
              }
            }}
          >
            {doc.content}
          </ReactMarkdown>
        </article>
      </div>
    </div>
  );
}

export function DocsView({ docs, selectedSlug, onSelectDoc, initialHeadingId = '', publicMode = false }) {
  const { t } = useI18n();
  const [searchQuery, setSearchQuery] = useState('');
  const [activeHeadingId, setActiveHeadingId] = useState(initialHeadingId);
  const [pendingHeadingId, setPendingHeadingId] = useState(initialHeadingId);
  const [mobileOpen, setMobileOpen] = useState(false);

  const selectedDoc = useMemo(() => {
    return docs.find((doc) => doc.slug === selectedSlug) || docs[0] || null;
  }, [docs, selectedSlug]);
  const selectedDocIndex = useMemo(() => docs.findIndex((doc) => doc.slug === selectedDoc?.slug), [docs, selectedDoc?.slug]);
  const previousDoc = selectedDocIndex > 0 ? docs[selectedDocIndex - 1] : null;
  const nextDoc = selectedDocIndex >= 0 && selectedDocIndex < docs.length - 1 ? docs[selectedDocIndex + 1] : null;

  useEffect(() => {
    setPendingHeadingId(initialHeadingId);
    if (initialHeadingId) {
      setActiveHeadingId(initialHeadingId);
    }
  }, [initialHeadingId, selectedDoc?.slug]);

  if (!selectedDoc) {
    return (
      <Card className="border bg-card/95">
        <CardHeader>
          <CardTitle>{t('docs.noDocsTitle')}</CardTitle>
          <CardDescription>{t('docs.noDocsDescription')}</CardDescription>
        </CardHeader>
      </Card>
    );
  }

  const docsRail = (
    <DocsRail
      docs={docs}
      selectedDoc={selectedDoc}
      searchQuery={searchQuery}
      setSearchQuery={setSearchQuery}
      onSelectDoc={(slug) => {
        setPendingHeadingId('');
        onSelectDoc(slug, '');
      }}
      closeMobile={() => setMobileOpen(false)}
    />
  );

  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-center justify-between gap-3 lg:hidden">
        <div className="min-w-0">
          <p className="text-sm font-medium">{publicMode ? t('docs.mobileTitle.public') : t('docs.mobileTitle.workspace')}</p>
          <p className="text-sm text-muted-foreground">{selectedDoc.title}</p>
        </div>
        <Sheet open={mobileOpen} onOpenChange={setMobileOpen}>
          <SheetTrigger asChild>
            <Button variant="outline" size="sm">
              <ListIcon data-icon="inline-start" />
              {t('docs.browse')}
            </Button>
          </SheetTrigger>
          <SheetContent side="left" className="w-full max-w-sm p-0 sm:max-w-sm">
            <SheetHeader className="border-b">
              <SheetTitle>{t('docs.browse')}</SheetTitle>
              <SheetDescription>{t('docs.browseDescription')}</SheetDescription>
            </SheetHeader>
            <div className="min-h-0 flex-1 overflow-hidden p-4">
              {docsRail}
            </div>
          </SheetContent>
        </Sheet>
      </div>

      <div className="grid gap-6 lg:grid-cols-[300px_minmax(0,1fr)] xl:grid-cols-[320px_minmax(0,1fr)_260px]">
        <aside className="hidden lg:block">
          <div className="sticky top-20 h-[calc(100dvh-6rem)]">
            <Card className="flex h-full flex-col overflow-hidden border bg-card/95">
              <CardHeader className="border-b">
                <div className="flex items-center gap-2">
                  <BookOpenTextIcon className="size-4" />
                  <div>
                    <CardTitle className="text-base">{publicMode ? t('docs.mobileTitle.public') : t('docs.mobileTitle.workspace')}</CardTitle>
                  </div>
                </div>
              </CardHeader>
              <CardContent className="min-h-0 flex-1 p-4">
                {docsRail}
              </CardContent>
            </Card>
          </div>
        </aside>

        <div className="min-w-0">
          <DocsArticle
            doc={selectedDoc}
            docs={docs}
            initialHeadingId={pendingHeadingId}
            onNavigateDoc={(slug, headingId) => {
              setPendingHeadingId(headingId || '');
              onSelectDoc(slug, headingId || '');
            }}
            onHeadingChange={setActiveHeadingId}
          />

          <div className="mt-4 xl:hidden">
            <DocsGuideTools
              selectedDoc={selectedDoc}
              selectedDocIndex={selectedDocIndex}
              docCount={docs.length}
              activeHeadingId={activeHeadingId}
              onSelectHeading={(headingId) => setPendingHeadingId(headingId)}
              previousDoc={previousDoc}
              nextDoc={nextDoc}
              onSelectDoc={(slug, headingId) => {
                setPendingHeadingId(headingId || '');
                onSelectDoc(slug, headingId || '');
              }}
            />
          </div>

          <div className="mt-4 flex flex-wrap items-center gap-2 text-sm text-muted-foreground">
            <FileTextIcon className="size-4" />
            <span>{t('docs.permalink')}</span>
            <a href={buildDocsPath(selectedDoc.slug)} className="font-medium text-foreground hover:underline">
              {buildDocsPath(selectedDoc.slug)}
            </a>
          </div>
        </div>

        <aside className="hidden xl:block">
          <div className="sticky top-20">
            <DocsGuideTools
              selectedDoc={selectedDoc}
              selectedDocIndex={selectedDocIndex}
              docCount={docs.length}
              activeHeadingId={activeHeadingId}
              onSelectHeading={(headingId) => setPendingHeadingId(headingId)}
              previousDoc={previousDoc}
              nextDoc={nextDoc}
              onSelectDoc={(slug, headingId) => {
                setPendingHeadingId(headingId || '');
                onSelectDoc(slug, headingId || '');
              }}
            />
          </div>
        </aside>
      </div>
    </div>
  );
}
