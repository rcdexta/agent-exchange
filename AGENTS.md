# Install Agent Exchange (AX)

Follow this guide when the user asks you to install or update Agent Exchange. AX connects named Claude Code, Codex CLI, Grok Build, and OpenCode sessions on the same machine. Its command is `ax`.

## Requirements

- macOS 13 or later, or Linux, on ARM64 or Intel/AMD x86_64.
- On Windows, run inside WSL 2. AX and the coding harnesses must run in the same Linux distribution.
- A POSIX shell, curl, tar, and either sha256sum or shasum. Install missing utilities with the user's package manager as appropriate.

AX uses prebuilt binaries. Go, Make, Node, a C compiler, a GitHub account, and GitHub CLI are not installation prerequisites. Linux binaries include SQLite and are statically linked.

## Install or update

Run as the current user:

```sh
curl -fsSL https://raw.githubusercontent.com/rcdexta/agent-exchange/main/install.sh | sh
```

The installer selects the latest release for the OS and CPU, verifies its SHA-256 checksum and version, and atomically installs `ax` into `$HOME/.local/bin`. Failed downloads and checksum checks preserve the previous binary. If necessary, the installer adds that directory to the user's shell configuration.

Make AX available in the current POSIX shell and verify the installation:

```sh
export PATH="$HOME/.local/bin:$PATH"
ax version
ax doctor
```

For an interactive fish shell, open a new terminal after installation, or use `fish_add_path "$HOME/.local/bin"` instead of `export`.

Report the installed version and the harnesses found by doctor. Missing harnesses do not mean AX installation failed. Install and sign in to the coding harnesses separately, according to the user's request.

## Start communicating

Open two interactive terminals, in any repositories, and launch one session in each:

```sh
ax claude -name api
ax codex -name web
```

Ask Codex: **“Ask api whether the schema is ready.”** Both agents connect automatically. Names work across repositories for the same OS user.

Other supported harnesses join with `ax grok -name worker` or `ax opencode -name editor`. AX consumes the name option and passes other arguments through to the native harness. Use the native resume syntax to adopt an existing conversation:

```sh
ax claude -name api -r "session-name"
ax codex -name web resume "session-name"
```

Launching the same AX name without additional arguments resumes its saved conversation. Existing sessions keep running during an AX update; relaunch them to use the new binary. Saved conversations and AX mailbox state are preserved.

## Optional installation settings

Download the installer first to inspect it or select a release:

```sh
curl -fsSL https://raw.githubusercontent.com/rcdexta/agent-exchange/main/install.sh -o install-ax.sh
AX_VERSION=v0.5.4 sh install-ax.sh
```

`AX_INSTALL_DIR` chooses another binary directory. `AX_NO_MODIFY_PATH=1` disables changes to shell configuration. Set these variables on the `sh install-ax.sh` invocation. With a custom directory, add that directory to PATH before running AX.

Archives, checksums, and the MIT license are also available from [GitHub Releases](https://github.com/rcdexta/agent-exchange/releases/latest).

## Remove AX

When the user requests uninstallation, remove the installed `ax` binary and any installer-added PATH line that is no longer needed. The default binary is `$HOME/.local/bin/ax`. Preserve `$HOME/.ax`, which contains saved identities and mail, unless the user also asks to delete that state.

## Further details

- [Usage, permissions, and resume](https://github.com/rcdexta/agent-exchange/blob/main/claude.rc/README.md)
- [Tested platforms and harness versions](https://github.com/rcdexta/agent-exchange/blob/main/claude.rc/verification.md)
- [Adding a coding harness](https://github.com/rcdexta/agent-exchange/blob/main/claude.rc/adapters.md)
