#!/usr/bin/env python
"""Render the Go and JS templates: fastapi-react's ``web/`` on each framework.

python scripts/templates.py build    # render, then go mod tidy / npm install and codegen
python scripts/templates.py check    # fail if a rendered file differs from the template
"""

from __future__ import annotations

import argparse
import json
import shutil
import subprocess
import sys
from dataclasses import dataclass
from pathlib import Path
from typing import Any

REPO_ROOT = Path(__file__).resolve().parent.parent
TEMPLATES = REPO_ROOT / "templates"
SHARED = TEMPLATES / "_shared"
WEB = TEMPLATES / "fastapi-react" / "web"
CODEGEN = REPO_ROOT / "ui" / "node_modules" / ".bin" / "chanx-codegen"

# The release these templates' copit.toml pins; bump it when tagging.
RELEASE = "v0.5.0"
COPIED_AT = "2026-11-03T00:00:00Z"
SOURCE = "github:chanx-dev/chanx-kit"

# Written by go mod tidy, npm install and codegen, so check leaves them alone.
TOOL_OUTPUTS = ("go.mod", "go.sum", "package-lock.json", "web/src/generated/")
WEB_SKIP = ("node_modules/", "dist/", "asyncapi.json", "src/generated/")

JS_COMMON = {
    "@chanx-js/layers": "^2.1.1",
    "@chanx-js/server": "^2.2.0",
    "redis": "^6.0.0",
    "zod": "^4.1.0",
}


@dataclass(frozen=True)
class Template:
    language: str  # "go" or "js"
    framework: str
    title: str
    requires: tuple[str, ...] = ()

    @property
    def name(self) -> str:
        prefix = "go-" if self.language == "go" else ""
        return f"{prefix}{self.framework}-react"

    @property
    def directory(self) -> Path:
        return TEMPLATES / self.name


ALL = (
    Template("go", "nethttp", "net/http"),
    Template("go", "chi", "chi", ("github.com/go-chi/chi/v5",)),
    Template("go", "gin", "gin", ("go.chanx.dev/adapters/gin/v2",)),
    Template("go", "echo", "Echo", ("go.chanx.dev/adapters/echo/v2",)),
    Template("go", "fiber", "Fiber", ("go.chanx.dev/adapters/fiber/v2",)),
    Template(
        "js",
        "express",
        "Express",
        ("express@^5.3.0", "ws@^8.22.0", "@types/express@^5.0.6"),
    ),
    Template(
        "js",
        "fastify",
        "Fastify",
        ("fastify@^5.12.0", "@fastify/websocket@^11.3.0", "@fastify/static@^10.1.0"),
    ),
    Template(
        "js",
        "hono",
        "Hono",
        ("hono@^4.13.0", "@hono/node-server@^2.1.0", "ws@^8.22.0", "@types/ws@^8.18.0"),
    ),
)

# Where a language's kits land, and how its registry is named.
KITS = {
    "go": ("chanx-kit-go", "internal/wskits"),
    "js": ("chanx-kit-js", "src/ws-kits"),
}


def index(folder: str) -> dict[str, Any]:
    return json.loads((REPO_ROOT / folder / "copit-registry.json").read_text())


def web_files(gen: str) -> dict[str, bytes]:
    files: dict[str, bytes] = {}
    for path in sorted(WEB.rglob("*")):
        relative = path.relative_to(WEB).as_posix()
        if not path.is_file() or relative.startswith(WEB_SKIP):
            continue
        files[f"web/{relative}"] = path.read_bytes()
    package = json.loads(files["web/package.json"])
    package["scripts"]["gen"] = gen
    files["web/package.json"] = (json.dumps(package, indent=2) + "\n").encode()
    return files


def kit_files(language: str, name: str) -> dict[str, bytes]:
    _, target = KITS[language]
    component = index(language)["components"][name]
    destination = f"{target}/{Path(component['path']).name}"
    files = {
        f"{destination}/{file}": (REPO_ROOT / component["path"] / file).read_bytes()
        for file in component["files"]
    }
    files[f"{destination}/LICENSE"] = (REPO_ROOT / "LICENSE").read_bytes()
    return files


def copit_toml(language: str) -> str:
    registry, target = KITS[language]
    kit = index(language)["components"]["notification"]
    ui = index("ui")["components"]["notification"]
    pinned = f"{SOURCE}@{RELEASE}"
    return f"""target = "vendor"

[registries.{registry}]
source = "{pinned}"
index = "{language}/copit-registry.json"
target = "{target}"

[registries.chanx-kit-ui]
source = "{pinned}"
index = "ui/copit-registry.json"
target = "web/src/chanx-kit"
variants = ["react"]

[[sources]]
path = "{target}/{Path(kit["path"]).name}"
source = "{pinned}/{kit["path"]}"
ref = "{RELEASE}"
copied_at = "{COPIED_AT}"
component = "{registry}:notification"
component_version = "{kit["version"]}"

[[sources]]
path = "web/src/chanx-kit/notification"
source = "{pinned}/{ui["path"]}"
ref = "{RELEASE}"
copied_at = "{COPIED_AT}"
component = "chanx-kit-ui:notification"
component_version = "{ui["version"]}"
variants = ["react"]
"""


def js_package(template: Template) -> str:
    dependencies = dict(JS_COMMON)
    dev = {"@types/node": "^24.0.0", "tsx": "^4.20.0", "typescript": "^5.9.3"}
    for spec in template.requires:
        name, version = spec.rsplit("@", 1)
        (dev if name.startswith("@types/") else dependencies)[name] = version
    package = {
        "name": "chanx-app",
        "private": True,
        "type": "module",
        "scripts": {
            "dev": "tsx watch src/server.ts",
            "start": "tsx src/server.ts",
            "notify": "tsx src/notify.ts",
            "typecheck": "tsc --noEmit",
        },
        "dependencies": dict(sorted(dependencies.items())),
        "devDependencies": dict(sorted(dev.items())),
    }
    return json.dumps(package, indent=2) + "\n"


def render(template: Template) -> dict[str, bytes]:
    shared = SHARED / template.language
    readme = (
        (shared / "README.md").read_text().replace("{framework_title}", template.title)
    )
    files: dict[str, bytes] = {
        "README.md": readme.encode(),
        ".gitignore": (shared / "gitignore").read_bytes(),
        "docker-compose.yml": (
            TEMPLATES / "fastapi-react" / "docker-compose.yml"
        ).read_bytes(),
        "copit.toml": copit_toml(template.language).encode(),
        **kit_files(template.language, "notification"),
    }
    match template.language:
        case "go":
            gen = "cd .. && go run ./cmd/asyncapi > web/asyncapi.json && cd web && chanx-codegen asyncapi.json -o src/generated"
            files |= {
                "main.go": (shared / f"main.{template.framework}.go").read_bytes(),
                "internal/app/app.go": (shared / "app.go").read_bytes(),
                "cmd/notify/main.go": (shared / "notify.go").read_bytes(),
                "cmd/asyncapi/main.go": (shared / "asyncapi.go").read_bytes(),
            }
        case "js":
            gen = "cd .. && npx tsx src/asyncapi.ts > web/asyncapi.json && cd web && chanx-codegen asyncapi.json -o src/generated"
            files |= {
                "package.json": js_package(template).encode(),
                "tsconfig.json": (shared / "tsconfig.json").read_bytes(),
                "src/server.ts": (
                    shared / f"server.{template.framework}.ts"
                ).read_bytes(),
                "src/app.ts": (shared / "app.ts").read_bytes(),
                "src/notify.ts": (shared / "notify.ts").read_bytes(),
                "src/asyncapi.ts": (shared / "asyncapi.ts").read_bytes(),
            }
        case _:
            raise ValueError(template.language)
    return files | web_files(gen)


def run(command: list[str], cwd: Path) -> str:
    return subprocess.run(
        command, cwd=cwd, check=True, capture_output=True, text=True
    ).stdout


def tooling(template: Template) -> None:
    """Dependencies, then the typed client generated from the server's own schema."""
    directory = template.directory
    match template.language:
        case "go":
            if not (directory / "go.mod").exists():
                (directory / "go.mod").write_text("module app\n\ngo 1.25.0\n")
            modules = ("go.chanx.dev/v2@v2.1.0", "go.chanx.dev/layers/redis/v2@v2.0.0")
            run(["go", "get", *modules, *template.requires], directory)
            run(["go", "mod", "tidy"], directory)
            spec = run(["go", "run", "./cmd/asyncapi"], directory)
        case _:
            run(["npm", "install", "--silent"], directory)
            spec = run(["npx", "--no-install", "tsx", "src/asyncapi.ts"], directory)
    schema = directory / "web" / "asyncapi.json"
    schema.write_text(spec)
    generated = directory / "web" / "src" / "generated"
    shutil.rmtree(generated, ignore_errors=True)
    run([str(CODEGEN), str(schema), "-o", str(generated)], directory)
    schema.unlink()


def build(template: Template) -> None:
    rendered = render(template)
    directory = template.directory
    keep = {*rendered, *(p for p in TOOL_OUTPUTS if not p.endswith("/"))}
    if directory.exists():
        for path in sorted(directory.rglob("*")):
            relative = path.relative_to(directory).as_posix()
            stale = (
                path.is_file()
                and relative not in keep
                and not relative.startswith(
                    ("node_modules/", "web/node_modules/", "web/src/generated/")
                )
            )
            if stale:
                path.unlink()
    for relative, content in rendered.items():
        target = directory / relative
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_bytes(content)
    tooling(template)
    print(f"Wrote templates/{template.name} ({len(rendered)} files)")


def check(template: Template) -> list[str]:
    return [
        f"templates/{template.name}/{relative}"
        for relative, content in render(template).items()
        if not (template.directory / relative).is_file()
        or (template.directory / relative).read_bytes() != content
    ]


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("command", choices=["build", "check"])
    parser.add_argument("names", nargs="*", help="only these templates")
    args = parser.parse_args()
    chosen = [t for t in ALL if not args.names or t.name in args.names]

    if args.command == "build":
        for template in chosen:
            build(template)
        return 0

    stale = [path for template in chosen for path in check(template)]
    if stale:
        print(
            "Templates out of date: "
            + ", ".join(stale)
            + ". Run: python scripts/templates.py build",
            file=sys.stderr,
        )
        return 1
    print(f"Templates OK, {len(chosen)} rendered from templates/_shared.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
