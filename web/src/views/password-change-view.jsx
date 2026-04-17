import { ShieldCheckIcon } from 'lucide-react';

import { AuthScreen } from '@/components/app/auth-screen';
import { BusyButtonContent } from '@/components/app/busy-button-content';
import { Button } from '@/components/ui/button';
import { CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Field, FieldGroup, FieldLabel } from '@/components/ui/field';
import { Input } from '@/components/ui/input';
import { useI18n } from '@/i18n';

export function PasswordChangeFormCard({
  passwordForm,
  setPasswordForm,
  onSubmit,
  passwordChanging = false,
  title,
  description,
  submitLabel
}) {
  const { t } = useI18n();
  const resolvedTitle = title || t('auth.password.cardTitle');
  const resolvedDescription = description || t('auth.password.cardDescription');
  const resolvedSubmitLabel = submitLabel || t('auth.password.submit');

  return (
    <>
      <CardHeader className="border-b">
        <CardTitle>{resolvedTitle}</CardTitle>
        <CardDescription>{resolvedDescription}</CardDescription>
      </CardHeader>
      <CardContent className="pt-6">
        <form className="flex flex-col gap-5" onSubmit={onSubmit}>
          <FieldGroup>
            <Field>
              <FieldLabel htmlFor="current-password">{t('auth.password.current')}</FieldLabel>
              <Input
                id="current-password"
                type="password"
                autoComplete="current-password"
                value={passwordForm.currentPassword}
                onChange={(event) => setPasswordForm((current) => ({ ...current, currentPassword: event.target.value }))}
              />
            </Field>
            <Field>
              <FieldLabel htmlFor="new-password">{t('auth.password.new')}</FieldLabel>
              <Input
                id="new-password"
                type="password"
                autoComplete="new-password"
                value={passwordForm.newPassword}
                onChange={(event) => setPasswordForm((current) => ({ ...current, newPassword: event.target.value }))}
              />
            </Field>
          </FieldGroup>
          <Button type="submit" className="w-full" disabled={passwordChanging} aria-busy={passwordChanging}>
            <BusyButtonContent
              busy={passwordChanging}
              label={resolvedSubmitLabel}
              icon={ShieldCheckIcon}
            />
          </Button>
        </form>
      </CardContent>
    </>
  );
}

export function PasswordChangeView({ passwordForm, setPasswordForm, onSubmit, passwordChanging = false }) {
  const { t } = useI18n();
  const passwordPoints = [
    {
      title: t('auth.password.point.required.title'),
      body: t('auth.password.point.required.body')
    },
    {
      title: t('auth.password.point.minimum.title'),
      body: t('auth.password.point.minimum.body')
    },
    {
      title: t('auth.password.point.immediate.title'),
      body: t('auth.password.point.immediate.body')
    }
  ];

  return (
    <AuthScreen
      eyebrow={t('auth.password.eyebrow')}
      title={t('auth.password.title')}
      description={t('auth.password.description')}
      points={passwordPoints}
    >
      <PasswordChangeFormCard
        passwordForm={passwordForm}
        setPasswordForm={setPasswordForm}
        onSubmit={onSubmit}
        passwordChanging={passwordChanging}
      />
    </AuthScreen>
  );
}
