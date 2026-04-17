import { CircleAlertIcon, XIcon } from 'lucide-react';
import { useEffect, useState } from 'react';

import { translate, useI18n } from '@/i18n';

const DEFAULT_TOAST_DURATION = 5000;

const listeners = new Set();

let nextToastId = 1;
let activeToasts = [];

function emitToasts() {
  listeners.forEach((listener) => listener(activeToasts));
}

function removeToast(id) {
  activeToasts = activeToasts.filter((toast) => toast.id !== id);
  emitToasts();
}

function pushToast(toast) {
  const duration = toast.duration ?? DEFAULT_TOAST_DURATION;
  const dedupeKey = toast.dedupeKey ?? [toast.variant, toast.title, toast.description].filter(Boolean).join(':');
  const expiresAt = Date.now() + duration;
  const existingToast = activeToasts.find((entry) => entry.dedupeKey === dedupeKey);

  if (existingToast) {
    activeToasts = activeToasts.map((entry) =>
      entry.id === existingToast.id
        ? {
            ...entry,
            ...toast,
            dedupeKey,
            duration,
            expiresAt
          }
        : entry
    );
  } else {
    activeToasts = [
      ...activeToasts,
      {
        id: nextToastId++,
        duration,
        expiresAt,
        variant: 'destructive',
        ...toast,
        dedupeKey
      }
    ];
  }

  emitToasts();
}

export function toastError({ title = translate('toast.requestFailed'), description, duration, dedupeKey }) {
  if (!description) {
    return;
  }

  pushToast({
    title,
    description,
    duration,
    dedupeKey,
    variant: 'destructive'
  });
}

function ToastItem({ toast }) {
  const { t } = useI18n();

  return (
    <div
      role="alert"
      className="pointer-events-auto flex w-full items-start gap-3 rounded-lg border border-destructive/20 bg-card px-4 py-3 text-card-foreground shadow-lg ring-1 ring-black/5 backdrop-blur-sm"
    >
      <div className="mt-0.5 rounded-full bg-destructive/10 p-1 text-destructive">
        <CircleAlertIcon className="size-4" aria-hidden="true" />
      </div>
      <div className="min-w-0 flex flex-1 flex-col gap-1">
        <p className="text-sm font-medium leading-none">{toast.title}</p>
        <p className="text-sm text-muted-foreground">{toast.description}</p>
      </div>
      <button
        type="button"
        className="cursor-pointer rounded-md p-1 text-muted-foreground transition-[color,background-color,box-shadow,transform] duration-150 ease-out motion-safe:hover:-translate-y-px hover:bg-muted hover:text-foreground hover:shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 motion-safe:active:translate-y-0 active:shadow-none"
        onClick={() => removeToast(toast.id)}
        aria-label={t('toast.dismissAria', { title: toast.title })}
      >
        <XIcon className="size-4" aria-hidden="true" />
      </button>
    </div>
  );
}

export function AppToaster() {
  const [toasts, setToasts] = useState(activeToasts);

  useEffect(() => {
    listeners.add(setToasts);
    return () => listeners.delete(setToasts);
  }, []);

  useEffect(() => {
    if (toasts.length === 0) {
      return undefined;
    }

    const timeouts = toasts.map((toast) =>
      window.setTimeout(() => removeToast(toast.id), Math.max(0, toast.expiresAt - Date.now()))
    );

    return () => {
      timeouts.forEach((timeout) => window.clearTimeout(timeout));
    };
  }, [toasts]);

  return (
    <div
      aria-live="assertive"
      aria-atomic="false"
      className="pointer-events-none fixed inset-x-4 bottom-4 z-50 flex flex-col items-end gap-3 sm:left-auto sm:right-4 sm:w-full sm:max-w-sm"
    >
      {toasts.map((toast) => (
        <ToastItem key={toast.id} toast={toast} />
      ))}
    </div>
  );
}
