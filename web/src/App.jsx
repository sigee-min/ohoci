import { Suspense, lazy, useEffect, useMemo, useState } from 'react';
import { BookOpenTextIcon, LogInIcon, LogOutIcon, RefreshCwIcon } from 'lucide-react';

import { AppSidebar } from '@/components/app/app-sidebar';
import { BrandLockup } from '@/components/app/brand-logo';
import { LoadingBlock } from '@/components/app/display-primitives';
import { LocaleSwitcher } from '@/components/app/locale-switcher';
import { WorkspaceHeader } from '@/components/app/workspace-header';
import { Button } from '@/components/ui/button';
import { Card } from '@/components/ui/card';
import { SidebarInset, SidebarProvider } from '@/components/ui/sidebar';
import { useWorkspaceApp } from '@/hooks/use-workspace-app';
import { useI18n } from '@/i18n';
import { buildDocsHref, buildDocsPath, parseDocsPath } from '@/lib/docs';
import { SETUP_STEP_META } from '@/lib/workspace-constants';
import { LoginView } from '@/views/login-view.jsx';
import { OverviewView } from '@/views/overview-view.jsx';
import { PasswordChangeFormCard } from '@/views/password-change-view.jsx';

const WorkspaceDocsRoute = lazy(() =>
  import('@/views/workspace-docs-route.jsx').then((module) => ({ default: module.WorkspaceDocsRoute }))
);

const PublicDocsRoute = lazy(() =>
  import('@/views/public-docs-route.jsx').then((module) => ({ default: module.PublicDocsRoute }))
);

const EventsView = lazy(() =>
  import('@/views/events-view.jsx').then((module) => ({ default: module.EventsView }))
);

const GitHubConfigView = lazy(() =>
  import('@/views/github-config-view.jsx').then((module) => ({ default: module.GitHubConfigView }))
);

const JobsView = lazy(() =>
  import('@/views/jobs-view.jsx').then((module) => ({ default: module.JobsView }))
);

const OCIAuthView = lazy(() =>
  import('@/views/oci-auth-view.jsx').then((module) => ({ default: module.OCIAuthView }))
);

const PoliciesView = lazy(() =>
  import('@/views/policies-view.jsx').then((module) => ({ default: module.PoliciesView }))
);

const RunnersView = lazy(() =>
  import('@/views/runners-view.jsx').then((module) => ({ default: module.RunnersView }))
);

const RunnerImagesView = lazy(() =>
  import('@/views/runner-images-view.jsx').then((module) => ({ default: module.RunnerImagesView }))
);

const SetupOnboardingView = lazy(() =>
  import('@/views/setup-onboarding-view.jsx').then((module) => ({ default: module.SetupOnboardingView }))
);

const SetupView = lazy(() =>
  import('@/views/setup-view.jsx').then((module) => ({ default: module.SetupView }))
);

function RouteLoadingFallback() {
  const { t } = useI18n();

  return (
    <LoadingBlock
      title={t('route.loading.title')}
      body={t('route.loading.body')}
      className="min-h-[240px] rounded-[1.5rem] border bg-card/70"
    />
  );
}

function useLocationState() {
  const [locationState, setLocationState] = useState(() => ({
    pathname: window.location.pathname,
    hash: window.location.hash
  }));

  useEffect(() => {
    const handlePopState = () => {
      setLocationState({
        pathname: window.location.pathname,
        hash: window.location.hash
      });
    };

    window.addEventListener('popstate', handlePopState);
    return () => window.removeEventListener('popstate', handlePopState);
  }, []);

  const navigate = (nextPath, options = {}) => {
    const target = String(nextPath || '').trim() || '/';
    const current = `${window.location.pathname}${window.location.hash}`;
    if (current === target) {
      setLocationState({
        pathname: window.location.pathname,
        hash: window.location.hash
      });
      return;
    }

    const method = options.replace ? 'replaceState' : 'pushState';
    window.history[method](null, '', target);
    setLocationState({
      pathname: window.location.pathname,
      hash: window.location.hash
    });
  };

  return [locationState, navigate];
}

function buildGitHubSetupProps(workspace) {
  return {
    githubConfigForm: workspace.githubConfigForm,
    setGithubConfigForm: workspace.setGithubConfigForm,
    githubConfigMode: workspace.githubConfigMode,
    setGithubConfigMode: workspace.setGithubConfigMode,
    githubManifestState: workspace.githubManifestState,
    onTest: workspace.handleGitHubTest,
    onSave: workspace.handleGitHubSave,
    onClear: workspace.handleGitHubClear,
    onPromote: workspace.handleGitHubPromote,
    onRemoveActiveApp: workspace.handleGitHubActiveAppRemove,
    onManifestCreate: workspace.handleGitHubManifestCreate,
    onDiscoverInstallations: workspace.handleGitHubInstallationDiscovery,
    githubConfigTesting: workspace.githubConfigTesting,
    githubConfigSaving: workspace.githubConfigSaving,
    githubConfigClearing: workspace.githubConfigClearing,
    githubConfigPromoting: workspace.githubConfigPromoting,
    githubActiveAppDeletingId: workspace.githubActiveAppDeletingId,
    githubDriftStatus: workspace.githubDriftStatus,
    githubDriftReconciling: workspace.githubDriftReconciling,
    onRefreshDrift: workspace.loadGitHubDrift,
    onReconcileDrift: workspace.handleGitHubDriftReconcile,
    githubConfigStatus: workspace.githubConfigStatus,
    githubConfigResult: workspace.githubConfigResult,
    githubReady: workspace.setupStatus.steps?.github?.completed
  };
}

function buildOCISetupProps(workspace, overrides = {}) {
  return {
    ociAuthForm: workspace.ociAuthForm,
    setOciAuthForm: workspace.setOciAuthForm,
    onFileUpload: workspace.handleOCIAuthFile,
    onTest: workspace.handleOCIAuthTest,
    onSave: workspace.handleOCIAuthSave,
    onClear: workspace.handleOCIAuthClear,
    ociAuthInspecting: workspace.ociAuthInspecting,
    ociAuthInspectResult: workspace.ociAuthInspectResult,
    ociAuthTesting: workspace.ociAuthTesting,
    ociAuthSaving: workspace.ociAuthSaving,
    ociAuthClearing: workspace.ociAuthClearing,
    ociAuthStatus: workspace.ociAuthStatus,
    ociAuthResult: workspace.ociAuthResult,
    ociRuntimeStatus: workspace.ociRuntimeStatus,
    ociRuntimeForm: workspace.ociRuntimeForm,
    setOciRuntimeForm: workspace.setOciRuntimeForm,
    runtimeCatalog: workspace.runtimeCatalog,
    runtimeCatalogValidation: workspace.runtimeCatalogValidation,
    onRuntimeCatalogRefresh: workspace.handleRuntimeCatalogRefresh,
    onRuntimeSave: workspace.handleOCIRuntimeSave,
    onRuntimeClear: workspace.handleOCIRuntimeClear,
    ociRuntimeSaving: workspace.ociRuntimeSaving,
    ociRuntimeClearing: workspace.ociRuntimeClearing,
    ...overrides
  };
}

function renderMainView(workspace, onNavigate) {
  switch (workspace.view) {
    case 'settings':
      return (
        <SetupView
          setupStatus={workspace.setupStatus}
          githubSetupProps={buildGitHubSetupProps(workspace)}
          ociAuthForm={workspace.ociAuthForm}
          setOciAuthForm={workspace.setOciAuthForm}
          onOCIFileUpload={workspace.handleOCIAuthFile}
          onOCITest={workspace.handleOCIAuthTest}
          onOCISave={workspace.handleOCIAuthSave}
          onOCIClear={workspace.handleOCIAuthClear}
          ociAuthInspecting={workspace.ociAuthInspecting}
          ociAuthInspectResult={workspace.ociAuthInspectResult}
          ociAuthTesting={workspace.ociAuthTesting}
          ociAuthSaving={workspace.ociAuthSaving}
          ociAuthClearing={workspace.ociAuthClearing}
          ociAuthStatus={workspace.ociAuthStatus}
          ociAuthResult={workspace.ociAuthResult}
          ociRuntimeStatus={workspace.ociRuntimeStatus}
          ociRuntimeForm={workspace.ociRuntimeForm}
          setOciRuntimeForm={workspace.setOciRuntimeForm}
          runtimeCatalog={workspace.runtimeCatalog}
          runtimeCatalogValidation={workspace.runtimeCatalogValidation}
          onRuntimeCatalogRefresh={workspace.handleRuntimeCatalogRefresh}
          onOCIRuntimeSave={workspace.handleOCIRuntimeSave}
          onOCIRuntimeClear={workspace.handleOCIRuntimeClear}
          ociRuntimeSaving={workspace.ociRuntimeSaving}
          ociRuntimeClearing={workspace.ociRuntimeClearing}
        />
      );
    case 'policies':
      return (
        <PoliciesView
          viewState={workspace.majorViewStates.policies}
          editingPolicyId={workspace.editingPolicyId}
          policyForm={workspace.policyForm}
          setPolicyForm={workspace.setPolicyForm}
          onSubmit={workspace.handlePolicySubmit}
          onRefreshSubnets={workspace.loadSubnetCandidates}
          onCancelEdit={workspace.handleCancelPolicyEdit}
          policies={workspace.policies}
          subnetCandidates={workspace.subnetCandidates}
          subnetById={workspace.subnetById}
          defaultSubnetId={workspace.defaultSubnetId}
          subnetError={workspace.subnetError}
          recommendedSubnets={workspace.recommendedSubnets}
          policyCatalog={workspace.policyCatalog}
          policyValidation={workspace.policyValidation}
          billingGuardrails={workspace.billingGuardrails}
          policyCompatibilityForm={workspace.policyCompatibilityForm}
          setPolicyCompatibilityForm={workspace.setPolicyCompatibilityForm}
          policyCompatibilityResult={workspace.policyCompatibilityResult}
          policyCompatibilityLoading={workspace.policyCompatibilityLoading}
          policyCompatibilityError={workspace.policyCompatibilityError}
          onCheckCompatibility={workspace.handlePolicyCompatibilityCheck}
          onUseCurrentPolicyLabels={workspace.handlePolicyCompatibilityUseCurrentLabels}
          onPolicyShapeChange={workspace.handlePolicyShapeChange}
          onEditPolicy={workspace.handlePolicyEdit}
          onDeletePolicy={workspace.handlePolicyDelete}
        />
      );
    case 'runners':
      return (
        <RunnersView
          viewState={workspace.majorViewStates.runners}
          runners={workspace.runners}
          onTerminateRunner={workspace.handleTerminateRunner}
        />
      );
    case 'runner-images':
      return (
        <RunnerImagesView
          viewState={workspace.majorViewStates.runnerImages}
          runnerImages={workspace.runnerImages}
          runnerImageRecipeForm={workspace.runnerImageRecipeForm}
          setRunnerImageRecipeForm={workspace.setRunnerImageRecipeForm}
          editingRunnerImageRecipeId={workspace.editingRunnerImageRecipeId}
          runnerImageRecipeSaving={workspace.runnerImageRecipeSaving}
          runnerImageRecipeDeletingId={workspace.runnerImageRecipeDeletingId}
          runnerImageBuildingId={workspace.runnerImageBuildingId}
          runnerImagePromotingId={workspace.runnerImagePromotingId}
          runnerImageReconciling={workspace.runnerImageReconciling}
          onRefresh={() => void workspace.loadRunnerImages()}
          onRecipeSubmit={workspace.handleRunnerImageRecipeSubmit}
          onRecipeEdit={workspace.handleRunnerImageRecipeEdit}
          onRecipeCancel={workspace.handleRunnerImageRecipeCancel}
          onRecipeDelete={workspace.handleRunnerImageRecipeDelete}
          onBuildCreate={workspace.handleRunnerImageBuildCreate}
          onBuildPromote={workspace.handleRunnerImagePromote}
          onReconcile={workspace.handleRunnerImageReconcile}
        />
      );
    case 'jobs':
      return (
        <JobsView
          viewState={workspace.majorViewStates.jobs}
          jobs={workspace.jobs}
          jobDiagnosticsByJobId={workspace.jobDiagnosticsByJobId}
          jobDiagnosticsErrorsByJobId={workspace.jobDiagnosticsErrorsByJobId}
          jobDiagnosticsLoadingId={workspace.jobDiagnosticsLoadingId}
          onLoadJobDiagnostics={workspace.handleJobDiagnosticsLoad}
        />
      );
    case 'events':
      return (
        <EventsView
          viewState={workspace.majorViewStates.events}
          events={workspace.events}
          filteredLogs={workspace.filteredLogs}
          eventSearch={workspace.eventSearch}
          setEventSearch={workspace.setEventSearch}
        />
      );
    case 'overview':
    default:
      return (
        <OverviewView
          viewState={workspace.majorViewStates.overview}
          enabledPolicies={workspace.enabledPolicies}
          liveRunners={workspace.liveRunners}
          queuedJobs={workspace.queuedJobs}
          errorLogs={workspace.errorLogs}
          billingReport={workspace.billingReport}
          billingGuardrails={workspace.billingGuardrails}
          blockedGuardrailItems={workspace.blockedGuardrailItems}
          overviewRunnerItems={workspace.overviewRunnerItems}
          overviewJobItems={workspace.overviewJobItems}
          overviewLogItems={workspace.overviewLogItems}
          githubDriftStatus={workspace.githubDriftStatus}
          ociAuthStatus={workspace.ociAuthStatus}
          warmPoolStatus={workspace.warmPoolStatus}
          cacheCompatStatus={workspace.cacheCompatStatus}
          recommendedSubnets={workspace.recommendedSubnets}
          subnetError={workspace.subnetError}
          subnetById={workspace.subnetById}
          defaultSubnetId={workspace.defaultSubnetId}
          onNavigate={onNavigate}
        />
      );
  }
}

function renderOnboardingStep(workspace, t) {
  switch (workspace.activeOnboardingStep) {
    case 'github':
      return (
        <GitHubConfigView
          {...buildGitHubSetupProps(workspace)}
          title={t(SETUP_STEP_META.github.titleKey)}
          description={t(SETUP_STEP_META.github.descriptionKey)}
          layout="onboarding"
        />
      );
    case 'oci':
      return (
        <OCIAuthView
          {...buildOCISetupProps(workspace, {
            title: t(SETUP_STEP_META.oci.titleKey),
            description: t(SETUP_STEP_META.oci.descriptionKey),
            layout: 'onboarding'
          })}
        />
      );
    case 'password':
    default:
      return (
        <div className="mx-auto flex w-full max-w-[720px] flex-1 flex-col">
          <Card className="w-full border bg-card/95">
            <PasswordChangeFormCard
              passwordForm={workspace.passwordForm}
              setPasswordForm={workspace.setPasswordForm}
              onSubmit={workspace.handlePasswordChange}
              passwordChanging={workspace.passwordChanging}
              title={t(SETUP_STEP_META.password.titleKey)}
              description={t(SETUP_STEP_META.password.descriptionKey)}
            />
          </Card>
        </div>
      );
  }
}

function PublicDocsShell({ locationState, navigate }) {
  const { t } = useI18n();
  const docsRoute = parseDocsPath(locationState.pathname);
  const selectedSlug = docsRoute.slug || '';

  return (
    <div className="min-h-svh bg-background">
      <header className="border-b bg-background/90 backdrop-blur supports-[backdrop-filter]:bg-background/80">
        <div className="mx-auto flex w-full max-w-[1360px] items-center justify-between gap-4 px-4 py-3 md:px-6 lg:px-8">
          <BrandLockup
            className="min-w-0"
            title={t('publicDocs.title')}
            subtitle={t('publicDocs.subtitle')}
            markClassName="size-11 rounded-[1.05rem] p-1.5"
            subtitleClassName="max-w-xl truncate text-sm"
          />
          <div className="flex items-center gap-2">
            <LocaleSwitcher />
            <Button asChild variant="outline" size="sm">
              <a href="/">
                <LogInIcon data-icon="inline-start" />
                {t('common.signIn')}
              </a>
            </Button>
          </div>
        </div>
      </header>

      <main className="mx-auto flex w-full max-w-[1360px] flex-1 flex-col gap-6 px-4 py-5 md:px-6 md:py-6 lg:px-8">
        <Suspense fallback={<RouteLoadingFallback />}>
          <PublicDocsRoute
            selectedSlug={selectedSlug}
            initialHeadingId={String(locationState.hash || '').replace(/^#/, '')}
            onSelectDoc={(slug, headingId) => navigate(buildDocsHref(slug, headingId))}
          />
        </Suspense>
      </main>
    </div>
  );
}

function DeferredSetupShell({ workspace }) {
  const { t } = useI18n();

  return (
    <div className="min-h-svh bg-background">
      <header className="border-b bg-background/90 backdrop-blur supports-[backdrop-filter]:bg-background/80">
        <div className="mx-auto flex w-full max-w-[1360px] flex-col gap-3 px-4 py-3 md:flex-row md:items-center md:justify-between md:px-6 lg:px-8">
          <BrandLockup
            className="min-w-0"
            title={t('setup.pendingShell.title')}
            subtitle={t('setup.pendingShell.subtitle')}
            markClassName="size-11 rounded-[1.05rem] p-1.5"
            subtitleClassName="max-w-2xl text-sm"
          />
          <div className="flex flex-wrap items-center gap-2">
            <LocaleSwitcher />
            <Button variant="outline" size="sm" onClick={() => void workspace.refreshAll()} disabled={workspace.refreshing} aria-busy={workspace.refreshing}>
              <RefreshCwIcon data-icon="inline-start" className={workspace.refreshing ? 'animate-spin' : undefined} />
              {t('common.refresh')}
            </Button>
            <Button variant="outline" size="sm" asChild>
              <a href="/docs">
                <BookOpenTextIcon data-icon="inline-start" />
                {t('common.openDocs')}
              </a>
            </Button>
            <Button variant="ghost" size="sm" onClick={() => void workspace.handleLogout()}>
              <LogOutIcon data-icon="inline-start" />
              {t('common.signOut')}
            </Button>
          </div>
        </div>
      </header>

      <main className="mx-auto flex w-full max-w-[1360px] flex-1 flex-col gap-6 px-4 py-5 md:px-6 md:py-6 lg:px-8">
        <Card className="border bg-card/92 p-5">
          <div className="space-y-2">
            <h1 className="text-xl font-semibold tracking-tight">{t('header.finishSetup')}</h1>
            <p className="max-w-3xl text-sm text-muted-foreground">{t('setup.pendingShell.body')}</p>
          </div>
        </Card>

        <Suspense fallback={<RouteLoadingFallback />}>
          <SetupView
            setupStatus={workspace.setupStatus}
            githubSetupProps={buildGitHubSetupProps(workspace)}
            ociAuthForm={workspace.ociAuthForm}
            setOciAuthForm={workspace.setOciAuthForm}
            onOCIFileUpload={workspace.handleOCIAuthFile}
            onOCITest={workspace.handleOCIAuthTest}
            onOCISave={workspace.handleOCIAuthSave}
            onOCIClear={workspace.handleOCIAuthClear}
            ociAuthInspecting={workspace.ociAuthInspecting}
            ociAuthInspectResult={workspace.ociAuthInspectResult}
            ociAuthTesting={workspace.ociAuthTesting}
            ociAuthSaving={workspace.ociAuthSaving}
            ociAuthClearing={workspace.ociAuthClearing}
            ociAuthStatus={workspace.ociAuthStatus}
            ociAuthResult={workspace.ociAuthResult}
            ociRuntimeStatus={workspace.ociRuntimeStatus}
            ociRuntimeForm={workspace.ociRuntimeForm}
            setOciRuntimeForm={workspace.setOciRuntimeForm}
            runtimeCatalog={workspace.runtimeCatalog}
            runtimeCatalogValidation={workspace.runtimeCatalogValidation}
            onRuntimeCatalogRefresh={workspace.handleRuntimeCatalogRefresh}
            onOCIRuntimeSave={workspace.handleOCIRuntimeSave}
            onOCIRuntimeClear={workspace.handleOCIRuntimeClear}
            ociRuntimeSaving={workspace.ociRuntimeSaving}
            ociRuntimeClearing={workspace.ociRuntimeClearing}
          />
        </Suspense>
      </main>
    </div>
  );
}

function AuthenticatedApp() {
  const { t } = useI18n();
  const workspace = useWorkspaceApp();
  const [selectedDocState, setSelectedDocState] = useState({ slug: '', headingId: '' });

  if (workspace.loading) {
    return (
      <div className="grid min-h-svh place-items-center bg-background px-4">
        <LoadingBlock
          title={t('workspace.loading.title')}
          body={t('workspace.loading.body')}
          className="w-full max-w-xl border bg-card/92"
        />
      </div>
    );
  }

  if (!workspace.session?.authenticated) {
    return (
      <LoginView
        loginForm={workspace.loginForm}
        setLoginForm={workspace.setLoginForm}
        onSubmit={workspace.handleLogin}
      />
    );
  }

  if (workspace.needsOnboarding) {
    return (
      <Suspense fallback={<RouteLoadingFallback />}>
        <SetupOnboardingView
          setupStatus={workspace.setupStatus}
          activeStepId={workspace.activeOnboardingStep}
          currentStepId={workspace.currentOnboardingStep}
          onSelectStep={workspace.selectOnboardingStep}
          refreshing={workspace.refreshing}
          onRefresh={() => void workspace.refreshAll()}
          onLogout={() => void workspace.handleLogout()}
        >
          {renderOnboardingStep(workspace, t)}
        </SetupOnboardingView>
      </Suspense>
    );
  }

  if (!workspace.setupStatus.completed) {
    return <DeferredSetupShell workspace={workspace} />;
  }

  return (
    <SidebarProvider defaultOpen>
      <AppSidebar
        view={workspace.view}
        setView={workspace.setView}
        policies={workspace.policies}
        runners={workspace.runners}
        runnerImages={workspace.runnerImages}
        jobs={workspace.jobs}
        logs={workspace.logs}
        onLogout={() => void workspace.handleLogout()}
      />
      <SidebarInset>
        <div className="flex min-h-svh flex-col">
          <WorkspaceHeader
            currentView={workspace.currentView}
            ociAuthStatus={workspace.ociAuthStatus}
            refreshing={workspace.refreshing}
            onRefresh={() => void workspace.refreshAll()}
            onCleanup={() => void workspace.handleCleanup()}
          />

          <main className="mx-auto flex w-full max-w-[1360px] flex-1 flex-col gap-6 px-4 py-5 md:px-6 md:py-6 lg:px-8">
            <Suspense fallback={<RouteLoadingFallback />}>
              {workspace.view === 'docs' ? (
                <WorkspaceDocsRoute
                  selectedSlug={selectedDocState.slug}
                  initialHeadingId={selectedDocState.headingId}
                  onSelectDoc={(slug, headingId) => setSelectedDocState({ slug, headingId: headingId || '' })}
                />
              ) : (
                renderMainView(workspace, workspace.setView)
              )}
            </Suspense>
          </main>
        </div>
      </SidebarInset>
    </SidebarProvider>
  );
}

export function App() {
  const [locationState, navigate] = useLocationState();
  const docsRoute = useMemo(() => parseDocsPath(locationState.pathname), [locationState.pathname]);

  if (docsRoute.isDocsRoute) {
    return <PublicDocsShell locationState={locationState} navigate={navigate} />;
  }

  return <AuthenticatedApp />;
}
