#!/usr/bin/env python3
import json
import sys


def latest_run(runs, workflow_name, expected_commit, expected_ref_name):
    matches = []
    for run in runs:
        if workflow_name and run.get("name") != workflow_name:
            continue
        if run.get("event") != "push" or run.get("head_sha") != expected_commit:
            continue
        head_branch = run.get("head_branch") or ""
        if expected_ref_name and head_branch != expected_ref_name:
            continue
        matches.append(run)

    if not matches:
        return None

    matches.sort(
        key=lambda run: (
            run.get("created_at", ""),
            int(run.get("run_attempt") or 0),
            int(run.get("id") or 0),
        )
    )
    return matches[-1]


def main():
    if len(sys.argv) != 5:
        raise SystemExit(
            "usage: select-release-workflow-run.py WORKFLOW COMMIT REF_NAME RUNS_JSON"
        )

    workflow_name, expected_commit, expected_ref_name, runs_path = sys.argv[1:]
    with open(runs_path, encoding="utf-8") as f:
        runs = json.load(f).get("workflow_runs", [])

    run = latest_run(runs, workflow_name, expected_commit, expected_ref_name)
    if run is None:
        print("not_found\t\t\t")
        return

    print(
        "\t".join(
            [
                run.get("status") or "",
                run.get("conclusion") or "",
                str(run.get("id") or ""),
                run.get("html_url") or "",
            ]
        )
    )


if __name__ == "__main__":
    main()
