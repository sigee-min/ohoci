---
app_docs: true
access: public
title: Troubleshooting
slug: troubleshooting
order: 60
section: Diagnose
summary: Find the fastest checks when jobs stay queued, warm or budget signals degrade, cache compatibility misses, or billing gaps appear.
---

# Troubleshooting

When something looks wrong, start from the narrowest page that can prove the issue.

## First triage order

Use this order before changing anything:

1. check Overview for the current high-level state
2. open Jobs diagnostics, GitHub drift, or the most relevant page for the symptom
3. only then change Settings, Policies, or cleanup state

## Jobs stay queued

Check these first:

- the repository is included in the GitHub installation and selected in OhoCI
- an enabled policy exactly matches the job labels
- OCI runtime settings are still valid in Settings
- the policy has not hit its `Max runners` limit
- if budget guardrails are enabled, confirm the job is actually budget-blocked instead of only budget-degraded
- if warm capacity is enabled, confirm the job was allowed to reuse warm capacity or to fall through to launch

If the match still looks correct, inspect **Jobs** diagnostics and **Events** together. The Jobs view shows the control-plane order that passed or blocked, while Events tells you why webhook handling or launch execution stopped.

## Runner launched but workflow did not attach

Use **Events** and **Runners** together:

- confirm the runner registration token was issued
- check whether the runner moved to `in_progress`
- review event logs for GitHub installation lookup, shared-webhook signature, or delivery processing failures

Also confirm the repository is both included in the GitHub installation and selected in OhoCI. A repo that is only present in one side of that intersection will not receive managed runner handling.

If Jobs diagnostics already passed repo, policy, capacity, budget, and warm stages, focus on registration and attachment instead of re-checking policy logic.

## Runner did not terminate

1. Check whether the job reached a terminal state.
2. Confirm a terminate request appears in Events.
3. Run manual cleanup if the runner still looks live.
4. Re-check the runner record after cleanup finishes.

If the same runner repeatedly survives cleanup, treat that as a control-plane defect rather than normal lag and capture the corresponding event logs.

## Warm capacity was expected, but OhoCI launched fresh capacity

Check the warm contract, not just the label match:

- the policy has warm capacity enabled
- the repository is listed in that policy's warm allowlist
- the policy still has room inside `Max runners`
- the target does not already have its one idle warm runner for this v1 contract

If no idle warm runner is available, OhoCI falls back to the normal launch path. That is expected behavior, not a separate failure.

## OCI setup looks ready, but launch still fails

Re-check Settings with the OCI catalog in mind:

- refresh the catalog from the runtime tab
- confirm the saved subnet is still returned by OCI
- confirm the saved image is still returned by OCI
- confirm the saved credential still tests successfully

Stale launch targets often look like random launch failures until you refresh the inventory.

## Runner image build does not become available

Start from **Runner Images** instead of guessing from runner job behavior:

- confirm the **Preflight** panel still shows OCI runtime as ready
- confirm the recipe base image is still valid in OCI
- if the recipe pins a bake subnet, confirm that subnet is still valid and reachable
- use the latest build summary to separate setup failures from verify failures
- refresh state or advance builds again if the OCI image is still being captured

If a build repeatedly returns to `failed`, treat the saved build summary as the source of truth before editing the recipe again.

## Runner image resources seem to disappear after OCI reconnect

Open **Runner Images** and check **Discovered OCI resources**.

OhoCI marks bake instances and captured images with OhoCI ownership tags, so it can find them again after OCI access is restored. If local tracking was lost, a resource can return as `Discovered` instead of `Tracked`.

If nothing reappears, confirm the OCI credential can still read the same compartment and that the original bake completed far enough to create a managed bake instance or captured image.

## Budget guardrail shows degraded or stale

Start from the policy budget signal instead of assuming launches should stop:

- policy budget snapshots refresh every 15 minutes
- the guardrail evaluates a rolling 7-day window
- missing, errored, or stale snapshots fail open and mark the policy as degraded

Fail-open means new launches can still proceed. The problem is reduced incident visibility, not an automatic launch stop.

If a policy is actually blocked, confirm the latest fresh snapshot shows cost at or above the configured cap.

## Billing card shows gaps

The billing card can show issues like:

- missing policy tag with resource fallback
- stale policy tag without a tracked runner
- unmapped resource usage

Those issues usually mean one of three things:

- OCI usage data is delayed
- the runner record existed but tags were incomplete
- OCI returned cost for a resource that OhoCI could not safely map

Treat unmapped cost as something to explain, not something to ignore. If the same pattern repeats, compare the issue list against runner records and recent Events.

## GitHub setup verified, but no repositories can be selected

That usually means the installation scope and the local allowlist do not overlap:

- the app installation does not include the repository
- or the repository is not selected in OhoCI
- or the installation is limited to selected repositories and the repo was never added there

Fix the installation scope first, then update the local OhoCI allowlist.

## GitHub drift is reported

Read the drift signal as a scope review problem:

- `Selected repositories missing from installation` means your promoted local selection no longer exists in the GitHub installation scope
- `New repositories visible` means GitHub can now see repositories that OhoCI has not selected locally

Use **Reconcile drift** after GitHub-side changes, or wait for the 15-minute background reconcile. OhoCI does not auto-prune the local selection for you.

## Duplicate webhook deliveries or repeated launches

Check the GitHub side before changing policies:

- confirm the GitHub App webhook points only to OhoCI's shared `/api/v1/github/webhook` endpoint
- remove leftover repository-level webhooks from older pre-App deployments
- if a staged App exists, promote it only after the installation and selected repository set both show ready

## Cache compatibility is unavailable or always misses

Treat cache compatibility as optional runtime behavior:

- it is experimental in this version
- it applies only to OhoCI-managed Linux runners
- the runner version should be `2.327.1` or newer
- the OCI runtime settings must include the cache bucket and retention fields

If the cache backend is unavailable, workflows should degrade to cache miss behavior rather than failing cache steps outright. If cache steps are failing hard, inspect the workflow and runner setup instead of assuming the cache service blocked the job.

## Related guides

- [Setup guide](./setup-guide.md)
- [Getting started](./getting-started.md)
- [Runner Images](./runner-images.md)
- [Operations and billing](./operations-and-billing.md)
