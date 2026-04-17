---
app_docs: true
access: public
title: Setup guide
slug: setup-guide
order: 10
section: Start here
summary: Finish password, GitHub, and OCI onboarding in the right order before the dashboard unlocks.
---

# Setup guide

OhoCI blocks the main dashboard until the first-run setup is complete. The onboarding order is fixed so the control plane always has a valid admin password, a verified GitHub source, and a usable OCI launch target before it accepts runner work.

This guide assumes one active OhoCI instance. In this version there is no distributed locking or multi-replica coordination.

## What you should have ready

| Item | Why it matters |
| --- | --- |
| Local admin access | You sign in with the local administrator account first. |
| A GitHub App registration and installation | OhoCI stages the app config, verifies the installation, and only manages repositories that are both installed and locally selected. |
| OCI config and private key | The OCI step reads your profile details from the config, then stores only the derived credential material it needs. |
| OCI launch target values | You need a compartment, availability domain, subnet, and image before runner launches can be marked ready. |
| Optional Object Storage bucket details | Only needed if you plan to enable experimental `actions/cache` compatibility for OhoCI-managed Linux runners. |

## Sign in and enter onboarding

Use the bootstrap account once, then replace it immediately:

- username: `admin`
- password: `admin`

After the first successful sign-in, OhoCI does **not** drop you into the dashboard. It opens the onboarding rail and keeps the later steps locked until the current required step is saved.

If your operators prefer Korean, switch the UI locale after the app loads. English remains the default locale for the control plane.

## Step 1: replace the bootstrap password

![Password step](/docs-assets/onboarding/password-step.png)

The password step is intentionally simple:

1. Enter the current bootstrap password.
2. Choose the new long-lived admin password.
3. Save it before touching any GitHub or OCI settings.

What to watch for:

- the rail should mark **Password** as `Done`
- the next step should unlock automatically
- the "Still missing" panel should stop listing `new password`

## Step 2: connect GitHub

![GitHub step](/docs-assets/onboarding/github-step.png)

The GitHub step stages GitHub App credentials, verifies installation access, and then narrows OhoCI to the repositories you explicitly choose.

### What to enter

- save the GitHub App credentials and installation binding for the environment you want OhoCI to use, or stage another credential set before promotion
- saving directly to an already-live routed app installation automatically retires the older live row for that same GitHub API URL + App ID + installation ID
- add an optional operator-facing app name and audit tags if you want a clearer label in OhoCI
- leave **GitHub API URL** empty unless you are pointing to a GitHub Enterprise API base URL
- point the app webhook at OhoCI's shared webhook endpoint
- verify the installation before promoting any staged config

### Using the github.com manifest helper

If you are onboarding against github.com, OhoCI can create the GitHub App registration for you:

1. Leave **GitHub API URL** empty, or keep it at `https://api.github.com`.
2. Choose where you will use the app: **Personal repositories** or **Organization repositories**.
3. If you chose **Organization repositories**, enter the GitHub organization slug from `github.com/<slug>`.
4. Click **Create on GitHub** in the GitHub setup card. OhoCI opens the matching github.com app creation page with the webhook, required permissions, and the explicit `workflow_job` subscription already prepared. GitHub delivers installation lifecycle events such as `installation` and `installation_repositories` by default, so they are not listed separately in the helper manifest.
5. Finish the GitHub registration page that opens from the OhoCI manifest.
6. Install the new app on GitHub.
7. After you approve the installation, GitHub sends you back to OhoCI with the returned installation. OhoCI restores the created app in the current session, fills the installation ID when GitHub returns it, and resumes installation discovery when it has enough data.
8. If you manually return before install is done, OhoCI keeps that created app loaded so you can reopen the install page or check installations from the setup screen.
9. Verify access, confirm the local repository allowlist, stage the config, then promote it when ready.

If GitHub returns without a single resolved installation, OhoCI keeps the created app loaded and you can finish by choosing the correct installation from the setup screen.

The manifest helper is intentionally disabled when you set a non-default GitHub API URL. In this version, GitHub Enterprise Server-style API endpoints still require manual GitHub App registration.

### How repository selection works

- GitHub exposes the repositories visible to the installation
- OhoCI lets you choose a local allowlist from that installed set
- the effective managed scope is the intersection of the installation scope and the local allowlist

OhoCI also keeps watching for scope drift after setup:

- installation webhook updates can change what GitHub says the app can see
- a background reconcile runs every 15 minutes even when no webhook changed recently
- local repository selections are never auto-pruned just because GitHub visibility changed

If a selected repository disappears from the installation, OhoCI flags that drift for operator review instead of silently deleting the local selection. If new repositories become visible, OhoCI shows them as newly visible until you choose whether to add them locally.

If more than one GitHub App is already active, Setup shows them as a compact **Active apps** list with the app name, install target, local repo count, and audit tags. Rotating credentials for the same routed app installation replaces the older live row automatically, while unrelated apps or installations stay listed side by side. Use that list to review routing coverage and audit labels before you stage or promote another one.

### What "ready" means here

The GitHub step is ready only when all of these are true:

- the active or staged App credentials verify successfully
- the installation is reachable and visible
- a webhook secret is available
- at least one repository is selected

If you are rotating GitHub credentials or moving to a different installation, stage the new credential set, confirm it is ready, then promote it. Promotion automatically retires the previous live row for that same GitHub API URL + App ID + installation ID. Different routed apps or installations stay active. The staged config must use a different webhook secret from any active config.

## Step 3: configure OCI access and launch settings

![OCI step](/docs-assets/onboarding/oci-step.png)

The OCI step has two tabs because OhoCI needs both a credential path and a launch target.

### Credential tab

Use the **Credential** tab to save the OCI identity material:

- **Credential name** is your operator-facing label for the saved credential
- **Profile name** usually stays `DEFAULT` unless your OCI config uses another profile
- **OCI config file** and **Private key file** can be uploaded directly
- **Parsed config** and **Private key PEM** let you review or paste the values manually
- **Passphrase** stays empty unless your private key is protected

The safe sequence is:

1. Upload the OCI config file.
2. Upload the private key file.
3. Confirm the parsed profile, region, tenancy, and fingerprint.
4. Run **Test connection**.
5. Click **Save and use**.

### Runtime target tab

Use the **Runtime target** tab to tell OhoCI where runners should launch:

- enter **Compartment OCID**
- click **Refresh catalog**
- choose an **Availability domain**
- choose a **Subnet OCID**
- choose an **Image OCID**
- optionally add **NSG OCIDs**
- decide whether to enable **Assign public IP**

The image you choose here is the current default runtime image. Later, [Runner Images](./runner-images.md) can replace that default by promoting a verified baked image.

OhoCI marks launch setup ready only when the compartment, subnet, and image are all present and still valid against the latest OCI catalog.

### Optional experimental cache compatibility

The runtime target also carries the optional cache compatibility settings backed by OCI Object Storage.

- enable it only for OhoCI-managed Linux runners
- provide the Object Storage bucket and retention days
- keep the runner version at `2.327.1` or newer
- treat it as experimental in this version

If the cache backend becomes unavailable, workflows fall back to cache misses instead of failing cache steps outright.

## Before you leave setup

You should see:

- **Password** marked `Done`
- **GitHub** marked `Done`
- **OCI** marked `Done`
- the setup counter reading `Ready`

If the dashboard still does not unlock, use the rail support panel and the "Still missing" message as the source of truth for the next field to fix.

The onboarding rail, header, and setup actions follow the selected UI locale, but the guide text here remains English-first in this version.

## What to do next

Once onboarding is complete:

1. open [Getting started](./getting-started.md) for the workspace tour
2. create one policy from [Policies and capacity](./policies-and-capacity.md)
3. if you plan to use warm capacity, start with one repository allowlist target and a single idle runner expectation
4. if the base OCI image still needs packages or tools, prepare it in [Runner Images](./runner-images.md)
5. send a single test workflow job before scaling further
