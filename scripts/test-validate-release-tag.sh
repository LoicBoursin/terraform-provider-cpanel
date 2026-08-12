#!/usr/bin/env bash

set -euo pipefail

script_directory="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
validator="${script_directory}/validate-release-tag.sh"
temporary_directory="$(mktemp -d)"
cleanup() {
  rm -rf "${temporary_directory}"
}
trap cleanup EXIT
changelog="${temporary_directory}/CHANGELOG.md"

for tag in \
  v0.0.0 \
  v1.0.0 \
  v12.34.56 \
  v1.0.0-rc.1 \
  v1.0.0-alpha.beta \
  v1.0.0+build.7 \
  v1.0.0-rc.1+build.7; do
  version="${tag#v}"
  version="${version%%[-+]*}"
  printf '# Changelog\n\n## %s - 2026-07-20\n' "${version}" >"${changelog}"
  "${validator}" "${tag}" "${changelog}"
done

printf '# Changelog\n\n## 1.0.0 - 2026-07-20\n' >"${changelog}"

for tag in \
  '' \
  1.0.0 \
  v1 \
  v1.0 \
  v1.0.0.0 \
  v01.0.0 \
  v1.01.0 \
  v1.0.01 \
  v1.0.0-01 \
  v1.0.0- \
  v1.0.0+ \
  v1.0.0_rc1 \
  latest; do
  if GITHUB_REF_NAME=v1.0.0 \
    "${validator}" "${tag}" "${changelog}" >/dev/null 2>&1; then
    printf 'Expected invalid release tag to be rejected: %s\n' "${tag}" >&2
    exit 1
  fi
done

GITHUB_REF_NAME=v1.0.0 "${validator}"

if "${validator}" v1.0.1 "${changelog}" >/dev/null 2>&1; then
  printf 'Expected a tag that differs from the changelog to be rejected\n' >&2
  exit 1
fi

printf '# Changelog\n\n## Unreleased\n' >"${changelog}"
if "${validator}" v1.0.0 "${changelog}" >/dev/null 2>&1; then
  printf 'Expected an invalid changelog release to be rejected\n' >&2
  exit 1
fi
