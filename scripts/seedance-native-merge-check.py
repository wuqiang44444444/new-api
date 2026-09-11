#!/usr/bin/env python3
"""Compare conflict contents for the six Seedance native wiring files.

Reads Git blobs and working files only; merge-file writes into a temporary
directory. This is a file-level comparison, not a rename-aware tree merge,
conflict resolution, or proof that the upstream merge passes regression tests.
"""

import argparse
from collections import Counter
import hashlib
import json
from pathlib import Path
import re
import subprocess
import tempfile


ROOT = Path(__file__).resolve().parent.parent
PATHS = (
    "constant/channel.go",
    "controller/channel.go",
    "controller/channel_authz.go",
    "model/channel.go",
    "model/task_plugin.go",
    "router/channel-router.go",
)


def git(*args):
    return subprocess.check_output(["git", *args], cwd=ROOT)


def blob(revision, path):
    if not git("ls-tree", revision, "--", path):
        return None
    return git("show", f"{revision}:{path}")


def merge_observation(local, base, upstream):
    with tempfile.TemporaryDirectory(prefix="seedance-native-merge-") as tmp:
        files = [Path(tmp) / name for name in ("local", "base", "upstream")]
        for path, content in zip(files, (local, base or b"", upstream)):
            path.write_bytes(content)
        result = subprocess.run(
            ["git", "merge-file", "-p", "--diff3", "-L", "LOCAL", "-L", "BASE",
             "-L", "UPSTREAM", *(str(path) for path in files)],
            capture_output=True,
        )
        if result.returncode < 0 or result.returncode > 127:
            raise RuntimeError(result.stderr.decode())
        blocks = re.findall(rb"^<<<<<<< LOCAL\n.*?^>>>>>>> UPSTREAM\n", result.stdout, re.M | re.S)
        if result.returncode != min(len(blocks), 127):
            raise RuntimeError("merge-file status and conflict markers disagree")
        return {
            "conflict_blocks": len(blocks),
            "conflict_hashes": [hashlib.sha256(block).hexdigest() for block in blocks],
            "merged_sha256": hashlib.sha256(result.stdout).hexdigest(),
        }


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--local", required=True, help="local commit before the Seedance migration")
    parser.add_argument("--upstream", required=True, help="upstream target commit")
    args = parser.parse_args()
    local = git("rev-parse", "--verify", args.local + "^{commit}").decode().strip()
    upstream = git("rev-parse", "--verify", args.upstream + "^{commit}").decode().strip()
    bases = git("merge-base", "--all", local, upstream).decode().splitlines()
    if not bases:
        raise RuntimeError("no common ancestor")
    # Compare every best base separately. In a criss-cross history these are
    # sensitivity checks, not Git's recursively synthesized virtual merge base.
    report = {"git_version": git("--version").decode().strip(), "local": local,
              "upstream": upstream, "comparisons": []}
    for revision in bases:
        comparison = {"base": revision, "files": []}
        for path in PATHS:
            before, base, target = blob(local, path), blob(revision, path), blob(upstream, path)
            if before is None or target is None:
                raise RuntimeError(f"{path}: addition/deletion/rename requires tree-level review")
            current = (ROOT / path).read_bytes()
            baseline = merge_observation(before, base, target)
            workspace = merge_observation(current, base, target)
            old_blocks, new_blocks = Counter(baseline["conflict_hashes"]), Counter(workspace["conflict_hashes"])
            comparison["files"].append({
                "path": path,
                "base_absent": base is None,
                "workspace_sha256": hashlib.sha256(current).hexdigest(),
                "baseline": baseline,
                "workspace": workspace,
                "removed_conflict_contents": sum((old_blocks - new_blocks).values()),
                "added_conflict_contents": sum((new_blocks - old_blocks).values()),
            })
        report["comparisons"].append(comparison)
    print(json.dumps(report, indent=2))


if __name__ == "__main__":
    main()
