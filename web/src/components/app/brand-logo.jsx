import { cn } from '@/lib/utils';
import { useI18n } from '@/i18n';

export function BrandMark({ className, imageClassName, alt }) {
  const { t } = useI18n();
  const resolvedAlt = alt || t('brand.logoAlt');

  return (
    <span
      className={cn(
        'relative flex shrink-0 items-center justify-center overflow-hidden rounded-2xl border border-border/70 bg-gradient-to-br from-white via-card to-muted/55 shadow-[0_10px_30px_-20px_rgba(15,23,42,0.45)]',
        className
      )}
    >
      <img
        src="/brand/ohoci-logo.png"
        alt={resolvedAlt}
        className={cn('h-full w-full object-contain', imageClassName)}
      />
    </span>
  );
}

export function BrandLockup({
  className,
  title,
  subtitle,
  markClassName,
  titleClassName,
  subtitleClassName,
  center = false
}) {
  const { t } = useI18n();
  const resolvedTitle = title || 'OhoCI';
  const resolvedSubtitle = subtitle === undefined ? t('brand.subtitle') : subtitle;

  return (
    <div className={cn('flex items-center gap-3', center && 'justify-center text-center', className)}>
      <BrandMark className={cn('size-11 p-1.5', markClassName)} />
      <div className="min-w-0">
        <p className={cn('text-sm font-semibold tracking-tight text-foreground', titleClassName)}>{resolvedTitle}</p>
        {resolvedSubtitle ? (
          <p className={cn('truncate text-xs text-muted-foreground', subtitleClassName)}>
            {resolvedSubtitle}
          </p>
        ) : null}
      </div>
    </div>
  );
}
