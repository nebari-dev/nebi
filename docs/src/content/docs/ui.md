---
title: "Nebi UI"
---

Nebi has a graphical interface you can use in two ways:

- **Desktop app**: a locally-installed application, started from your system Application drawer or from the CLI with `nebi-desktop`. See [installation](/installation/) for how to get it.
- **Nebi server**: the web UI served by a running Nebi server at `http://localhost:8460`. See [Server Setup](/server-setup/).

The screenshots and instructions below apply to either option.

<img src="/img/desktop-landing.png" alt="Nebi UI landing page" />

## Browse Public Registries

The UI includes a **registry browser** for discovering public environments. Open the **Registries** tab to see configured registries. If the one you want is not listed, click **Manage Registries** to add it.

![Registries tab showing the nebari-environments registry](/img/community-pull-registries.png)

Click **Browse** on a registry to see every public repository under that namespace. Each row has a tag dropdown and a **nebi import** button that copies the command for the selected tag to your clipboard.

![Repository list with inline tag dropdown and nebi import copy button](/img/community-pull-tags.png)

Pick a tag, click **nebi import** next to the repository you want, and paste the command into your terminal:

```bash
nebi import quay.io/nebari_environments/data-science-demo:0.1.0
```

## Groups (Admin)

Groups come from your identity provider: every group in a user's `groups` claim appears in Nebi when they sign in. Open **Admin → Groups** in the sidebar to see them and their members. Membership is managed in the identity provider, not in Nebi.

- **Grant access**: share a project with a group via the project's Share dialog (User/Group toggle). Registry grants for groups are admin-only operations.
- **Admins**: members of the groups configured in `oidc_admin_groups` are admins (see [Authentication](/server-setup/#authentication)).
