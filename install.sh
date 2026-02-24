#!/usr/bin/env bash
set -euo pipefail

REPO="${ENVSYNC_INSTALL_REPO:-Aditya190803/envsync}"
VERSION="${ENVSYNC_INSTALL_VERSION:-latest}"
INSTALL_DIR="${ENVSYNC_INSTALL_DIR:-$HOME/.local/bin}"
BASE_URL="${ENVSYNC_INSTALL_BASE_URL:-}"
CHECKSUMS_URL="${ENVSYNC_INSTALL_CHECKSUMS_URL:-}"
SKIP_VERIFY="${ENVSYNC_INSTALL_SKIP_VERIFY:-false}"
WITH_SERVER=false
ASSUME_YES=false
ENV_FILES=()
ENVSYNC_PROFILE="$HOME/.config/envsync/env"

usage() {
  cat <<'EOF'
Install Env-Sync from GitHub Releases.

Usage:
  install.sh [--repo <owner/repo>] [--version <tag|latest>] [--install-dir <dir>] [--base-url <url>] [--checksums-url <url>] [--skip-verify] [--with-server] [--yes]

Env vars:
  ENVSYNC_INSTALL_REPO     GitHub repo (default: Aditya190803/envsync)
  ENVSYNC_INSTALL_VERSION  Release tag or "latest" (default: latest)
  ENVSYNC_INSTALL_DIR      Destination dir (default: ~/.local/bin)
  ENVSYNC_INSTALL_BASE_URL Optional release base URL override (for CI/artifact tests)
  ENVSYNC_INSTALL_CHECKSUMS_URL Optional checksums URL override
  ENVSYNC_INSTALL_SKIP_VERIFY Set to "true" to skip checksum verification
EOF
}

need_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "error: missing required command: $1" >&2
    exit 1
  fi
}

parse_args() {
  while [ "$#" -gt 0 ]; do
    case "$1" in
      --repo)
        REPO="${2:-}"
        shift 2
        ;;
      --version)
        VERSION="${2:-}"
        shift 2
        ;;
      --install-dir)
        INSTALL_DIR="${2:-}"
        shift 2
        ;;
      --base-url)
        BASE_URL="${2:-}"
        shift 2
        ;;
      --checksums-url)
        CHECKSUMS_URL="${2:-}"
        shift 2
        ;;
      --skip-verify)
        SKIP_VERIFY=true
        shift
        ;;
      --with-server)
        WITH_SERVER=true
        shift
        ;;
      --yes)
        ASSUME_YES=true
        shift
        ;;
      -h|--help)
        usage
        exit 0
        ;;
      *)
        echo "error: unknown argument: $1" >&2
        usage >&2
        exit 1
        ;;
    esac
  done
}

detect_platform() {
  local uos uarch
  uos="$(uname -s | tr '[:upper:]' '[:lower:]')"
  uarch="$(uname -m)"
  case "$uos" in
    linux) OS="linux" ;;
    darwin) OS="darwin" ;;
    *)
      echo "error: unsupported OS: $uos (expected linux or darwin)" >&2
      exit 1
      ;;
  esac
  case "$uarch" in
    x86_64|amd64) ARCH="amd64" ;;
    arm64|aarch64) ARCH="arm64" ;;
    *)
      echo "error: unsupported arch: $uarch (expected amd64 or arm64)" >&2
      exit 1
      ;;
  esac
}

resolve_version() {
  if [ -n "$BASE_URL" ] && [ -n "${VERSION}" ] && [ "${VERSION}" != "latest" ]; then
    TAG="${VERSION}"
    return
  fi

  if [ -n "${VERSION}" ] && [ "${VERSION}" != "latest" ]; then
    TAG="${VERSION}"
    return
  fi

  local api tag
  api="https://api.github.com/repos/${REPO}/releases/latest"
  tag="$(curl -fsSL "$api" | sed -n 's/.*"tag_name":[[:space:]]*"\([^"]*\)".*/\1/p' | head -n1)"
  if [ -z "$tag" ]; then
    echo "error: unable to resolve latest release tag for ${REPO}" >&2
    exit 1
  fi
  TAG="$tag"
}

asset_exists() {
  local url
  url="$1"
  if curl -fsI "$url" >/dev/null 2>&1; then
    return 0
  fi
  return 1
}

choose_asset() {
  local name base ver_no_v
  name="$1"
  ver_no_v="${TAG#v}"

  CANDIDATES=(
    "${name}_${ver_no_v}_${OS}_${ARCH}.tar.gz"
    "${name}_${TAG}_${OS}_${ARCH}.tar.gz"
    "${name}-${ver_no_v}-${OS}-${ARCH}.tar.gz"
    "${name}-${TAG}-${OS}-${ARCH}.tar.gz"
    "${name}_${OS}_${ARCH}.tar.gz"
    "${name}-${OS}-${ARCH}.tar.gz"
    "${name}_${ver_no_v}_${OS}_${ARCH}"
    "${name}_${TAG}_${OS}_${ARCH}"
    "${name}-${ver_no_v}-${OS}-${ARCH}"
    "${name}-${TAG}-${OS}-${ARCH}"
    "${name}_${OS}_${ARCH}"
    "${name}-${OS}-${ARCH}"
  )

  for asset in "${CANDIDATES[@]}"; do
    if [ -n "$BASE_URL" ]; then
      base="${BASE_URL%/}/${TAG}/${asset}"
    else
      base="https://github.com/${REPO}/releases/download/${TAG}/${asset}"
    fi
    if asset_exists "$base"; then
      ASSET_URL="$base"
      ASSET_NAME="$asset"
      return 0
    fi
  done

  echo "error: no matching release asset found for ${name} (${OS}/${ARCH}) in ${REPO}@${TAG}" >&2
  echo "tried names:" >&2
  for asset in "${CANDIDATES[@]}"; do
    echo "  - $asset" >&2
  done
  exit 1
}

resolve_checksums_url() {
  if [ -n "$CHECKSUMS_URL" ]; then
    return
  fi
  if [ -n "$BASE_URL" ]; then
    CHECKSUMS_URL="${BASE_URL%/}/${TAG}/checksums.txt"
  else
    CHECKSUMS_URL="https://github.com/${REPO}/releases/download/${TAG}/checksums.txt"
  fi
}

verify_checksum() {
  local file sha_file expected actual line
  file="$1"
  if [ "$SKIP_VERIFY" = true ]; then
    echo "warning: checksum verification skipped"
    return 0
  fi
  resolve_checksums_url

  sha_file="$(mktemp)"
  if ! curl -fsSL "$CHECKSUMS_URL" -o "$sha_file"; then
    echo "error: failed to download checksums: $CHECKSUMS_URL" >&2
    echo "hint: publish checksums.txt in release assets or run with --skip-verify" >&2
    rm -f "$sha_file"
    exit 1
  fi

  line=""
  while IFS= read -r raw; do
    [ -z "$raw" ] && continue
    set -- $raw
    [ "$#" -lt 2 ] && continue
    candidate="$2"
    candidate="${candidate#\\*}"
    candidate="${candidate#./}"
    if [ "$candidate" = "$ASSET_NAME" ]; then
      line="$raw"
      break
    fi
  done <"$sha_file"
  if [ -z "$line" ]; then
    echo "error: checksum entry for ${ASSET_NAME} not found in checksums file" >&2
    rm -f "$sha_file"
    exit 1
  fi
  expected="$(echo "$line" | awk '{print $1}')"

  if command -v sha256sum >/dev/null 2>&1; then
    actual="$(sha256sum "$file" | awk '{print $1}')"
  elif command -v shasum >/dev/null 2>&1; then
    actual="$(shasum -a 256 "$file" | awk '{print $1}')"
  else
    echo "error: missing sha256sum/shasum for checksum verification" >&2
    rm -f "$sha_file"
    exit 1
  fi

  rm -f "$sha_file"
  if [ "$expected" != "$actual" ]; then
    echo "error: checksum mismatch for ${ASSET_NAME}" >&2
    exit 1
  fi
}

download_and_install() {
  local bin_name tmp_file extract_dir installed_path
  bin_name="$1"

  choose_asset "$bin_name"
  tmp_file="$(mktemp)"
  extract_dir="$(mktemp -d)"

  curl -fsSL "$ASSET_URL" -o "$tmp_file"
  verify_checksum "$tmp_file"

  if [[ "$ASSET_NAME" == *.tar.gz ]]; then
    tar -xzf "$tmp_file" -C "$extract_dir"
    if [ -f "$extract_dir/$bin_name" ]; then
      installed_path="$extract_dir/$bin_name"
    else
      installed_path="$(find "$extract_dir" -type f -name "$bin_name" | head -n1 || true)"
    fi
  else
    installed_path="$tmp_file"
  fi

  if [ -z "${installed_path:-}" ] || [ ! -f "$installed_path" ]; then
    echo "error: downloaded asset did not contain binary: $bin_name" >&2
    exit 1
  fi

  chmod +x "$installed_path"
  mkdir -p "$INSTALL_DIR"
  install -m 0755 "$installed_path" "$INSTALL_DIR/$bin_name"

  rm -rf "$tmp_file" "$extract_dir"
  echo "installed $bin_name -> $INSTALL_DIR/$bin_name"
}

confirm_install() {
  if [ "$ASSUME_YES" = true ]; then
    return
  fi
  echo "About to install from:"
  echo "  repo:    $REPO"
  echo "  version: $TAG"
  echo "  target:  $INSTALL_DIR"
  if [ -n "$BASE_URL" ]; then
    echo "  base-url: $BASE_URL"
  fi
  if [ "$SKIP_VERIFY" = true ]; then
    echo "  integrity: checksum verification disabled"
  else
    echo "  integrity: checksum verification enabled"
  fi
  if [ "$WITH_SERVER" = true ]; then
    echo "  binaries: envsync, envsync-server"
  else
    echo "  binaries: envsync"
  fi
  printf "Continue? [y/N] "
  read -r answer
  case "$answer" in
    y|Y|yes|YES) ;;
    *) echo "aborted"; exit 1 ;;
  esac
}

detect_env_files() {
  while IFS= read -r f; do
    ENV_FILES+=("${f#./}")
  done < <(find . -maxdepth 1 -type f -name ".env*" ! -iname "*example*" | sort)
}

shell_rc_file() {
  local sh
  sh="$(basename "${SHELL:-}")"
  case "$sh" in
    zsh) echo "$HOME/.zshrc" ;;
    bash) echo "$HOME/.bashrc" ;;
    *) echo "$HOME/.profile" ;;
  esac
}

normalize_convex_url() {
  local raw
  raw="$(echo "$1" | sed 's/[[:space:]]*$//')"
  raw="${raw%/}"
  echo "$raw"
}

convex_query_endpoint() {
  local base
  base="$(normalize_convex_url "$1")"
  if [[ "$base" == */api ]]; then
    echo "$base/query"
  else
    echo "$base/api/query"
  fi
}

validate_convex_url() {
  local url endpoint payload
  url="$1"
  endpoint="$(convex_query_endpoint "$url")"
  payload='{"path":"backup:getStore","args":{},"format":"json"}'
  curl -fsS --max-time 8 -H "Content-Type: application/json" -d "$payload" "$endpoint" >/dev/null
}

persist_envsync_var() {
  local name value tmp
  name="$1"
  value="$2"
  mkdir -p "$(dirname "$ENVSYNC_PROFILE")"
  touch "$ENVSYNC_PROFILE"
  tmp="$(mktemp)"
  grep -v "^export ${name}=" "$ENVSYNC_PROFILE" >"$tmp" || true
  printf "export %s=%q\n" "$name" "$value" >>"$tmp"
  mv "$tmp" "$ENVSYNC_PROFILE"
  chmod 600 "$ENVSYNC_PROFILE"
}

ensure_profile_sourced() {
  local rc source_line
  rc="$(shell_rc_file)"
  source_line="[ -f \"$ENVSYNC_PROFILE\" ] && . \"$ENVSYNC_PROFILE\""
  touch "$rc"
  if ! grep -Fq "$source_line" "$rc"; then
    {
      echo
      echo "# envsync installer config"
      echo "$source_line"
    } >>"$rc"
  fi
}

prompt_convex_onboarding() {
  local answer existing input url
  if [ "$ASSUME_YES" = true ]; then
    return
  fi

  existing="$(normalize_convex_url "${ENVSYNC_CONVEX_URL:-}")"
  echo
  printf "Configure Convex now so Env-Sync can push encrypted secrets? [Y/n] "
  read -r answer
  case "$answer" in
    n|N|no|NO)
      echo "Skipping Convex onboarding."
      return
      ;;
  esac

  while true; do
    if [ -n "$existing" ]; then
      printf "Convex URL [%s]: " "$existing"
    else
      printf "Convex URL (https://<deployment>.convex.cloud): "
    fi
    read -r input
    if [ -n "$input" ]; then
      url="$(normalize_convex_url "$input")"
    else
      url="$existing"
    fi

    if [ -z "$url" ]; then
      echo "Please enter a Convex URL."
      continue
    fi
    if [[ ! "$url" =~ ^https?:// ]]; then
      echo "URL must start with http:// or https://"
      continue
    fi

    if validate_convex_url "$url"; then
      persist_envsync_var "ENVSYNC_CONVEX_URL" "$url"
      ensure_profile_sourced
      echo "Saved ENVSYNC_CONVEX_URL to $ENVSYNC_PROFILE"
      echo "New terminals will load it automatically."
      echo "Current shell: export ENVSYNC_CONVEX_URL=$url"
      break
    fi

    echo "Could not reach Convex query endpoint at $(convex_query_endpoint "$url")."
    printf "Try again? [Y/n] "
    read -r answer
    case "$answer" in
      n|N|no|NO)
        echo "Skipping Convex onboarding."
        break
        ;;
    esac
  done
}

prompt_cloud_push() {
  detect_env_files
  if [ "${#ENV_FILES[@]}" -eq 0 ]; then
    return
  fi

  echo
  echo "Detected local env files (excluding examples):"
  for f in "${ENV_FILES[@]}"; do
    echo "  - $f"
  done

  if [ "$ASSUME_YES" = true ]; then
    echo "Non-interactive mode enabled; skipping cloud push prompt."
    return
  fi

  printf "Do you want to push these secrets to cloud with Env-Sync now? [y/N] "
  read -r answer
  case "$answer" in
    y|Y|yes|YES)
      echo
      echo "Next steps:"
      echo "  1) Configure remote:"
      echo "     export ENVSYNC_CONVEX_URL=https://<your-deployment>.convex.cloud"
      echo "     # or: export ENVSYNC_REMOTE_URL=http://127.0.0.1:8080"
      echo "  2) Initialize and pick project:"
      echo "     envsync init"
      echo "     envsync project create <name>"
      echo "     envsync project use <name>"
      echo "  3) Add secrets and push:"
      echo "     envsync set KEY value"
      echo "     envsync push"
      ;;
    *)
      echo "Skipping cloud push setup."
      ;;
  esac
}

main() {
  parse_args "$@"
  need_cmd curl
  need_cmd tar
  need_cmd install

  detect_platform
  resolve_version
  confirm_install

  download_and_install "envsync"
  if [ "$WITH_SERVER" = true ]; then
    download_and_install "envsync-server"
  fi

  echo
  echo "Done."
  echo "Add to PATH if needed:"
  echo "  export PATH=\"$INSTALL_DIR:\$PATH\""
  echo
  echo "Check:"
  echo "  envsync help"

  prompt_convex_onboarding
  prompt_cloud_push
}

main "$@"
