---
app_docs: true
access: public
title: Setup guide
slug: setup-guide
order: 10
section: Start here
summary: Finish the five first-run setup tasks that unlock jobs and the main workspace.
---

# Setup guide

OhoCI blocks jobs and the main workspace until the first-run setup is complete. After sign-in, the app stays in one setup shell and walks through the same five tasks every time.

This guide assumes one active OhoCI instance. In this version there is no distributed locking or multi-replica coordination.

## What you should have ready

| Item | Why it matters |
| --- | --- |
| Local admin access | You sign in with the local administrator account first. |
| A GitHub App registration and installation | OhoCI verifies the route first, then only manages repositories that are both installed and selected locally. |
| OCI config and private key | The OCI credential task stores the access path the control plane needs. |
| Repository choices | Jobs stay locked until OhoCI has at least one selected repository. |
| OCI launch target values | You need a compartment, availability domain, subnet, and image before runner launches can be marked ready. |

## Sign in and enter setup

Use the bootstrap account once, then replace it immediately:

- username: `admin`
- password: `admin`

After the first successful sign-in, OhoCI does **not** drop you into the dashboard. It opens the setup shell, shows one current task, and keeps the later tasks read-only until the current blocking task is saved.

If your operators prefer Korean, switch the UI locale after the app loads. English remains the default locale for the control plane.

## Task 1: Change the admin password

This step is intentionally simple:

1. Enter the current bootstrap password.
2. Choose the new long-lived admin password.
3. Save it before touching any GitHub or OCI settings.

What to watch for:

- the current task should move to **Connect GitHub App**
- the checklist should show password as complete
- the setup shell should stay in place instead of switching frames

## Task 2: Connect the GitHub App

The first GitHub task is route verification only. OhoCI keeps the setup screen focused on connection and activation, then moves repository selection into the next task.

### What to enter

- choose **Create new app** or **Use existing app**
- leave **GitHub API URL** empty unless you are pointing to a GitHub Enterprise API base URL
- point the app webhook at OhoCI's shared webhook endpoint
- click **Verify GitHub App**
- click **Save GitHub access**

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
9. Verify access and save the route. Repository selection happens in the next setup task.

If GitHub returns without a single resolved installation, OhoCI keeps the created app loaded and you can finish by choosing the correct installation from the setup screen.

The manifest helper is intentionally disabled when you set a non-default GitHub API URL. In this version, GitHub Enterprise Server-style API endpoints still require manual GitHub App registration.

### What happens after save

- if there is no live GitHub route yet, OhoCI activates the verified route automatically
- if activation still needs one more pass, stay on the same task and click **Activate GitHub Route**
- repository selection does not appear until this route task is complete

## Task 3: Save the OCI credential

The first OCI task stores the credential only. Runtime placement happens later.

Use these fields:

- **Credential name**
- **Profile name**
- **OCI config file** and **Private key file**, or the matching paste fields
- **Passphrase** when the private key needs it
- **Test connection**
- **Save OCI credential**

What to watch for:

- the next task should move to **Choose repositories**
- the setup shell should still be the same shell
- runtime placement fields should still be hidden at this point

## Task 4: Choose repositories

This task only shows the repository chooser and one save action.

The rule is simple:

- pick at least one repository from the installation-visible list
- click **Save Repository Scope**
- jobs do not open until at least one repository is selected

OhoCI only manages the intersection of:

- the repositories the GitHub App installation can see
- the repositories you select locally here

## Task 5: Save the OCI launch target

This task is the first place that runner launch placement is configured. Keep it minimal:

- enter **Compartment OCID**
- click **Refresh catalog**
- choose an **Availability domain**
- choose a **Subnet OCID**
- choose an **Image OCID**
- decide whether to enable **Assign public IP**
- click **Save launch target**

The image you choose here is the current default runtime image. Later, [Runner Images](./runner-images.md) can replace that default by promoting a verified baked image.

OhoCI marks launch setup ready only when the compartment, subnet, and image are all present and still valid against the latest OCI catalog.

## Before you leave setup

You should see:

- all five tasks marked complete in the checklist
- the setup shell disappear after the launch target is saved
- the authenticated workspace open automatically

If the workspace does not unlock, use the current task title and the checklist as the source of truth for the next missing item.

Advanced route operations, repository review, and OCI tuning stay in **Settings** after setup is complete.

## What to do next

Once setup is complete:

1. open [Revisit Settings after setup](./getting-started.md#revisit-settings-after-setup) if you want to review GitHub or OCI settings again
2. open [Getting started](./getting-started.md) for the workspace tour
3. create one policy from [Policies and capacity](./policies-and-capacity.md)
4. if you plan to use warm capacity, start with one repository allowlist target and a single idle runner expectation
5. if the base OCI image still needs packages or tools, prepare it in [Runner Images](./runner-images.md)
6. send a single test workflow job before scaling further
