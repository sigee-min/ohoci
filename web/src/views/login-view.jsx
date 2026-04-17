import { ShieldCheckIcon } from 'lucide-react';

import { AuthScreen } from '@/components/app/auth-screen';
import { Button } from '@/components/ui/button';
import { CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Field, FieldGroup, FieldLabel } from '@/components/ui/field';
import { Input } from '@/components/ui/input';
import { useI18n } from '@/i18n';

export function LoginView({ loginForm, setLoginForm, onSubmit }) {
  const { t } = useI18n();
  const loginPoints = [
    {
      title: t('auth.login.point.policies.title'),
      body: t('auth.login.point.policies.body')
    },
    {
      title: t('auth.login.point.runtime.title'),
      body: t('auth.login.point.runtime.body')
    },
    {
      title: t('auth.login.point.activity.title'),
      body: t('auth.login.point.activity.body')
    }
  ];

  return (
    <AuthScreen
      eyebrow={t('auth.login.eyebrow')}
      title={t('auth.login.title')}
      description={t('auth.login.description')}
      points={loginPoints}
    >
      <CardHeader className="border-b">
        <CardTitle>{t('auth.login.cardTitle')}</CardTitle>
        <CardDescription>{t('auth.login.cardDescription')}</CardDescription>
      </CardHeader>
      <CardContent className="pt-6">
        <form className="flex flex-col gap-5" onSubmit={onSubmit}>
          <FieldGroup>
            <Field>
              <FieldLabel htmlFor="username">{t('auth.login.username')}</FieldLabel>
              <Input
                id="username"
                autoComplete="username"
                placeholder="admin"
                value={loginForm.username}
                onChange={(event) => setLoginForm((current) => ({ ...current, username: event.target.value }))}
              />
            </Field>
            <Field>
              <FieldLabel htmlFor="password">{t('auth.login.password')}</FieldLabel>
              <Input
                id="password"
                type="password"
                autoComplete="current-password"
                value={loginForm.password}
                onChange={(event) => setLoginForm((current) => ({ ...current, password: event.target.value }))}
              />
            </Field>
          </FieldGroup>
          <Button type="submit" className="w-full">
            <ShieldCheckIcon data-icon="inline-start" />
            {t('auth.login.submit')}
          </Button>
          <Button asChild variant="ghost" className="w-full">
            <a href="/docs">{t('auth.login.readDocs')}</a>
          </Button>
        </form>
      </CardContent>
    </AuthScreen>
  );
}
