# Quick Start

Nebi lets you version your software environments the same way git versions your code, and Nebi server lets you share and manage these versioned environments in your team. In this guide, you will create a workspace, push two versions, compare what changed, and roll back to a working state.

## Prerequisites

- [Nebi CLI installed](./installation.md)
- [Pixi](https://pixi.sh) installed
- A running Nebi server (see [Server Setup](./server-setup.md))

## Create a Workspace

Before you can version or share an environment, Nebi needs to track it. `nebi init` creates a workspace in the current directory.

```bash
mkdir my-project && cd my-project
nebi init
```

```bash title="Output"
No pixi.toml found; running pixi init...
✔ Created /home/user/my-project/pixi.toml
Workspace 'my-project' initialized (/home/user/my-project)
```

## Add Packages

Add dependencies to your workspace. This records them in `pixi.toml` so they become part of the environment you version and share.

```bash
pixi add python numpy scikit-learn
```

Your workspace now has two files:

```text
my-project/
├── pixi.toml   # environment spec (dependencies, tasks, platforms)
└── pixi.lock   # exact version pins for reproducibility
```

## Add a Task

Tasks are named commands stored in `pixi.toml`. Anyone who imports the environment gets the same commands.

To add a training task, run:

```bash
pixi task add train "python -c \"
from sklearn.datasets import load_iris
from sklearn.tree import DecisionTreeClassifier
from sklearn.model_selection import train_test_split
from sklearn.metrics import accuracy_score

X, y = load_iris(return_X_y=True)
X_train, X_test, y_train, y_test = train_test_split(X, y, test_size=0.3, random_state=42)

model = DecisionTreeClassifier(random_state=42)
model.fit(X_train, y_train)

y_pred = model.predict(X_test)
print(f'Accuracy: {accuracy_score(y_test, y_pred):.2f}')
\""
```

Run the training task:

```bash
pixi run train
```

```bash title="Output"
Accuracy: 1.00
```

## Push to the Server

Just like `git push` saves your code to a remote, `nebi push` saves your environment spec to a Nebi server.

You need a running server to push to. See [Server Setup](./server-setup.md) if you haven't set one up yet, then log in:

```bash
nebi login http://localhost:8460
```

Then push the workspace with a version tag:

```bash
nebi push my-project:v1.0
```

```bash title="Output"
Pushed my-project (version 1, tags: sha-a1b2c3d4, latest, v1.0)
```

## Make Changes and Push Again

To see how versioning helps, you need at least two versions to compare. Add a package and push again:

```bash
pixi add pandas
nebi push my-project:v2.0
```

## Compare Versions

Before pulling a version, you can preview what would change. `nebi diff` compares any two versions on the server, or a server version against your local workspace.

```bash
nebi diff my-project:v1.0 my-project:v2.0
```

```bash title="Output"
--- my-project:v1.0
+++ my-project:v2.0
@@ pixi.toml @@
 [dependencies]
+pandas = ">=2.2"
```

## Pull a Version

Once a version is on the server, anyone on your team can pull it to their own machine. To try this locally, pull into a fresh directory:

```bash
mkdir teammate && cd teammate
nebi pull my-project:v2.0
```

```bash title="Output"
Pulled my-project:v2.0
```

This writes `pixi.toml` and `pixi.lock` into the current directory. To install the packages, run:

```bash
pixi install
```

## Next Steps

- [Local CLI Workflows](./cli-local.md): activate workspaces by name, run tasks from anywhere
- [Version Rollback](./examples/version-rollback.md): diff, debug, and roll back to a known good version
- [Registry Setup](./registry-setup.md): configure GHCR, Quay.io, or Docker Hub for publishing
