#!/usr/bin/env bash

set -euo pipefail

script_directory="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repository_directory="$(cd "${script_directory}/.." && pwd)"
if (($# > 0)); then
  tag="$1"
else
  tag="${GITHUB_REF_NAME:-}"
fi
changelog="${2:-${repository_directory}/CHANGELOG.md}"
numeric_identifier='(0|[1-9][0-9]*)'
non_numeric_identifier='[0-9]*[A-Za-z-][0-9A-Za-z-]*'
prerelease_identifier="(${numeric_identifier}|${non_numeric_identifier})"
prerelease="(-${prerelease_identifier}(\\.${prerelease_identifier})*)"
build_identifier='[0-9A-Za-z-]+'
build="(\\+${build_identifier}(\\.${build_identifier})*)"
pattern="^v${numeric_identifier}\\.${numeric_identifier}\\.${numeric_identifier}${prerelease}?${build}?$"

if [[ -z "${tag}" || ! "${tag}" =~ ${pattern} ]]; then
  printf 'Invalid release tag %q; expected vMAJOR.MINOR.PATCH with optional SemVer prerelease or build metadata\n' \
    "${tag}" >&2
  exit 1
fi

if [[ ! -f "${changelog}" ]]; then
  printf 'Missing changelog: %s\n' "${changelog}" >&2
  exit 1
fi

changelog_version="$(awk '$1 == "##" {print $2; exit}' "${changelog}")"
release_version="${tag#v}"
release_version="${release_version%%[-+]*}"
stable_version_pattern="^${numeric_identifier}\.${numeric_identifier}\.${numeric_identifier}$"
if [[ ! "${changelog_version}" =~ ${stable_version_pattern} ]]; then
  printf 'The first changelog release must be a stable MAJOR.MINOR.PATCH version\n' >&2
  exit 1
fi
if [[ "${release_version}" != "${changelog_version}" ]]; then
  printf 'Release tag %q targets version %s, but the first changelog release is %s\n' \
    "${tag}" \
    "${release_version}" \
    "${changelog_version}" >&2
  exit 1
fi
