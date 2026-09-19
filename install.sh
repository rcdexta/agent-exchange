#!/bin/sh
set -eu

main() {
  repo=rcdexta/agent-exchange
  command -v gh >/dev/null 2>&1 || { echo 'Install GitHub CLI and run: gh auth login' >&2; exit 1; }
  case "$(uname -s)" in
    Darwin) platform=darwin ;;
    Linux) platform=linux ;;
    *) echo 'Run this installer on macOS, Linux, or inside WSL.' >&2; exit 1 ;;
  esac
  case "$(uname -m)" in
    arm64|aarch64) arch=arm64 ;;
    x86_64|amd64) arch=amd64 ;;
    *) echo 'AX requires an arm64 or x86_64 machine.' >&2; exit 1 ;;
  esac
  if command -v sha256sum >/dev/null 2>&1; then
    checksum=sha256sum
  elif command -v shasum >/dev/null 2>&1; then
    checksum='shasum -a 256'
  else
    echo 'A SHA-256 utility is required: sha256sum or shasum.' >&2
    exit 1
  fi
  tag=${AX_VERSION:-$(gh api "repos/$repo/releases/latest" -q .tag_name)}
  case "$tag" in
    v[0-9]*) ;;
    *) echo 'AX_VERSION must be a release tag such as v0.5.1.' >&2; exit 1 ;;
  esac
  asset="ax_${platform}_${arch}.tar.gz"
  stage=$(mktemp -d)
  next=
  trap 'rm -rf "$stage"; if [ -n "$next" ]; then rm -f "$next"; fi' EXIT
  trap 'exit 1' HUP INT TERM
  echo "Downloading Agent Exchange $tag for $platform/$arch..."
  gh release download "$tag" -R "$repo" -p "$asset" -p checksums.txt -D "$stage"
  expected=$(awk -v file="$asset" '$2 == file {print $1}' "$stage/checksums.txt")
  actual=$($checksum "$stage/$asset" | awk '{print $1}')
  if [ -z "$expected" ] || [ "$actual" != "$expected" ]; then
    echo 'Checksum verification failed. The installed AX was left unchanged.' >&2
    exit 1
  fi
  tar -xzf "$stage/$asset" -C "$stage" ax LICENSE
  if [ "$("$stage/ax" version)" != "${tag#v}" ]; then
    echo 'The downloaded AX version does not match the release.' >&2
    exit 1
  fi
  bindir=${AX_INSTALL_DIR:-"$HOME/.local/bin"}
  mkdir -p "$bindir"
  if [ -L "$bindir/ax" ] || { [ -e "$bindir/ax" ] && [ ! -f "$bindir/ax" ]; }; then
    echo 'The AX install target must be a regular file. Choose AX_INSTALL_DIR.' >&2
    exit 1
  fi
  next=$(mktemp "$bindir/.ax.XXXXXX")
  cp "$stage/ax" "$next"
  chmod 755 "$next"
  mv -f "$next" "$bindir/ax"
  next=
  echo "Installed ax ${tag#v} at $bindir/ax"
  case ":$PATH:" in
    *":$bindir:"*) ;;
    *)
      if [ "$bindir" = "$HOME/.local/bin" ] && [ "${AX_NO_MODIFY_PATH:-0}" != 1 ]; then
        # Expanded by the user's shell when it starts, not by this installer.
        # shellcheck disable=SC2016
        line='export PATH="$HOME/.local/bin:$PATH"'
        case "${SHELL:-/bin/sh}" in
          */zsh) profile="${ZDOTDIR:-$HOME}/.zshrc" ;;
          */bash)
            if [ "$platform" = darwin ]; then profile="$HOME/.bash_profile"; else profile="$HOME/.bashrc"; fi ;;
          */fish)
            profile="${XDG_CONFIG_HOME:-$HOME/.config}/fish/conf.d/agent-exchange.fish"
            mkdir -p "$(dirname "$profile")"
            # shellcheck disable=SC2016
            line='fish_add_path "$HOME/.local/bin"' ;;
          *) profile="$HOME/.profile" ;;
        esac
        if ! grep -Fqx "$line" "$profile" 2>/dev/null; then printf '\n%s\n' "$line" >> "$profile"; fi
        echo 'Open a new terminal, then run: ax doctor'
      else
        echo "Add $bindir to PATH, then run: ax doctor"
      fi
      ;;
  esac
}

main "$@"
