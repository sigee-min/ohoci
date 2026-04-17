import { AlertTriangleIcon, InboxIcon, LoaderCircleIcon } from 'lucide-react';

import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { cn } from '@/lib/utils';
import { formatStatusLabel, statusVariant } from '@/lib/workspace-formatters';

export function StatusBadge({ value }) {
  return <Badge variant={statusVariant(value)}>{formatStatusLabel(value)}</Badge>;
}

export function StatCard({ label, value, note, accent = false }) {
  return (
    <Card size="sm" className={cn('border bg-card/88 shadow-none', accent && 'bg-accent/45')}>
      <CardHeader className="gap-1.5">
        <CardDescription className="text-sm text-muted-foreground">{label}</CardDescription>
        <CardTitle className="text-2xl font-semibold tracking-tight">{value}</CardTitle>
      </CardHeader>
      {note ? (
        <CardContent className="pt-0 text-sm text-muted-foreground">
          {note}
        </CardContent>
      ) : null}
    </Card>
  );
}

export function ViewStateBlock({
  state = 'empty',
  title,
  body,
  actionLabel,
  onAction,
  className
}) {
  const Icon = state === 'loading' ? LoaderCircleIcon : state === 'error' ? AlertTriangleIcon : InboxIcon;

  return (
    <div
      className={cn(
        'flex min-h-[180px] flex-col items-center justify-center rounded-xl border px-5 py-8 text-center',
        state === 'error'
          ? 'border-destructive/20 bg-destructive/5'
          : 'border-dashed bg-muted/20',
        className
      )}
    >
      <div
        className={cn(
          'mb-4 flex size-11 items-center justify-center rounded-full border bg-background',
          state === 'error' ? 'border-destructive/20 text-destructive' : 'border-border/70 text-muted-foreground'
        )}
      >
        <Icon className={cn('size-5', state === 'loading' && 'animate-spin')} />
      </div>
      <p className="text-sm font-medium">{title}</p>
      <p className="mt-2 max-w-xl text-sm leading-6 text-muted-foreground">{body}</p>
      {actionLabel && typeof onAction === 'function' ? (
        <Button className="mt-4" variant={state === 'error' ? 'outline' : 'default'} size="sm" onClick={() => void onAction()}>
          {actionLabel}
        </Button>
      ) : null}
    </div>
  );
}

export function LoadingBlock(props) {
  return <ViewStateBlock state="loading" {...props} />;
}

export function ErrorBlock(props) {
  return <ViewStateBlock state="error" {...props} />;
}

export function EmptyBlock(props) {
  return <ViewStateBlock state="empty" {...props} />;
}

export function ActivityList({ items, emptyTitle, emptyBody, renderMeta }) {
  if (!items.length) {
    return <EmptyBlock title={emptyTitle} body={emptyBody} />;
  }
  return (
    <div className="flex flex-col divide-y">
      {items.map((item) => (
        <div key={item.key} className="flex items-start justify-between gap-4 py-3.5">
          <div className="min-w-0 flex flex-col gap-1">
            <p className="truncate text-sm font-medium">{item.title}</p>
            <p className="text-sm text-muted-foreground">{item.subtitle}</p>
          </div>
          <div className="shrink-0 text-right text-xs text-muted-foreground">
            {renderMeta(item)}
          </div>
        </div>
      ))}
    </div>
  );
}
