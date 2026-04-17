---
app_docs: true
access: public
title: Policies and capacity
slug: policies-and-capacity
order: 30
section: Configure
summary: Match repository labels to OCI shapes, capacity limits, warm reuse, and budget guardrails.
---

# Policies and capacity

Policies are the launch lanes for GitHub jobs. Each one defines which labels match, what OCI shape to use, how many runners can be alive at once, and whether warm reuse or budget guardrails apply.

## Admission order

OhoCI evaluates queued work in a fixed control-plane order:

1. setup readiness
2. repository scope
3. policy match
4. capacity
5. budget
6. warm
7. launch and attach

That order matters operationally:

- setup must be complete before repository scope and policy matching begin
- the repository must be visible to the GitHub App installation and selected locally before policy matching starts
- label matching happens only after repository scope is accepted
- `Max runners` is checked before any fresh launch
- budget guardrails can stop a new launch only after a policy matched
- warm reuse is checked after repo, policy, capacity, and budget all passed

The Jobs diagnostics view and the policy compatibility check expose the same stages so operators can explain exactly where admission stopped.

## How matching works

- `self-hosted` is treated as a managed infrastructure label and is not used as the business match key.
- The remaining labels must match one enabled policy exactly.
- If no enabled policy matches, the job stays queued and OhoCI records the mismatch in activity logs.

## What belongs in the label set

Keep label sets small and intentional:

- use labels that describe runner intent, not incidental repo metadata
- avoid creating two policies that differ only by one vague label
- keep the GitHub workflow label list identical to the policy label list after `self-hosted` is removed

If a job is expected to land on a certain policy, you should be able to explain that match in one sentence.

## What you configure in a policy

### Shape and capacity

- Choose one shape from the current OCI runtime catalog.
- Fixed shapes lock OCPU and memory to OCI defaults.
- Flexible shapes keep OCPU and memory editable, but only inside OCI limits.
- `Max runners` caps how many live runners this policy can keep at once.
- Warm capacity does not create a separate quota. It still lives inside the same policy capacity.
- TTL defines how long a runner can survive before cleanup considers it expired.

### Warm capacity

- Warm capacity is off by default and is a policy-level opt-in.
- Warm targets are repository-scoped. Only repositories in the policy allowlist can receive a warm runner.
- In v1, `warmMinIdle` is effectively `0` or `1`.
- OhoCI keeps at most one idle warm runner per policy and repository target in v1.
- If no idle warm runner is available, the job falls back to the normal launch path.
- Keep the allowlist narrow. Warm capacity is meant for the few repositories where startup latency matters enough to pay for one idle runner.

### Budget guardrails

- Budget guardrails are off by default and are a policy-level opt-in.
- OhoCI evaluates them from billing snapshots refreshed every 15 minutes over a rolling 7-day window.
- A fresh snapshot that meets or exceeds the policy cap blocks new launches for that policy.
- Missing, errored, or stale billing snapshots mark budget as degraded and fail open. Launches continue, but incident visibility is degraded until billing data recovers.
- Use guardrails to slow or stop fresh launches, not as a settlement or quota system.

### Network choice

- Leave **Runner subnet** empty to use the default subnet from Settings.
- Pin a subnet only when one label lane must launch in a different network segment.
- Refresh subnet suggestions if OCI inventory changed or the default subnet is no longer the best fit.

### Availability tradeoffs

- enable spot capacity only when the workflow can tolerate interruption
- keep capacity low on brand-new policies until the first job completes cleanly
- use one policy per operationally distinct runner lane instead of one giant catch-all policy
- enable warm capacity only for repositories that actually benefit from one kept-idle runner
- treat budget-degraded state as an observability incident even when launches still continue

## Safe policy workflow

1. Confirm OCI runtime settings are valid in Settings.
2. Create a narrow label set.
3. Save one enabled policy.
4. If you need warm capacity, start with one repository in the warm allowlist.
5. Send one test job with the same labels.
6. Check Overview, Jobs diagnostics, Runners, and Events before widening capacity.

## What to watch after saving

The first matched job should confirm all of these:

- the job leaves queued state
- Jobs diagnostics show the expected pass or block order
- a runner appears with the expected policy and launch target
- the runner registers to the correct repository
- cleanup terminates the runner after the job reaches a terminal state

If any of those fail, go directly to [Troubleshooting](./troubleshooting.md) before adding more policies.

## Related guides

- Go back to [Getting started](./getting-started.md) for the workspace tour.
- Continue with [Operations and billing](./operations-and-billing.md) to read runtime and cost signals after launch.
- Use [Troubleshooting](./troubleshooting.md) when a matched job still does not launch correctly.
