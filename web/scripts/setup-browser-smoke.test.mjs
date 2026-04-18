import test from "node:test";
import assert from "node:assert/strict";
import path from "node:path";

import { chromium } from "playwright";

import { startBrowserTestServer } from "./browser-test-server.mjs";

const WEB_ROOT = path.resolve(import.meta.dirname, "..");
const FIXTURE_TIMESTAMP = "2026-04-17T08:00:00.000Z";
const GITHUB_ACCOUNT = "seeded-org";
const GITHUB_REPOSITORIES = [
  {
    fullName: "seeded-org/repo-alpha",
    owner: GITHUB_ACCOUNT,
    name: "repo-alpha",
    admin: true,
    private: true,
  },
  {
    fullName: "seeded-org/repo-beta",
    owner: GITHUB_ACCOUNT,
    name: "repo-beta",
    admin: true,
    private: false,
  },
];
const COMPARTMENT_OCID = "ocid1.compartment.oc1..setupcompartment";
const SUBNET_OCID = "ocid1.subnet.oc1..setupsubnet";
const IMAGE_OCID = "ocid1.image.oc1..setupimage";
const AVAILABILITY_DOMAIN = "kIdk:AP-SEOUL-1-AD-1";

function fulfillJson(route, payload, status = 200) {
  return route.fulfill({
    status,
    headers: {
      "Content-Type": "application/json",
    },
    body: JSON.stringify(payload),
  });
}

function createGitHubConfig(form = {}, selectedRepos = [], options = {}) {
  const appId = Number(form.appId) || 123;
  const installationId = Number(form.installationId) || 456;

  return {
    id:
      options.id ||
      `github-${options.state || "config"}-${appId}-${installationId}`,
    name: form.name || "Seeded GitHub route",
    apiBaseUrl: form.apiBaseUrl || "https://api.github.com",
    authMode: "app",
    appId,
    installationId,
    accountLogin: GITHUB_ACCOUNT,
    accountType: "Organization",
    selectedRepos,
    installationState: "installed",
    installationRepositorySelection: "selected",
    installationRepositories: GITHUB_REPOSITORIES.map(
      (repository) => repository.fullName,
    ),
    installationReady: true,
    isActive: Boolean(options.active),
    isStaged: Boolean(options.staged),
    lastTestedAt: FIXTURE_TIMESTAMP,
    createdAt: FIXTURE_TIMESTAMP,
    updatedAt: FIXTURE_TIMESTAMP,
  };
}

function createSetupApiFixture(options = {}) {
  const requestCounts = new Map();
  const unexpectedRequests = [];
  const state = {
    session: {
      authenticated: true,
      mustChangePassword: options.mustChangePassword ?? true,
      username: "seeded-admin",
    },
    github: {
      verifiedResult: null,
      stagedConfig: null,
      activeConfig: null,
      promoteFailuresRemaining: options.promoteFailures ?? 0,
    },
    oci: {
      activeCredential: null,
      runtime: null,
    },
  };

  function recordRequest(request) {
    const url = new URL(request.url());
    const key = `${request.method()} ${url.pathname}`;
    requestCounts.set(key, (requestCounts.get(key) || 0) + 1);
    return { key, url };
  }

  function buildSetupStatus() {
    const passwordCompleted = !state.session.mustChangePassword;
    const githubRouteLive = Boolean(
      state.github.activeConfig?.installationReady,
    );
    const selectedRepos = Array.isArray(
      state.github.activeConfig?.selectedRepos,
    )
      ? state.github.activeConfig.selectedRepos
      : [];
    const credentialSaved = Boolean(state.oci.activeCredential);
    const runtimeReady = Boolean(state.oci.runtime);

    return {
      completed: Boolean(
        passwordCompleted &&
        githubRouteLive &&
        selectedRepos.length > 0 &&
        runtimeReady,
      ),
      updatedAt: FIXTURE_TIMESTAMP,
      steps: {
        password: {
          completed: passwordCompleted,
          missing: passwordCompleted ? [] : ["setup.missing.newPassword"],
        },
        github: {
          completed: Boolean(githubRouteLive && selectedRepos.length > 0),
          missing: githubRouteLive
            ? selectedRepos.length > 0
              ? []
              : ["selectedRepos"]
            : ["appId", "installationId", "webhookSecret"],
        },
        oci: {
          completed: runtimeReady,
          missing: runtimeReady
            ? []
            : [
                "OHOCI_OCI_COMPARTMENT_OCID",
                "OHOCI_OCI_SUBNET_OCID",
                "OHOCI_OCI_IMAGE_OCID",
              ],
        },
      },
      bootstrapSteps: {
        password: {
          completed: passwordCompleted,
          missing: passwordCompleted ? [] : ["setup.missing.newPassword"],
        },
        github: {
          completed: githubRouteLive || Boolean(state.github.stagedConfig),
          missing:
            githubRouteLive || state.github.stagedConfig
              ? []
              : ["appId", "installationId", "webhookSecret"],
        },
        oci: {
          completed: credentialSaved,
          missing: credentialSaved ? [] : ["credential"],
        },
      },
    };
  }

  function buildGitHubConfigStatus() {
    const activeConfig = state.github.activeConfig;
    const stagedConfig = state.github.stagedConfig;
    const activeConfigs = activeConfig ? [activeConfig] : [];
    const selectedRepos = Array.isArray(activeConfig?.selectedRepos)
      ? activeConfig.selectedRepos
      : [];

    return {
      status: {
        source: activeConfig || stagedConfig ? "cms" : "env",
        ready: Boolean(activeConfig && selectedRepos.length > 0),
        stagedReady: Boolean(stagedConfig),
        configured: Boolean(activeConfig || stagedConfig),
        hasWebhookSecret: Boolean(activeConfig || stagedConfig),
        hasAppCredentials: Boolean(activeConfig || stagedConfig),
        accountLogin:
          activeConfig?.accountLogin || stagedConfig?.accountLogin || "",
        accountType:
          activeConfig?.accountType || stagedConfig?.accountType || "",
        selectedRepos,
        missing: activeConfig
          ? selectedRepos.length > 0
            ? []
            : ["selectedRepos"]
          : ["appId", "installationId", "webhookSecret"],
        stagedMissing: stagedConfig ? [] : [],
        stagedError: "",
        activeConfig,
        activeConfigs,
        stagedConfig,
      },
    };
  }

  function buildGitHubVerificationResult(form = {}) {
    const selectedRepos = Array.isArray(form.selectedRepos)
      ? form.selectedRepos
      : [];

    return {
      accountLogin: GITHUB_ACCOUNT,
      owners: [GITHUB_ACCOUNT],
      repositories: GITHUB_REPOSITORIES,
      config: {
        appId: Number(form.appId) || 123,
        installationId: Number(form.installationId) || 456,
        selectedRepos,
        installationRepositories: GITHUB_REPOSITORIES.map(
          (repository) => repository.fullName,
        ),
      },
    };
  }

  function buildOCIAuthStatus() {
    return {
      effectiveMode: state.oci.activeCredential ? "api_key" : "",
      defaultMode: state.oci.activeCredential ? "api_key" : "",
      activeCredential: state.oci.activeCredential,
      runtimeConfigReady: Boolean(state.oci.runtime),
      runtimeConfigMissing: state.oci.runtime ? [] : ["runtime"],
    };
  }

  function buildOCIRuntimeStatus() {
    const effectiveSettings = state.oci.runtime || {
      compartmentOcid: "",
      availabilityDomain: "",
      subnetOcid: "",
      imageOcid: "",
      assignPublicIp: false,
      nsgOcids: [],
      cacheCompatEnabled: false,
      cacheBucketName: "",
      cacheObjectPrefix: "",
      cacheRetentionDays: 0,
    };

    return {
      source: state.oci.runtime ? "cms" : "env",
      overrideSettings: state.oci.runtime,
      effectiveSettings,
      ready: Boolean(state.oci.runtime),
      missing: state.oci.runtime
        ? []
        : [
            "OHOCI_OCI_COMPARTMENT_OCID",
            "OHOCI_OCI_SUBNET_OCID",
            "OHOCI_OCI_IMAGE_OCID",
          ],
    };
  }

  async function handle(route) {
    const request = route.request();
    const { key, url } = recordRequest(request);

    switch (key) {
      case "GET /api/v1/auth/session":
        return fulfillJson(route, { session: state.session });
      case "POST /api/v1/auth/change-password":
        state.session.mustChangePassword = false;
        return fulfillJson(route, { ok: true });
      case "GET /api/v1/setup/status":
        return fulfillJson(route, buildSetupStatus());
      case "GET /api/v1/github/config":
        return fulfillJson(route, buildGitHubConfigStatus());
      case "GET /api/v1/github/drift":
        return fulfillJson(route, {
          available: true,
          generatedAt: FIXTURE_TIMESTAMP,
          severity: "ok",
          issues: [],
        });
      case "GET /api/v1/github/config/manifest/pending":
        return fulfillJson(route, { pending: null });
      case "POST /api/v1/github/config/test": {
        const form = request.postDataJSON() || {};
        state.github.verifiedResult = buildGitHubVerificationResult(form);
        return fulfillJson(route, state.github.verifiedResult);
      }
      case "POST /api/v1/github/config/staged": {
        const form = request.postDataJSON() || {};
        const selectedRepos = Array.isArray(form.selectedRepos)
          ? form.selectedRepos
          : [];
        state.github.stagedConfig = createGitHubConfig(form, selectedRepos, {
          state: "staged",
          staged: true,
        });
        state.github.verifiedResult = buildGitHubVerificationResult(form);
        return fulfillJson(route, state.github.verifiedResult);
      }
      case "POST /api/v1/github/config/staged/promote":
        if (state.github.promoteFailuresRemaining > 0) {
          state.github.promoteFailuresRemaining -= 1;
          return fulfillJson(
            route,
            { error: "The GitHub route still needs activation." },
            500,
          );
        }
        if (state.github.stagedConfig) {
          state.github.activeConfig = {
            ...state.github.stagedConfig,
            isActive: true,
            isStaged: false,
            updatedAt: FIXTURE_TIMESTAMP,
          };
          state.github.stagedConfig = null;
        }
        return fulfillJson(route, { ok: true });
      case "GET /api/v1/oci/auth":
        return fulfillJson(route, buildOCIAuthStatus());
      case "POST /api/v1/oci/auth/test": {
        const form = request.postDataJSON() || {};
        return fulfillJson(route, {
          credential: {
            name: form.name || "Setup credential",
            profileName: form.profileName || "DEFAULT",
            region: "ap-seoul-1",
          },
          regionSubscriptions: ["ap-seoul-1"],
          availabilityDomains: [AVAILABILITY_DOMAIN],
        });
      }
      case "POST /api/v1/oci/auth": {
        const form = request.postDataJSON() || {};
        state.oci.activeCredential = {
          id: "setup-credential",
          name: form.name || "Setup credential",
          profileName: form.profileName || "DEFAULT",
          region: "ap-seoul-1",
          tenancyOcid: "ocid1.tenancy.oc1..setuptenancy",
          userOcid: "ocid1.user.oc1..setupuser",
          lastTestedAt: FIXTURE_TIMESTAMP,
        };
        return fulfillJson(route, {
          credential: state.oci.activeCredential,
          regionSubscriptions: ["ap-seoul-1"],
          availabilityDomains: [AVAILABILITY_DOMAIN],
        });
      }
      case "GET /api/v1/oci/runtime":
        return fulfillJson(route, buildOCIRuntimeStatus());
      case "POST /api/v1/oci/catalog":
        return fulfillJson(route, {
          availabilityDomains: [AVAILABILITY_DOMAIN],
          subnets: [
            {
              id: SUBNET_OCID,
              displayName: "Setup subnet",
              availabilityDomain: AVAILABILITY_DOMAIN,
              cidrBlock: "10.0.0.0/24",
            },
          ],
          images: [
            {
              id: IMAGE_OCID,
              displayName: "Setup image",
              operatingSystem: "Oracle Linux",
              operatingSystemVersion: "9",
            },
          ],
          shapes: [
            {
              shape: "VM.Standard.E4.Flex",
              isFlexible: true,
              ocpuMin: 1,
              ocpuMax: 4,
              memoryMinGb: 16,
              memoryMaxGb: 64,
              memoryMinPerOcpuGb: 16,
              memoryMaxPerOcpuGb: 64,
            },
          ],
          sourceRegion: "ap-seoul-1",
          validatedAt: FIXTURE_TIMESTAMP,
        });
      case "PUT /api/v1/oci/runtime": {
        const form = request.postDataJSON() || {};
        state.oci.runtime = {
          compartmentOcid: form.compartmentOcid || "",
          availabilityDomain: form.availabilityDomain || "",
          subnetOcid: form.subnetOcid || "",
          imageOcid: form.imageOcid || "",
          assignPublicIp: Boolean(form.assignPublicIp),
          nsgOcids: Array.isArray(form.nsgOcids) ? form.nsgOcids : [],
          cacheCompatEnabled: Boolean(form.cacheCompatEnabled),
          cacheBucketName: form.cacheBucketName || "",
          cacheObjectPrefix: form.cacheObjectPrefix || "",
          cacheRetentionDays: Number(form.cacheRetentionDays) || 0,
        };
        return fulfillJson(route, buildOCIRuntimeStatus());
      }
      case "GET /api/v1/oci/subnets":
        return fulfillJson(route, {
          items: [
            {
              id: SUBNET_OCID,
              displayName: "Setup subnet",
              isRecommended: true,
            },
          ],
          defaultSubnetId: "",
        });
      case "GET /api/v1/policies":
        return fulfillJson(route, { items: [] });
      case "GET /api/v1/runners":
        return fulfillJson(route, { items: [] });
      case "GET /api/v1/jobs":
        return fulfillJson(route, { items: [] });
      case "GET /api/v1/events":
        return fulfillJson(route, { events: [], logs: [] });
      case "GET /api/v1/billing/guardrails":
        return fulfillJson(route, {
          generatedAt: FIXTURE_TIMESTAMP,
          windowDays: 7,
          items: [],
        });
      case "GET /api/v1/runner-images":
        return fulfillJson(route, {});
      default:
        if (key === "GET /api/v1/billing/policies") {
          const days = Number(url.searchParams.get("days")) || 7;
          return fulfillJson(route, {
            generatedAt: FIXTURE_TIMESTAMP,
            windowDays: days,
            currency: "USD",
            items: [],
            issues: [],
          });
        }

        unexpectedRequests.push(key);
        return fulfillJson(route, { error: `Unexpected request: ${key}` }, 500);
    }
  }

  return {
    async attach(page) {
      await page.route("**/api/v1/**", handle);
    },
    getCount(key) {
      return requestCounts.get(key) || 0;
    },
    getUnexpectedRequests() {
      return [...unexpectedRequests];
    },
  };
}

async function selectComboboxOption(page, label, optionText) {
  await page.getByRole("combobox", { name: label }).click();
  await page.getByRole("option", { name: optionText }).click();
}

test("setup shell walks the five-task flow and unlocks the main workspace", async () => {
  const fixture = createSetupApiFixture();
  const { server, baseUrl } = await startBrowserTestServer({ root: WEB_ROOT });
  const browser = await chromium.launch();
  const page = await browser.newPage();
  const pageErrors = [];

  page.on("pageerror", (error) => {
    pageErrors.push(error);
  });

  await fixture.attach(page);

  try {
    await page.goto(baseUrl, { waitUntil: "domcontentloaded" });
    await page
      .getByRole("heading", { name: "Change admin password" })
      .waitFor();
    await page.getByText("Finish setup").first().waitFor();

    await page.getByLabel("Current password").fill("admin");
    await page.getByLabel("New password").fill("better-password");
    await page.getByRole("button", { name: "Save password" }).click();

    await page.getByRole("heading", { name: "Connect GitHub App" }).waitFor();
    assert.equal(fixture.getCount("POST /api/v1/auth/change-password"), 1);
    assert.equal(await page.getByText("Active apps").count(), 0);
    assert.equal(await page.getByText("GitHub drift").count(), 0);
    assert.equal(
      await page
        .getByRole("button", { name: "Promote staged settings" })
        .count(),
      0,
    );
    assert.equal(await page.getByText("Local repository allowlist").count(), 0);

    await page.getByRole("button", { name: "Use existing app" }).click();
    await page.getByLabel("GitHub App ID").fill("123");
    await page.getByLabel("Installation ID").fill("456");
    await page
      .getByLabel("Private key PEM")
      .fill(
        "-----BEGIN RSA PRIVATE KEY-----\nTEST\n-----END RSA PRIVATE KEY-----",
      );
    await page.getByLabel("Webhook secret").fill("top-secret-webhook");
    await page.getByRole("button", { name: "Verify GitHub App" }).click();
    await page.getByRole("button", { name: "Save GitHub access" }).click();

    await page.getByRole("heading", { name: "Save OCI credential" }).waitFor();
    assert.equal(
      fixture.getCount("POST /api/v1/github/config/staged/promote") >= 1,
      true,
    );
    assert.equal(
      await page.getByRole("tab", { name: "Credential" }).count(),
      0,
    );
    assert.equal(
      await page.getByRole("tab", { name: "Runtime target" }).count(),
      0,
    );
    assert.equal(await page.getByText("Enable cache compatibility").count(), 0);
    assert.equal(await page.getByText("NSG OCIDs").count(), 0);

    await page.getByLabel("Credential name").fill("Primary OCI");
    await page.getByLabel("Profile name").fill("DEFAULT");
    await page
      .getByLabel("Parsed config")
      .fill(
        "[DEFAULT]\nuser=ocid1.user.oc1..setupuser\ntenancy=ocid1.tenancy.oc1..setuptenancy\nregion=ap-seoul-1",
      );
    await page
      .getByLabel("Private key PEM")
      .last()
      .fill("-----BEGIN PRIVATE KEY-----\nTEST\n-----END PRIVATE KEY-----");
    await page
      .getByRole("button", { name: "Save OCI credential", exact: true })
      .click();

    await page.getByRole("heading", { name: "Choose repositories" }).waitFor();
    await page
      .getByText("Choose at least one repository to unlock jobs.")
      .waitFor();
    await page.getByLabel("Select seeded-org/repo-alpha").click();
    await page.getByRole("button", { name: "Save repository scope" }).click();

    await page.getByRole("heading", { name: "Save launch target" }).waitFor();
    assert.equal(
      await page.getByRole("tab", { name: "Credential" }).count(),
      0,
    );
    assert.equal(
      await page.getByRole("tab", { name: "Runtime target" }).count(),
      0,
    );
    assert.equal(await page.getByText("NSG OCIDs").count(), 0);
    assert.equal(await page.getByText("Enable cache compatibility").count(), 0);
    await page.getByRole("button", { name: "Refresh catalog" }).waitFor();
    const catalogLoadCount = fixture.getCount("POST /api/v1/oci/catalog");
    await page.getByLabel("Compartment OCID").fill(COMPARTMENT_OCID);
    for (
      let attempt = 0;
      attempt < 30 &&
      fixture.getCount("POST /api/v1/oci/catalog") === catalogLoadCount;
      attempt += 1
    ) {
      await page.waitForTimeout(100);
    }
    assert.equal(
      fixture.getCount("POST /api/v1/oci/catalog") > catalogLoadCount,
      true,
    );
    await selectComboboxOption(page, "Subnet OCID", /Setup subnet/);
    await selectComboboxOption(page, "Image OCID", /Setup image/);
    await selectComboboxOption(
      page,
      "Availability domain",
      AVAILABILITY_DOMAIN,
    );
    const runtimeSaveButton = page
      .locator("button", { hasText: "Save launch target" })
      .last();
    const runtimeSaveCount = fixture.getCount("PUT /api/v1/oci/runtime");

    await runtimeSaveButton.waitFor({ state: "visible" });
    let stableCatalogChecks = 0;
    let previousCatalogCount = fixture.getCount("POST /api/v1/oci/catalog");
    for (let attempt = 0; attempt < 100 && stableCatalogChecks < 3; attempt += 1) {
      const currentCatalogCount = fixture.getCount("POST /api/v1/oci/catalog");
      const saveEnabled = !(await runtimeSaveButton.isDisabled());
      if (saveEnabled && currentCatalogCount === previousCatalogCount) {
        stableCatalogChecks += 1;
      } else {
        stableCatalogChecks = 0;
      }
      previousCatalogCount = currentCatalogCount;
      await page.waitForTimeout(100);
    }
    for (
      let attempt = 0;
      attempt < 5 &&
      fixture.getCount("PUT /api/v1/oci/runtime") === runtimeSaveCount;
      attempt += 1
    ) {
      await runtimeSaveButton.evaluate((button) => {
        button.click();
      });
      await page.waitForTimeout(300);
    }

    for (
      let attempt = 0;
      attempt < 50 &&
      fixture.getCount("PUT /api/v1/oci/runtime") === runtimeSaveCount;
      attempt += 1
    ) {
      await page.waitForTimeout(100);
    }
    assert.equal(
      fixture.getCount("PUT /api/v1/oci/runtime") > runtimeSaveCount,
      true,
    );
    await page
      .getByRole("heading", { name: "Overview" })
      .waitFor({ timeout: 20000 });
    assert.deepEqual(fixture.getUnexpectedRequests(), []);
    assert.deepEqual(pageErrors, []);
  } finally {
    await page.close();
    await browser.close();
    await server.close();
  }
});

test("setup connect shows a retry action when the first auto-activation fails", async () => {
  const fixture = createSetupApiFixture({
    mustChangePassword: false,
    promoteFailures: 1,
  });
  const { server, baseUrl } = await startBrowserTestServer({ root: WEB_ROOT });
  const browser = await chromium.launch();
  const page = await browser.newPage();
  const pageErrors = [];

  page.on("pageerror", (error) => {
    pageErrors.push(error);
  });

  await fixture.attach(page);

  try {
    await page.goto(baseUrl, { waitUntil: "domcontentloaded" });
    await page.getByRole("heading", { name: "Connect GitHub App" }).waitFor();
    await page.getByRole("button", { name: "Use existing app" }).click();
    await page.getByLabel("GitHub App ID").fill("123");
    await page.getByLabel("Installation ID").fill("456");
    await page
      .getByLabel("Private key PEM")
      .fill(
        "-----BEGIN RSA PRIVATE KEY-----\nTEST\n-----END RSA PRIVATE KEY-----",
      );
    await page.getByLabel("Webhook secret").fill("top-secret-webhook");
    await page.getByRole("button", { name: "Verify GitHub App" }).click();
    await page.getByRole("button", { name: "Save GitHub access" }).click();

    await page
      .getByText("GitHub route still needs activation", { exact: true })
      .first()
      .waitFor();
    await page.getByRole("button", { name: "Activate GitHub Route" }).waitFor();
    await page.getByRole("heading", { name: "Connect GitHub App" }).waitFor();
    assert.equal(
      fixture.getCount("POST /api/v1/github/config/staged/promote"),
      1,
    );
    assert.equal(
      await page
        .getByRole("button", { name: "Promote staged settings" })
        .count(),
      0,
    );
    assert.equal(
      await page.getByRole("button", { name: "Stage settings" }).count(),
      0,
    );
    assert.deepEqual(fixture.getUnexpectedRequests(), []);
    assert.deepEqual(pageErrors, []);
  } finally {
    await page.close();
    await browser.close();
    await server.close();
  }
});
