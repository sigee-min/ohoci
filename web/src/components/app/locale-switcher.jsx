import { useEffect, useId, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { CheckIcon, ChevronDownIcon, LanguagesIcon } from 'lucide-react';

import { LOCALE_OPTIONS, useI18n } from '@/i18n';
import { cn } from '@/lib/utils';

const VIEWPORT_PADDING = 12;
const MENU_GAP = 8;
const MIN_MENU_WIDTH = 176;

function measureMenuPosition(triggerElement) {
  if (!triggerElement || typeof window === 'undefined') {
    return null;
  }

  const rect = triggerElement.getBoundingClientRect();
  const width = Math.max(MIN_MENU_WIDTH, Math.round(rect.width));
  const left = Math.min(
    Math.max(VIEWPORT_PADDING, rect.right - width),
    Math.max(VIEWPORT_PADDING, window.innerWidth - VIEWPORT_PADDING - width)
  );
  const top = rect.bottom + MENU_GAP;
  const maxHeight = Math.max(120, window.innerHeight - top - VIEWPORT_PADDING);

  return {
    left,
    top,
    width,
    maxHeight
  };
}

export function LocaleSwitcher({ className }) {
  const { locale, setLocale, t } = useI18n();
  const activeOption = LOCALE_OPTIONS.find((option) => option.value === locale) || LOCALE_OPTIONS[0];
  const menuId = useId();
  const rootRef = useRef(null);
  const triggerRef = useRef(null);
  const menuRef = useRef(null);
  const [open, setOpen] = useState(false);
  const [menuStyle, setMenuStyle] = useState(null);

  useEffect(() => {
    if (!open) {
      return undefined;
    }

    const updateMenuPosition = () => {
      setMenuStyle(measureMenuPosition(triggerRef.current));
    };

    const handlePointerDown = (event) => {
      const target = event.target;
      if (rootRef.current?.contains(target) || menuRef.current?.contains(target)) {
        return;
      }
      setOpen(false);
    };

    const handleKeyDown = (event) => {
      if (event.key === 'Escape') {
        setOpen(false);
        triggerRef.current?.focus();
      }
    };

    updateMenuPosition();

    window.addEventListener('resize', updateMenuPosition);
    window.addEventListener('scroll', updateMenuPosition, true);
    document.addEventListener('mousedown', handlePointerDown);
    document.addEventListener('touchstart', handlePointerDown, { passive: true });
    document.addEventListener('keydown', handleKeyDown);

    return () => {
      window.removeEventListener('resize', updateMenuPosition);
      window.removeEventListener('scroll', updateMenuPosition, true);
      document.removeEventListener('mousedown', handlePointerDown);
      document.removeEventListener('touchstart', handlePointerDown);
      document.removeEventListener('keydown', handleKeyDown);
    };
  }, [open]);

  return (
    <div ref={rootRef} className={cn('relative inline-flex', className)}>
      <button
        ref={triggerRef}
        type="button"
        aria-label={t('locale.switcher.ariaLabel')}
        aria-controls={menuId}
        aria-expanded={open}
        aria-haspopup="menu"
        title={t('locale.switcher.ariaLabel')}
        data-slot="locale-switcher-trigger"
        className={cn(
          'flex w-full min-w-[8.75rem] cursor-pointer items-center justify-between gap-2 rounded-xl border bg-background/85 px-3 py-2 text-sm shadow-none outline-none transition-[color,background-color,border-color,box-shadow,transform] duration-150 ease-out motion-safe:hover:-translate-y-px hover:border-foreground/10 hover:bg-accent/40 hover:shadow-sm focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 motion-safe:active:translate-y-0'
        )}
        onClick={() => setOpen((current) => !current)}
        onKeyDown={(event) => {
          if (event.key === 'ArrowDown' || event.key === 'Enter' || event.key === ' ') {
            event.preventDefault();
            setOpen(true);
          }
        }}
      >
        <span className="flex min-w-0 items-center gap-2">
          <LanguagesIcon className="size-3.5 text-muted-foreground" />
          <span className="truncate text-sm font-medium">{t(`locale.option.${activeOption.value}`)}</span>
        </span>
        <ChevronDownIcon className={cn('size-4 text-muted-foreground transition-transform', open && 'rotate-180')} />
      </button>

      {open && menuStyle && typeof document !== 'undefined'
        ? createPortal(
            <div
              ref={menuRef}
              id={menuId}
              role="menu"
              aria-label={t('locale.switcher.ariaLabel')}
              data-slot="locale-switcher-menu"
              className="fixed z-50 overflow-hidden rounded-xl border bg-popover text-popover-foreground shadow-lg ring-1 ring-foreground/10"
              style={{
                left: `${menuStyle.left}px`,
                top: `${menuStyle.top}px`,
                width: `${menuStyle.width}px`,
                maxHeight: `${menuStyle.maxHeight}px`
              }}
            >
              <div className="max-h-full overflow-y-auto p-1">
                {LOCALE_OPTIONS.map((option) => {
                  const selected = option.value === locale;

                  return (
                    <button
                      key={option.value}
                      type="button"
                      role="menuitemradio"
                      aria-checked={selected}
                      className={cn(
                        'flex w-full items-center justify-between gap-3 rounded-lg px-3 py-2 text-left text-sm transition-[background-color,color,transform] duration-150 ease-out motion-safe:hover:-translate-y-px hover:bg-accent/50 hover:text-accent-foreground',
                        selected && 'bg-accent/70 font-medium text-accent-foreground'
                      )}
                      onClick={() => {
                        setLocale(option.value);
                        setOpen(false);
                        triggerRef.current?.focus();
                      }}
                    >
                      <span className="flex min-w-0 items-center gap-2">
                        <span>{t(`locale.option.${option.value}`)}</span>
                        <span className="text-xs font-semibold uppercase tracking-[0.12em] text-muted-foreground">
                          {option.shortLabel}
                        </span>
                      </span>
                      {selected ? <CheckIcon className="size-4" /> : null}
                    </button>
                  );
                })}
              </div>
            </div>,
            document.body
          )
        : null}
    </div>
  );
}
