#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
FORK_DIR="${ROOT_DIR}/any/discordgo"
FORK_REPO="https://github.com/Sigumaa/discordgo.git"

need_cmd() {
  local name="$1"
  if command -v "$name" >/dev/null 2>&1; then
    return 0
  fi
  printf 'missing required command: %s\n' "$name" >&2
  return 1
}

print_prerequisites() {
  cat >&2 <<'EOF'
missing build prerequisites for DAVE setup.

macOS:
  brew install cmake pkg-config

Ubuntu / Debian:
  sudo apt-get install cmake pkg-config build-essential git
EOF
}

write_go_work() {
  cat >"${ROOT_DIR}/go.work" <<EOF
go 1.24.0

use .

replace github.com/bwmarrin/discordgo => ./any/discordgo
EOF
}

main() {
  local missing=0

  need_cmd git || missing=1
  need_cmd cmake || missing=1
  need_cmd pkg-config || missing=1

  if [[ "$missing" -ne 0 ]]; then
    print_prerequisites
    exit 1
  fi

  mkdir -p "${ROOT_DIR}/any"

  if [[ ! -d "${FORK_DIR}/.git" ]]; then
    git clone --depth=1 "${FORK_REPO}" "${FORK_DIR}"
  else
    printf 'using existing fork: %s\n' "${FORK_DIR}"
  fi

  (
    cd "${FORK_DIR}"
    ./scripts/setup-dave.sh
  )

  write_go_work

  printf 'local DAVE fork is ready.\n'
  printf 'fork path: %s\n' "${FORK_DIR}"
  printf 'go.work written at: %s/go.work\n' "${ROOT_DIR}"
}

main "$@"
