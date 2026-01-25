## Workflow 1: Desktop App Setup (Individual Developer)

No admin, no cloud account, no server setup required.

(Prepopulate with some curated public registries by openteams)

1. User downloads nebi desktop app
2. User logs into nebi
    - `nebi login`
3. User creates free OCI registry (Docker Hub, ghcr.io) if not already created and runs `docker login` or equivalent.
4. User adds registry to nebi
    - `nebi registry add my-dhub docker.io/<username>`
        - nebi automatically looks for credentials (see [crane auth](https://github.com/google/go-containerregistry/blob/main/cmd/crane/doc/crane.md) for inspiration)
5. User sees newly added registry
    - `nebi registry list`
6. User sets registry as default
    - `nebi registry set-default my-dhub`
7. User can now create, push, pull environments
    - Create pixi manifest using pixi
    - `nebi push data-science:v1`
        - checks for local pixi.toml/pixi.lock and pushes workspace
8. User views pushed workspace
    - `nebi workspace list`
    - `nebi workspace tags data-science`
9. User deletes local pixi manifest
    - `pixi clean`
10. User either pulls pixi workspace explicitly (optional step, installs)
    - `nebi pull data-science:v1`
11. User activates shell (pulls and installs if not already local)
    - `nebi shell data-science:v1`

