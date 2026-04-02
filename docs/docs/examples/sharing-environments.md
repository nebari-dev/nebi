---
sidebar_position: 1
---

# Share and Reuse Environments

Your coworker is starting a new project but needs the same environment you've been using. They're on a different machine, maybe even a different OS. How do you share your exact setup with them?

This example walks through the full publish-and-consume workflow: **Alice** creates and publishes an environment, and **Bob** downloads and runs it with no manual setup needed.

Here's a visual overview of the workflow:

```mermaid
graph LR
    subgraph Alice
        direction TD
        A["Create env"] --> C["Publish"]
    end
    C --> R[("OCI Registry")]
    R --> D
    subgraph Bob
        direction TD
        D["Import"] --> E["Run"]
    end
```

## What You'll Need

- [Nebi CLI installed](../installation.md)
- [Pixi](https://pixi.sh) installed
- A configured OCI registry (see [Registry Setup](../registry-setup.md))

## Alice: Create and Publish the Environment

:::info Follow along
Clone the example to follow along with this tutorial:

```bash
git clone https://github.com/nebari-dev/nebi.git
cd nebi/docs/examples/data-science-demo
nebi init
```

:::

### Step 1: Create the workspace

Alice creates a data science environment with Python, scikit-learn, and Streamlit. Here's her `pixi.toml`:

```toml
[workspace]
name = "data-science-demo"
channels = ["conda-forge"]
platforms = ["linux-64", "linux-aarch64", "osx-arm64", "osx-64"]
version = "0.1.0"

[dependencies]
python = ">=3.11"
scikit-learn = ">=1.4"
streamlit = ">=1.30"
```

### Step 2: Add tasks

Alice adds a training task directly in `pixi.toml`. Since the task is defined inline, it travels with the environment when published:

```toml
[tasks]
train = """python -c "
from sklearn.datasets import load_iris
from sklearn.tree import DecisionTreeClassifier
from sklearn.model_selection import train_test_split
from sklearn.metrics import accuracy_score, confusion_matrix

X, y = load_iris(return_X_y=True)
X_train, X_test, y_train, y_test = train_test_split(X, y, test_size=0.3, random_state=42)

model = DecisionTreeClassifier(random_state=42)
model.fit(X_train, y_train)

y_pred = model.predict(X_test)
print(f'Accuracy: {accuracy_score(y_test, y_pred):.2f}')
cm = confusion_matrix(y_test, y_pred)
print('Confusion Matrix:')
print(cm)
" """
```

She also adds a Streamlit app for interactive predictions:

```toml
app = """python -c "
import tempfile, os, subprocess, sys
code = '''
import streamlit as st
from sklearn.datasets import load_iris
from sklearn.tree import DecisionTreeClassifier

iris = load_iris()
model = DecisionTreeClassifier(random_state=42)
model.fit(iris.data, iris.target)

st.title('Iris Species Predictor')
features = [[
    st.slider('Sepal length', 4.0, 8.0, 5.8),
    st.slider('Sepal width', 2.0, 4.5, 3.0),
    st.slider('Petal length', 1.0, 7.0, 4.0),
    st.slider('Petal width', 0.1, 2.5, 1.2),
]]
st.subheader(f'Predicted: {iris.target_names[model.predict(features)[0]]}')
'''
f = tempfile.NamedTemporaryFile(suffix='.py', delete=False, mode='w')
f.write(code)
f.close()
subprocess.run([sys.executable, '-m', 'streamlit', 'run', f.name])
os.unlink(f.name)
" """
```

Alice installs the environment to generate the lock file:

```bash
pixi install
```

This creates `pixi.lock`, which pins the exact package versions for reproducibility.

She can then verify the tasks work locally by running the training task:

```bash
pixi run train
```

```bash title="Output"
Accuracy: 1.00
Confusion Matrix:
[[19  0  0]
 [ 0 13  0]
 [ 0  0 13]]
```

Or launch the Streamlit app:

```bash
pixi run app
```

![Streamlit prediction app](/img/example-streamlit-app.png)

### Step 3: Publish to an OCI registry

Before publishing, configure a default registry (see [Registry Setup](../registry-setup.md) for details):

```bash
nebi registry add \
  --default \
  --name <name> \
  --url <url> \
  --namespace <namespace> \
  --username <username> \
  --local
```

For example, here's how it looks with Docker Hub:

```bash
nebi registry add \
  --name dockerhub \
  --url docker.io \
  --namespace alice \
  --username alice \
  --default \
  --local
Password:
Added local registry 'dockerhub' (docker.io)
```

Then publish. The `--tag` sets the version and `--repo` names the repository:

```bash
nebi publish data-science-demo --tag v1.0 --repo data-science-demo --local
```

Example output with Docker Hub:

```bash title="Output"
Published docker.io/alice/data-science-demo:v1.0 (digest: sha256:...)
```

## Bob: Download and Run the Environment

Bob doesn't need to know what packages Alice chose or how the environment was built. He just needs one command.

### Import from the OCI registry

```bash
nebi import <url>/<namespace>/data-science-demo:v1.0
```

For example, with Alice's Docker Hub registry, the command would be:

```bash
nebi import docker.io/alice/data-science-demo:v1.0
```

This creates `pixi.toml` and `pixi.lock` in the current directory.

### Run the task

Now that Bob has the environment, he can run the training task:

```bash
pixi run train
```

```bash title="Output"
Accuracy: 1.00
Confusion Matrix:
[[19  0  0]
 [ 0 13  0]
 [ 0  0 13]]
```

Or launch the Streamlit app:

```bash
pixi run app
```

## What Just Happened

Here's the full flow at a glance:

| Step | Who | Command |
|------|-----|---------|
| Create workspace | Alice | `nebi init` + `pixi add` |
| Add tasks | Alice | Edit `pixi.toml` |
| Publish to OCI | Alice | `nebi publish` |
| Import environment | Bob | `nebi import` |
| Run task | Bob | `pixi run train` |

With Nebi, Bob gets the same packages, the same versions, and the same results as Alice without any manual setup.