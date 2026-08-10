#!/usr/bin/env bash

set -euo pipefail

script_directory="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repository_directory="$(cd "${script_directory}/.." && pwd)"
tag="${1:-${GITHUB_REF_NAME:-}}"
repository="${GITHUB_REPOSITORY:-}"
changelog="${CHANGELOG_PATH:-${repository_directory}/CHANGELOG.md}"

"${script_directory}/validate-release-tag.sh" "${tag}" "${changelog}"

if [[ ! "${repository}" =~ ^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$ ]]; then
  printf 'Invalid or missing GITHUB_REPOSITORY: %s\n' "${repository}" >&2
  exit 1
fi
for command in gh gpg; do
  if ! command -v "${command}" >/dev/null 2>&1; then
    printf 'Missing required command: %s\n' "${command}" >&2
    exit 1
  fi
done

temporary_directory="$(mktemp -d)"
cleanup() {
  rm -rf "${temporary_directory}"
}
trap cleanup EXIT

downloaded=0
for attempt in 1 2 3 4 5; do
  rm -f "${temporary_directory}"/*
  if gh release download \
    "${tag}" \
    --repo "${repository}" \
    --dir "${temporary_directory}"; then
    downloaded=1
    break
  fi
  if [[ "${attempt}" != "5" ]]; then
    sleep 5
  fi
done
if [[ "${downloaded}" != "1" ]]; then
  printf 'Unable to download published release assets for %s\n' "${tag}" >&2
  exit 1
fi

EXPECTED_RELEASE_NAME="terraform-provider-cpanel_${tag#v}" \
  DIST_DIR="${temporary_directory}" \
  "${script_directory}/verify-release-snapshot.sh"

shopt -s nullglob
assets=("${temporary_directory}"/*)
checksum_files=("${temporary_directory}"/*_SHA256SUMS)
signature_files=("${temporary_directory}"/*_SHA256SUMS.sig)
if [[ "${#assets[@]}" != "29" ]]; then
  printf 'Expected exactly 29 published release assets, found %d\n' \
    "${#assets[@]}" >&2
  exit 1
fi
if [[ "${#checksum_files[@]}" != "1" || "${#signature_files[@]}" != "1" ]]; then
  printf 'Expected exactly one checksum file and detached signature\n' >&2
  exit 1
fi

gpg --batch --verify "${signature_files[0]}" "${checksum_files[0]}"
printf 'Published release asset verification passed for %s\n' "${tag}"
