---
app_docs: true
access: public
title: Getting started
slug: getting-started
order: 20
section: Start here
summary: Use the main workspace views after setup is complete and know where to go for the next action.
---

# Getting started

After setup finishes, OhoCI opens the authenticated workspace. The app keeps setup, policies, live activity, and billing observability in one shell so you can move from readiness checks to runner operations without switching tools.

## Start with the workspace map

| View | Use it for |
| --- | --- |
| Overview | The fastest answer to "are we ready?" and "what needs attention right now?" |
| Docs | User-facing product guides inside the authenticated shell. |
| Policies | Exact label-set matching, subnet choice, shape choice, concurrency, and TTL. |
| Runners | Machine-level lifecycle state for active and recent OCI runner instances. |
| Runner Images | Prepared-image recipes, bake history, promotion, and OCI rediscovery for runner images. |
| Jobs | Workflow job state and whether each job found a policy match. |
| Events | Webhook deliveries, control-plane logs, and failure details. |
| Settings | GitHub routing and staged settings, local repo allowlist, OCI credential storage, and launch defaults. |

## Revisit Settings after setup

![Settings workspace](/docs-assets/workspace/settings.png)

Settings is not just a first-run screen. Come back here when you need to:

- stage GitHub settings or review active app routing
- change which repositories OhoCI is allowed to manage within the installed set
- save a different OCI credential
- refresh the launch catalog and switch subnet or image targets
- fall back to environment defaults if you want to clear saved values

## What the first day should look like

The calm, low-risk path is:

1. confirm Setup shows launch readiness
2. create one narrow policy
3. queue one GitHub Actions job with the exact same labels
4. watch Overview, Runners, and Events together until the full lifecycle is clear
5. only then widen repository scope or policy capacity

## Public docs and in-app docs

OhoCI exposes the same curated docs in two places:

- public docs at `/docs` without authentication
- the in-app **Docs** view after sign-in

That means operators can keep setup instructions open publicly while they finish setup inside the authenticated shell.

## Language support

The UI now supports two locales:

- English (`en`) as the default
- Korean (`ko`) as an optional operator-selected locale

The language switch affects the app chrome, setup flow, and docs navigation UI. The markdown document body itself remains English-first in this version.

## Recommended next reads

- Start with [Setup guide](./setup-guide.md) if setup is not complete yet.
- Read [Policies and capacity](./policies-and-capacity.md) before creating label rules.
- Read [Runner Images](./runner-images.md) if the base OCI image needs extra tools or a repeatable bake workflow.
- Read [Operations and billing](./operations-and-billing.md) to understand runtime and cost signals.
- Keep [Troubleshooting](./troubleshooting.md) nearby for the first live test run.
