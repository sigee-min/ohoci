import test from 'node:test';
import assert from 'node:assert/strict';

import { buildSetupFlow } from '../src/lib/setup-flow.js';

function buildFixture(overrides = {}) {
  return {
    session: {
      authenticated: true,
      mustChangePassword: false
    },
    setupStatus: {
      completed: false,
      steps: {
        password: { completed: true, missing: [] },
        github: { completed: false, missing: ['selectedRepos'] },
        oci: { completed: false, missing: ['runtime'] }
      },
      bootstrapSteps: {
        password: { completed: true, missing: [] },
        github: { completed: false, missing: ['app'] },
        oci: { completed: false, missing: ['credential'] }
      }
    },
    githubConfigStatus: {
      hasAppCredentials: false,
      hasWebhookSecret: false,
      activeConfig: null,
      selectedRepos: []
    },
    ociAuthStatus: {
      effectiveMode: '',
      defaultMode: '',
      activeCredential: null
    },
    ociRuntimeStatus: {
      ready: false
    },
    ...overrides
  };
}

test('buildSetupFlow stays inactive before authentication', () => {
  const flow = buildSetupFlow(buildFixture({
    session: {
      authenticated: false,
      mustChangePassword: true
    }
  }));

  assert.equal(flow.currentTaskId, '');
  assert.equal(flow.tasks[0].status, 'upcoming');
  assert.equal(flow.tasks[0].isEditable, false);
});

test('buildSetupFlow starts at password when a password change is still required', () => {
  const flow = buildSetupFlow(buildFixture({
    session: {
      authenticated: true,
      mustChangePassword: true
    },
    setupStatus: {
      completed: false,
      steps: {
        password: { completed: false, missing: ['setup.missing.newPassword'] },
        github: { completed: false, missing: [] },
        oci: { completed: false, missing: [] }
      },
      bootstrapSteps: {
        password: { completed: false, missing: ['setup.missing.newPassword'] },
        github: { completed: false, missing: [] },
        oci: { completed: false, missing: [] }
      }
    }
  }));

  assert.equal(flow.currentTaskId, 'password');
  assert.equal(flow.tasks[0].status, 'current');
});

test('buildSetupFlow advances to OCI credential after GitHub access is live', () => {
  const flow = buildSetupFlow(buildFixture({
    githubConfigStatus: {
      hasAppCredentials: true,
      hasWebhookSecret: true,
      activeConfig: {
        installationReady: true,
        selectedRepos: []
      },
      selectedRepos: []
    }
  }));

  assert.equal(flow.currentTaskId, 'oci-credential');
  assert.equal(flow.tasks.find((task) => task.id === 'github-connect')?.status, 'complete');
});

test('buildSetupFlow treats a saved OCI credential as enough to clear the access phase', () => {
  const flow = buildSetupFlow(buildFixture({
    githubConfigStatus: {
      hasAppCredentials: true,
      hasWebhookSecret: true,
      activeConfig: {
        installationReady: true,
        selectedRepos: []
      },
      selectedRepos: []
    },
    ociAuthStatus: {
      effectiveMode: 'api_key',
      defaultMode: 'api_key',
      activeCredential: {
        id: 'cred-1'
      }
    }
  }));

  assert.equal(flow.currentTaskId, 'github-repositories');
  assert.equal(flow.tasks.find((task) => task.id === 'oci-credential')?.status, 'complete');
});

test('buildSetupFlow keeps repository scope blocking until at least one repo is selected', () => {
  const flow = buildSetupFlow(buildFixture({
    githubConfigStatus: {
      hasAppCredentials: true,
      hasWebhookSecret: true,
      activeConfig: {
        installationReady: true,
        selectedRepos: []
      },
      selectedRepos: []
    },
    ociAuthStatus: {
      effectiveMode: 'api_key',
      defaultMode: 'api_key',
      activeCredential: { id: 'cred-1' }
    }
  }));

  assert.equal(flow.currentTaskId, 'github-repositories');
  assert.equal(flow.tasks.find((task) => task.id === 'oci-runtime')?.status, 'upcoming');
});

test('buildSetupFlow advances to launch target after repositories are selected', () => {
  const flow = buildSetupFlow(buildFixture({
    githubConfigStatus: {
      hasAppCredentials: true,
      hasWebhookSecret: true,
      activeConfig: {
        installationReady: true,
        selectedRepos: ['acme/repo']
      },
      selectedRepos: ['acme/repo']
    },
    ociAuthStatus: {
      effectiveMode: 'api_key',
      defaultMode: 'api_key',
      activeCredential: { id: 'cred-1' }
    }
  }));

  assert.equal(flow.currentTaskId, 'oci-runtime');
  assert.equal(flow.tasks.find((task) => task.id === 'github-repositories')?.status, 'complete');
});

test('buildSetupFlow ignores staged repository scope until the live route carries it', () => {
  const flow = buildSetupFlow(buildFixture({
    githubConfigStatus: {
      hasAppCredentials: true,
      hasWebhookSecret: true,
      activeConfig: {
        installationReady: true,
        selectedRepos: []
      },
      selectedRepos: ['acme/repo']
    },
    ociAuthStatus: {
      effectiveMode: 'api_key',
      defaultMode: 'api_key',
      activeCredential: { id: 'cred-1' }
    }
  }));

  assert.equal(flow.currentTaskId, 'github-repositories');
  assert.equal(flow.tasks.find((task) => task.id === 'github-repositories')?.status, 'current');
});

test('buildSetupFlow reports completion when all five tasks are complete', () => {
  const flow = buildSetupFlow(buildFixture({
    setupStatus: {
      completed: true,
      steps: {
        password: { completed: true, missing: [] },
        github: { completed: true, missing: [] },
        oci: { completed: true, missing: [] }
      },
      bootstrapSteps: {
        password: { completed: true, missing: [] },
        github: { completed: true, missing: [] },
        oci: { completed: true, missing: [] }
      }
    },
    githubConfigStatus: {
      hasAppCredentials: true,
      hasWebhookSecret: true,
      activeConfig: {
        installationReady: true,
        selectedRepos: ['acme/repo']
      },
      selectedRepos: ['acme/repo']
    },
    ociAuthStatus: {
      effectiveMode: 'api_key',
      defaultMode: 'api_key',
      activeCredential: { id: 'cred-1' }
    },
    ociRuntimeStatus: {
      ready: true
    }
  }));

  assert.equal(flow.completed, true);
  assert.deepEqual(flow.blockingTasks, []);
});
