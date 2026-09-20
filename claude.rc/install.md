# Install Agent Exchange

The canonical installation instructions are in [AGENTS.md](../AGENTS.md). That guide covers macOS, Linux, Windows through WSL 2, verification, updates, version selection, and removal.

To let a coding agent install AX, ask it to read:

https://useax.dev/agents.md

## Building from source

Contributors can still use `make install` from the repository root with Go and a C compiler installed. Released binaries are built and tested on their native OS and CPU in GitHub Actions. The release workflow validates the tag against the binary version and publishes archives, checksums, and the MIT license.
