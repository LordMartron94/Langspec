#!/usr/bin/env bash
#
# Run LangSpec PRAGMA-driven toolchains (Sublime, go_bindings, tm_comments) via the
# langspec-toolchain CLI. This is for users who want generated artifacts only, not a
# runtime LangParser embedded in their application.
#
# PRAGMA blocks in the .lspec control which tools are enabled; the -toolchains flag
# (optional) filters which named steps may run among those enabled in the spec.
#
# Usage (non-interactive, CI-friendly):
#   LANGSPEC_ROOT=/path/to/langspec ./run_toolchains.sh --spec path/to/lang.lspec [--toolchains sublime] [--sublime-json-config path/to/sublime.json]
#   ./run_toolchains.sh --go-config-file libs/lingua/go/gomod_config.go --go-config-function GoModConfig
#
# Usage (interactive): run with no arguments.
#
# LANGSPEC_ROOT: absolute path to the langspec module root (directory containing go.mod).
# If unset, the script assumes it lives at <langspec>/scripts/run_toolchains.sh and
# sets the root to the parent of this file's directory.

set -euo pipefail

TMP_RUNNER_DIR=""

function cleanup_tmp_runner() {
  if [[ -n "${TMP_RUNNER_DIR:-}" ]] && [[ -d "${TMP_RUNNER_DIR}" ]]; then
    rm -rf "${TMP_RUNNER_DIR}"
  fi
}

trap cleanup_tmp_runner EXIT

function usage() {
  cat <<'EOF'
run_toolchains.sh — run LangSpec toolchains (langspec-toolchain wrapper)

  --spec PATH                 Path to the .lspec file (required in non-interactive mode)
  --toolchains NAMES          Optional comma-separated filter, e.g. sublime,tm_comments
  --sublime-json-config PATH  Optional path to Sublime JSON (same shape as PRAGMA configuration-path);
                              uses in-memory bootstrap path (no configuration-path in PRAGMA)
  --go-config-file PATH       Optional Go config file (e.g. libs/lingua/go/gomod_config.go)
  --go-config-function NAME   Function from go-config-file package returning RunnerConfig
  -h, --help                  Show this help

Environment:
  LANGSPEC_ROOT        Path to the langspec module root (optional if this script
                       is still under .../langspec/scripts/)

PRAGMA in the spec enables tools; --toolchains only restricts which runners execute.
Go-config mode writes a tiny temporary main under ${TMPDIR} that calls
langspec/cliutil.RunInMemorySublimeToolchainsFromAny(<package>.<function>()).
EOF
}

function resolve_langspec_root() {
  if [[ -n "${LANGSPEC_ROOT:-}" ]]; then
    echo "$(cd "${LANGSPEC_ROOT}" && pwd)" || return 1
    return 0
  fi

  local here
  here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
  local candidate
  candidate="$(cd "${here}/.." && pwd)"
  if [[ -f "${candidate}/go.mod" ]] && grep -qE '^module[[:space:]]+langspec([[:space:]]|$)' "${candidate}/go.mod" 2>/dev/null; then
    echo "${candidate}"
    return 0
  fi

  return 1
}

function abs_path() {
  local p="$1"
  if [[ "${p}" == /* ]]; then
    printf '%s' "${p}"
  else
    printf '%s' "$(pwd)/${p}"
  fi
}

function find_go_module_dir() {
  local dir="$1"
  while [[ "${dir}" != "/" ]]; do
    if [[ -f "${dir}/go.mod" ]]; then
      echo "${dir}"
      return 0
    fi
    dir="$(dirname "${dir}")"
  done
  return 1
}

function go_module_path_from_dir() {
  local module_dir="$1"
  awk '/^module[[:space:]]+/ { print $2; exit }' "${module_dir}/go.mod"
}

function run_go_config_provider() {
  local config_file_abs="$1"
  local config_function="$2"

  local config_dir
  config_dir="$(dirname "${config_file_abs}")"
  local module_dir
  module_dir="$(find_go_module_dir "${config_dir}")" || {
    echo "error: could not find go.mod for config file: ${config_file_abs}" >&2
    exit 2
  }

  local module_path
  module_path="$(go_module_path_from_dir "${module_dir}")"
  if [[ -z "${module_path}" ]]; then
    echo "error: failed to resolve module path from ${module_dir}/go.mod" >&2
    exit 2
  fi

  local pkg_suffix="${config_dir#${module_dir}/}"
  local config_import_path="${module_path}"
  if [[ "${pkg_suffix}" != "${config_dir}" ]]; then
    config_import_path="${module_path}/${pkg_suffix}"
  fi

  TMP_RUNNER_DIR="$(mktemp -d "${TMPDIR:-/tmp}/langspec-toolchain-runner.XXXXXX")"
  local tmp_file="${TMP_RUNNER_DIR}/runner.go"

  cat > "${tmp_file}" <<EOF
package main

import (
  cfgpkg "${config_import_path}"
  "fmt"
  "langspec/cliutil"
  "os"
)

func main() {
  if err := cliutil.RunInMemorySublimeToolchainsFromAny(cfgpkg.${config_function}()); err != nil {
    fmt.Fprintf(os.Stderr, "generation failed: %v\n", err)
    os.Exit(1)
  }
}
EOF

  echo "Go config mode"
  echo "Module: ${module_path}"
  echo "Config import: ${config_import_path}"
  echo "Running: go run ${tmp_file}"
  echo

  local run_status=0
  (
    cd "${config_dir}"
    go run "${tmp_file}"
  ) || run_status=$?

  cleanup_tmp_runner
  TMP_RUNNER_DIR=""
  return "${run_status}"
}

spec_path=""
toolchains_filter=""
sublime_json_config=""
go_config_file=""
go_config_function=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --spec)
      if [[ $# -lt 2 ]]; then echo "error: --spec requires a value" >&2; exit 2; fi
      spec_path="$2"
      shift 2
      ;;
    --toolchains)
      if [[ $# -lt 2 ]]; then echo "error: --toolchains requires a value" >&2; exit 2; fi
      toolchains_filter="$2"
      shift 2
      ;;
    --sublime-json-config)
      if [[ $# -lt 2 ]]; then echo "error: --sublime-json-config requires a value" >&2; exit 2; fi
      sublime_json_config="$2"
      shift 2
      ;;
    --go-config-file)
      if [[ $# -lt 2 ]]; then echo "error: --go-config-file requires a value" >&2; exit 2; fi
      go_config_file="$2"
      shift 2
      ;;
    --go-config-function)
      if [[ $# -lt 2 ]]; then echo "error: --go-config-function requires a value" >&2; exit 2; fi
      go_config_function="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "error: unknown argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

if [[ -z "${spec_path}" ]] && [[ -z "${go_config_file}" ]]; then
  echo "LangSpec toolchain runner (interactive)"
  echo "Pick mode:"
  echo "  1) .lspec + PRAGMA (langspec-toolchain)"
  echo "  2) Go config provider (e.g. Lingua config + complex override factory)"
  echo
  read -r -p "Mode [1/2]: " mode_choice
  if [[ "${mode_choice}" == "2" ]]; then
    read -r -p "Go config file path (e.g. libs/lingua/go/gomod_config.go): " go_config_file
    read -r -p "Go config function (e.g. GoModConfig): " go_config_function
  else
    read -r -p "Path to .lspec file: " spec_path
    read -r -p "Toolchains filter (comma-separated, or empty for all enabled in PRAGMA): " toolchains_filter
    read -r -p "Sublime JSON config path (-sublime-json-config), or empty to omit: " sublime_json_config
  fi
fi

if [[ -n "${go_config_file}" ]]; then
  if [[ -n "${spec_path}" ]]; then
    echo "error: --go-config-file cannot be combined with --spec" >&2
    exit 2
  fi
  if [[ -z "${go_config_function}" ]]; then
    echo "error: --go-config-function is required when --go-config-file is set" >&2
    exit 2
  fi
  if [[ ! "${go_config_function}" =~ ^[A-Za-z_][A-Za-z0-9_]*$ ]]; then
    echo "error: invalid --go-config-function name: ${go_config_function}" >&2
    exit 2
  fi
  go_config_file_abs="$(abs_path "${go_config_file}")"
  if [[ ! -f "${go_config_file_abs}" ]]; then
    echo "error: go config file not found: ${go_config_file_abs}" >&2
    exit 2
  fi
  run_go_config_provider "${go_config_file_abs}" "${go_config_function}"
  exit 0
fi

spec_abs="$(abs_path "${spec_path}")"
if [[ ! -f "${spec_abs}" ]]; then
  echo "error: spec file not found: ${spec_abs}" >&2
  exit 2
fi

if [[ -n "${sublime_json_config}" ]]; then
  sublime_json_abs="$(abs_path "${sublime_json_config}")"
  if [[ ! -f "${sublime_json_abs}" ]]; then
    echo "error: Sublime JSON config not found: ${sublime_json_abs}" >&2
    exit 2
  fi
  sublime_json_config="${sublime_json_abs}"
fi

if ! langspec_root="$(resolve_langspec_root)"; then
  langspec_root=""
fi
if [[ -z "${langspec_root}" ]]; then
  echo "error: could not determine LangSpec module root." >&2
  echo "Set LANGSPEC_ROOT to the directory containing langspec's go.mod, or run this script from" >&2
  echo "a copy still located at <langspec>/scripts/run_toolchains.sh." >&2
  exit 2
fi

cmd=(go run ./cmd/langspec-toolchain -spec "${spec_abs}")
if [[ -n "${toolchains_filter}" ]]; then
  cmd+=(-toolchains "${toolchains_filter}")
fi
if [[ -n "${sublime_json_config}" ]]; then
  cmd+=(-sublime-json-config "${sublime_json_config}")
fi

echo "LangSpec root: ${langspec_root}"
echo "Running: ${cmd[*]}"
echo

cd "${langspec_root}"
exec "${cmd[@]}"
