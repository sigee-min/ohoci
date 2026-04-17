import { AlertTriangleIcon, ArrowRightIcon, BoxIcon, ImageUpIcon, RefreshCwIcon, SearchCheckIcon, Trash2Icon } from 'lucide-react';

import { BusyButtonContent } from '@/components/app/busy-button-content';
import { EmptyBlock, ErrorBlock, LoadingBlock, StatCard, StatusBadge } from '@/components/app/display-primitives';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from '@/components/ui/card';
import {
  Field,
  FieldContent,
  FieldDescription,
  FieldGroup,
  FieldLabel,
  FieldSet
} from '@/components/ui/field';
import { Input } from '@/components/ui/input';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import { Textarea } from '@/components/ui/textarea';
import { useI18n } from '@/i18n';
import { normalizeOperatorList } from '@/lib/operator-text';
import { compactValue, formatDateTime } from '@/lib/workspace-formatters';

function terminalBuild(build) {
  const status = String(build?.status || '').toLowerCase();
  return status === 'available' || status === 'failed' || status === 'promoted';
}

function selectionSummary(selection, t) {
  if (!selection?.imageReference) {
    return t('runnerImages.selection.none');
  }
  return selection.imageReference;
}

function PreflightCheckList({ checks = [] }) {
  if (!checks.length) {
    return null;
  }

  return (
    <div className="flex flex-col gap-3">
      {checks.map((check) => (
        <div key={check.name} className="flex items-start justify-between gap-4 rounded-xl border bg-background/70 px-4 py-3">
          <div className="min-w-0">
            <p className="text-sm font-medium">{check.name}</p>
            {check.detail ? <p className="mt-1 text-sm text-muted-foreground">{check.detail}</p> : null}
          </div>
          <StatusBadge value={check.status} />
        </div>
      ))}
    </div>
  );
}

export function RunnerImagesView({
  viewState,
  runnerImages,
  runnerImageRecipeForm,
  setRunnerImageRecipeForm,
  editingRunnerImageRecipeId,
  runnerImageRecipeSaving,
  runnerImageRecipeDeletingId,
  runnerImageBuildingId,
  runnerImagePromotingId,
  runnerImageReconciling,
  onRefresh,
  onRecipeSubmit,
  onRecipeEdit,
  onRecipeCancel,
  onRecipeDelete,
  onBuildCreate,
  onBuildPromote,
  onReconcile
}) {
  const { t } = useI18n();
  const activeBuilds = runnerImages.builds.filter((build) => !terminalBuild(build)).length;
  const trackedResources = runnerImages.resources.filter((resource) => resource.tracked).length;

  return (
    <div className="flex flex-col gap-6">
      <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
        <StatCard
          label={t('runnerImages.stats.recipes')}
          value={runnerImages.recipes.length}
          note={t('runnerImages.stats.recipesNote')}
        />
        <StatCard
          label={t('runnerImages.stats.activeBuilds')}
          value={activeBuilds}
          note={t('runnerImages.stats.activeBuildsNote')}
          accent={activeBuilds > 0}
        />
        <StatCard
          label={t('runnerImages.stats.discovered')}
          value={runnerImages.resources.length}
          note={t('runnerImages.stats.discoveredNote', { tracked: trackedResources })}
          accent={runnerImages.resources.length > 0}
        />
        <StatCard
          label={t('runnerImages.stats.defaultImage')}
          value={runnerImages.defaultImage?.imageReference ? t('common.ready') : t('common.notSet')}
          note={selectionSummary(runnerImages.defaultImage, t)}
          accent={Boolean(runnerImages.defaultImage?.imageReference)}
        />
      </div>

      {viewState?.status === 'loading' ? (
        <LoadingBlock
          title={t('viewState.loading.title', { view: t('nav.runnerImages.label') })}
          body={t('viewState.loading.body', { view: t('nav.runnerImages.label') })}
        />
      ) : null}

      {viewState?.status === 'error' ? (
        <ErrorBlock
          title={t('viewState.error.title', { view: t('nav.runnerImages.label') })}
          body={viewState.error || t('viewState.error.body')}
          actionLabel={t('common.refresh')}
          onAction={onRefresh}
        />
      ) : null}

      {viewState?.status === 'empty' ? (
        <EmptyBlock title={t('runnerImages.empty.title')} body={t('runnerImages.empty.body')} />
      ) : null}

      {viewState?.status !== 'error' && runnerImages.error ? (
        <Alert>
          <AlertTriangleIcon />
          <AlertTitle>{t('runnerImages.error.title')}</AlertTitle>
          <AlertDescription>{runnerImages.error}</AlertDescription>
        </Alert>
      ) : null}

      {viewState?.status === 'loading' || viewState?.status === 'error' ? null : (
        <div className="grid gap-6 xl:grid-cols-[minmax(0,1.05fr)_minmax(360px,0.95fr)]">
          <div className="flex flex-col gap-6">
            <Card className="border bg-card/95">
              <CardHeader className="border-b">
                <div className="flex flex-col gap-1">
                  <CardTitle>{t('runnerImages.preflight.title')}</CardTitle>
                  <CardDescription>{t('runnerImages.preflight.description')}</CardDescription>
                </div>
              </CardHeader>
              <CardContent className="flex flex-col gap-4 pt-4">
                <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl bg-muted/20 px-4 py-3">
                  <div className="min-w-0">
                    <p className="text-sm font-medium">{t('runnerImages.preflight.summaryLabel')}</p>
                    <p className="mt-1 text-sm text-muted-foreground">
                      {runnerImages.preflight.summary || t('runnerImages.preflight.summaryFallback')}
                    </p>
                  </div>
                  <StatusBadge value={runnerImages.preflight.status || (runnerImages.preflight.ready ? 'ready' : 'blocked')} />
                </div>

                {runnerImages.preflight.missing?.length ? (
                  <div className="rounded-xl border bg-background/70 px-4 py-3">
                    <p className="text-sm font-medium">{t('runnerImages.preflight.missingTitle')}</p>
                    <p className="mt-1 text-sm text-muted-foreground">
                      {normalizeOperatorList(runnerImages.preflight.missing)}
                    </p>
                  </div>
                ) : null}

                <PreflightCheckList checks={runnerImages.preflight.checks} />
              </CardContent>
              <CardFooter className="flex flex-wrap justify-between gap-3">
                <p className="text-sm text-muted-foreground">
                  {runnerImages.preflight.updatedAt
                    ? t('runnerImages.preflight.updatedAt', { value: formatDateTime(runnerImages.preflight.updatedAt) })
                    : t('runnerImages.preflight.updatedAtMissing')}
                </p>
                <div className="flex flex-wrap gap-2">
                  <Button variant="outline" size="sm" onClick={onRefresh}>
                    <RefreshCwIcon data-icon="inline-start" />
                    {t('runnerImages.actions.refresh')}
                  </Button>
                  <Button variant="outline" size="sm" onClick={onReconcile} disabled={runnerImageReconciling} aria-busy={runnerImageReconciling}>
                    <BusyButtonContent
                      busy={runnerImageReconciling}
                      label={t('runnerImages.actions.reconcile')}
                      icon={SearchCheckIcon}
                    />
                  </Button>
                </div>
              </CardFooter>
            </Card>

            <Card className="border bg-card/95">
              <CardHeader className="border-b">
                <div className="flex flex-col gap-1">
                  <CardTitle>{t('runnerImages.builds.title')}</CardTitle>
                  <CardDescription>{t('runnerImages.builds.description')}</CardDescription>
                </div>
              </CardHeader>
              <CardContent className="pt-4">
                {!runnerImages.builds.length ? (
                  <EmptyBlock title={t('runnerImages.builds.emptyTitle')} body={t('runnerImages.builds.emptyBody')} />
                ) : (
                  <Table>
                    <TableHeader>
                      <TableRow>
                        <TableHead>{t('runnerImages.table.recipe')}</TableHead>
                        <TableHead>{t('runnerImages.table.status')}</TableHead>
                        <TableHead>{t('runnerImages.table.summary')}</TableHead>
                        <TableHead>{t('runnerImages.table.updated')}</TableHead>
                        <TableHead className="text-right">{t('runnerImages.table.actions')}</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {runnerImages.builds.map((build) => {
                        const promoting = runnerImagePromotingId === String(build.id);

                        return (
                          <TableRow key={build.id}>
                            <TableCell className="align-top">
                              <div className="flex flex-col gap-1">
                                <span className="font-medium">{build.recipeName || build.name}</span>
                                {build.imageReference ? (
                                  <span className="text-xs text-muted-foreground">{compactValue(build.imageReference)}</span>
                                ) : null}
                              </div>
                            </TableCell>
                            <TableCell className="align-top">
                              <StatusBadge value={build.status} />
                            </TableCell>
                            <TableCell className="max-w-80 align-top whitespace-normal text-sm text-muted-foreground">
                              {build.summary || t('runnerImages.builds.summaryFallback')}
                            </TableCell>
                            <TableCell className="align-top text-sm text-muted-foreground">
                              {formatDateTime(build.finishedAt || build.startedAt)}
                            </TableCell>
                            <TableCell className="align-top">
                              <div className="flex justify-end gap-2">
                                <Button
                                  variant="ghost"
                                  size="sm"
                                  disabled={!build.canPromote || promoting}
                                  aria-busy={promoting}
                                  onClick={() => onBuildPromote(build.id)}
                                >
                                  <BusyButtonContent
                                    busy={promoting}
                                    label={t('runnerImages.actions.promote')}
                                    icon={ArrowRightIcon}
                                  />
                                </Button>
                              </div>
                            </TableCell>
                          </TableRow>
                        );
                      })}
                    </TableBody>
                  </Table>
                )}
              </CardContent>
            </Card>

          <Card className="border bg-card/95">
            <CardHeader className="border-b">
              <div className="flex flex-col gap-1">
                <CardTitle>{t('runnerImages.discovery.title')}</CardTitle>
                <CardDescription>{t('runnerImages.discovery.description')}</CardDescription>
              </div>
            </CardHeader>
            <CardContent className="pt-4">
              {!runnerImages.resources.length ? (
                <EmptyBlock title={t('runnerImages.discovery.emptyTitle')} body={t('runnerImages.discovery.emptyBody')} />
              ) : (
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>{t('runnerImages.table.resource')}</TableHead>
                      <TableHead>{t('runnerImages.table.kind')}</TableHead>
                      <TableHead>{t('runnerImages.table.status')}</TableHead>
                      <TableHead>{t('runnerImages.table.tracking')}</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {runnerImages.resources.map((resource) => (
                      <TableRow key={resource.id}>
                        <TableCell className="align-top">
                          <div className="flex flex-col gap-1">
                            <span className="font-medium">{resource.name}</span>
                            <span className="text-xs text-muted-foreground">{compactValue(resource.id)}</span>
                          </div>
                        </TableCell>
                        <TableCell className="align-top">
                          <Badge variant="outline">{resource.kind}</Badge>
                        </TableCell>
                        <TableCell className="align-top">
                          <StatusBadge value={resource.status} />
                        </TableCell>
                        <TableCell className="align-top">
                          <div className="flex flex-col gap-1">
                            <Badge variant={resource.tracked ? 'secondary' : 'outline'}>
                              {resource.tracked ? t('runnerImages.discovery.tracked') : t('runnerImages.discovery.discovered')}
                            </Badge>
                            {(resource.sourceBuildId || resource.tags?.ohoci_build_id) ? (
                              <span className="text-xs text-muted-foreground">
                                {t('runnerImages.discovery.buildId', {
                                  value: resource.sourceBuildId || resource.tags?.ohoci_build_id
                                })}
                              </span>
                            ) : null}
                          </div>
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              )}
            </CardContent>
          </Card>
        </div>

        <div className="flex flex-col gap-6">
          <Card className="border bg-card/95">
            <CardHeader className="border-b">
              <div className="flex flex-col gap-1">
                <CardTitle>{t('runnerImages.selection.title')}</CardTitle>
                <CardDescription>{t('runnerImages.selection.description')}</CardDescription>
              </div>
            </CardHeader>
            <CardContent className="grid gap-4 pt-4">
              <div className="rounded-xl border bg-background/70 px-4 py-3">
                <p className="text-sm font-medium">{t('runnerImages.selection.current')}</p>
                <p className="mt-1 text-sm text-muted-foreground">{selectionSummary(runnerImages.defaultImage, t)}</p>
              </div>
              <div className="rounded-xl border bg-background/70 px-4 py-3">
                <p className="text-sm font-medium">{t('runnerImages.selection.promoted')}</p>
                <p className="mt-1 text-sm text-muted-foreground">{selectionSummary(runnerImages.promotedImage, t)}</p>
              </div>
              <div className="rounded-xl bg-muted/20 px-4 py-3 text-sm text-muted-foreground">
                {t('runnerImages.selection.note')}
              </div>
            </CardContent>
          </Card>

          <Card className="border bg-card/95">
            <CardHeader className="border-b">
              <div className="flex flex-col gap-1">
                <CardTitle>{t('runnerImages.recipes.title')}</CardTitle>
                <CardDescription>{t('runnerImages.recipes.description')}</CardDescription>
              </div>
            </CardHeader>
            <CardContent className="pt-4">
              {!runnerImages.recipes.length ? (
                <EmptyBlock title={t('runnerImages.recipes.emptyTitle')} body={t('runnerImages.recipes.emptyBody')} />
              ) : (
                <div className="flex flex-col divide-y rounded-xl border bg-background/70">
                  {runnerImages.recipes.map((recipe) => {
                    const building = runnerImageBuildingId === String(recipe.id);
                    const deleting = runnerImageRecipeDeletingId === String(recipe.id);

                    return (
                      <div key={recipe.id} className="flex flex-col gap-3 px-4 py-4">
                        <div className="flex items-start justify-between gap-3">
                          <div className="min-w-0">
                            <p className="truncate text-sm font-medium">{recipe.name}</p>
                            <p className="mt-1 text-sm text-muted-foreground">
                              {recipe.description || compactValue(recipe.baseImage)}
                            </p>
                          </div>
                          {editingRunnerImageRecipeId === String(recipe.id) ? (
                            <Badge variant="secondary">{t('common.current')}</Badge>
                          ) : null}
                        </div>
                        <div className="flex flex-wrap gap-2 text-xs text-muted-foreground">
                          <span>{recipe.shape}</span>
                          <span>{t('runnerImages.recipe.capacity', { ocpu: recipe.ocpu, memoryGb: recipe.memoryGb })}</span>
                        </div>
                        <div className="flex flex-wrap gap-2">
                          <Button
                            variant="outline"
                            size="sm"
                            disabled={building}
                            aria-busy={building}
                            onClick={() => onBuildCreate(recipe.id)}
                          >
                            <BusyButtonContent
                              busy={building}
                              label={t('runnerImages.actions.startBuild')}
                              icon={ImageUpIcon}
                            />
                          </Button>
                          <Button variant="ghost" size="sm" onClick={() => onRecipeEdit(recipe)}>
                            {t('runnerImages.actions.edit')}
                          </Button>
                          <Button
                            variant="ghost"
                            size="sm"
                            disabled={deleting}
                            aria-busy={deleting}
                            onClick={() => onRecipeDelete(recipe.id)}
                          >
                            <BusyButtonContent
                              busy={deleting}
                              label={t('runnerImages.actions.delete')}
                              icon={Trash2Icon}
                            />
                          </Button>
                        </div>
                      </div>
                    );
                  })}
                </div>
              )}
            </CardContent>
          </Card>

          <Card className="border bg-card/95">
            <CardHeader className="border-b">
              <div className="flex flex-col gap-1">
                <CardTitle>
                  {editingRunnerImageRecipeId
                    ? t('runnerImages.form.editTitle')
                    : t('runnerImages.form.createTitle')}
                </CardTitle>
                <CardDescription>{t('runnerImages.form.description')}</CardDescription>
              </div>
            </CardHeader>
            <CardContent className="pt-4">
              <form className="flex flex-col gap-5" onSubmit={onRecipeSubmit}>
                <FieldSet>
                  <FieldGroup className="md:grid md:grid-cols-2">
                    <Field>
                      <FieldLabel htmlFor="runner-image-name">{t('runnerImages.form.name')}</FieldLabel>
                      <Input
                        id="runner-image-name"
                        value={runnerImageRecipeForm.name}
                        onChange={(event) => setRunnerImageRecipeForm((current) => ({ ...current, name: event.target.value }))}
                        placeholder="node22"
                      />
                    </Field>
                    <Field>
                      <FieldLabel htmlFor="runner-image-display-name">{t('runnerImages.form.displayName')}</FieldLabel>
                      <Input
                        id="runner-image-display-name"
                        value={runnerImageRecipeForm.displayName}
                        onChange={(event) => setRunnerImageRecipeForm((current) => ({ ...current, displayName: event.target.value }))}
                        placeholder="ohoci-node22"
                      />
                    </Field>
                    <Field className="md:col-span-2">
                      <FieldLabel htmlFor="runner-image-description">{t('runnerImages.form.summary')}</FieldLabel>
                      <Input
                        id="runner-image-description"
                        value={runnerImageRecipeForm.description}
                        onChange={(event) => setRunnerImageRecipeForm((current) => ({ ...current, description: event.target.value }))}
                        placeholder={t('runnerImages.form.summaryPlaceholder')}
                      />
                    </Field>
                    <Field className="md:col-span-2">
                      <FieldLabel htmlFor="runner-image-base">{t('runnerImages.form.baseImage')}</FieldLabel>
                      <Input
                        id="runner-image-base"
                        value={runnerImageRecipeForm.baseImage}
                        onChange={(event) => setRunnerImageRecipeForm((current) => ({ ...current, baseImage: event.target.value }))}
                        placeholder="ocid1.image.oc1..base"
                      />
                      <FieldDescription>{t('runnerImages.form.baseImageDescription')}</FieldDescription>
                    </Field>
                    <Field>
                      <FieldLabel htmlFor="runner-image-subnet">{t('runnerImages.form.subnet')}</FieldLabel>
                      <Input
                        id="runner-image-subnet"
                        value={runnerImageRecipeForm.subnetOcid}
                        onChange={(event) => setRunnerImageRecipeForm((current) => ({ ...current, subnetOcid: event.target.value }))}
                        placeholder={t('runnerImages.form.subnetPlaceholder')}
                      />
                      <FieldDescription>{t('runnerImages.form.subnetDescription')}</FieldDescription>
                    </Field>
                    <Field>
                      <FieldLabel htmlFor="runner-image-shape">{t('runnerImages.form.shape')}</FieldLabel>
                      <Input
                        id="runner-image-shape"
                        value={runnerImageRecipeForm.shape}
                        onChange={(event) => setRunnerImageRecipeForm((current) => ({ ...current, shape: event.target.value }))}
                        placeholder="VM.Standard.E4.Flex"
                      />
                    </Field>
                    <Field>
                      <FieldLabel htmlFor="runner-image-ocpu">{t('runnerImages.form.ocpu')}</FieldLabel>
                      <Input
                        id="runner-image-ocpu"
                        type="number"
                        min="1"
                        step="1"
                        value={runnerImageRecipeForm.ocpu}
                        onChange={(event) => setRunnerImageRecipeForm((current) => ({ ...current, ocpu: event.target.value }))}
                      />
                    </Field>
                    <Field>
                      <FieldLabel htmlFor="runner-image-memory">{t('runnerImages.form.memory')}</FieldLabel>
                      <Input
                        id="runner-image-memory"
                        type="number"
                        min="1"
                        step="1"
                        value={runnerImageRecipeForm.memoryGb}
                        onChange={(event) => setRunnerImageRecipeForm((current) => ({ ...current, memoryGb: event.target.value }))}
                      />
                    </Field>
                    <Field className="md:col-span-2">
                      <FieldLabel htmlFor="runner-image-setup">{t('runnerImages.form.setupCommands')}</FieldLabel>
                      <Textarea
                        id="runner-image-setup"
                        value={runnerImageRecipeForm.setupCommandsText}
                        onChange={(event) => setRunnerImageRecipeForm((current) => ({ ...current, setupCommandsText: event.target.value }))}
                        rows={7}
                        placeholder={'sudo apt-get update\nsudo apt-get install -y docker.io'}
                      />
                      <FieldDescription>{t('runnerImages.form.setupDescription')}</FieldDescription>
                    </Field>
                    <Field className="md:col-span-2">
                      <FieldLabel htmlFor="runner-image-verify">{t('runnerImages.form.verifyCommands')}</FieldLabel>
                      <Textarea
                        id="runner-image-verify"
                        value={runnerImageRecipeForm.verifyCommandsText}
                        onChange={(event) => setRunnerImageRecipeForm((current) => ({ ...current, verifyCommandsText: event.target.value }))}
                        rows={5}
                        placeholder={'docker --version\nnode --version'}
                      />
                      <FieldDescription>{t('runnerImages.form.verifyDescription')}</FieldDescription>
                    </Field>
                  </FieldGroup>
                </FieldSet>
                <div className="flex flex-wrap justify-end gap-2">
                  {editingRunnerImageRecipeId ? (
                    <Button type="button" variant="ghost" onClick={onRecipeCancel}>
                      {t('runnerImages.actions.cancel')}
                    </Button>
                  ) : null}
                  <Button type="submit" disabled={runnerImageRecipeSaving} aria-busy={runnerImageRecipeSaving}>
                    <BusyButtonContent
                      busy={runnerImageRecipeSaving}
                      label={t('runnerImages.actions.saveRecipe')}
                      icon={BoxIcon}
                    />
                  </Button>
                </div>
              </form>
            </CardContent>
          </Card>
        </div>
        </div>
      )}
    </div>
  );
}
