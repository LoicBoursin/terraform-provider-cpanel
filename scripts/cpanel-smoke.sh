#!/usr/bin/env bash

set -euo pipefail

env_file="${CPANEL_ENV_FILE:-${HOME}/.config/terraform-provider-cpanel/acceptance.env}"

if [[ -f "${env_file}" ]]; then
  set -a
  # shellcheck disable=SC1090
  source "${env_file}"
  set +a
fi

for variable in CPANEL_HOST CPANEL_USERNAME CPANEL_API_TOKEN; do
  if [[ -z "${!variable:-}" ]]; then
    printf 'Missing required environment variable: %s\n' "${variable}" >&2
    exit 1
  fi
done

for command in curl jq; do
  if ! command -v "${command}" >/dev/null 2>&1; then
    printf 'Missing required command: %s\n' "${command}" >&2
    exit 1
  fi
done

host="${CPANEL_HOST%/}"
authorization="Authorization: cpanel ${CPANEL_USERNAME}:${CPANEL_API_TOKEN}"
response_file="$(mktemp)"

cleanup() {
  rm -f "${response_file}"
}

trap cleanup EXIT

request() {
  local endpoint="$1"
  local label="$2"
  local status
  local http_code

  http_code="$(
    curl \
      --silent \
      --show-error \
      --max-time 20 \
      --output "${response_file}" \
      --write-out '%{http_code}' \
      --header "${authorization}" \
      "${host}/${endpoint}"
  )"

  if [[ "${http_code}" != "200" ]]; then
    printf '%s failed with HTTP %s\n' "${label}" "${http_code}" >&2
    exit 1
  fi

  status="$(jq -r '.status // .cpanelresult.event.result // empty' "${response_file}")"
  if [[ "${status}" != "1" ]]; then
    printf '%s failed: %s\n' \
      "${label}" \
      "$(jq -c '{errors, messages, cpanelresult}' "${response_file}")" >&2
    exit 1
  fi
}

request \
  'execute/StatsBar/get_stats?display=cpanelversion' \
  'cPanel version check'
cpanel_version="$(
  jq -r '.data[] | select(.name == "cpanelversion") | .value' "${response_file}"
)"

request 'execute/Features/list_features' 'feature check'
for feature in cron mysql postgres; do
  if [[ "$(jq -r --arg feature "${feature}" '.data[$feature] // 0' "${response_file}")" != "1" ]]; then
    printf 'Required cPanel feature is disabled: %s\n' "${feature}" >&2
    exit 1
  fi
done

request 'execute/Postgresql/list_databases' 'PostgreSQL database check'
database_count="$(jq -r '.data | length' "${response_file}")"

request 'execute/Postgresql/list_users' 'PostgreSQL user check'
user_count="$(jq -r '.data | length' "${response_file}")"

test_prefix="${CPANEL_USERNAME}_tf"

request 'execute/Mysql/list_databases' 'MySQL database check'
mysql_database_count="$(jq -r '.data | length' "${response_file}")"
mysql_test_database_count="$(
  jq -r --arg prefix "${test_prefix}" \
    '[.data[].database | select(startswith($prefix))] | length' \
    "${response_file}"
)"

request 'execute/Mysql/list_users' 'MySQL user check'
mysql_user_count="$(jq -r '.data | length' "${response_file}")"
mysql_test_user_count="$(
  jq -r --arg prefix "${test_prefix}" \
    '[.data[].user | select(startswith($prefix))] | length' \
    "${response_file}"
)"

request \
  "json-api/cpanel?cpanel_jsonapi_user=${CPANEL_USERNAME}&cpanel_jsonapi_apiversion=2&cpanel_jsonapi_module=Cron&cpanel_jsonapi_func=fetchcron" \
  'cron check'
cron_count="$(jq -r '.cpanelresult.data | length' "${response_file}")"
cron_command_count="$(
  jq -r '[.cpanelresult.data[] | select(.type == "command")] | length' "${response_file}"
)"
cron_variable_count="$(
  jq -r '[.cpanelresult.data[] | select(.type == "variable")] | length' "${response_file}"
)"

if [[ "${CPANEL_REQUIRE_EMPTY:-0}" == "1" ]]; then
  if [[
    "${database_count}" != "0"
    || "${user_count}" != "0"
    || "${mysql_test_database_count}" != "0"
    || "${mysql_test_user_count}" != "0"
    || "${cron_count}" != "0"
  ]]; then
    printf 'cPanel test-managed inventory is not empty\n' >&2
    printf '  PostgreSQL databases: %s\n' "${database_count}" >&2
    printf '  PostgreSQL users: %s\n' "${user_count}" >&2
    printf '  MySQL test databases: %s\n' "${mysql_test_database_count}" >&2
    printf '  MySQL test users: %s\n' "${mysql_test_user_count}" >&2
    printf '  cron commands: %s\n' "${cron_command_count}" >&2
    printf '  cron variables: %s\n' "${cron_variable_count}" >&2
    exit 1
  fi
fi

printf 'cPanel smoke test passed\n'
printf '  account: %s\n' "${CPANEL_USERNAME}"
printf '  host: %s\n' "${host}"
printf '  version: %s\n' "${cpanel_version}"
printf '  PostgreSQL databases: %s\n' "${database_count}"
printf '  PostgreSQL users: %s\n' "${user_count}"
printf '  MySQL databases: %s (%s test-managed)\n' \
  "${mysql_database_count}" \
  "${mysql_test_database_count}"
printf '  MySQL users: %s (%s test-managed)\n' \
  "${mysql_user_count}" \
  "${mysql_test_user_count}"
printf '  cron commands: %s\n' "${cron_command_count}"
printf '  cron variables: %s\n' "${cron_variable_count}"
