#!/usr/bin/env python
"""Install every Go and JS kit, tests included, into fresh projects; build and test them.

COPIT=../copit/target/release/copit uv run python scripts/port_conformance.py
uv run python scripts/port_conformance.py --keep go    # one language, keep the project
"""

from __future__ import annotations

import argparse
import json
import os
import shutil
import subprocess
import sys
import tempfile
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parent.parent

GO_MOD = "module example.com/conformance\n\ngo 1.25.0\n"
TSCONFIG = {
    "compilerOptions": {
        "target": "ES2022",
        "module": "ESNext",
        "moduleResolution": "bundler",
        "types": ["node"],
        "strict": True,
        "skipLibCheck": True,
        "noEmit": True,
    },
    "include": ["src"],
}
VITEST = "export default { test: { include: ['src/**/tests/*.test.ts'] } };\n"


def run(command: list[str], cwd: Path) -> None:
    result = subprocess.run(
        command, cwd=cwd, capture_output=True, text=True, check=False
    )
    if result.returncode:
        raise SystemExit(
            f"{' '.join(command)} failed in {cwd}:\n{result.stdout}{result.stderr}"
        )


def scaffold(language: str, project: Path) -> None:
    match language:
        case "go":
            (project / "go.mod").write_text(GO_MOD)
        case "js":
            package = {"name": "conformance", "private": True, "type": "module"}
            (project / "package.json").write_text(json.dumps(package))
            (project / "tsconfig.json").write_text(json.dumps(TSCONFIG))
            (project / "vitest.config.ts").write_text(VITEST)
            run(
                ["npm", "install", "--silent", "typescript@5", "@types/node@24"],
                project,
            )
        case _:
            raise SystemExit(f"unknown language {language}")


def verify(language: str, project: Path) -> None:
    match language:
        case "go":
            run(["go", "vet", "./..."], project)
            run(["go", "test", "./..."], project)
        case "js":
            run(["npx", "--no-install", "tsc", "--noEmit"], project)
            run(["npx", "--no-install", "vitest", "run"], project)
        case _:
            raise SystemExit(f"unknown language {language}")


def check(language: str, copit: str, keep: bool) -> None:
    index = json.loads((REPO_ROOT / language / "copit-registry.json").read_text())
    registry = index["name"]
    project = Path(tempfile.mkdtemp(prefix=f"chanx-kit-{language}-"))
    try:
        scaffold(language, project)
        run([copit, "init"], project)
        # Found through the root index's catalog, as a user's `registry add` finds it.
        run([copit, "registry", "add", registry, str(REPO_ROOT)], project)
        kits = [f"@{registry}/{name}" for name in index["components"]]
        run([copit, "add", *kits, "--with", "tests", "-y"], project)
        verify(language, project)
        print(f"{language}: {len(kits)} kits install, build and pass their tests")
    finally:
        if keep:
            print(f"  kept {project}")
        else:
            shutil.rmtree(project, ignore_errors=True)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("languages", nargs="*", help="go, js, or both by default")
    parser.add_argument("--keep", action="store_true", help="leave the projects")
    args = parser.parse_args()

    copit = os.environ.get("COPIT") or shutil.which("copit")
    if not copit:
        print("copit is not installed; set COPIT to a build", file=sys.stderr)
        return 1
    for language in args.languages or ["go", "js"]:
        check(language, copit, args.keep)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
