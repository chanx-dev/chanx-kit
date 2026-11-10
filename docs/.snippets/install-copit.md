```bash
curl -fsSL https://raw.githubusercontent.com/huynguyengl99/copit/main/install.sh | bash
```

[copit](https://github.com/huynguyengl99/copit) is the CLI that copies starters and kits
into your project, and updates them later. The line above installs it as one standalone
binary on macOS and Linux, whatever your server's language.

??? note "Other ways to get copit: npm, uv, pip, Windows"

    === "npm"

        ```bash
        npm install -g @copit/cli
        ```

        Or run it once without installing: `npx @copit/cli` in place of `copit`. The
        plain `copit` package on npm is a different tool.

    === "uv or pip"

        ```bash
        uv tool install copit      # or: pip install copit
        ```

        Or run it once without installing: `uvx copit` in place of `copit`.

    On Windows, use npm or pip, or download the binary from the
    [releases](https://github.com/huynguyengl99/copit/releases).
