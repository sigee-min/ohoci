import {
  RefreshCwIcon,
  ShieldCheckIcon,
  Trash2Icon,
  TriangleAlertIcon,
  UploadIcon,
} from "lucide-react";

import { BusyButtonContent } from "@/components/app/busy-button-content";
import { EmptyBlock, StatCard } from "@/components/app/display-primitives";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Field,
  FieldContent,
  FieldError,
  FieldGroup,
  FieldLabel,
} from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Separator } from "@/components/ui/separator";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Textarea } from "@/components/ui/textarea";
import { translateMaybeKey, useI18n } from "@/i18n";
import { cn } from "@/lib/utils";
import {
  catalogImageOptionLabel,
  catalogSubnetOptionLabel,
  compactValue,
  formatDateTime,
  summarizeOCIAuthMode,
} from "@/lib/workspace-formatters";

function summarizeOCIResultTitle(result, t) {
  return t("oci.result.verifiedTitle", {
    profile: result?.credential?.profileName || t("common.default"),
  });
}

function summarizeOCIResult(result, t) {
  if (!result) {
    return "";
  }

  const regions = Array.isArray(result.regionSubscriptions)
    ? result.regionSubscriptions.filter(Boolean)
    : [];
  const availabilityDomains = Array.isArray(result.availabilityDomains)
    ? result.availabilityDomains.filter(Boolean)
    : [];

  return [
    result.credential?.name
      ? t("oci.result.credential", { name: result.credential.name })
      : "",
    result.credential?.region
      ? t("oci.result.region", { region: result.credential.region })
      : "",
    t("oci.current.regions", {
      regions: regions.join(", ") || t("common.none"),
    }),
    t("oci.current.availabilityDomains", {
      domains: availabilityDomains.join(", ") || t("common.notSet"),
    }),
  ]
    .filter(Boolean)
    .join(" ");
}

const OCI_RUNTIME_CONTRACT_KEY_MAP = {
  OHOCI_OCI_COMPARTMENT_OCID: "oci.contract.compartmentOcid",
  OHOCI_OCI_AVAILABILITY_DOMAIN: "oci.contract.availabilityDomain",
  OHOCI_OCI_SUBNET_OCID: "oci.contract.subnetOcid",
  OHOCI_OCI_IMAGE_OCID: "oci.contract.imageOcid",
};

function formatOCIRuntimeMissing(values, t, locale) {
  return values
    .map((value) => {
      const normalizedValue = String(value || "").trim();
      if (!normalizedValue) {
        return "";
      }

      const contractKey = OCI_RUNTIME_CONTRACT_KEY_MAP[normalizedValue];
      if (contractKey) {
        return t(contractKey);
      }

      return translateMaybeKey(normalizedValue, {}, locale);
    })
    .filter(Boolean)
    .join(", ");
}

export function OCIAuthView({
  ociAuthForm,
  setOciAuthForm,
  onFileUpload,
  onTest,
  onSave,
  onClear,
  ociAuthTesting,
  ociAuthSaving,
  ociAuthClearing,
  ociAuthStatus,
  ociAuthResult,
  ociAuthInspecting,
  ociAuthInspectResult,
  ociRuntimeStatus,
  ociRuntimeForm,
  setOciRuntimeForm,
  runtimeCatalog,
  runtimeCatalogValidation,
  onRuntimeCatalogRefresh,
  onRuntimeSave,
  onRuntimeClear,
  ociRuntimeSaving,
  ociRuntimeClearing,
  title,
  description,
  mode = "settings",
}) {
  const { locale, t } = useI18n();
  const settingsMode = mode === "settings";
  const setupCredentialMode = mode === "setup-credential";
  const setupRuntimeMode = mode === "setup-runtime";
  const setupMode = setupCredentialMode || setupRuntimeMode;
  const resolvedTitle = title || t("oci.title");
  const resolvedDescription =
    description === undefined ? t("oci.description") : description;
  const credentialBusy = ociAuthTesting || ociAuthSaving || ociAuthClearing;
  const runtimeActionsBusy = ociRuntimeSaving || ociRuntimeClearing;
  const effectiveSettings = ociRuntimeStatus.effectiveSettings || {};
  const runtimeFieldErrors = runtimeCatalogValidation.fieldErrors || {};
  const runtimeSaveDisabled =
    runtimeActionsBusy || !runtimeCatalogValidation.canSave;
  const staleAvailabilityDomain =
    Boolean(ociRuntimeForm.availabilityDomain) &&
    !runtimeCatalog.availabilityDomains.includes(
      ociRuntimeForm.availabilityDomain,
    );
  const staleSubnet =
    Boolean(ociRuntimeForm.subnetOcid) &&
    !runtimeCatalog.subnets.some(
      (item) => item.id === ociRuntimeForm.subnetOcid,
    );
  const staleImage =
    Boolean(ociRuntimeForm.imageOcid) &&
    !runtimeCatalog.images.some((item) => item.id === ociRuntimeForm.imageOcid);
  const inspectSummary = ociAuthInspectResult
    ? [
        ociAuthInspectResult.profileName
          ? t("oci.inspect.profile", {
              profile: ociAuthInspectResult.profileName,
            })
          : "",
        ociAuthInspectResult.region
          ? t("oci.inspect.region", { region: ociAuthInspectResult.region })
          : "",
        ociAuthInspectResult.availableProfiles?.length
          ? t("oci.inspect.profileCount", {
              count: ociAuthInspectResult.availableProfiles.length,
            })
          : "",
      ]
        .filter(Boolean)
        .join(" · ")
    : "";
  const cacheCompatEnabled = Boolean(ociRuntimeForm.cacheCompatEnabled);
  const runtimeSaveButtonLabel = setupRuntimeMode
    ? t("oci.button.saveLaunchTarget")
    : t("oci.button.saveLaunchSettings");
  const credentialForm = (
    <form className="flex flex-col gap-5" onSubmit={onTest}>
      {settingsMode ? (
        <Alert>
          <ShieldCheckIcon />
          <AlertTitle>{t("oci.alert.storedSecurelyTitle")}</AlertTitle>
          <AlertDescription>
            {t("oci.alert.storedSecurelyBody")}
          </AlertDescription>
        </Alert>
      ) : null}

      {ociAuthInspecting ? (
        <Alert>
          <UploadIcon />
          <AlertTitle>{t("oci.alert.inspectingTitle")}</AlertTitle>
          <AlertDescription>{t("oci.alert.inspectingBody")}</AlertDescription>
        </Alert>
      ) : null}

      {ociAuthInspectResult ? (
        <Alert>
          <ShieldCheckIcon />
          <AlertTitle>
            {inspectSummary || t("oci.alert.inspectParsedFallback")}
          </AlertTitle>
          <AlertDescription>
            {ociAuthInspectResult.tenancyOcid
              ? `${t("oci.inspect.tenancy", { value: compactValue(ociAuthInspectResult.tenancyOcid) })} `
              : ""}
            {ociAuthInspectResult.userOcid
              ? `${t("oci.inspect.user", { value: compactValue(ociAuthInspectResult.userOcid) })} `
              : ""}
            {ociAuthInspectResult.fingerprint
              ? `${t("oci.inspect.fingerprint", { value: ociAuthInspectResult.fingerprint })} `
              : ""}
            {ociAuthInspectResult.keyFile
              ? t("oci.inspect.keyFile", {
                  value: ociAuthInspectResult.keyFile,
                })
              : t("oci.inspect.reviewValues")}
          </AlertDescription>
        </Alert>
      ) : null}

      <FieldGroup className="md:grid md:grid-cols-2">
        <Field>
          <FieldLabel htmlFor="credential-name">
            {t("oci.form.credentialName")}
          </FieldLabel>
          <Input
            id="credential-name"
            value={ociAuthForm.name}
            onChange={(event) =>
              setOciAuthForm((current) => ({
                ...current,
                name: event.target.value,
              }))
            }
            placeholder={t("oci.form.credentialNamePlaceholder")}
          />
        </Field>

        <Field>
          <FieldLabel htmlFor="profile-name">
            {t("oci.form.profileName")}
          </FieldLabel>
          <Input
            id="profile-name"
            value={ociAuthForm.profileName}
            onChange={(event) =>
              setOciAuthForm((current) => ({
                ...current,
                profileName: event.target.value,
              }))
            }
            placeholder={t("oci.form.profileNamePlaceholder")}
          />
        </Field>

        <Field>
          <FieldLabel htmlFor="oci-config-file">
            {t("oci.form.configFile")}
          </FieldLabel>
          <Input
            id="oci-config-file"
            type="file"
            accept=".conf,.config,.txt"
            onChange={(event) =>
              void onFileUpload("configText", event.target.files?.[0])
            }
          />
        </Field>

        <Field>
          <FieldLabel htmlFor="oci-private-key-file">
            {t("oci.form.privateKeyFile")}
          </FieldLabel>
          <Input
            id="oci-private-key-file"
            type="file"
            accept=".pem,.key,.txt"
            onChange={(event) =>
              void onFileUpload("privateKeyPem", event.target.files?.[0])
            }
          />
        </Field>

        <Field className="md:col-span-2">
          <FieldLabel htmlFor="oci-config-text">
            {t("oci.form.parsedConfig")}
          </FieldLabel>
          <Textarea
            id="oci-config-text"
            rows={10}
            value={ociAuthForm.configText}
            onChange={(event) =>
              setOciAuthForm((current) => ({
                ...current,
                configText: event.target.value,
              }))
            }
            placeholder="[DEFAULT]"
          />
        </Field>

        <Field className="md:col-span-2">
          <FieldLabel htmlFor="oci-private-key-pem">
            {t("oci.form.privateKeyPem")}
          </FieldLabel>
          <Textarea
            id="oci-private-key-pem"
            rows={12}
            value={ociAuthForm.privateKeyPem}
            onChange={(event) =>
              setOciAuthForm((current) => ({
                ...current,
                privateKeyPem: event.target.value,
              }))
            }
            placeholder="-----BEGIN PRIVATE KEY-----"
          />
        </Field>

        <Field className="md:col-span-2">
          <FieldLabel htmlFor="oci-passphrase">
            {t("oci.form.passphrase")}
          </FieldLabel>
          <Input
            id="oci-passphrase"
            type="password"
            value={ociAuthForm.passphrase}
            onChange={(event) =>
              setOciAuthForm((current) => ({
                ...current,
                passphrase: event.target.value,
              }))
            }
            placeholder={t("oci.form.passphrasePlaceholder")}
          />
        </Field>
      </FieldGroup>

      <div className="flex flex-wrap gap-2 border-t pt-5">
        <Button
          type="submit"
          variant="outline"
          disabled={credentialBusy}
          aria-busy={ociAuthTesting}
        >
          <BusyButtonContent
            busy={ociAuthTesting}
            label={t("oci.button.testConnection")}
            icon={ShieldCheckIcon}
          />
        </Button>
        <Button
          type="button"
          onClick={() => void onSave()}
          disabled={credentialBusy}
          aria-busy={ociAuthSaving}
        >
          <BusyButtonContent
            busy={ociAuthSaving}
            label={
              setupCredentialMode
                ? t("oci.button.saveCredential")
                : t("oci.button.saveAndUse")
            }
            icon={UploadIcon}
          />
        </Button>
        {settingsMode ? (
          <Button
            type="button"
            variant="ghost"
            onClick={() => void onClear()}
            disabled={credentialBusy}
            aria-busy={ociAuthClearing}
          >
            <BusyButtonContent
              busy={ociAuthClearing}
              label={t("oci.button.clearCredential")}
              icon={Trash2Icon}
            />
          </Button>
        ) : null}
      </div>
    </form>
  );

  const runtimeForm = (
    <form className="flex flex-col gap-5">
      <FieldGroup className="md:grid md:grid-cols-2">
        <Field className="md:col-span-2">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <FieldLabel htmlFor="runtime-compartment">
              {t("oci.form.compartmentOcid")}
            </FieldLabel>
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => void onRuntimeCatalogRefresh()}
              disabled={runtimeActionsBusy || runtimeCatalog.loading}
              aria-busy={runtimeCatalog.loading}
            >
              <BusyButtonContent
                busy={runtimeCatalog.loading}
                label={t("oci.button.refreshCatalog")}
                icon={RefreshCwIcon}
                busyIcon={RefreshCwIcon}
                spin
              />
            </Button>
          </div>
          <Input
            id="runtime-compartment"
            value={ociRuntimeForm.compartmentOcid}
            onChange={(event) =>
              setOciRuntimeForm((current) => ({
                ...current,
                compartmentOcid: event.target.value,
              }))
            }
            placeholder="ocid1.compartment.oc1.."
          />
          <FieldError>{runtimeFieldErrors.compartmentOcid}</FieldError>
        </Field>

        {runtimeCatalogValidation.catalogMessage ? (
          <Alert className="md:col-span-2">
            <TriangleAlertIcon />
            <AlertTitle>
              {runtimeCatalog.error
                ? t("oci.catalog.unavailableTitle")
                : runtimeCatalog.loading
                  ? t("oci.catalog.loadingTitle")
                  : t("oci.catalog.attentionTitle")}
            </AlertTitle>
            <AlertDescription>
              {runtimeCatalogValidation.catalogMessage}
            </AlertDescription>
          </Alert>
        ) : null}

        <Field>
          <FieldLabel htmlFor="runtime-ad">
            {t("oci.form.availabilityDomain")}
          </FieldLabel>
          <Select
            value={ociRuntimeForm.availabilityDomain || undefined}
            onValueChange={(value) =>
              setOciRuntimeForm((current) => ({
                ...current,
                availabilityDomain: value,
              }))
            }
            disabled={runtimeCatalog.loading}
          >
            <SelectTrigger id="runtime-ad" className="w-full">
              <SelectValue
                placeholder={t("oci.form.availabilityDomainPlaceholder")}
              />
            </SelectTrigger>
            <SelectContent align="start">
              <SelectGroup>
                {staleAvailabilityDomain ? (
                  <SelectItem value={ociRuntimeForm.availabilityDomain}>
                    {t("oci.form.availabilityDomainUnavailable", {
                      value: ociRuntimeForm.availabilityDomain,
                    })}
                  </SelectItem>
                ) : null}
                {runtimeCatalog.availabilityDomains.map((item) => (
                  <SelectItem key={item} value={item}>
                    {item}
                  </SelectItem>
                ))}
              </SelectGroup>
            </SelectContent>
          </Select>
          <FieldError>{runtimeFieldErrors.availabilityDomain}</FieldError>
        </Field>

        <Field>
          <FieldLabel htmlFor="runtime-subnet">
            {t("oci.form.subnetOcid")}
          </FieldLabel>
          <Select
            value={ociRuntimeForm.subnetOcid || undefined}
            onValueChange={(value) =>
              setOciRuntimeForm((current) => ({
                ...current,
                subnetOcid: value,
              }))
            }
            disabled={runtimeCatalog.loading}
          >
            <SelectTrigger id="runtime-subnet" className="w-full">
              <SelectValue placeholder={t("oci.form.subnetPlaceholder")} />
            </SelectTrigger>
            <SelectContent align="start">
              <SelectGroup>
                {staleSubnet ? (
                  <SelectItem value={ociRuntimeForm.subnetOcid}>
                    {t("oci.form.subnetUnavailable", {
                      value: compactValue(ociRuntimeForm.subnetOcid),
                    })}
                  </SelectItem>
                ) : null}
                {runtimeCatalog.subnets.map((item) => (
                  <SelectItem key={item.id} value={item.id}>
                    {catalogSubnetOptionLabel(item)}
                  </SelectItem>
                ))}
              </SelectGroup>
            </SelectContent>
          </Select>
          <FieldError>{runtimeFieldErrors.subnetOcid}</FieldError>
        </Field>

        <Field>
          <FieldLabel htmlFor="runtime-image">
            {t("oci.form.imageOcid")}
          </FieldLabel>
          <Select
            value={ociRuntimeForm.imageOcid || undefined}
            onValueChange={(value) =>
              setOciRuntimeForm((current) => ({ ...current, imageOcid: value }))
            }
            disabled={runtimeCatalog.loading}
          >
            <SelectTrigger id="runtime-image" className="w-full">
              <SelectValue placeholder={t("oci.form.imagePlaceholder")} />
            </SelectTrigger>
            <SelectContent align="start">
              <SelectGroup>
                {staleImage ? (
                  <SelectItem value={ociRuntimeForm.imageOcid}>
                    {t("oci.form.imageUnavailable", {
                      value: compactValue(ociRuntimeForm.imageOcid),
                    })}
                  </SelectItem>
                ) : null}
                {runtimeCatalog.images.map((item) => (
                  <SelectItem key={item.id} value={item.id}>
                    {catalogImageOptionLabel(item)}
                  </SelectItem>
                ))}
              </SelectGroup>
            </SelectContent>
          </Select>
          <FieldError>{runtimeFieldErrors.imageOcid}</FieldError>
        </Field>

        {settingsMode ? (
          <Field className="md:col-span-2">
            <FieldLabel htmlFor="runtime-nsgs">
              {t("oci.form.nsgOcids")}
            </FieldLabel>
            <Textarea
              id="runtime-nsgs"
              rows={5}
              value={ociRuntimeForm.nsgOcidText}
              onChange={(event) =>
                setOciRuntimeForm((current) => ({
                  ...current,
                  nsgOcidText: event.target.value,
                }))
              }
              placeholder={"ocid1.nsg.oc1..\nocid1.nsg.oc1.."}
            />
          </Field>
        ) : null}

        <Field
          orientation="horizontal"
          className="rounded-xl border bg-background/60 p-3 md:col-span-2"
        >
          <Checkbox
            id="runtime-public-ip"
            checked={ociRuntimeForm.assignPublicIp}
            onCheckedChange={(checked) =>
              setOciRuntimeForm((current) => ({
                ...current,
                assignPublicIp: Boolean(checked),
              }))
            }
          />
          <FieldContent>
            <FieldLabel htmlFor="runtime-public-ip">
              {t("oci.form.assignPublicIp")}
            </FieldLabel>
          </FieldContent>
        </Field>

        {settingsMode ? (
          <>
            <Field
              orientation="horizontal"
              className="rounded-xl border bg-background/60 p-3 md:col-span-2"
            >
              <Checkbox
                id="runtime-cache-compat"
                checked={cacheCompatEnabled}
                onCheckedChange={(checked) =>
                  setOciRuntimeForm((current) => ({
                    ...current,
                    cacheCompatEnabled: Boolean(checked),
                  }))
                }
              />
              <FieldContent>
                <FieldLabel htmlFor="runtime-cache-compat">
                  {t("oci.form.cacheCompatEnabled")}
                </FieldLabel>
              </FieldContent>
            </Field>

            <Field>
              <FieldLabel htmlFor="runtime-cache-bucket">
                {t("oci.form.cacheBucketName")}
              </FieldLabel>
              <Input
                id="runtime-cache-bucket"
                value={ociRuntimeForm.cacheBucketName}
                disabled={!cacheCompatEnabled}
                onChange={(event) =>
                  setOciRuntimeForm((current) => ({
                    ...current,
                    cacheBucketName: event.target.value,
                  }))
                }
                placeholder={t("oci.form.cacheBucketNamePlaceholder")}
              />
              <FieldError>{runtimeFieldErrors.cacheBucketName}</FieldError>
            </Field>

            <Field>
              <FieldLabel htmlFor="runtime-cache-retention">
                {t("oci.form.cacheRetentionDays")}
              </FieldLabel>
              <Input
                id="runtime-cache-retention"
                type="number"
                min="1"
                value={ociRuntimeForm.cacheRetentionDays}
                disabled={!cacheCompatEnabled}
                onChange={(event) =>
                  setOciRuntimeForm((current) => ({
                    ...current,
                    cacheRetentionDays: event.target.value,
                  }))
                }
              />
              <FieldError>{runtimeFieldErrors.cacheRetentionDays}</FieldError>
            </Field>

            <Field className="md:col-span-2">
              <FieldLabel htmlFor="runtime-cache-prefix">
                {t("oci.form.cacheObjectPrefix")}
              </FieldLabel>
              <Input
                id="runtime-cache-prefix"
                value={ociRuntimeForm.cacheObjectPrefix}
                disabled={!cacheCompatEnabled}
                onChange={(event) =>
                  setOciRuntimeForm((current) => ({
                    ...current,
                    cacheObjectPrefix: event.target.value,
                  }))
                }
                placeholder={t("oci.form.cacheObjectPrefixPlaceholder")}
              />
            </Field>
          </>
        ) : null}
      </FieldGroup>

      <div className="flex flex-wrap gap-2 border-t pt-5">
        <Button
          type="button"
          onClick={() => void onRuntimeSave()}
          disabled={runtimeSaveDisabled}
          aria-busy={ociRuntimeSaving}
        >
          <BusyButtonContent
            busy={ociRuntimeSaving}
            label={runtimeSaveButtonLabel}
            icon={UploadIcon}
          />
        </Button>
        {settingsMode ? (
          <Button
            type="button"
            variant="ghost"
            onClick={() => void onRuntimeClear()}
            disabled={runtimeActionsBusy}
            aria-busy={ociRuntimeClearing}
          >
            <BusyButtonContent
              busy={ociRuntimeClearing}
              label={t("oci.button.useEnvDefaults")}
              icon={Trash2Icon}
            />
          </Button>
        ) : null}
      </div>
    </form>
  );

  return (
    <div
      className={cn(
        "flex flex-col gap-6",
        setupMode && "mx-auto w-full max-w-[860px] gap-5",
      )}
    >
      {settingsMode ? (
        <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
          <StatCard
            label={t("oci.stat.accessMode")}
            value={summarizeOCIAuthMode(ociAuthStatus.effectiveMode, t)}
            note={t("oci.stat.defaultMode", {
              mode: summarizeOCIAuthMode(ociAuthStatus.defaultMode, t),
            })}
            accent={ociAuthStatus.effectiveMode === "api_key"}
          />
          <StatCard
            label={t("oci.stat.savedCredential")}
            value={ociAuthStatus.activeCredential?.name || t("common.none")}
            note={
              ociAuthStatus.activeCredential
                ? t("oci.inspect.profile", {
                    profile:
                      ociAuthStatus.activeCredential.profileName ||
                      t("common.default"),
                  })
                : t("oci.current.defaultPath")
            }
            accent={Boolean(ociAuthStatus.activeCredential)}
          />
          <StatCard
            label={t("oci.stat.launchSource")}
            value={
              ociRuntimeStatus.source === "cms"
                ? t("common.savedHere")
                : t("common.environment")
            }
            note={
              effectiveSettings.subnetOcid
                ? compactValue(effectiveSettings.subnetOcid)
                : t("oci.stat.noSubnetSaved")
            }
            accent={ociRuntimeStatus.source === "cms"}
          />
          <StatCard
            label={t("oci.stat.launchSetup")}
            value={
              ociRuntimeStatus.ready
                ? t("common.ready")
                : t("common.needsSetup")
            }
            note={
              ociRuntimeStatus.ready
                ? t("oci.stat.launchReadyBody")
                : t("oci.stat.launchNeedsSetupBody", {
                    count: ociRuntimeStatus.missing.length,
                  })
            }
            accent={ociRuntimeStatus.ready}
          />
        </div>
      ) : null}

      <div
        className={cn(
          "grid gap-6",
          settingsMode &&
            "xl:grid-cols-[minmax(0,1.05fr)_minmax(340px,0.95fr)]",
        )}
      >
        <Card className="border bg-card/95">
          {settingsMode ? (
            <Tabs defaultValue="credential" className="gap-0">
              <CardHeader className="border-b">
                <div>
                  <CardTitle>{resolvedTitle}</CardTitle>
                  {resolvedDescription ? (
                    <CardDescription>{resolvedDescription}</CardDescription>
                  ) : null}
                </div>
                <CardAction className="col-start-1 row-start-3 w-full justify-self-stretch md:col-start-2 md:row-span-2 md:row-start-1 md:w-auto md:justify-self-end">
                  <TabsList variant="line" className="w-full sm:w-auto">
                    <TabsTrigger value="credential">
                      {t("oci.tab.credential")}
                    </TabsTrigger>
                    <TabsTrigger value="runtime">
                      {t("oci.tab.runtime")}
                    </TabsTrigger>
                  </TabsList>
                </CardAction>
              </CardHeader>
              <CardContent className="pt-4">
                <TabsContent value="credential" className="mt-0">
                  {credentialForm}
                </TabsContent>
                <TabsContent value="runtime" className="mt-0">
                  {runtimeForm}
                </TabsContent>
              </CardContent>
            </Tabs>
          ) : (
            <>
              <CardContent className="p-5">
                {setupCredentialMode ? credentialForm : runtimeForm}
              </CardContent>
            </>
          )}
        </Card>

        {settingsMode ? (
          <Card className="border bg-card/95">
            <CardHeader className="border-b">
              <CardTitle>{t("oci.current.title")}</CardTitle>
            </CardHeader>
            <CardContent className="flex flex-col gap-5 pt-4">
              <div className="grid gap-3 md:grid-cols-2">
                <div className="rounded-xl border bg-background/70 px-4 py-3">
                  <p className="text-sm font-medium">
                    {t("oci.stat.accessMode")}
                  </p>
                  <p className="mt-1 text-sm text-muted-foreground">
                    {summarizeOCIAuthMode(ociAuthStatus.effectiveMode, t)}
                  </p>
                </div>
                <div className="rounded-xl border bg-background/70 px-4 py-3">
                  <p className="text-sm font-medium">
                    {t("oci.stat.launchSource")}
                  </p>
                  <p className="mt-1 text-sm text-muted-foreground">
                    {ociRuntimeStatus.source === "cms"
                      ? t("common.savedHere")
                      : t("common.environment")}
                  </p>
                </div>
                <div className="rounded-xl border bg-background/70 px-4 py-3">
                  <p className="text-sm font-medium">
                    {t("oci.current.label.subnet")}
                  </p>
                  <p className="mt-1 font-mono text-xs text-muted-foreground">
                    {compactValue(effectiveSettings.subnetOcid)}
                  </p>
                </div>
                <div className="rounded-xl border bg-background/70 px-4 py-3">
                  <p className="text-sm font-medium">
                    {t("oci.current.label.image")}
                  </p>
                  <p className="mt-1 font-mono text-xs text-muted-foreground">
                    {compactValue(effectiveSettings.imageOcid)}
                  </p>
                </div>
              </div>

              {ociAuthStatus.activeCredential ? (
                <Card size="sm" className="border bg-background/70">
                  <CardHeader className="border-b">
                    <CardTitle className="text-sm">
                      {t("oci.current.savedCredentialTitle")}
                    </CardTitle>
                  </CardHeader>
                  <CardContent className="grid gap-3 pt-4 text-sm">
                    <div className="flex items-center justify-between gap-4">
                      <span className="text-muted-foreground">
                        {t("oci.current.label.name")}
                      </span>
                      <span className="font-medium">
                        {ociAuthStatus.activeCredential.name}
                      </span>
                    </div>
                    <Separator />
                    <div className="flex items-center justify-between gap-4">
                      <span className="text-muted-foreground">
                        {t("oci.current.label.profile")}
                      </span>
                      <span className="font-medium">
                        {ociAuthStatus.activeCredential.profileName ||
                          t("common.default")}
                      </span>
                    </div>
                    <Separator />
                    <div className="flex items-center justify-between gap-4">
                      <span className="text-muted-foreground">
                        {t("oci.current.label.region")}
                      </span>
                      <span className="font-medium">
                        {ociAuthStatus.activeCredential.region ||
                          t("common.notSet")}
                      </span>
                    </div>
                    <Separator />
                    <div className="flex items-center justify-between gap-4">
                      <span className="text-muted-foreground">
                        {t("oci.current.label.tenancy")}
                      </span>
                      <span className="font-mono text-xs">
                        {compactValue(
                          ociAuthStatus.activeCredential.tenancyOcid,
                        )}
                      </span>
                    </div>
                    <Separator />
                    <div className="flex items-center justify-between gap-4">
                      <span className="text-muted-foreground">
                        {t("oci.current.label.user")}
                      </span>
                      <span className="font-mono text-xs">
                        {compactValue(ociAuthStatus.activeCredential.userOcid)}
                      </span>
                    </div>
                    <Separator />
                    <div className="flex items-center justify-between gap-4">
                      <span className="text-muted-foreground">
                        {t("oci.current.label.lastTested")}
                      </span>
                      <span className="font-medium">
                        {formatDateTime(
                          ociAuthStatus.activeCredential.lastTestedAt,
                        )}
                      </span>
                    </div>
                  </CardContent>
                </Card>
              ) : (
                <EmptyBlock
                  title={t("oci.current.noSavedCredentialTitle")}
                  body={t("oci.current.noSavedCredentialBody")}
                />
              )}

              <Card size="sm" className="border bg-background/70">
                <CardHeader className="border-b">
                  <CardTitle className="text-sm">
                    {t("oci.current.launchSettingsTitle")}
                  </CardTitle>
                </CardHeader>
                <CardContent className="grid gap-3 pt-4 text-sm">
                  <div className="flex items-center justify-between gap-4">
                    <span className="text-muted-foreground">
                      {t("oci.current.label.compartment")}
                    </span>
                    <span className="font-mono text-xs">
                      {compactValue(effectiveSettings.compartmentOcid)}
                    </span>
                  </div>
                  <Separator />
                  <div className="flex items-center justify-between gap-4">
                    <span className="text-muted-foreground">
                      {t("oci.form.availabilityDomain")}
                    </span>
                    <span className="font-medium">
                      {effectiveSettings.availabilityDomain ||
                        t("common.notSet")}
                    </span>
                  </div>
                  <Separator />
                  <div className="flex items-center justify-between gap-4">
                    <span className="text-muted-foreground">
                      {t("oci.current.label.subnet")}
                    </span>
                    <span className="font-mono text-xs">
                      {compactValue(effectiveSettings.subnetOcid)}
                    </span>
                  </div>
                  <Separator />
                  <div className="flex items-center justify-between gap-4">
                    <span className="text-muted-foreground">
                      {t("oci.current.label.image")}
                    </span>
                    <span className="font-mono text-xs">
                      {compactValue(effectiveSettings.imageOcid)}
                    </span>
                  </div>
                  <Separator />
                  <div className="flex items-center justify-between gap-4">
                    <span className="text-muted-foreground">
                      {t("oci.current.label.publicIp")}
                    </span>
                    <span className="font-medium">
                      {effectiveSettings.assignPublicIp === true
                        ? t("oci.current.publicIp.on")
                        : effectiveSettings.assignPublicIp === false
                          ? t("oci.current.publicIp.off")
                          : t("oci.current.publicIp.notSet")}
                    </span>
                  </div>
                  <Separator />
                  <div className="flex items-center justify-between gap-4">
                    <span className="text-muted-foreground">
                      {t("oci.current.label.cacheCompat")}
                    </span>
                    <span className="font-medium">
                      {effectiveSettings.cacheCompatEnabled
                        ? t("oci.current.cacheCompat.on")
                        : t("oci.current.cacheCompat.off")}
                    </span>
                  </div>
                  {effectiveSettings.cacheCompatEnabled ? (
                    <>
                      <Separator />
                      <div className="flex items-center justify-between gap-4">
                        <span className="text-muted-foreground">
                          {t("oci.current.label.cacheBucket")}
                        </span>
                        <span className="font-medium">
                          {effectiveSettings.cacheBucketName ||
                            t("common.notSet")}
                        </span>
                      </div>
                      <Separator />
                      <div className="flex items-center justify-between gap-4">
                        <span className="text-muted-foreground">
                          {t("oci.current.label.cachePrefix")}
                        </span>
                        <span className="font-medium">
                          {effectiveSettings.cacheObjectPrefix ||
                            t("common.notSet")}
                        </span>
                      </div>
                      <Separator />
                      <div className="flex items-center justify-between gap-4">
                        <span className="text-muted-foreground">
                          {t("oci.current.label.cacheRetention")}
                        </span>
                        <span className="font-medium">
                          {effectiveSettings.cacheRetentionDays
                            ? t("oci.current.cacheRetentionValue", {
                                days: effectiveSettings.cacheRetentionDays,
                              })
                            : t("common.notSet")}
                        </span>
                      </div>
                    </>
                  ) : null}
                </CardContent>
              </Card>

              {!ociRuntimeStatus.ready ? (
                <Alert>
                  <TriangleAlertIcon />
                  <AlertTitle>
                    {t("oci.current.runtimeMissingTitle")}
                  </AlertTitle>
                  <AlertDescription>
                    {t("oci.current.runtimeMissingBody", {
                      missing: formatOCIRuntimeMissing(
                        ociRuntimeStatus.missing,
                        t,
                        locale,
                      ),
                    })}
                  </AlertDescription>
                </Alert>
              ) : null}

              {ociAuthResult ? (
                <Alert>
                  <ShieldCheckIcon />
                  <AlertTitle>
                    {summarizeOCIResultTitle(ociAuthResult, t)}
                  </AlertTitle>
                  <AlertDescription>
                    {summarizeOCIResult(ociAuthResult, t)}
                  </AlertDescription>
                </Alert>
              ) : null}
            </CardContent>
            <CardFooter className="justify-between gap-3 text-sm text-muted-foreground">
              <p>{t("oci.current.footerCredential")}</p>
              <p>
                {ociRuntimeStatus.ready
                  ? t("oci.current.footerReady")
                  : t("oci.current.footerNeedsSetup")}
              </p>
            </CardFooter>
          </Card>
        ) : null}
      </div>
    </div>
  );
}
