"""MkDocs hooks for the docs build."""

from __future__ import annotations

import re
from pathlib import Path
from textwrap import indent
from typing import Any

SNIPPET = re.compile(r'^([ \t]*)--8<-- "([^"]+)"[ \t]*$', re.M)
# Each page's Markdown, by URL, and the nav's page order, for the AI-readable copies.
MARKDOWN: dict[str, str] = {}
ORDER: list[str] = []


def on_nav(nav: Any, config: Any, files: Any) -> Any:
    """Drop the nav source once literate-nav has read it, so it is not a page."""
    summary = files.get_file_from_path("SUMMARY.md")
    if summary is not None:
        files.remove(summary)
    ORDER[:] = [page.url for page in nav.pages]
    return nav


def on_page_markdown(markdown: str, page: Any, config: Any, files: Any) -> str:
    root = Path(config["config_file_path"]).parent
    MARKDOWN[page.url] = SNIPPET.sub(
        lambda m: indent((root / m[2]).read_text(), m[1]), markdown
    )
    return markdown


def on_post_build(config: Any) -> None:
    """Every page as ``<page>.md``, and all of them in nav order as ``llms-full.txt``."""
    site = Path(config["site_dir"])
    for url, markdown in MARKDOWN.items():
        (site / f"{url.rstrip('/') or 'index'}.md").write_text(markdown)
    pages = [MARKDOWN[url] for url in ORDER if url in MARKDOWN]
    (site / "llms-full.txt").write_text("\n\n---\n\n".join(pages))
