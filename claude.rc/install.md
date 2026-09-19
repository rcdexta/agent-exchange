# Install Agent Exchange

AX installs from a prebuilt binary. Go, Make, Node, and a C compiler are not needed. Install and sign in to whichever coding harnesses you want to use separately.

## macOS and Linux

While the repository is private, install [GitHub CLI](https://cli.github.com/) and sign in with an account that can access `rcdexta/agent-exchange`:

```sh
gh auth login
```

Then install AX:

```sh
gh api repos/rcdexta/agent-exchange/contents/install.sh -H 'Accept: application/vnd.github.raw+json' | sh
```

The installer selects the latest release for your OS and CPU, verifies its SHA-256 checksum and version, and installs it in your user's `.local/bin` directory. If needed, it adds that directory to the configuration for your current shell. Open a new terminal afterward.

```sh
ax version
ax doctor
ax claude -name api
```

Supported binary targets are macOS 13 or later and Linux, each on Intel/AMD x86_64 and ARM64. Linux binaries include SQLite and use static linking, so there is no separate SQLite or libc package to install.

## Windows through WSL

For this release, use [WSL 2](https://learn.microsoft.com/windows/wsl/install). Open your Linux distribution's terminal and run the same installation commands above. GitHub CLI, AX, and your coding harnesses must be installed and run inside that distribution.

AX shares names and messages within that WSL distribution. Native Windows harnesses and other WSL distributions are separate environments; there is no native `ax.exe` in this release.

## Update and remove

Run the install command again to update. Failed downloads and checksum checks leave the existing binary unchanged. Existing sessions keep running; close and relaunch them when you want to use new adapter behavior. The installer does not delete conversations or AX mailbox state.

Set `AX_VERSION=v0.5.2` in the installer's environment to select a specific release. Set `AX_INSTALL_DIR` to select another binary directory, or `AX_NO_MODIFY_PATH=1` to manage PATH yourself. For these options, download the script to a file and invoke it with the environment variables set.

To uninstall, remove `.local/bin/ax` under your home directory and the PATH line added by the installer if it is no longer needed. Your `.ax` directory holds saved identities and mail and is preserved unless you explicitly remove it.

## Building from source

Contributors can still use `make install` from the repository root with Go and a C compiler installed. Released binaries are built and tested on their native OS and CPU in GitHub Actions. The release workflow validates the tag against the binary version and publishes archives, checksums, and the MIT license.
