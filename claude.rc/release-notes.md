# Agent Exchange 0.5.2

Install AX from a prebuilt binary on macOS, Linux, and Windows through WSL 2. Go and Make are no longer installation prerequisites.

- Intel/AMD x86_64 and ARM64 archives for macOS and Linux.
- Automatic platform selection, SHA-256 verification, atomic installation, and shell PATH setup.
- Private repository downloads use your GitHub CLI login. Run the installer again to update.
- Claude Code, Codex CLI, Grok Build, and OpenCode retain their native launch and resume experience.

See `claude.rc/install.md` for installation and `claude.rc/verification.md` for harness compatibility. Windows support in this release means AX and the harnesses run inside the same WSL distribution; a native Windows executable is not included.
