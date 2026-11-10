"""Generate the kit pages at docs build time. Every page derives from the code (the
registry index, kit READMEs, message classes), so docs cannot drift from the wire
format. Run by ``mkdocs-gen-files``, not standalone.
"""

from __future__ import annotations

import html
import importlib
import inspect
import json
import re
import sys
from pathlib import Path
from typing import Any

import mkdocs_gen_files
import yaml
from chanx.messages.base import BaseMessage
from markdown.extensions.toc import slugify

REPO_ROOT = Path(__file__).resolve().parent.parent
sys.path.insert(0, str(REPO_ROOT / "python"))
REGISTRY = json.loads((REPO_ROOT / "copit-registry.json").read_text())
COMPONENTS: dict[str, Any] = REGISTRY["components"]
UI_COMPONENTS: dict[str, Any] = json.loads(
    (REPO_ROOT / "ui" / "copit-registry.json").read_text()
)["components"]
# Go and JS ports of the Python kits: folder, label, registry, install target, packages.
PORT_LANGUAGES = {
    "go": ("Go", "chanx-kit-go", "internal/wskits", "Go modules"),
    "js": ("Node", "chanx-kit-js", "src/ws-kits", "npm packages"),
}
PORTS: dict[str, dict[str, Any]] = {
    folder: json.loads((REPO_ROOT / folder / "copit-registry.json").read_text())[
        "components"
    ]
    for folder in PORT_LANGUAGES
}
REPO_URL = "https://github.com/chanx-dev/chanx-kit"
SHADCN_URL = "https://kit.chanx.dev/r"


def manifest(component: dict[str, Any]) -> dict[str, Any]:
    return yaml.safe_load((REPO_ROOT / component["path"] / "kit.yaml").read_text())


# Contracts live in kit.yaml, not the published index.
SERVERS_OF: dict[str, list[str]] = {}
UIS_OF: dict[str, list[str]] = {}
for _name, _component in COMPONENTS.items():
    _meta = manifest(_component)
    for _contract in [_meta.get("defines"), *(_meta.get("implements") or [])]:
        if _contract:
            SERVERS_OF.setdefault(_contract, []).append(_name)
for _name, _component in UI_COMPONENTS.items():
    for _contract in manifest(_component).get("consumes") or []:
        UIS_OF.setdefault(_contract, []).append(_name)


AREAS = {
    "messaging": "Messaging",
    "presence": "Presence",
    "agents": "Agents",
    "voice": "Voice",
    "tooling": "Tooling",
}
ROLES = {
    "feature": "Feature",
    "store": "Store",
    "foundation": "Foundation",
    "tooling": "Tooling",
    "provider": "Provider",
}
# Matrix columns on the Providers page; other contracts follow by name.
CAPABILITIES = {
    "transcriber@1": "Speech to text",
    "synthesizer@1": "Text to speech",
    "ag-ui@2": "Agent",
}


def required_by(components: dict[str, Any]) -> dict[str, list[str]]:
    found: dict[str, list[str]] = {}
    for name, component in components.items():
        for required in component.get("requires") or []:
            found.setdefault(required, []).append(name)
    return found


# Per registry: both have an ag-ui and a presence.
SERVER_REQUIRED_BY = required_by(COMPONENTS)
UI_REQUIRED_BY = required_by(UI_COMPONENTS)
DEFINED_IN = {
    meta["defines"]: name
    for name, meta in ((n, manifest(c)) for n, c in COMPONENTS.items())
    if meta.get("defines")
}


def areas_of(component: dict[str, Any]) -> list[str]:
    """A kit's area; a provider's are those of the contracts it implements."""
    meta = manifest(component)
    if area := meta.get("area"):
        return [area]
    found = [
        manifest(COMPONENTS[DEFINED_IN[c]]).get("area")
        for c in meta.get("implements") or []
        if c in DEFINED_IN
    ]
    return sorted({a for a in found if a}, key=list(AREAS).index)


def page_tags(component: dict[str, Any], *, ui: bool) -> str:
    """Front matter for Material's tags plugin: area, role, side and contracts."""
    meta = manifest(component)
    tags = [
        *(AREAS[a] for a in areas_of(component)),
        ROLES[meta["role"]],
        "UI kit" if ui else "Server kit",
        *contracts_of(component),
    ]
    return "---\ntags:\n" + "".join(f"  - {t}\n" for t in tags) + "---\n\n"


def nav_groups(components: dict[str, Any], prefix: str) -> list[str]:
    """Kits grouped by area, features first; providers are listed on their own."""
    lines: list[str] = []
    for area, title in AREAS.items():
        members = [
            name for name, c in components.items() if manifest(c).get("area") == area
        ]
        if not members:
            continue
        members.sort(
            key=lambda n: (
                list(ROLES).index(manifest(components[n])["role"]),
                components[n]["title"],
            )
        )
        lines.append(f"    * {title}")
        lines += [
            f"        * [{components[n]['title']}]({prefix}{n}.md)" for n in members
        ]
    return lines


def foundation_note(
    name: str, component: dict[str, Any], users_of: dict[str, list[str]]
) -> list[str]:
    """Who installs a foundation kit, and when to open it yourself."""
    meta = manifest(component)
    if meta["role"] != "foundation":
        return []
    users = link_list(sorted(users_of.get(name, [])), "")
    use = (
        f"Open it to write your own provider, or as the reference for `{meta['defines']}`."
        if meta.get("defines")
        else "Open it to build a kit on top of it."
    )
    return ['!!! note "Installed for you"', f"    With {users}. {use}", ""]


def contracts_of(component: dict[str, Any]) -> list[str]:
    meta = manifest(component)
    found = [meta.get("defines"), *(meta.get("implements") or [])]
    return [contract for contract in found if contract] + list(
        meta.get("consumes") or []
    )


def link_list(names: list[str], prefix: str) -> str:
    return ", ".join(f"[`{name}`]({prefix}{name}.md)" for name in names)


nav_lines: list[str] = []


def resolve(schema: dict[str, Any], root: dict[str, Any]) -> dict[str, Any]:
    """Follow a local ``$ref`` one level."""
    ref = schema.get("$ref")
    if not ref:
        return schema
    name = ref.rsplit("/", 1)[-1]
    return root.get("$defs", {}).get(name, {})


def type_label(schema: dict[str, Any]) -> str:
    if "$ref" in schema:
        return schema["$ref"].rsplit("/", 1)[-1]
    if "const" in schema:
        return f"`{schema['const']}`"
    if "enum" in schema:
        return " | ".join(f"`{value}`" for value in schema["enum"])
    for key in ("anyOf", "oneOf"):
        if key in schema:
            return " | ".join(type_label(option) for option in schema[key])
    if schema.get("type") == "array":
        return f"{type_label(schema.get('items', {}))}[]"
    schema_type = schema.get("type")
    if isinstance(schema_type, list):
        return " | ".join(str(item) for item in schema_type)
    return str(schema_type or "any")


def payload_table(message_schema: dict[str, Any]) -> str:
    payload = message_schema.get("properties", {}).get("payload")
    if not payload:
        return ""

    resolved = resolve(payload, message_schema)
    properties = resolved.get("properties")
    if not properties:
        return f"Payload: `{type_label(payload)}`\n"

    required = set(resolved.get("required", []))
    rows = ["| Field | Type | Required |", "|---|---|---|"]
    for name, prop in properties.items():
        # An unescaped pipe in a union type would split the table cell.
        label = type_label(prop).replace("|", "\\|")
        rows.append(f"| `{name}` | {label} | {'yes' if name in required else 'no'} |")
    return "\n".join(rows) + "\n"


def rewrite_links(markdown: str) -> str:
    """Point a kit README's repo-relative links at their docs-site equivalents."""
    replacements = {
        "../docs/authoring-a-kit.md": "../../authoring-a-kit.md",
        "../CONTRIBUTING.md": f"{REPO_URL}/blob/main/CONTRIBUTING.md",
        "(python/": f"({REPO_URL}/tree/main/python/",
    }
    for old, new in replacements.items():
        markdown = markdown.replace(old, new)
    return markdown


def kit_messages(package: str) -> dict[str, tuple[str, dict[str, Any]]]:
    """Message classes a kit defines, keyed by action."""
    try:
        module = importlib.import_module(f"kits.{package}")
    except ModuleNotFoundError as error:
        # A bridge kit whose library the docs build does not install.
        if (error.name or "").startswith("kits"):
            raise
        return {}
    found: dict[str, tuple[str, dict[str, Any]]] = {}

    for _, obj in inspect.getmembers(module, inspect.isclass):
        if not issubclass(obj, BaseMessage) or obj is BaseMessage:
            continue
        if not obj.__module__.startswith(module.__name__):
            continue
        action = obj.model_fields["action"].default
        found[action] = (obj.__name__, obj.model_json_schema())

    return dict(sorted(found.items()))


# Server tabs, in this order on every page; linked, so one choice applies site-wide.
SERVERS = (("js", "Node"), ("python", "Python"), ("go", "Go"))
FENCE = re.compile(r"^\s*(```|~~~)")
HEADING = re.compile(r"^(#{2,6})\s+(.*?)\s*$")


def servers_of(name: str) -> list[str]:
    """Labels of the servers a kit exists for, in tab order."""
    have = {"python", *(f for f, c in PORTS.items() if name in c)}
    return [label for folder, label in SERVERS if folder in have]


def tab_body(readme: str, id_prefix: str) -> str:
    """A README as tab content: no title or intro, headings kept out of the TOC."""
    lines = re.sub(r"\A#\s+.*\n+", "", readme, count=1).splitlines()
    first_fence = next((i for i, line in enumerate(lines) if FENCE.match(line)), 0)
    lines = lines[first_fence:]
    out: list[str] = []
    in_code = False
    for line in lines:
        if FENCE.match(line):
            in_code = not in_code
        heading = None if in_code else HEADING.match(line)
        if heading:
            level = len(heading[1]) + 1
            title = html.escape(heading[2])
            title = re.sub(r"`([^`]+)`", r"<code>\1</code>", title)
            anchor = id_prefix + slugify(heading[2].replace("`", ""), "-")
            out.append(f'<h{level} class="kit-tab-h" id="{anchor}">{title}</h{level}>')
        else:
            out.append(line)
    return "\n".join(f"    {line}" if line else "" for line in out)


def missing_tab(label: str, component: dict[str, Any]) -> str:
    meta = manifest(component)
    if meta.get("only_variants"):
        why = f"It is built on {', '.join(meta['only_variants'])}, so it stays Python."
    elif meta["role"] == "provider":
        why = f"{component['title']} is a Python provider for now; providers come to a server when asked for."
    elif meta["role"] in {"foundation", "tooling"} or not contracts_of(component):
        why = "Python only for now: it supports Python kits that have no port yet."
    else:
        why = "Not ported yet."
    return (
        f'    !!! note "Not available for {label}"\n'
        f"        {why} See which kits each server has in the "
        "[catalog](index.md#catalog)."
    )


def availability(name: str, component: dict[str, Any]) -> str:
    """Badges for the servers a kit runs on, then its UI kits and contracts."""
    have = set(servers_of(name))
    badges = " ".join(
        f'<span class="kit-badge" data-on="{str(label in have).lower()}">{label}</span>'
        for _, label in SERVERS
    )
    contracts = contracts_of(component)
    uis = sorted({u for c in contracts for u in UIS_OF.get(c, [])})
    facts = []
    if uis:
        facts.append("UI kit: " + link_list(uis, "../ui/"))
    if contracts:
        facts.append("Contract: " + ", ".join(f"`{c}`" for c in contracts))
    return badges + (" · " + " · ".join(facts) if facts else "")


def kit_page(name: str, component: dict[str, Any]) -> str:
    folders = {
        "python": component,
        **{f: c[name] for f, c in PORTS.items() if name in c},
    }
    header = [
        f"# {component['title']}",
        "",
        component["description"],
        "",
        availability(name, component),
        "",
        *foundation_note(name, component, SERVER_REQUIRED_BY),
        "<small>First time? Install copit and set up the registries once:"
        " [add kits to your app](../add-to-your-app.md#1-set-up-the-registries).</small>",
        "",
    ]
    tabs: list[str] = []
    for folder, label in SERVERS:
        tabs.append(f'=== "{label}"\n')
        if folder in folders:
            readme = (REPO_ROOT / folders[folder]["path"] / "README.md").read_text()
            prefix = "" if folder == "python" else f"{folder}-"
            tabs.append(rewrite_links(tab_body(readme, prefix)) + "\n")
        else:
            tabs.append(missing_tab(label, component) + "\n")

    sections = [page_tags(component, ui=False) + "\n".join(header), *tabs]

    messages = kit_messages(Path(component["path"]).name)
    if messages:
        sections.append("\n## Messages\n")
        sections.append(
            "_The same on every server: generated from the kit's schema._\n"
        )
        for action, (class_name, schema) in messages.items():
            sections.append(f"### `{action}`\n")
            sections.append(f"`{class_name}`\n")
            table = payload_table(schema)
            if table:
                sections.append(table)

    rows = ["| Server | Install | Version | Source |", "|---|---|---|---|"]
    for folder, label in SERVERS:
        if folder in folders:
            c = folders[folder]
            registry = "chanx-kit" if folder == "python" else PORT_LANGUAGES[folder][1]
            rows.append(
                f"| {label} | `copit add @{registry}/{name}` | {c['version']} "
                f"| [{c['path']}]({REPO_URL}/tree/main/{c['path']}) |"
            )
    sections.append('\n??? info "Install commands, versions and source"\n')
    sections.append("\n".join(f"    {row}" for row in rows) + "\n")
    return "\n".join(sections)


# --- kit pages ------------------------------------------------------------------
REGISTRY_OF = {"Node": "chanx-kit-js", "Python": "chanx-kit", "Go": "chanx-kit-go"}


def recipe_command(add: str, server: str) -> str | None:
    """A recipe's command for one server, or None when a server kit in it has no port."""
    kits = re.findall(r"@chanx-kit/([\w-]+)", add)
    if not all(server in servers_of(kit) for kit in kits):
        return None
    return add.replace("@chanx-kit/", f"@{REGISTRY_OF[server]}/")


def recipe_rows() -> list[str]:
    """The feature table, from docs/recipes.yaml, as one tab per server."""
    recipes = yaml.safe_load((REPO_ROOT / "docs" / "recipes.yaml").read_text())
    out: list[str] = ['<div class="recipes" markdown>', ""]
    for _, server in SERVERS:
        out += [f'=== "{server}"', "", "    | Feature | Install |", "    |---|---|"]
        for recipe in recipes:
            command = recipe_command(recipe["add"], server)
            install = f"`copit add {command}`" if command else "Python only for now"
            if command and (providers := recipe.get("providers")):
                install += "<br>Other providers: " + ", ".join(
                    f"`{p}`" for p in providers
                )
            out.append(f"    | **{recipe['title']}**<br>{recipe['what']} | {install} |")
        out.append("")
    return [*out, "</div>", ""]


def catalog_row(name: str, component: dict[str, Any]) -> str:
    areas = ", ".join(AREAS[a] for a in areas_of(component))
    role = ROLES[manifest(component)["role"]]
    uis = sorted({u for c in contracts_of(component) for u in UIS_OF.get(c, [])})
    ui = link_list(uis, "../ui/") if uis else ""
    first = component["description"].split(". ")[0].rstrip(".") + "."
    return (
        f"| [`{name}`]({name}.md) | {', '.join(servers_of(name))} | {ui} "
        f"| {areas} | {role} | {first} |"
    )


index_rows = [
    "---\nhide:\n  - toc\n---\n",
    "# Kits",
    "",
    "Each kit is copied into your project and owned by you. Install with"
    " [copit](https://github.com/huynguyengl99/copit), server and"
    " [UI](../ui-kits.md) halves in one command.",
    "",
    "## By feature",
    "",
    *recipe_rows(),
    "",
    "## Catalog",
    "",
    "Every server kit, and the servers it runs on. Kits that carry a contract reach every"
    " server; stores follow their kit; integrations live where their ecosystem is."
    " [UI kits](../ui/index.md) are listed on their own.",
    "",
    '<div class="kit-filter" data-servers="Node,Python,Go"'
    f' data-area="{",".join(AREAS.values())}" data-kind="{",".join(ROLES.values())}">'
    "</div>",
    "",
    "| Kit | Servers | UI kit | Area | Kind | What it does |",
    "|---|---|---|---|---|---|",
    *(catalog_row(n, COMPONENTS[n]) for n in sorted(COMPONENTS)),
]

with mkdocs_gen_files.open("kits/index.md", "w") as handle:
    handle.write("\n".join(index_rows) + "\n")

for name in sorted(COMPONENTS):
    with mkdocs_gen_files.open(f"kits/{name}.md", "w") as handle:
        handle.write(kit_page(name, COMPONENTS[name]))
    mkdocs_gen_files.set_edit_path(
        f"kits/{name}.md", f"{COMPONENTS[name]['path']}/README.md"
    )
nav_lines.append("* [Kits](kits/index.md)")
nav_lines += nav_groups(COMPONENTS, "kits/")


# --- providers ------------------------------------------------------------------
PROVIDERS = sorted(
    (n for n, c in COMPONENTS.items() if manifest(c)["role"] == "provider"),
    key=lambda n: COMPONENTS[n]["title"].lower(),
)


def capabilities(name: str) -> dict[str, str]:
    """Contract to how it is installed: the part's name, or a tick for the whole kit."""
    meta = manifest(COMPONENTS[name])
    if parts := meta.get("parts"):
        return {
            c: f"`{p}`"
            for p, spec in parts.items()
            for c in spec.get("implements") or []
        }
    return dict.fromkeys(meta.get("implements") or [], "✓")


columns = sorted(
    {c for n in PROVIDERS for c in capabilities(n)},
    key=lambda c: (list(CAPABILITIES).index(c) if c in CAPABILITIES else 99, c),
)
provider_rows = [
    "# Providers",
    "",
    "A provider plugs a vendor or framework into a contract, so the UI kits and every"
    " recipe work unchanged whichever you pick. A vendor with several capabilities is"
    " one kit with a part each: install all, or one with `--only <part>`.",
    "",
    "| Provider | "
    + " | ".join(f"{CAPABILITIES.get(c, c)} (`{c}`)" for c in columns)
    + " | Tier |",
    "|---|" + "---|" * len(columns) + "---|",
]
for name in PROVIDERS:
    has = capabilities(name)
    provider_rows.append(
        f"| [{COMPONENTS[name]['title']}](../kits/{name}.md) | "
        + " | ".join(has.get(c, "") for c in columns)
        + f" | {COMPONENTS[name]['tier']} |"
    )
provider_rows += [
    "",
    "## Add a provider",
    "",
    "Subclass the contract's topic and run its shared test suite against it; see"
    " [writing a provider](../kits/audio-stream-in.md#writing-a-provider). Providers"
    " kept in other copit registries are welcome in this table once they pass the"
    " suite: open an issue with the registry's link.",
]
with mkdocs_gen_files.open("providers/index.md", "w") as handle:
    handle.write("\n".join(provider_rows) + "\n")
nav_lines.append("    * [Providers](providers/index.md)")
nav_lines += [f"        * [{COMPONENTS[n]['title']}](kits/{n}.md)" for n in PROVIDERS]


# --- UI kit pages ---------------------------------------------------------------
def ui_page(name: str, component: dict[str, Any]) -> str:
    readme = (REPO_ROOT / component["path"] / "README.md").read_text()
    body = re.sub(r"\A#\s+.*\n+", "", readme, count=1)

    facts = []
    for contract in contracts_of(component):
        facts.append(f"Contract: `{contract}`")
        if SERVERS_OF.get(contract):
            facts.append(f"Works with: {link_list(SERVERS_OF[contract], '../kits/')}")
    if component.get("requires"):
        facts.append(f"Brings: {link_list(component['requires'], '')}")
    if variants := ", ".join(component.get("variants", {})):
        facts.append(f"{variants.title()}, on a framework-free core")

    # The README opens with the description and the copit command.
    header = [
        f"# {component['title']}",
        "",
        " · ".join(facts),
        "",
        *foundation_note(name, component, UI_REQUIRED_BY),
        "<small>First time? Set up the UI registry once:"
        " [using UI kits](../ui-kits.md#install).</small>",
        "",
    ]
    packages = ", ".join(f"`{d}`" for d in component["dependencies"])
    details = [
        '??? info "With shadcn, version, packages and source"',
        "",
        "    ```bash",
        f"    npx shadcn add {SHADCN_URL}/{name}.json",
        "    ```",
        "",
        f"    Version {component['version']}. npm packages: {packages}. Source:"
        f" [{component['path']}]({REPO_URL}/tree/main/{component['path']}).",
        "",
    ]
    return "\n".join(
        [
            page_tags(component, ui=True) + "\n".join(header),
            rewrite_links(body),
            "",
            *details,
        ]
    )


ui_index = [
    "# UI kits",
    "",
    "Copy-in UI that binds to a **contract**, the messages on a topic, rather than to"
    " one server kit, so it works with every server speaking that contract. Each has a"
    " framework-free core and React components; styling is one CSS file driven by"
    " `--chanx-*` variables. Set the registry up once:",
    "",
    "```bash",
    "copit registry add chanx-kit-ui github:chanx-dev/chanx-kit@v0.5.0 --to web/src/chanx-kit",
    "```",
    "",
    "[Using UI kits](../ui-kits.md) covers composing and styling them; the"
    " [catalog](../kits/index.md#catalog) pairs each with its server kit.",
    "",
    "| UI kit | Contract | Description |",
    "|---|---|---|",
]
for name in sorted(UI_COMPONENTS):
    component = UI_COMPONENTS[name]
    contract = ", ".join(f"`{c}`" for c in contracts_of(component))
    ui_index.append(
        f"| [`{name}`]({name}.md) | {contract} | {component['description']} |"
    )

with mkdocs_gen_files.open("ui/index.md", "w") as handle:
    handle.write("\n".join(ui_index) + "\n")

for name in sorted(UI_COMPONENTS):
    with mkdocs_gen_files.open(f"ui/{name}.md", "w") as handle:
        handle.write(ui_page(name, UI_COMPONENTS[name]))
    mkdocs_gen_files.set_edit_path(
        f"ui/{name}.md", f"{UI_COMPONENTS[name]['path']}/README.md"
    )
nav_lines.append("* [UI kits](ui/index.md)")
nav_lines += nav_groups(UI_COMPONENTS, "ui/")

with mkdocs_gen_files.open("tags.md", "w") as handle:
    handle.write(
        "# Tags\n\nEvery kit page by area, role, side and contract.\n\n<!-- material/tags -->\n"
    )

# --- contributing page ----------------------------------------------------------
# Rendered from the repository's own CONTRIBUTING.md so the two cannot disagree. Its
# links are written for someone reading the file on GitHub, so they are repointed the
# way kit READMEs are.
contributing = (REPO_ROOT / "CONTRIBUTING.md").read_text()
for old, new in {
    "(docs/": "(",
    "(python/kits/README.md)": f"({REPO_URL}/blob/main/python/kits/README.md)",
}.items():
    contributing = contributing.replace(old, new)
# A kit's README is its docs page there.
contributing = re.sub(
    r"\(python/kits/(\w+)/README\.md",
    lambda m: f"(kits/{m[1].replace('_', '-')}.md",
    contributing,
)

with mkdocs_gen_files.open("contributing.md", "w") as handle:
    handle.write(contributing)
mkdocs_gen_files.set_edit_path("contributing.md", "CONTRIBUTING.md")

# --- navigation -----------------------------------------------------------------
with mkdocs_gen_files.open("SUMMARY.md", "w") as handle:
    handle.write(
        "\n".join(
            [
                "* [Home](index.md)",
                "* Get started",
                "    * [Build a live inbox](getting-started.md)",
                "    * [Start a project](start-a-project.md)",
                "    * [Set up an existing app](set-up-your-app.md)",
                "    * [Add kits to your app](add-to-your-app.md)",
                "    * [How it works](how-it-works.md)",
                "* Guides",
                "    * [Chat room with who's online](chat-room.md)",
                "    * [Stream an AI agent](agent-streaming.md)",
                "    * [Add voice](voice.md)",
                "    * [Send across processes](multiple-processes.md)",
                "    * [Use and style the UI kits](ui-kits.md)",
                *nav_lines,
                "* Contributing",
                "    * [Checklist and workflow](contributing.md)",
                "    * [Authoring a kit](authoring-a-kit.md)",
                "    * [Questions](questions.md)",
                "    * [Our registry](registry-conventions.md)",
            ]
        )
        + "\n"
    )


# --- llms.txt -------------------------------------------------------------------
# An index for AI tools (llmstxt.org), from the pages' own titles and descriptions. It
# links each page's Markdown copy, which docs/hooks.py writes next to the HTML.
SITE = "https://kit.chanx.dev/"


def page_entry(path: str) -> str:
    source = (REPO_ROOT / "docs" / path).read_text()
    title = re.search(r"^# (.+)$", source, re.M)
    about = re.search(r'^description: "(.+)"$', source, re.M)
    url = SITE + path
    return f"- [{title[1] if title else path}]({url}): {about[1] if about else ''}"


llms = [
    "# chanx-kit",
    "",
    "> Realtime features you copy into your app: notifications, chat, presence, AI agent"
    " streaming and voice, for Node, Python and Go servers, with React UI that works with"
    " any of them. Kits are copied in with copit and owned by the project.",
    "",
    "## Get started",
    *(
        page_entry(p)
        for p in (
            "getting-started.md",
            "start-a-project.md",
            "set-up-your-app.md",
            "add-to-your-app.md",
            "how-it-works.md",
        )
    ),
    "",
    "## Guides",
    *(
        page_entry(p)
        for p in (
            "chat-room.md",
            "agent-streaming.md",
            "voice.md",
            "multiple-processes.md",
            "ui-kits.md",
        )
    ),
    "",
    "## Server kits",
    *(
        f"- [{c['title']}]({SITE}kits/{n}.md): {c['description']} Servers: {', '.join(servers_of(n))}."
        for n, c in sorted(COMPONENTS.items())
    ),
    "",
    "## UI kits",
    *(
        f"- [{c['title']}]({SITE}ui/{n}.md): {c['description']}"
        for n, c in sorted(UI_COMPONENTS.items())
    ),
    "",
    "## Optional",
    f"- [Everything in one file]({SITE}llms-full.txt): every page above, in order.",
    "",
]
with mkdocs_gen_files.open("llms.txt", "w") as handle:
    handle.write("\n".join(llms))
