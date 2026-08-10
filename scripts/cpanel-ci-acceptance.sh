#!/usr/bin/env bash

set -euo pipefail

script_directory="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

for variable in \
  CPANEL_HOST \
  CPANEL_USERNAME \
  CPANEL_API_TOKEN \
  CPANEL_API_TOKEN_NAME; do
  if [[ -z "${!variable:-}" ]]; then
    printf 'Missing required CI secret: %s\n' "${variable}" >&2
    exit 1
  fi
done

# shellcheck source=tool-versions.sh
source "${script_directory}/tool-versions.sh"

temporary_directory="$(mktemp -d)"
cleanup() {
  rm -rf "${temporary_directory}"
}
trap cleanup EXIT

baseline_file="${temporary_directory}/acceptance-baseline.env"
install -m 0600 \
  "${script_directory}/cpanel-ci-baseline.env" \
  "${baseline_file}"

export CPANEL_BASELINE_FILE="${baseline_file}"
export CPANEL_EXPECTED_TEST_HOST="${CPANEL_HOST}"
export CPANEL_EXPECTED_TEST_USERNAME="${CPANEL_USERNAME}"
export CPANEL_EXPECTED_VERSION="${cpanel_test_version}"
export CPANEL_ACCEPT_DESTRUCTIVE=1

"${script_directory}/test-acceptance.sh" "$@"
