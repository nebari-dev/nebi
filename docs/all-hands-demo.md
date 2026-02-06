# All Hands Demo 
## Server - Amit show GUI
Nebi server is centralized management of pixi envs including sharing across teams with a permissions model for sharing in an organization and governance on environments (allowed versions, packages, etc.).  Auditability as well.  Also supports publishing/pulling to OCI Registries as artifacts.

What is pushed/pulled are pixi manifest and lock files.

## Local

### Local - GUI
- same as server GUI

### Local - CLI (Adam)
Locally, you'll need to push/pull those envs so we have CLI and a desktop app.  We wanted to enable you to continue using pixi envs exactly as before - in project specific directories, but we also want you to be able to track which envs you have locally, and check for changes between what you pulled and what's available in the nebi server.  
- nebi ws list
- nebi ws list -s test-server
- nebi pull
- pixi add/rm fastapi
- a few git-like commands (status, diff) can check how your state differs from the remote state.
    - nebi status
    - nebi diff
- nebi push
- nebi publish


### Another Local feature: Conda-like workflow (Adam)
We also added a centralized repository to enable you to list pixi workspaces.  You can promote pixi envs to global envs.
- nebi promote


