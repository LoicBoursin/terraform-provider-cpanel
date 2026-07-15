#!/usr/bin/env bash

set -euo pipefail

script_directory="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repository_directory="$(cd "${script_directory}/.." && pwd)"
env_file="${CPANEL_ENV_FILE:-${HOME}/.config/terraform-provider-cpanel/acceptance.env}"
baseline_file="${CPANEL_BASELINE_FILE:-${env_file%.env}-baseline.env}"

for file in "${env_file}" "${baseline_file}"; do
  if [[ -f "${file}" ]]; then
    set -a
    # shellcheck disable=SC1090
    source "${file}"
    set +a
  fi
done

for variable in \
  CPANEL_HOST \
  CPANEL_USERNAME \
  CPANEL_API_TOKEN \
  CPANEL_EXPECTED_LOCALE \
  CPANEL_EXPECTED_LOG_ARCHIVE \
  CPANEL_EXPECTED_LOG_PRUNE \
  CPANEL_EXPECTED_LOG_RETENTION; do
  if [[ -z "${!variable:-}" ]]; then
    printf 'Missing required environment variable: %s\n' "${variable}" >&2
    exit 1
  fi
done

finalizer_active=1
finalize() {
  local command_status=$?
  local cleanup_status
  local inventory_status
  local restore_status

  trap - EXIT
  if [[ "${finalizer_active}" != "1" ]]; then
    exit "${command_status}"
  fi

  set +e
  "${script_directory}/cpanel-restore-test-singletons.sh"
  restore_status=$?
  "${script_directory}/cpanel-clean-test-artifacts.sh"
  cleanup_status=$?
  CPANEL_REQUIRE_EMPTY=1 "${script_directory}/cpanel-smoke.sh"
  inventory_status=$?
  set -e

  if [[ "${command_status}" != "0" ]]; then
    exit "${command_status}"
  fi
  if [[ "${restore_status}" != "0" ]]; then
    exit "${restore_status}"
  fi
  if [[ "${cleanup_status}" != "0" ]]; then
    exit "${cleanup_status}"
  fi
  exit "${inventory_status}"
}

trap finalize EXIT

"${script_directory}/cpanel-restore-test-singletons.sh"
"${script_directory}/cpanel-clean-test-artifacts.sh"
CPANEL_REQUIRE_EMPTY=1 "${script_directory}/cpanel-smoke.sh"

cd "${repository_directory}"

export CGO_ENABLED="${CGO_ENABLED:-0}"
export TF_ACC=1

set +e
go test ./internal/provider -v -count=1 -timeout 120m "$@"
test_status=$?
set -e

exit "${test_status}"
