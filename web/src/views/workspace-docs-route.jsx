import { APP_DOCS } from '@/generated/docs-data';
import { DocsView } from '@/views/docs-view.jsx';

export function WorkspaceDocsRoute({ selectedSlug, initialHeadingId = '', onSelectDoc }) {
  return (
    <DocsView
      docs={APP_DOCS}
      selectedSlug={selectedSlug || APP_DOCS[0]?.slug || ''}
      initialHeadingId={initialHeadingId}
      onSelectDoc={onSelectDoc}
    />
  );
}
