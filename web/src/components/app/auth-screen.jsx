import { BrandLockup } from '@/components/app/brand-logo';
import { LocaleSwitcher } from '@/components/app/locale-switcher';
import { Card } from '@/components/ui/card';

export function AuthScreen({ eyebrow, title, description, points, children }) {
  return (
    <div className="min-h-svh bg-background">
      <div className="mx-auto grid min-h-svh max-w-6xl gap-6 px-4 py-4 sm:px-6 lg:grid-cols-[minmax(0,1fr)_440px] lg:items-center lg:px-8">
        <section className="order-2 relative flex flex-col overflow-hidden rounded-[calc(var(--radius-2xl)+0.25rem)] border bg-card/72 px-6 py-8 sm:px-8 sm:py-10 lg:order-1 lg:self-center">
          <div className="flex flex-col gap-8">
            <div className="flex flex-col gap-4">
              <div className="flex items-start justify-between gap-4">
                <BrandLockup
                  markClassName="size-12 rounded-[1.15rem] p-1.5"
                  titleClassName="text-base"
                  subtitleClassName="text-sm"
                />
                <LocaleSwitcher />
              </div>
              {eyebrow ? <p className="text-sm text-muted-foreground">{eyebrow}</p> : null}
              <div className="max-w-xl flex flex-col gap-3">
                <h1 className="text-4xl font-semibold tracking-tight text-balance sm:text-5xl">{title}</h1>
                <p className="text-base leading-7 text-muted-foreground">{description}</p>
              </div>
            </div>
            <div className="grid gap-4 sm:grid-cols-3 lg:grid-cols-1">
              {points.map((point, index) => (
                <div key={point.title} className="flex gap-3 border-t pt-4 first:border-t-0 first:pt-0">
                  <div className="flex size-7 shrink-0 items-center justify-center rounded-full bg-secondary text-xs font-medium text-secondary-foreground">
                    {index + 1}
                  </div>
                  <div className="min-w-0 flex flex-col gap-1">
                    <p className="text-sm font-medium">{point.title}</p>
                    <p className="text-sm text-muted-foreground">{point.body}</p>
                  </div>
                </div>
              ))}
            </div>
          </div>
        </section>
        <div className="order-1 flex items-center justify-center lg:order-2 lg:justify-end">
          <Card className="w-full max-w-[440px] border bg-background/88">
            {children}
          </Card>
        </div>
      </div>
    </div>
  );
}
