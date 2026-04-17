---
app_docs: true
access: public
title: Operations and billing
slug: operations-and-billing
order: 50
section: Operate
summary: Understand Overview signals, admission diagnostics, runner lifecycle pages, and billing guardrails.
---

# Operations and billing

OhoCI keeps operational activity and OCI billing observability in the same workspace so you can compare launch behavior against cost signals.

## Overview is the first stop

![Overview workspace](/docs-assets/workspace/overview.png)

Overview is the fastest place to answer three questions:

1. Is setup complete and launch-ready?
2. Are jobs and runners moving as expected?
3. Is tracked runner cost lining up with the policy lanes you expect?

## What Overview is telling you

The page is split into a few operator jobs:

- the headline explains the current state in plain language
- the incident strip surfaces the single highest-priority risk or readiness signal
- the snapshot tiles summarize policies, runners, queued jobs, and errors
- the **Next action** card keeps one primary action and one supporting action visible
- the **Setup** card confirms access mode, launch readiness, and the default subnet
- the recent activity tabs give a short list of runners, jobs, and logs without leaving the page

Overview is also where the newer contract tends to surface first:

- budget-blocked policies show up as an incident before operators hunt through queues
- GitHub scope drift is called out when promoted routing no longer matches installation visibility
- warm coverage degradation is called out when a configured warm target no longer has its idle runner
- cache compatibility setup problems are summarized at a high level instead of being hidden inside runtime settings

## Admission diagnostics

When a job stays queued, open **Jobs** and read **Diagnostics** before guessing from symptoms.

OhoCI records admission in the same control-plane order it used for the decision:

1. setup readiness
2. repository scope
3. policy match
4. capacity
5. budget
6. warm pool
7. launch, registration, and attachment

A blocked stage tells you where launch stopped. A degraded budget stage means billing visibility is stale or unavailable, not that the launch was denied.

## Billing observability

The billing card combines two scopes for the same window:

- tenancy-scope OCI total billed cost
- OhoCI-tracked runner attribution

Budget guardrails use the same billing pipeline, but they evaluate policy snapshots refreshed every 15 minutes over a rolling 7-day window.

### What it includes

- OCI total billed cost
- OhoCI tracked runner cost
- non-OhoCI remainder
- OhoCI coverage
- tag-verified cost
- resource fallback cost
- unmapped cost

### How attribution works

- OhoCI first uses OCI billing tags on the launched instance.
- If tags are missing or do not match, it falls back to the tracked runner record by instance OCID.
- If cost cannot be mapped safely, it is shown as an explicit issue instead of being hidden.

### How budget guardrails behave

- A fresh snapshot over the cap blocks new launches for that policy.
- A missing, errored, or stale snapshot marks the policy as degraded and fails open.
- Fail-open means the launch path stays available, but operator incident visibility is reduced until snapshot freshness recovers.

### What it does not mean

This view is not a quota or settlement system. OCI usage aggregation can lag, and some attached-resource or unrelated tenancy charges can remain outside the tracked runner attribution view even while they are still counted in OCI total billed cost.

## GitHub scope drift

OhoCI compares promoted routing against current installation visibility from two sources:

- GitHub webhook updates when the installation scope changes
- a background reconcile that runs every 15 minutes

Drift is intentionally review-first:

- missing selected repositories are treated as a problem because OhoCI can no longer manage what you promoted locally
- newly visible repositories are shown as optional review work, not auto-adopted scope
- OhoCI does not auto-prune the local repository allowlist when GitHub visibility changes

Use the high-level drift surfaces in Setup, Settings, and Overview to decide whether to adjust the GitHub installation or the local selection.

## Runner, job, and event pages

- **Runners** answer "what machine exists right now, and what state is it in?"
- **Jobs** answer "did GitHub send work, and did OhoCI find a policy for it?"
- **Events** answer "what exactly happened inside webhook handling and control-plane execution?"

Use them together rather than in isolation:

1. start from Overview
2. jump to Jobs if work is still queued
3. jump to Runners if a machine launched
4. jump to Events whenever the reason is still unclear

## Warm and cache signals

- Warm capacity is a policy and repository opt-in. In v1, OhoCI keeps at most one idle runner per policy and repository target.
- Experimental cache compatibility stores cache data in OCI Object Storage alongside OCI runtime settings and OhoCI-managed runners. If it is unavailable, workflows degrade to cache miss behavior instead of failing cache steps outright.

## When the top-bar actions matter

- **Refresh** is the lightweight way to re-read current state after you changed setup, policies, or GitHub-side workflow conditions.
- **Run cleanup** is for cases where a runner looks stale or termination seems delayed.

Those controls are intentionally absent from the in-app docs screen because they are runtime controls, not reading tools.

## Related guides

- [Setup guide](./setup-guide.md)
- [Policies and capacity](./policies-and-capacity.md)
- [Runner Images](./runner-images.md)
- [Troubleshooting](./troubleshooting.md)
