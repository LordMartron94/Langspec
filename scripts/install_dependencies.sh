#!/usr/bin/env bash

set -euo pipefail

# Fill this with the dependency repositories you want to clone.
# Example: REQUIRED_REPOS=("lexarch" "syntaxa" "memforge")
REQUIRED_REPOS=(
  "Autarch"
  "Echo"
  "Essence"
  "Foundation"
  "Lexarch"
  "Memarch"
  "Memcore"
  "Memforge"
  "Memstruct"
  "Structarch"
  "Syntaxa"
)

GITHUB_OWNER="LordMartron94"
GITHUB_BASE_URL="https://github.com/${GITHUB_OWNER}"

function sanitize_project_name() {
  local raw_name="$1"
  local lowered
  local sanitized

  lowered="$(printf '%s' "${raw_name}" | tr '[:upper:]' '[:lower:]')"
  sanitized="$(printf '%s' "${lowered}" | sed -E 's/[^a-z0-9._-]+/-/g; s/^-+//; s/-+$//')"

  if [[ -z "${sanitized}" ]]; then
    echo "invalid-project"
    return
  fi

  echo "${sanitized}"
}

if [[ ${#REQUIRED_REPOS[@]} -eq 0 ]]; then
  echo "No repositories configured."
  echo "Edit REQUIRED_REPOS in this script before running it."
  exit 1
fi

echo "LangSpec dependency installer"
echo "This will clone required repositories from ${GITHUB_BASE_URL}."
echo "You still need to add cloned modules to go.work manually."
echo

read -r -p "Clone dependencies into directory (relative or absolute): " target_input

if [[ -z "${target_input}" ]]; then
  echo "No directory provided."
  exit 1
fi

if [[ "${target_input}" == ~* ]]; then
  target_input="${HOME}${target_input:1}"
fi

if [[ "${target_input}" = /* ]]; then
  target_dir="${target_input}"
else
  target_dir="$(pwd)/${target_input}"
fi

mkdir -p "${target_dir}"
echo "Using directory: ${target_dir}"
echo

for repo_name in "${REQUIRED_REPOS[@]}"; do
  project_dir_name="$(sanitize_project_name "${repo_name}")"
  repo_url="${GITHUB_BASE_URL}/${repo_name}"
  repo_path="${target_dir}/${project_dir_name}"

  if [[ -d "${repo_path}/.git" ]]; then
    echo "Skipping ${repo_name}: already cloned at ${repo_path}"
    continue
  fi

  echo "Cloning ${repo_url} -> ${repo_path}"
  git clone "${repo_url}" "${repo_path}"
done

echo
echo "Done."
echo "Reminder: update your go.work file manually with these modules."
