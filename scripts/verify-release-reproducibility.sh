#!/usr/bin/env bash

set -euo pipefail

script_directory="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repository_directory="$(cd "${script_directory}/.." && pwd)"
tools_directory="${TOOLS_DIR:-${repository_directory}/.git/tools}"
distribution_directory="${repository_directory}/dist"
temporary_directory="$(mktemp -d)"

cleanup() {
  rm -rf "${temporary_directory}"
}
trap cleanup EXIT

snapshot_hashes() {
  local output_file="$1"

  PATH="${tools_directory}:${PATH}" \
    "${tools_directory}/goreleaser" \
      release --snapshot --clean --skip=sign >/dev/null
  "${script_directory}/materialize-release-manifest.sh" >/dev/null
  "${script_directory}/verify-release-snapshot.sh" >/dev/null

  (
    cd "${distribution_directory}"
    find . -maxdepth 1 -type f \
      \( -name '*.zip' -o -name '*_manifest.json' \) \
      -print0 |
      sort -z |
      while IFS= read -r -d '' artifact; do
        if command -v sha256sum >/dev/null 2>&1; then
          sha256sum "${artifact}"
        else
          shasum -a 256 "${artifact}"
        fi
      done
  ) >"${output_file}"
}

snapshot_hashes "${temporary_directory}/first.sha256"
snapshot_hashes "${temporary_directory}/second.sha256"

if ! diff -u \
  "${temporary_directory}/first.sha256" \
  "${temporary_directory}/second.sha256"; then
  printf '%s\n' \
    'Release archives or the Terraform Registry manifest are not reproducible.' >&2
  exit 1
fi

printf '%s\n' \
  'Release archive and Terraform Registry manifest reproducibility passed'
