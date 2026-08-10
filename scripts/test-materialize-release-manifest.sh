#!/usr/bin/env bash

set -euo pipefail

script_directory="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repository_directory="$(cd "${script_directory}/.." && pwd)"
temporary_directory="$(mktemp -d)"
distribution_directory="${temporary_directory}/dist"
source_manifest="${repository_directory}/terraform-registry-manifest.json"
release_manifest_name='terraform-provider-cpanel_1.0.0_manifest.json'
release_manifest="${distribution_directory}/${release_manifest_name}"
checksum_file="${distribution_directory}/terraform-provider-cpanel_1.0.0_SHA256SUMS"

cleanup() {
  rm -rf "${temporary_directory}"
}
trap cleanup EXIT

mkdir -p "${distribution_directory}"
if command -v sha256sum >/dev/null 2>&1; then
  source_checksum="$(sha256sum "${source_manifest}" | awk '{ print $1 }')"
else
  source_checksum="$(shasum -a 256 "${source_manifest}" | awk '{ print $1 }')"
fi
printf '%s  %s\n' \
  "${source_checksum}" \
  "${release_manifest_name}" >"${checksum_file}"

DIST_DIR="${distribution_directory}" \
  "${script_directory}/materialize-release-manifest.sh" >/dev/null
if ! cmp -s "${source_manifest}" "${release_manifest}"; then
  printf 'Materialized Registry manifest does not match the source\n' >&2
  exit 1
fi

rm -f "${release_manifest}"
printf '%064d  %s\n' 0 "${release_manifest_name}" >"${checksum_file}"
if DIST_DIR="${distribution_directory}" \
  "${script_directory}/materialize-release-manifest.sh" >/dev/null 2>&1; then
  printf 'Materializer accepted a mismatched Registry manifest checksum\n' >&2
  exit 1
fi
if [[ -e "${release_manifest}" ]]; then
  printf 'Materializer wrote a Registry manifest after checksum rejection\n' >&2
  exit 1
fi
