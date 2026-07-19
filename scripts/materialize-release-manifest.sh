#!/usr/bin/env bash

set -euo pipefail

script_directory="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repository_directory="$(cd "${script_directory}/.." && pwd)"
distribution_directory="${DIST_DIR:-${repository_directory}/dist}"
source_manifest="${REGISTRY_MANIFEST:-${repository_directory}/terraform-registry-manifest.json}"

if [[ ! -d "${distribution_directory}" ]]; then
  printf 'Missing release distribution directory: %s\n' \
    "${distribution_directory}" >&2
  exit 1
fi
if [[ ! -f "${source_manifest}" ]]; then
  printf 'Missing Terraform Registry manifest: %s\n' \
    "${source_manifest}" >&2
  exit 1
fi

shopt -s nullglob
checksum_files=("${distribution_directory}"/*_SHA256SUMS)
if [[ "${#checksum_files[@]}" != "1" ]]; then
  printf 'Expected exactly one release checksum file in %s\n' \
    "${distribution_directory}" >&2
  exit 1
fi

checksum_file="${checksum_files[0]}"
release_manifest="${checksum_file%_SHA256SUMS}_manifest.json"
release_manifest_name="$(basename "${release_manifest}")"
declared_checksum="$(
  awk -v name="${release_manifest_name}" '$2 == name { print $1 }' \
    "${checksum_file}"
)"
if [[ -z "${declared_checksum}" || "${declared_checksum}" == *$'\n'* ]]; then
  printf 'Expected one checksum entry for %s\n' \
    "${release_manifest_name}" >&2
  exit 1
fi

if command -v sha256sum >/dev/null 2>&1; then
  source_checksum="$(sha256sum "${source_manifest}" | awk '{ print $1 }')"
else
  source_checksum="$(shasum -a 256 "${source_manifest}" | awk '{ print $1 }')"
fi
if [[ "${declared_checksum}" != "${source_checksum}" ]]; then
  printf 'Registry manifest checksum does not match the release metadata\n' >&2
  exit 1
fi

install -m 0644 "${source_manifest}" "${release_manifest}"
printf 'Materialized release Registry manifest: %s\n' \
  "${release_manifest_name}"
