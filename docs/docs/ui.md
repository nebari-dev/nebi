# Nebi UI

Nebi has a graphical interface you can use in two ways:

- **Desktop app**: a locally-installed application, started from your system Application drawer. See [installation](./installation.md) for how to get it.
- **Nebi server**: the web UI served by a running Nebi server at `http://localhost:8460`. See [Server Setup](./server-setup.md).

The screenshots and instructions below apply to either option.

<img src="/img/desktop-landing.png" alt="Nebi UI landing page" />

## Browse Public Registries

The UI includes a **registry browser** for discovering public environments. Open the **Registries** tab to see configured registries. If the one you want is not listed, click **Manage Registries** to add it.

![Registries tab showing the nebari-environments registry](/img/community-pull-registries.png)

Click **Browse** on a registry to see every public repository under that namespace.

![Repository list under the nebari-environments registry](/img/community-pull-browse.png)

Pick a repository and click **View Tags**. Each tag has a copy-ready `nebi import` command you can paste into your terminal.

![Tags page with copy-ready nebi import commands](/img/community-pull-tags.png)

Clicking the copy button places a command like this on your clipboard:

```bash
nebi import quay.io/nebari_environments/data-science-demo:0.1.0
```
