import { LoaderCircleIcon } from 'lucide-react';

import { cn } from '@/lib/utils';

export function BusyButtonContent({ busy = false, label, icon: Icon, busyIcon: BusyIcon = LoaderCircleIcon, spin = false, className }) {
  const ResolvedIcon = busy ? BusyIcon : Icon;
  const shouldSpin = busy && (spin || BusyIcon === LoaderCircleIcon || BusyIcon === Icon);

  return (
    <>
      <ResolvedIcon
        data-icon="inline-start"
        aria-hidden="true"
        className={cn(shouldSpin && 'animate-spin', className)}
      />
      {label}
    </>
  );
}
