import { NetworkIcon, RefreshCwIcon, ShieldCheckIcon } from 'lucide-react';

import { EmptyBlock, ErrorBlock, LoadingBlock, StatCard } from '@/components/app/display-primitives';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle
} from '@/components/ui/card';
import { Checkbox } from '@/components/ui/checkbox';
import {
  Field,
  FieldContent,
  FieldDescription,
  FieldError,
  FieldGroup,
  FieldLabel,
  FieldLegend,
  FieldSet
} from '@/components/ui/field';
import { Input } from '@/components/ui/input';
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { Textarea } from '@/components/ui/textarea';
import { useI18n } from '@/i18n';
import { normalizeOperatorList, normalizeOperatorText } from '@/lib/operator-text';
import { DEFAULT_SUBNET_VALUE } from '@/lib/workspace-constants';
import { describeSubnet, formatDateTime, formatNumber, shapeOptionLabel, subnetOptionLabel } from '@/lib/workspace-formatters';

const DIAGNOSTIC_STAGE_ORDER = [
  'setup_ready',
  'repo_allowed',
  'policy_match',
  'capacity_ok',
  'budget_ok',
  'warm_candidate',
  'launch_required',
  'runner_registration',
  'runner_attachment',
  'cleanup'
];

function diagnosticStateVariant(state) {
  switch (String(state || '').trim().toLowerCase()) {
    case 'blocked':
      return 'destructive';
    case 'passed':
      return 'secondary';
    case 'degraded':
      return 'outline';
    case 'skipped':
      return 'outline';
    default:
      return 'outline';
  }
}

function summarizeDiagnosticSummary(summaryCode, t) {
  return normalizeOperatorText(summaryCode, { keyPrefixes: ['operator.diagnostic.summary'] }) || t('common.notSet');
}

function summarizeDiagnosticStage(stageName, t) {
  return normalizeOperatorText(stageName, { keyPrefixes: ['operator.diagnostic.stage'] }) || t('common.notSet');
}

function summarizeDiagnosticCode(code) {
  return normalizeOperatorText(code, {
    keyPrefixes: ['operator.diagnostic.code', 'operator.billing.reason', 'operator.githubDrift.issue']
  });
}

export function PoliciesView({
  viewState,
  editingPolicyId,
  policyForm,
  setPolicyForm,
  onSubmit,
  onRefreshSubnets,
  onCancelEdit,
  policies,
  subnetCandidates,
  subnetById,
  defaultSubnetId,
  subnetError,
  recommendedSubnets,
  policyCatalog,
  policyValidation,
  billingGuardrails,
  policyCompatibilityForm,
  setPolicyCompatibilityForm,
  policyCompatibilityResult,
  policyCompatibilityLoading,
  policyCompatibilityError,
  onCheckCompatibility,
  onUseCurrentPolicyLabels,
  onPolicyShapeChange,
  onEditPolicy,
  onDeletePolicy
}) {
  const { t } = useI18n();
  const enabledPolicies = policies.filter((item) => item.enabled).length;
  const spotPolicies = policies.filter((item) => item.spot).length;
  const warmPolicies = policies.filter((item) => item.warmEnabled && item.warmMinIdle > 0).length;
  const budgetPolicies = policies.filter((item) => item.budgetEnabled).length;
  const recommendedNote = subnetError
    ? t('policies.stats.subnetSuggestions.note.unavailable')
    : describeSubnet('', subnetById, defaultSubnetId);
  const policyFieldErrors = policyValidation.fieldErrors || {};
  const selectedShape = policyValidation.selectedShape;
  const fixedShape = Boolean(selectedShape && !selectedShape.isFlexible);
  const staleShape = Boolean(policyForm.shape) && !selectedShape;
  const displayedOcpu = fixedShape ? (selectedShape.defaultOcpu ?? policyForm.ocpu) : policyForm.ocpu;
  const displayedMemoryGb = fixedShape ? (selectedShape.defaultMemoryGb ?? policyForm.memoryGb) : policyForm.memoryGb;
  const policySaveDisabled = !policyValidation.canSave;
  const guardrailByPolicyId = new Map((billingGuardrails?.items || []).map((item) => [item.policyId, item]));

  return (
    <div className="flex flex-col gap-6">
      <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-6">
        <StatCard label={t('policies.stats.total.label')} value={policies.length} note={t('policies.stats.total.note')} />
        <StatCard
          label={t('policies.stats.enabled.label')}
          value={enabledPolicies}
          note={t('policies.stats.enabled.note')}
          accent={enabledPolicies > 0}
        />
        <StatCard label={t('policies.stats.spot.label')} value={spotPolicies} note={t('policies.stats.spot.note')} />
        <StatCard
          label={t('policies.stats.warm.label')}
          value={warmPolicies}
          note={t('policies.stats.warm.note')}
          accent={warmPolicies > 0}
        />
        <StatCard
          label={t('policies.stats.budget.label')}
          value={budgetPolicies}
          note={t('policies.stats.budget.note')}
          accent={budgetPolicies > 0}
        />
        <StatCard
          label={t('policies.stats.subnetSuggestions.label')}
          value={recommendedSubnets.length}
          note={recommendedNote}
          accent={recommendedSubnets.length > 0 && !subnetError}
        />
      </div>

      <div className="grid gap-6 xl:grid-cols-[minmax(0,1.1fr)_minmax(340px,0.9fr)]">
        <Card className="border bg-card/95">
          <CardHeader className="border-b">
            <div>
              <CardTitle>{t('policies.title')}</CardTitle>
              <CardDescription>{t('policies.description')}</CardDescription>
            </div>
          </CardHeader>
          <CardContent className="flex flex-col gap-4 pt-4">
            {viewState?.status === 'loading' ? (
              <LoadingBlock
                title={t('viewState.loading.title', { view: t('policies.title') })}
                body={t('viewState.loading.body', { view: t('policies.title') })}
              />
            ) : viewState?.status === 'error' ? (
              <ErrorBlock
                title={t('viewState.error.title', { view: t('policies.title') })}
                body={viewState.error || t('viewState.error.body')}
              />
            ) : viewState?.status === 'empty' ? (
              <>
                <EmptyBlock title={t('policies.empty.title')} body={t('policies.empty.body')} />
                <div className="grid gap-3 md:grid-cols-2">
                  <Alert>
                    <NetworkIcon />
                    <AlertTitle>{t('policies.empty.alert.match.title')}</AlertTitle>
                    <AlertDescription>{t('policies.empty.alert.match.body')}</AlertDescription>
                  </Alert>
                  <Alert>
                    <NetworkIcon />
                    <AlertTitle>{t('policies.empty.alert.defaultSubnet.title')}</AlertTitle>
                    <AlertDescription>{describeSubnet('', subnetById, defaultSubnetId)}</AlertDescription>
                  </Alert>
                </div>
              </>
            ) : (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t('policies.table.labels')}</TableHead>
                    <TableHead>{t('policies.table.network')}</TableHead>
                    <TableHead>{t('policies.table.capacity')}</TableHead>
                    <TableHead>{t('policies.table.state')}</TableHead>
                    <TableHead className="text-right">{t('policies.table.actions')}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {policies.map((item) => {
                    const guardrail = guardrailByPolicyId.get(item.id);

                    return (
                    <TableRow key={item.id}>
                      <TableCell className="align-top">
                        <div className="flex flex-col gap-2">
                          <div className="flex flex-wrap gap-1">
                            {(item.labels || []).map((label) => (
                              <Badge key={label} variant="outline">{label}</Badge>
                            ))}
                          </div>
                          <p className="text-xs text-muted-foreground">{t('policies.table.labelsNote')}</p>
                        </div>
                      </TableCell>
                      <TableCell className="max-w-64 align-top whitespace-normal">
                        <div className="flex flex-col gap-1">
                          <span className="font-medium">{describeSubnet(item.subnetOcid, subnetById, defaultSubnetId)}</span>
                          <span className="text-xs text-muted-foreground">
                            {item.subnetOcid
                              ? t('policies.table.networkNote.custom')
                              : t('policies.table.networkNote.default')}
                          </span>
                        </div>
                      </TableCell>
                      <TableCell className="align-top">
                        <div className="flex flex-col gap-1">
                          <span className="font-medium">{item.shape}</span>
                          <span className="text-sm text-muted-foreground">
                            {t('policies.table.capacityValue', {
                              ocpu: item.ocpu,
                              memoryGb: item.memoryGb,
                              maxRunners: item.maxRunners
                            })}
                          </span>
                        </div>
                      </TableCell>
                      <TableCell className="align-top">
                        <div className="flex flex-col gap-1">
                          <div>
                            <Badge variant={item.enabled ? 'secondary' : 'outline'}>
                              {item.enabled ? t('common.ready') : t('policies.table.stateValue.off')}
                            </Badge>
                          </div>
                          {item.spot ? <span className="text-xs text-muted-foreground">{t('policies.table.spotAllowed')}</span> : null}
                          {item.warmEnabled ? (
                            <span className="text-xs text-muted-foreground">
                              {t('policies.table.warmValue', {
                                minIdle: item.warmMinIdle,
                                minutes: item.warmTtlMinutes
                              })}
                            </span>
                          ) : null}
                          {item.budgetEnabled ? (
                            <div className="flex flex-wrap items-center gap-2">
                              <Badge
                                variant={
                                  guardrail?.blocked
                                    ? 'destructive'
                                    : guardrail?.degraded
                                      ? 'outline'
                                      : 'secondary'
                                }
                              >
                                {guardrail?.blocked
                                  ? t('policies.table.budgetState.blocked')
                                  : guardrail?.degraded
                                    ? t('policies.table.budgetState.degraded')
                                    : t('policies.table.budgetState.ready')}
                              </Badge>
                              <span className="text-xs text-muted-foreground">
                                {t('policies.table.budgetValue', {
                                  amount: formatNumber(item.budgetCapAmount, 2),
                                  days: item.budgetWindowDays
                                })}
                              </span>
                            </div>
                          ) : null}
                          {guardrail?.message ? (
                            <span className="text-xs text-muted-foreground">{guardrail.message}</span>
                          ) : null}
                          <span className="text-xs text-muted-foreground">{t('policies.table.ttl', { minutes: item.ttlMinutes })}</span>
                        </div>
                      </TableCell>
                      <TableCell>
                        <div className="flex justify-end gap-2">
                          <Button variant="ghost" size="sm" onClick={() => onEditPolicy(item)}>
                            {t('policies.actions.edit')}
                          </Button>
                          <Button variant="ghost" size="sm" onClick={() => void onDeletePolicy(item.id)}>
                            {t('policies.actions.delete')}
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
          <CardFooter className="justify-between gap-3 text-sm text-muted-foreground">
            <p>{t('policies.footer.matchingHint')}</p>
            <p>
              {recommendedSubnets.length > 0
                ? t('policies.footer.suggested', {
                  count: recommendedSubnets.length,
                  suffix: recommendedSubnets.length === 1 ? '' : 's'
                })
                : t('policies.footer.none')}
            </p>
          </CardFooter>
        </Card>

        <div className="flex flex-col gap-6">
        <Card className="border bg-card/95">
          <Tabs defaultValue="match" className="gap-0">
            <CardHeader className="border-b">
              <div>
                <CardTitle>{editingPolicyId ? t('policies.form.editTitle') : t('policies.form.createTitle')}</CardTitle>
                <CardDescription>{t('policies.form.description')}</CardDescription>
              </div>
              <CardAction className="col-start-1 row-start-3 w-full justify-self-stretch md:col-start-2 md:row-span-2 md:row-start-1 md:w-auto md:justify-self-end">
                <TabsList variant="line" className="w-full sm:w-auto">
                  <TabsTrigger value="match">{t('policies.tabs.match')}</TabsTrigger>
                  <TabsTrigger value="network">{t('policies.tabs.network')}</TabsTrigger>
                  <TabsTrigger value="capacity">{t('policies.tabs.capacity')}</TabsTrigger>
                </TabsList>
              </CardAction>
            </CardHeader>
            <CardContent className="pt-4">
              <form className="flex flex-col gap-5" onSubmit={onSubmit}>
                <Alert>
                  <NetworkIcon />
                  <AlertTitle>{t('policies.form.alert.matchTitle')}</AlertTitle>
                  <AlertDescription>{t('policies.form.alert.matchBody')}</AlertDescription>
                </Alert>

                {policyValidation.settingsMessage ? (
                  <Alert>
                    <NetworkIcon />
                    <AlertTitle>{t('policies.form.alert.settingsTitle')}</AlertTitle>
                    <AlertDescription>{policyValidation.settingsMessage}</AlertDescription>
                  </Alert>
                ) : null}

                {!policyValidation.settingsMessage && policyCatalog.loaded ? (
                  <Alert>
                    <NetworkIcon />
                    <AlertTitle>{t('policies.form.alert.catalogTitle')}</AlertTitle>
                    <AlertDescription>
                      {t('policies.form.alert.catalogBody', {
                        region: policyCatalog.sourceRegion || t('policies.form.alert.catalogRegionFallback'),
                        validatedAt: policyCatalog.validatedAt
                          ? formatDateTime(policyCatalog.validatedAt)
                          : t('policies.form.alert.catalogTimestampFallback')
                      })}
                    </AlertDescription>
                  </Alert>
                ) : null}

                <TabsContent value="match" className="mt-0">
                  <FieldGroup className="md:grid md:grid-cols-2">
                    <Field className="md:col-span-2">
                      <FieldLabel htmlFor="policy-labels">{t('policies.form.labels')}</FieldLabel>
                      <Input
                        id="policy-labels"
                        value={policyForm.labels}
                        onChange={(event) => setPolicyForm((current) => ({ ...current, labels: event.target.value }))}
                        placeholder={t('policies.form.labelsPlaceholder')}
                      />
                      <FieldDescription>{t('policies.form.labelsDescription')}</FieldDescription>
                    </Field>

                    <Field orientation="horizontal" className="rounded-xl border bg-background/70 p-3 md:col-span-2">
                      <Checkbox
                        id="policy-enabled"
                        checked={policyForm.enabled}
                        onCheckedChange={(checked) => setPolicyForm((current) => ({ ...current, enabled: checked === true }))}
                      />
                      <FieldContent>
                        <FieldLabel htmlFor="policy-enabled">{t('policies.form.enabled')}</FieldLabel>
                        <FieldDescription>{t('policies.form.enabledDescription')}</FieldDescription>
                      </FieldContent>
                    </Field>
                  </FieldGroup>
                </TabsContent>

                <TabsContent value="network" className="mt-0">
                  <FieldGroup className="md:grid md:grid-cols-2">
                    <Field className="md:col-span-2">
                      <FieldLabel id="policy-runner-subnet-label" htmlFor="policy-runner-subnet">
                        {t('policies.form.runnerSubnet')}
                      </FieldLabel>
                      <Select
                        value={policyForm.subnetOcid || DEFAULT_SUBNET_VALUE}
                        onValueChange={(value) => setPolicyForm((current) => ({ ...current, subnetOcid: value === DEFAULT_SUBNET_VALUE ? '' : value }))}
                      >
                        <SelectTrigger
                          id="policy-runner-subnet"
                          aria-labelledby="policy-runner-subnet-label"
                          className="w-full"
                        >
                          <SelectValue placeholder={t('policies.form.runnerSubnetPlaceholder')} />
                        </SelectTrigger>
                        <SelectContent align="start">
                          <SelectGroup>
                            <SelectItem value={DEFAULT_SUBNET_VALUE}>{describeSubnet('', subnetById, defaultSubnetId)}</SelectItem>
                            {subnetCandidates.map((item) => (
                              <SelectItem key={item.id} value={item.id}>
                                {subnetOptionLabel(item)}
                              </SelectItem>
                            ))}
                          </SelectGroup>
                        </SelectContent>
                      </Select>
                      <FieldDescription>{t('policies.form.runnerSubnetDescription')}</FieldDescription>
                    </Field>

                    {subnetError ? (
                      <div className="md:col-span-2 rounded-xl border bg-muted/30 px-4 py-3 text-sm text-muted-foreground">
                        {t('policies.form.subnetUnavailable')}
                      </div>
                    ) : null}

                    {recommendedSubnets.length ? (
                      <Alert className="md:col-span-2">
                        <NetworkIcon />
                        <AlertTitle>{t('policies.form.suggestedSubnetsTitle')}</AlertTitle>
                        <AlertDescription>
                          {recommendedSubnets.slice(0, 2).map((item) => item.displayName || item.id).join(', ')}
                        </AlertDescription>
                      </Alert>
                    ) : null}
                  </FieldGroup>
                </TabsContent>

                <TabsContent value="capacity" className="mt-0">
                  <FieldGroup className="md:grid md:grid-cols-2">
                    <Field>
                      <FieldLabel htmlFor="policy-shape">{t('policies.form.shape')}</FieldLabel>
                      <Select
                        value={policyForm.shape || undefined}
                        onValueChange={onPolicyShapeChange}
                        disabled={Boolean(policyValidation.settingsMessage)}
                      >
                        <SelectTrigger id="policy-shape" className="w-full">
                          <SelectValue placeholder={t('policies.form.shapePlaceholder')} />
                        </SelectTrigger>
                        <SelectContent align="start">
                          <SelectGroup>
                            {staleShape ? (
                              <SelectItem value={policyForm.shape}>
                                {t('policies.form.staleShape', { shape: policyForm.shape })}
                              </SelectItem>
                            ) : null}
                            {policyCatalog.shapes.map((item) => (
                              <SelectItem key={item.shape} value={item.shape}>
                                {shapeOptionLabel(item)}
                              </SelectItem>
                            ))}
                          </SelectGroup>
                        </SelectContent>
                      </Select>
                      <FieldDescription>{t('policies.form.shapeDescription')}</FieldDescription>
                      <FieldError>{policyFieldErrors.shape}</FieldError>
                    </Field>
                    <Field>
                      <FieldLabel htmlFor="policy-ttl">{t('policies.form.ttl')}</FieldLabel>
                      <Input
                        id="policy-ttl"
                        type="number"
                        min="1"
                        value={policyForm.ttlMinutes}
                        onChange={(event) => setPolicyForm((current) => ({ ...current, ttlMinutes: event.target.value }))}
                      />
                    </Field>
                    <Field>
                      <FieldLabel htmlFor="policy-ocpu">{t('policies.form.ocpu')}</FieldLabel>
                      <Input
                        id="policy-ocpu"
                        type="number"
                        min="1"
                        value={displayedOcpu}
                        readOnly={fixedShape}
                        onChange={(event) => setPolicyForm((current) => ({ ...current, ocpu: event.target.value }))}
                      />
                      <FieldDescription>
                        {fixedShape
                          ? t('policies.form.ocpuFixedDescription')
                          : policyValidation.capacityMessage || t('policies.form.capacityFallback')}
                      </FieldDescription>
                      <FieldError>{policyFieldErrors.ocpu}</FieldError>
                    </Field>
                    <Field>
                      <FieldLabel htmlFor="policy-memory">{t('policies.form.memory')}</FieldLabel>
                      <Input
                        id="policy-memory"
                        type="number"
                        min="1"
                        value={displayedMemoryGb}
                        readOnly={fixedShape}
                        onChange={(event) => setPolicyForm((current) => ({ ...current, memoryGb: event.target.value }))}
                      />
                      <FieldDescription>
                        {fixedShape
                          ? t('policies.form.memoryFixedDescription')
                          : policyValidation.capacityMessage || t('policies.form.capacityFallback')}
                      </FieldDescription>
                      <FieldError>{policyFieldErrors.memoryGb}</FieldError>
                    </Field>
                    <Field className="md:col-span-2">
                      <FieldLabel htmlFor="policy-max-runners">{t('policies.form.maxRunners')}</FieldLabel>
                      <Input
                        id="policy-max-runners"
                        type="number"
                        min="1"
                        value={policyForm.maxRunners}
                        onChange={(event) => setPolicyForm((current) => ({ ...current, maxRunners: event.target.value }))}
                      />
                    </Field>
                  </FieldGroup>

                  <FieldSet>
                    <FieldLegend variant="label">{t('policies.form.options')}</FieldLegend>
                    <div className="grid gap-3 sm:grid-cols-2">
                      <Field orientation="horizontal" className="rounded-xl border bg-background/70 p-3">
                        <Checkbox
                          id="policy-spot"
                          checked={policyForm.spot}
                          onCheckedChange={(checked) => setPolicyForm((current) => ({ ...current, spot: checked === true }))}
                        />
                        <FieldContent>
                          <FieldLabel htmlFor="policy-spot">{t('policies.form.spot')}</FieldLabel>
                          <FieldDescription>{t('policies.form.spotDescription')}</FieldDescription>
                        </FieldContent>
                      </Field>
                    </div>
                  </FieldSet>

                  <FieldSet>
                    <FieldLegend variant="label">{t('policies.form.warm.title')}</FieldLegend>
                    <div className="grid gap-3">
                      <Field orientation="horizontal" className="rounded-xl border bg-background/70 p-3">
                        <Checkbox
                          id="policy-warm-enabled"
                          checked={policyForm.warmEnabled}
                          onCheckedChange={(checked) => setPolicyForm((current) => ({ ...current, warmEnabled: checked === true }))}
                        />
                        <FieldContent>
                          <FieldLabel htmlFor="policy-warm-enabled">{t('policies.form.warm.enabled')}</FieldLabel>
                          <FieldDescription>{t('policies.form.warm.enabledDescription')}</FieldDescription>
                        </FieldContent>
                      </Field>

                      <FieldGroup className="md:grid md:grid-cols-2">
                        <Field>
                          <FieldLabel htmlFor="policy-warm-min-idle">{t('policies.form.warm.minIdle')}</FieldLabel>
                          <Input
                            id="policy-warm-min-idle"
                            type="number"
                            min="0"
                            max="1"
                            step="1"
                            value={policyForm.warmMinIdle}
                            disabled={!policyForm.warmEnabled}
                            onChange={(event) => setPolicyForm((current) => ({ ...current, warmMinIdle: event.target.value }))}
                          />
                          <FieldDescription>{t('policies.form.warm.minIdleDescription')}</FieldDescription>
                          <FieldError>{policyFieldErrors.warmMinIdle}</FieldError>
                        </Field>

                        <Field>
                          <FieldLabel htmlFor="policy-warm-ttl">{t('policies.form.warm.ttl')}</FieldLabel>
                          <Input
                            id="policy-warm-ttl"
                            type="number"
                            min="1"
                            value={policyForm.warmTtlMinutes}
                            disabled={!policyForm.warmEnabled}
                            onChange={(event) => setPolicyForm((current) => ({ ...current, warmTtlMinutes: event.target.value }))}
                          />
                          <FieldDescription>{t('policies.form.warm.ttlDescription')}</FieldDescription>
                          <FieldError>{policyFieldErrors.warmTtlMinutes}</FieldError>
                        </Field>

                        <Field className="md:col-span-2">
                          <FieldLabel htmlFor="policy-warm-allowlist">{t('policies.form.warm.repoAllowlist')}</FieldLabel>
                          <Textarea
                            id="policy-warm-allowlist"
                            rows={4}
                            value={policyForm.warmRepoAllowlistText}
                            disabled={!policyForm.warmEnabled}
                            onChange={(event) => setPolicyForm((current) => ({ ...current, warmRepoAllowlistText: event.target.value }))}
                            placeholder={t('policies.form.warm.repoAllowlistPlaceholder')}
                          />
                          <FieldDescription>{t('policies.form.warm.repoAllowlistDescription')}</FieldDescription>
                          <FieldError>{policyFieldErrors.warmRepoAllowlistText}</FieldError>
                        </Field>
                      </FieldGroup>
                    </div>
                  </FieldSet>

                  <FieldSet>
                    <FieldLegend variant="label">{t('policies.form.budget.title')}</FieldLegend>
                    <div className="grid gap-3">
                      <Field orientation="horizontal" className="rounded-xl border bg-background/70 p-3">
                        <Checkbox
                          id="policy-budget-enabled"
                          checked={policyForm.budgetEnabled}
                          onCheckedChange={(checked) => setPolicyForm((current) => ({ ...current, budgetEnabled: checked === true }))}
                        />
                        <FieldContent>
                          <FieldLabel htmlFor="policy-budget-enabled">{t('policies.form.budget.enabled')}</FieldLabel>
                          <FieldDescription>{t('policies.form.budget.enabledDescription')}</FieldDescription>
                        </FieldContent>
                      </Field>

                      <FieldGroup className="md:grid md:grid-cols-2">
                        <Field>
                          <FieldLabel htmlFor="policy-budget-cap">{t('policies.form.budget.capAmount')}</FieldLabel>
                          <Input
                            id="policy-budget-cap"
                            type="number"
                            min="0"
                            step="0.01"
                            value={policyForm.budgetCapAmount}
                            disabled={!policyForm.budgetEnabled}
                            onChange={(event) => setPolicyForm((current) => ({ ...current, budgetCapAmount: event.target.value }))}
                          />
                          <FieldDescription>{t('policies.form.budget.capAmountDescription')}</FieldDescription>
                          <FieldError>{policyFieldErrors.budgetCapAmount}</FieldError>
                        </Field>

                        <Field>
                          <FieldLabel htmlFor="policy-budget-window">{t('policies.form.budget.windowDays')}</FieldLabel>
                          <Input
                            id="policy-budget-window"
                            type="number"
                            min="7"
                            max="7"
                            value={policyForm.budgetWindowDays}
                            disabled
                            readOnly
                          />
                          <FieldDescription>{t('policies.form.budget.windowDaysDescription')}</FieldDescription>
                          <FieldError>{policyFieldErrors.budgetWindowDays}</FieldError>
                        </Field>
                      </FieldGroup>
                    </div>
                  </FieldSet>
                </TabsContent>

                <div className="flex flex-wrap gap-2 border-t pt-5">
                  <Button type="submit" disabled={policySaveDisabled}>
                    {editingPolicyId ? t('policies.actions.save') : t('policies.actions.create')}
                  </Button>
                  <Button type="button" variant="outline" onClick={() => void onRefreshSubnets()}>
                    <RefreshCwIcon data-icon="inline-start" />
                    {t('policies.actions.refreshSubnets')}
                  </Button>
                  {editingPolicyId ? (
                    <Button type="button" variant="ghost" onClick={onCancelEdit}>
                      {t('policies.actions.cancel')}
                    </Button>
                  ) : null}
                </div>
              </form>
            </CardContent>
          </Tabs>
        </Card>
          <Card className="border bg-card/95">
            <CardHeader className="border-b">
              <div>
                <CardTitle>{t('policies.compatibility.title')}</CardTitle>
                <CardDescription>{t('policies.compatibility.description')}</CardDescription>
              </div>
            </CardHeader>
            <CardContent className="flex flex-col gap-5 pt-4">
              <form className="flex flex-col gap-4" onSubmit={onCheckCompatibility}>
                <FieldGroup className="md:grid md:grid-cols-2">
                  <Field>
                    <FieldLabel htmlFor="policy-compat-repo-owner">{t('policies.compatibility.repoOwner')}</FieldLabel>
                    <Input
                      id="policy-compat-repo-owner"
                      value={policyCompatibilityForm.repoOwner}
                      onChange={(event) => setPolicyCompatibilityForm((current) => ({ ...current, repoOwner: event.target.value }))}
                      placeholder="example-org"
                    />
                  </Field>
                  <Field>
                    <FieldLabel htmlFor="policy-compat-repo-name">{t('policies.compatibility.repoName')}</FieldLabel>
                    <Input
                      id="policy-compat-repo-name"
                      value={policyCompatibilityForm.repoName}
                      onChange={(event) => setPolicyCompatibilityForm((current) => ({ ...current, repoName: event.target.value }))}
                      placeholder="repository"
                    />
                  </Field>
                  <Field className="md:col-span-2">
                    <FieldLabel htmlFor="policy-compat-labels">{t('policies.compatibility.labels')}</FieldLabel>
                    <Textarea
                      id="policy-compat-labels"
                      rows={3}
                      value={policyCompatibilityForm.labelsText}
                      onChange={(event) => setPolicyCompatibilityForm((current) => ({ ...current, labelsText: event.target.value }))}
                      placeholder={t('policies.form.labelsPlaceholder')}
                    />
                    <FieldDescription>{t('policies.compatibility.labelsDescription')}</FieldDescription>
                  </Field>
                </FieldGroup>

                <div className="flex flex-wrap gap-2">
                  <Button type="submit" disabled={policyCompatibilityLoading} aria-busy={policyCompatibilityLoading}>
                    <ShieldCheckIcon data-icon="inline-start" />
                    {t('policies.compatibility.actions.check')}
                  </Button>
                  <Button type="button" variant="outline" onClick={onUseCurrentPolicyLabels}>
                    {t('policies.compatibility.actions.useCurrentLabels')}
                  </Button>
                </div>
              </form>

              {policyCompatibilityLoading ? (
                <LoadingBlock
                  title={t('policies.compatibility.loading.title')}
                  body={t('policies.compatibility.loading.body')}
                />
              ) : null}

              {!policyCompatibilityLoading && policyCompatibilityError ? (
                <ErrorBlock
                  title={t('policies.compatibility.error.title')}
                  body={policyCompatibilityError}
                />
              ) : null}

              {!policyCompatibilityLoading && !policyCompatibilityError && !policyCompatibilityResult ? (
                <EmptyBlock
                  title={t('policies.compatibility.empty.title')}
                  body={t('policies.compatibility.empty.body')}
                />
              ) : null}

              {!policyCompatibilityLoading && !policyCompatibilityError && policyCompatibilityResult ? (
                <div className="flex flex-col gap-5">
                  <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-4">
                    <div className="rounded-xl border bg-background/70 px-4 py-3">
                      <p className="text-sm font-medium">{t('policies.compatibility.summary.summaryCode')}</p>
                      <p className="mt-1 text-sm text-muted-foreground">
                        {summarizeDiagnosticSummary(policyCompatibilityResult.summaryCode, t)}
                      </p>
                    </div>
                    <div className="rounded-xl border bg-background/70 px-4 py-3">
                      <p className="text-sm font-medium">{t('policies.compatibility.summary.blockingStage')}</p>
                      <p className="mt-1 text-sm text-muted-foreground">
                        {policyCompatibilityResult.blockingStage
                          ? summarizeDiagnosticStage(policyCompatibilityResult.blockingStage, t)
                          : t('policies.compatibility.summary.none')}
                      </p>
                    </div>
                    <div className="rounded-xl border bg-background/70 px-4 py-3">
                      <p className="text-sm font-medium">{t('policies.compatibility.summary.launch')}</p>
                      <p className="mt-1 text-sm text-muted-foreground">
                        {policyCompatibilityResult.launchRequired
                          ? t('policies.compatibility.summary.launchRequired')
                          : t('policies.compatibility.summary.launchNotRequired')}
                      </p>
                    </div>
                    <div className="rounded-xl border bg-background/70 px-4 py-3">
                      <p className="text-sm font-medium">{t('policies.compatibility.summary.requestedLabels')}</p>
                      <div className="mt-2 flex flex-wrap gap-1">
                        {policyCompatibilityResult.requestedLabels.length ? (
                          policyCompatibilityResult.requestedLabels.map((label) => (
                            <Badge key={label} variant="outline">{label}</Badge>
                          ))
                        ) : (
                          <span className="text-sm text-muted-foreground">{t('common.none')}</span>
                        )}
                      </div>
                    </div>
                  </div>

                  {policyCompatibilityResult.matchedPolicy ? (
                    <div className="rounded-xl border bg-background/70 px-4 py-4">
                      <div className="flex flex-wrap items-start justify-between gap-3">
                        <div>
                          <p className="text-sm font-medium">{t('policies.compatibility.match.title')}</p>
                          <p className="mt-1 text-sm text-muted-foreground">
                            {t('policies.compatibility.match.body', {
                              shape: policyCompatibilityResult.matchedPolicy.shape,
                              maxRunners: policyCompatibilityResult.matchedPolicy.maxRunners
                            })}
                          </p>
                        </div>
                        <div className="flex flex-wrap gap-1">
                          {(policyCompatibilityResult.matchedPolicy.labels || []).map((label) => (
                            <Badge key={label} variant="outline">{label}</Badge>
                          ))}
                        </div>
                      </div>
                      {policyCompatibilityResult.warmCandidate ? (
                        <p className="mt-3 text-sm text-muted-foreground">
                          {t('policies.compatibility.match.warmCandidate', {
                            runnerId: policyCompatibilityResult.warmCandidate.id || t('common.notSet')
                          })}
                        </p>
                      ) : null}
                    </div>
                  ) : null}

                  <div className="grid gap-3 md:grid-cols-2">
                    {DIAGNOSTIC_STAGE_ORDER
                      .filter((stageName) => policyCompatibilityResult.stageStatuses?.[stageName])
                      .map((stageName) => {
                        const stage = policyCompatibilityResult.stageStatuses[stageName];

                        return (
                          <div key={stageName} className="rounded-xl border bg-background/70 px-4 py-4">
                            <div className="flex items-center justify-between gap-3">
                              <p className="text-sm font-medium">{summarizeDiagnosticStage(stageName, t)}</p>
                              <Badge variant={diagnosticStateVariant(stage.state)}>
                                {normalizeOperatorText(stage.state, { keyPrefixes: ['formatter.status'] })}
                              </Badge>
                            </div>
                            <p className="mt-2 text-sm text-muted-foreground">{stage.message || t('common.notSet')}</p>
                            {stage.code ? (
                              <p className="mt-2 text-xs text-muted-foreground">
                                {summarizeDiagnosticCode(stage.code) || stage.code}
                              </p>
                            ) : null}
                            <p className="mt-2 text-xs text-muted-foreground">{formatDateTime(stage.updatedAt)}</p>
                          </div>
                        );
                      })}
                  </div>

                  <div className="rounded-xl border bg-background/70">
                    <div className="border-b px-4 py-3">
                      <p className="text-sm font-medium">{t('policies.compatibility.policyChecks.title')}</p>
                      <p className="text-sm text-muted-foreground">{t('policies.compatibility.policyChecks.description')}</p>
                    </div>
                    <div className="overflow-x-auto">
                      <Table>
                        <TableHeader>
                          <TableRow>
                            <TableHead>{t('policies.compatibility.policyChecks.policy')}</TableHead>
                            <TableHead>{t('policies.compatibility.policyChecks.match')}</TableHead>
                            <TableHead>{t('policies.compatibility.policyChecks.capacity')}</TableHead>
                            <TableHead>{t('policies.compatibility.policyChecks.budget')}</TableHead>
                            <TableHead>{t('policies.compatibility.policyChecks.warm')}</TableHead>
                            <TableHead>{t('policies.compatibility.policyChecks.notes')}</TableHead>
                          </TableRow>
                        </TableHeader>
                        <TableBody>
                          {policyCompatibilityResult.policyChecks.map((check) => (
                            <TableRow key={check.policyId || check.policyLabel}>
                              <TableCell className="align-top">
                                <div className="flex flex-col gap-2">
                                  <div className="flex flex-wrap gap-1">
                                    {(check.policyLabels || []).map((label) => (
                                      <Badge key={`${check.policyId}:${label}`} variant="outline">{label}</Badge>
                                    ))}
                                  </div>
                                  {check.policyLabel ? (
                                    <p className="text-xs text-muted-foreground">{check.policyLabel}</p>
                                  ) : null}
                                </div>
                              </TableCell>
                              <TableCell className="align-top">
                                <Badge variant={check.matched ? 'secondary' : 'outline'}>
                                  {check.matched ? t('policies.compatibility.policyChecks.matchYes') : t('policies.compatibility.policyChecks.matchNo')}
                                </Badge>
                              </TableCell>
                              <TableCell className="align-top text-sm text-muted-foreground">
                                {check.capacityBlocked
                                  ? t('policies.compatibility.policyChecks.capacityBlocked', {
                                      active: check.activeRunners,
                                      max: check.maxRunners
                                    })
                                  : t('policies.compatibility.policyChecks.capacityReady', {
                                      active: check.activeRunners,
                                      max: check.maxRunners
                                    })}
                              </TableCell>
                              <TableCell className="align-top text-sm text-muted-foreground">
                                {check.budgetBlocked
                                  ? t('policies.compatibility.policyChecks.budgetBlocked')
                                  : check.budgetDegraded
                                    ? t('policies.compatibility.policyChecks.budgetDegraded')
                                    : t('policies.compatibility.policyChecks.budgetReady')}
                              </TableCell>
                              <TableCell className="align-top text-sm text-muted-foreground">
                                {check.warmConfigured
                                  ? check.warmRepoEligible
                                    ? t('policies.compatibility.policyChecks.warmEligible')
                                    : t('policies.compatibility.policyChecks.warmConfigured')
                                  : t('policies.compatibility.policyChecks.warmDisabled')}
                              </TableCell>
                              <TableCell className="max-w-80 align-top whitespace-normal text-sm text-muted-foreground">
                                {normalizeOperatorList([
                                  ...(check.reasons || []),
                                  check.budgetMessage || ''
                                ]) || t('policies.compatibility.policyChecks.notesFallback')}
                              </TableCell>
                            </TableRow>
                          ))}
                        </TableBody>
                      </Table>
                    </div>
                  </div>
                </div>
              ) : null}
            </CardContent>
          </Card>
        </div>
      </div>
    </div>
  );
}
