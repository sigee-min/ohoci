---
app_docs: true
access: public
title: Runner Images
slug: runner-images
order: 40
section: Operate
summary: Create recipes, bake and verify images, promote a successful build to the default runtime image, and rediscover OhoCI-owned OCI resources.
---

# Runner Images

Runner Images is the prepared-image workflow for OhoCI. Use it when the default OCI image is too bare and you want a repeatable way to install tools, prove they work, and roll that image into future runner launches.

![Runner Images screen](/docs-assets/workspace/runner-images.png)

## What it manages

| Surface | Purpose |
| --- | --- |
| Recipe | Saved plan with a base image, build lane, setup commands, and verify commands. |
| Build | One bake attempt created from a saved recipe. |
| Promotion | Makes a successful captured image the default runtime image for new runner launches. |
| Discovered OCI resources | Bake instances and captured images that OhoCI can still find from its ownership tags. |

## Before you start

- Finish OCI runtime setup from [Setup guide](./setup-guide.md).
- Keep the first recipe narrow and tied to one clear toolchain or workflow need.
- Start from a base image that already has the right operating system and network posture.

## The normal workflow

### 1. Save a recipe

Each recipe should define:

- a recipe name and short summary
- the base image OCID to build from
- the build shape, OCPU, and memory
- an optional bake subnet override if image preparation needs a different network
- a captured image display name
- setup commands, one per line
- verify commands, one per line

Use setup commands for installation and configuration. Use verify commands for proof. If verify commands do not succeed, OhoCI does not capture the image.

### 2. Start one build

Start a build from the saved recipe. OhoCI queues the work, launches a temporary bake instance from the recipe's base image, runs the setup commands, runs the verify commands, and captures a new OCI image only after verification passes.

If setup fails, verification fails, or the instance stops before OhoCI sees a success result, the build ends in `failed` with the latest summary preserved on the build row.

### 3. Watch build progress

The first bake should stay small and easy to explain. Common states are:

- `queued`: OhoCI accepted the build and is waiting to start it
- `launching` or `provisioning`: the bake instance is coming up and running setup work
- `verifying`: OhoCI is checking the verify commands
- `creating_image`: verification passed and OCI image capture is in progress
- `available`: the image is ready to promote
- `failed`: the bake did not finish cleanly
- `promoted`: this build's image became the runtime default

The **Preflight** panel is the fastest way to confirm that OCI runtime settings are still ready before you retry a build.

### 4. Promote only after the image is ready

Promote only from an `available` build. Promotion updates the default runtime image OhoCI uses for future launches.

It does not rewrite policies, and it does not replace runners that are already alive. The next matching runner launch uses the promoted image.

### 5. Use rediscovery when local tracking is missing

The **Discovered OCI resources** panel exists for recovery as well as day-to-day visibility. OhoCI marks bake instances and captured images with OhoCI ownership tags so it can find them again later.

That means a successful bake can still show up after OCI access is restored, even if local tracking was lost. In that panel:

- `Tracked` means the resource is still linked to current local state
- `Discovered` means OhoCI found it again from its ownership tags in OCI

This is especially useful after reconnecting OCI credentials or recovering from lost database state.

## Safe rollout pattern

1. Save one narrow recipe.
2. Start one build.
3. Wait for `available`.
4. Promote the image.
5. Send one real workflow job that uses the same policy lane.
6. Only then widen recipe usage or create more prepared images.

## Related guides

- [Getting started](./getting-started.md)
- [Policies and capacity](./policies-and-capacity.md)
- [Operations and billing](./operations-and-billing.md)
- [Troubleshooting](./troubleshooting.md)
