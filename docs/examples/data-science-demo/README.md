# Data Science Demo

A sample Pixi environment used in the [Share and Reuse Environments](../../docs/docs/examples/sharing-environments.md) tutorial.

## What's included

- **pixi.toml** - environment spec with Python, scikit-learn, and Streamlit, plus inline tasks for training and interactive prediction

## Quick start

```bash
nebi init
pixi run train
```

## Tasks

Run any task with `pixi run <task>`:

| Task    | Description                                      |
|---------|--------------------------------------------------|
| `train` | Train a Decision Tree on Iris and print results  |
| `app`   | Launch a Streamlit app for species prediction    |
