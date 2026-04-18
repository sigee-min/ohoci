import { SETUP_FLOW_GROUPS, SETUP_FLOW_TASK_META, SETUP_FLOW_TASK_ORDER } from './workspace-constants.js';

function collectSelectedRepos(githubConfigStatus = {}) {
  if (!githubConfigStatus.activeConfig) {
    return [];
  }

  if (Array.isArray(githubConfigStatus.activeConfig.selectedRepos)) {
    return githubConfigStatus.activeConfig.selectedRepos;
  }

  const selectedRepos = Array.isArray(githubConfigStatus.selectedRepos)
    ? githubConfigStatus.selectedRepos
    : [];
  return selectedRepos;
}

function hasLiveGitHubRoute(githubConfigStatus = {}) {
  return Boolean(
    githubConfigStatus.activeConfig
    && githubConfigStatus.hasAppCredentials
    && githubConfigStatus.hasWebhookSecret
    && githubConfigStatus.activeConfig.installationReady !== false
  );
}

function hasOCISetupCredential(ociAuthStatus = {}, setupStatus = {}) {
  if (ociAuthStatus.activeCredential) {
    return true;
  }

  if (setupStatus?.bootstrapSteps?.oci?.completed) {
    return true;
  }

  return false;
}

export function buildSetupFlow({
  session = null,
  setupStatus = {},
  githubConfigStatus = {},
  ociAuthStatus = {},
  ociRuntimeStatus = {}
} = {}) {
  if (!session?.authenticated) {
    const tasks = SETUP_FLOW_TASK_ORDER.map((taskId) => ({
      id: taskId,
      complete: false,
      status: 'upcoming',
      isEditable: false,
      blocking: true
    }));

    return {
      completed: false,
      currentTaskId: '',
      blockingTasks: tasks,
      tasks,
      groups: SETUP_FLOW_GROUPS.map((group) => ({
        ...group,
        tasks: tasks.filter((task) => SETUP_FLOW_TASK_META[task.id]?.groupId === group.id)
      }))
    };
  }

  const passwordComplete = Boolean(setupStatus.steps?.password?.completed ?? !session?.mustChangePassword);
  const githubConnectComplete = hasLiveGitHubRoute(githubConfigStatus);
  const ociCredentialComplete = hasOCISetupCredential(ociAuthStatus, setupStatus);
  const githubRepositoriesComplete = collectSelectedRepos(githubConfigStatus).length > 0;
  const ociRuntimeComplete = Boolean(ociRuntimeStatus.ready || setupStatus.steps?.oci?.completed);

  const completionByTask = {
    password: passwordComplete,
    'github-connect': githubConnectComplete,
    'oci-credential': ociCredentialComplete,
    'github-repositories': githubRepositoriesComplete,
    'oci-runtime': ociRuntimeComplete
  };

  const currentTaskId = SETUP_FLOW_TASK_ORDER.find((taskId) => !completionByTask[taskId]) || '';
  const tasks = SETUP_FLOW_TASK_ORDER.map((taskId) => {
    const complete = completionByTask[taskId];
    return {
      id: taskId,
      complete,
      status: complete ? 'complete' : taskId === currentTaskId ? 'current' : 'upcoming',
      isEditable: complete || taskId === currentTaskId,
      blocking: !complete
    };
  });
  const blockingTasks = tasks.filter((task) => task.blocking);

  return {
    completed: tasks.every((task) => task.complete),
    currentTaskId,
    blockingTasks,
    tasks,
    groups: SETUP_FLOW_GROUPS.map((group) => ({
      ...group,
      tasks: tasks.filter((task) => SETUP_FLOW_TASK_META[task.id]?.groupId === group.id)
    }))
  };
}
