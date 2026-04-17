import {
  BookOpenTextIcon,
  BoxesIcon,
  CloudIcon,
  GitBranchIcon,
  ImageIcon,
  KeyRoundIcon,
  LayoutDashboardIcon,
  ScrollTextIcon,
  ServerIcon,
  ShieldCheckIcon,
  SlidersHorizontalIcon
} from 'lucide-react';

export const DEFAULT_SUBNET_VALUE = '__default_subnet__';
export const GITHUB_AUTH_MODE_APP = 'app';
export const DEFAULT_GITHUB_API_BASE_URL = 'https://api.github.com';
export const GITHUB_MANIFEST_QUERY_KEY = 'github_manifest';
export const GITHUB_MANIFEST_INSTALLATION_ID_QUERY_KEY = 'github_installation_id';
export const GITHUB_SETUP_MODE_CREATE = 'create';
export const GITHUB_SETUP_MODE_EXISTING = 'existing';
export const GITHUB_MANIFEST_OWNER_TARGET_PERSONAL = 'personal';
export const GITHUB_MANIFEST_OWNER_TARGET_ORGANIZATION = 'organization';
export const GITHUB_ORGANIZATION_SLUG_MAX_LENGTH = 39;

export const SETTINGS_NAV_ITEM = {
  id: 'settings',
  labelKey: 'nav.settings.label',
  descriptionKey: 'nav.settings.description',
  icon: KeyRoundIcon
};

export const NAV_ITEMS = [
  {
    id: 'overview',
    labelKey: 'nav.overview.label',
    descriptionKey: 'nav.overview.description',
    icon: LayoutDashboardIcon
  },
  {
    id: 'docs',
    labelKey: 'nav.docs.label',
    descriptionKey: 'nav.docs.description',
    icon: BookOpenTextIcon
  },
  {
    id: 'policies',
    labelKey: 'nav.policies.label',
    descriptionKey: 'nav.policies.description',
    icon: SlidersHorizontalIcon
  },
  {
    id: 'runners',
    labelKey: 'nav.runners.label',
    descriptionKey: 'nav.runners.description',
    icon: ServerIcon
  },
  {
    id: 'runner-images',
    labelKey: 'nav.runnerImages.label',
    descriptionKey: 'nav.runnerImages.description',
    icon: ImageIcon
  },
  {
    id: 'jobs',
    labelKey: 'nav.jobs.label',
    descriptionKey: 'nav.jobs.description',
    icon: BoxesIcon
  },
  {
    id: 'events',
    labelKey: 'nav.events.label',
    descriptionKey: 'nav.events.description',
    icon: ScrollTextIcon
  }
];

export const ALL_NAV_ITEMS = [...NAV_ITEMS, SETTINGS_NAV_ITEM];

export const SETUP_STEP_ORDER = ['password', 'github', 'oci'];

export const SETUP_STEP_META = {
  password: {
    id: 'password',
    labelKey: 'setup.step.password.label',
    titleKey: 'setup.step.password.title',
    descriptionKey: 'setup.step.password.description',
    icon: ShieldCheckIcon
  },
  github: {
    id: 'github',
    labelKey: 'setup.step.github.label',
    titleKey: 'setup.step.github.title',
    descriptionKey: 'setup.step.github.description',
    icon: GitBranchIcon
  },
  oci: {
    id: 'oci',
    labelKey: 'setup.step.oci.label',
    titleKey: 'setup.step.oci.title',
    descriptionKey: 'setup.step.oci.description',
    icon: CloudIcon
  }
};

export function createBlankGitHubConfigView() {
  return {
    id: '',
    name: '',
    tags: [],
    apiBaseUrl: '',
    authMode: '',
    appId: 0,
    installationId: 0,
    accountLogin: '',
    accountType: '',
    deletePath: '',
    selectedRepos: [],
    installationState: '',
    installationRepositorySelection: '',
    installationRepositories: [],
    installationReady: false,
    installationMissing: [],
    installationError: '',
    isActive: false,
    isStaged: false,
    lastTestedAt: '',
    createdAt: '',
    updatedAt: ''
  };
}

export function createBlankOCIAuthStatus() {
  return {
    effectiveMode: '',
    defaultMode: '',
    activeCredential: null,
    runtimeConfigReady: false,
    runtimeConfigMissing: []
  };
}

export function createBlankSetupStatus(session = null) {
  const passwordCompleted = !session?.mustChangePassword;
  const passwordStep = {
    completed: passwordCompleted,
    missing: passwordCompleted ? [] : ['setup.missing.newPassword']
  };
  const bootstrapSteps = {
    password: { ...passwordStep },
    github: {
      completed: false,
      missing: []
    },
    oci: {
      completed: false,
      missing: []
    }
  };

  return {
    completed: false,
    operationalReady: false,
    currentStep: passwordCompleted ? 'github' : 'password',
    bootstrapCompleted: false,
    bootstrapCurrentStep: passwordCompleted ? 'github' : 'password',
    updatedAt: '',
    steps: {
      password: { ...passwordStep },
      github: {
        completed: false,
        missing: []
      },
      oci: {
        completed: false,
        missing: []
      }
    },
    bootstrapSteps
  };
}

export function createBlankGitHubConfigStatus() {
  return {
    source: 'env',
    ready: false,
    stagedReady: false,
    configured: false,
    hasAppCredentials: false,
    hasWebhookSecret: false,
    lastTestedAt: '',
    webhookUrl: '',
    accountLogin: '',
    accountType: '',
    selectedRepos: [],
    missing: [],
    stagedMissing: [],
    stagedError: '',
    activeConfigs: [],
    activeAppDeleteSupported: false,
    activeAppDeletePathTemplate: '',
    activeConfig: null,
    stagedConfig: null,
    effectiveConfig: createBlankGitHubConfigView()
  };
}

export function createBlankGitHubManifestState() {
  return {
    pending: null,
    status: '',
    loading: false,
    creating: false,
    discovering: false,
    discoveryError: '',
    installations: [],
    autoInstallationId: 0
  };
}

export function createBlankOCIRuntimeStatus() {
  return {
    source: 'env',
    overrideSettings: null,
    effectiveSettings: {
      compartmentOcid: '',
      availabilityDomain: '',
      subnetOcid: '',
      nsgOcids: [],
      imageOcid: '',
      assignPublicIp: false,
      cacheCompatEnabled: false,
      cacheBucketName: '',
      cacheObjectPrefix: '',
      cacheRetentionDays: 0
    },
    ready: false,
    missing: []
  };
}

export function createBlankOCICatalogState() {
  return {
    availabilityDomains: [],
    subnets: [],
    images: [],
    shapes: [],
    sourceRegion: '',
    validatedAt: '',
    loading: false,
    loaded: false,
    error: '',
    params: {
      compartmentOcid: '',
      availabilityDomain: '',
      imageOcid: '',
      subnetOcid: ''
    }
  };
}

export function createBlankBillingReportState() {
  return {
    windowStart: '',
    windowEnd: '',
    granularity: 'DAILY',
    generatedAt: '',
    sourceRegion: '',
    tagNamespace: '',
    tagKey: '',
    tagAttributionReady: false,
    currency: '',
    ociBilledCost: 0,
    totalCost: 0,
    mappedCost: 0,
    tagVerifiedCost: 0,
    resourceFallbackCost: 0,
    tagOnlyCost: 0,
    unmappedCost: 0,
    lagNotice: '',
    scopeNote: '',
    items: [],
    issues: [],
    loading: false,
    loaded: false,
    error: '',
    days: 7
  };
}

export function createBlankBillingGuardrailsState() {
  return {
    generatedAt: '',
    windowDays: 0,
    items: [],
    loading: false,
    loaded: false,
    error: '',
    available: true
  };
}

export function createBlankGitHubDriftState() {
  return {
    generatedAt: '',
    severity: 'ok',
    activeConfigs: [],
    stagedConfig: null,
    issues: [],
    loading: false,
    loaded: false,
    error: '',
    available: true
  };
}

export function createBlankRunnerImagesState() {
  return {
    loaded: false,
    loading: false,
    error: '',
    preflight: {
      loaded: false,
      ready: false,
      blocked: false,
      status: '',
      summary: '',
      resultSummary: '',
      updatedAt: '',
      missing: [],
      notes: [],
      setupCommands: [],
      verifyCommands: [],
      checks: []
    },
    recipes: [],
    builds: [],
    resources: [],
    defaultImage: null,
    promotedImage: null
  };
}

export function createBlankRunnerImageRecipeForm() {
  return {
    name: '',
    displayName: '',
    baseImage: '',
    subnetOcid: '',
    shape: '',
    ocpu: 1,
    memoryGb: 16,
    description: '',
    setupCommandsText: '',
    verifyCommandsText: ''
  };
}
