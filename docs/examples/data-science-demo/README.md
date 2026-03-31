# Data Science Demo

A sample Pixi environment used in the [Share and Reuse Environments](../../docs/docs/examples/sharing-environments.md) tutorial.

## What's included

- **pixi.toml** - environment spec with Python and scikit-learn, plus an inline training task

## Quick start

```bash
nebi init
pixi run train
```

## Tasks

Run any task with `pixi run <task>`:

| Task | Description |
|------|-------------|
| `train` | Train a Decision Tree on the Iris dataset and print results |
