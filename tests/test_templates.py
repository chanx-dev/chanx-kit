"""Templates' copied kits must match the registries'. Refresh Python ones with
``copit add`` in the template, the rest with ``python scripts/templates.py build``."""

import json
import sys
import tomllib
from pathlib import Path
from typing import Any

import pytest

REPO_ROOT = Path(__file__).resolve().parent.parent
sys.path.insert(0, str(REPO_ROOT / "scripts"))

import templates as templates_module  # noqa: E402

TEMPLATES = sorted(p.parent for p in (REPO_ROOT / "templates").glob("*/copit.toml"))
# Server registries by name, with the index each one publishes.
INDEXES: dict[str, dict[str, Any]] = {
    name: json.loads((REPO_ROOT / folder / "copit-registry.json").read_text())
    for name, folder in {
        "chanx-kit": "python",
        "chanx-kit-go": "go",
        "chanx-kit-js": "js",
    }.items()
}


def installed_components(template: Path) -> list[tuple[str, str, dict[str, Any]]]:
    config = tomllib.loads((template / "copit.toml").read_text())
    return [
        (*source["component"].split(":", 1), source)
        for source in config.get("sources", [])
        if source.get("component", "").split(":", 1)[0] in INDEXES
    ]


def test_there_is_a_template_per_server_framework() -> None:
    names = {template.name for template in TEMPLATES}

    assert {"fastapi-react", *(t.name for t in templates_module.ALL)} <= names


@pytest.mark.parametrize("template", TEMPLATES, ids=lambda p: p.name)
def test_every_registry_is_pinned_to_a_release(template: Path) -> None:
    config = tomllib.loads((template / "copit.toml").read_text())

    for name, registry in config["registries"].items():
        assert registry["source"].startswith("github:chanx-dev/chanx-kit@v"), (
            f"{template.name}: {name} should be pinned to a release tag, "
            f"got {registry['source']}"
        )


@pytest.mark.parametrize("template", TEMPLATES, ids=lambda p: p.name)
def test_copied_kits_match_the_current_kits(template: Path) -> None:
    components = installed_components(template)
    assert components, f"{template.name}: no server kit in copit.toml"

    for registry, name, source in components:
        published = INDEXES[registry]["components"][name]
        assert source["component_version"] == published["version"], (
            f"{template.name}: {name} is {source['component_version']}, "
            f"{registry} publishes {published['version']}"
        )

        copied = template / source["path"]
        kit = REPO_ROOT / published["path"]
        for file in published["files"]:
            assert (copied / file).read_text() == (kit / file).read_text(), (
                f"{template.name}: {source['path']}/{file} differs from {kit}/{file}"
            )


@pytest.mark.parametrize("template", templates_module.ALL, ids=lambda t: t.name)
def test_rendered_templates_are_current(template: templates_module.Template) -> None:
    assert templates_module.check(template) == [], (
        "run: python scripts/templates.py build"
    )
