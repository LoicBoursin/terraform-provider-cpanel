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

get_request() {
  local endpoint="$1"
  local label="$2"
  local http_code
  local status

  http_code="$(
    curl \
      --silent \
      --show-error \
      --max-time 90 \
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

uapi_post() {
  local module="$1"
  local function="$2"
  local label="$3"
  local http_code
  local parameter
  local status
  local curl_arguments=(
    --silent
    --show-error
    --max-time 90
    --request POST
    --output "${response_file}"
    --write-out '%{http_code}'
    --header "${authorization}"
    --header 'Content-Type: application/x-www-form-urlencoded'
  )

  shift 3
  for parameter in "$@"; do
    curl_arguments+=(--data-urlencode "${parameter}")
  done

  http_code="$(curl "${curl_arguments[@]}" "${host}/execute/${module}/${function}")"

  if [[ "${http_code}" != "200" ]]; then
    printf '%s failed with HTTP %s\n' "${label}" "${http_code}" >&2
    exit 1
  fi

  status="$(jq -r '.status // empty' "${response_file}")"
  if [[ "${status}" != "1" ]]; then
    printf '%s failed: %s\n' \
      "${label}" \
      "$(jq -c '{errors, messages}' "${response_file}")" >&2
    exit 1
  fi
}

api2_post() {
  local module="$1"
  local function="$2"
  local label="$3"
  local http_code
  local parameter
  local status
  local curl_arguments=(
    --silent
    --show-error
    --max-time 90
    --request POST
    --output "${response_file}"
    --write-out '%{http_code}'
    --header "${authorization}"
    --header 'Content-Type: application/x-www-form-urlencoded'
    --data-urlencode 'cpanel_jsonapi_apiversion=2'
    --data-urlencode "cpanel_jsonapi_user=${CPANEL_USERNAME}"
    --data-urlencode "cpanel_jsonapi_module=${module}"
    --data-urlencode "cpanel_jsonapi_func=${function}"
  )

  shift 3
  for parameter in "$@"; do
    curl_arguments+=(--data-urlencode "${parameter}")
  done

  http_code="$(curl "${curl_arguments[@]}" "${host}/json-api/cpanel")"

  if [[ "${http_code}" != "200" ]]; then
    printf '%s failed with HTTP %s\n' "${label}" "${http_code}" >&2
    exit 1
  fi

  status="$(jq -r '.cpanelresult.event.result // empty' "${response_file}")"
  if [[ "${status}" != "1" ]]; then
    printf '%s failed: %s\n' \
      "${label}" \
      "$(jq -c '{cpanelresult}' "${response_file}")" >&2
    exit 1
  fi
}

purge_test_git_directory() {
  local directory="$1"
  local name="${directory##*/}"

  case "${name}" in
    tfcpanel-git-* | .terraform-cpanel-git-delete-*) ;;
    *)
      printf 'Refusing to purge non-test Git directory: %s\n' \
        "${directory}" >&2
      exit 1
      ;;
  esac

  api2_post \
    'Fileman' \
    'fileop' \
    "Move test Git directory ${directory} to trash" \
    'op=trash' \
    "sourcefiles=${directory}" \
    'doubledecode=0'
  if [[
    "$(jq -r '[.cpanelresult.data[]? | select(.result == 1)] | length' "${response_file}")" != "1"
  ]]; then
    printf 'Move test Git directory %s to trash failed: %s\n' \
      "${directory}" \
      "$(jq -c '.cpanelresult.data' "${response_file}")" >&2
    exit 1
  fi

  uapi_post \
    'Fileman' \
    'empty_trash' \
    "Permanently delete test Git directory ${name}" \
    "only_these_files=${name}"

  get_request \
    'execute/Fileman/list_files?dir=.trash&show_hidden=1&limit=1000' \
    'cPanel trash verification'
  if jq -e --arg name "${name}" \
    '.data[] | select(.file == $name)' \
    "${response_file}" >/dev/null; then
    printf 'Test Git trash entry still exists: %s\n' "${name}" >&2
    exit 1
  fi
}

remove_cron_line() {
  local linekey="$1"
  local label="$2"
  local http_code
  local status

  http_code="$(
    curl \
      --silent \
      --show-error \
      --max-time 90 \
      --request POST \
      --output "${response_file}" \
      --write-out '%{http_code}' \
      --header "${authorization}" \
      --header 'Content-Type: application/x-www-form-urlencoded' \
      --data-urlencode 'cpanel_jsonapi_apiversion=2' \
      --data-urlencode "cpanel_jsonapi_user=${CPANEL_USERNAME}" \
      --data-urlencode 'cpanel_jsonapi_module=Cron' \
      --data-urlencode 'cpanel_jsonapi_func=remove_line' \
      --data-urlencode "linekey=${linekey}" \
      "${host}/json-api/cpanel"
  )"

  if [[ "${http_code}" != "200" ]]; then
    printf '%s failed with HTTP %s\n' "${label}" "${http_code}" >&2
    exit 1
  fi

  status="$(jq -r '.cpanelresult.event.result // empty' "${response_file}")"
  if [[ "${status}" != "1" ]]; then
    printf '%s failed: %s\n' \
      "${label}" \
      "$(jq -c '{cpanelresult}' "${response_file}")" >&2
    exit 1
  fi
}

test_prefix="${CPANEL_USERNAME}_tf"
deleted_api_tokens=0
deleted_dynamic_dns_domains=0
deleted_redirects=0
deleted_mime_types=0
deleted_apache_handlers=0
deleted_passenger_applications=0
deleted_ssl_certificates=0
deleted_ssl_csrs=0
deleted_git_repositories=0
deleted_git_repository_directories=0
deleted_git_repository_trash_entries=0
deleted_databases=0
deleted_users=0
deleted_mysql_databases=0
deleted_mysql_users=0
deleted_mysql_remote_hosts=0
deleted_email_accounts=0
deleted_calendar_delegates=0
deleted_email_filters=0
unsuspended_email_restrictions=0
deleted_email_forwarders=0
deleted_email_domain_forwarders=0
deleted_email_auto_responders=0
deleted_ftp_accounts=0
deleted_ip_blocks=0
deleted_dns_records=0
deleted_addon_domains=0
deleted_domain_aliases=0
enabled_modsecurity_domains=0
deleted_subdomains=0
deleted_domain_directories=0
deleted_directory_privacy_password_directories=0
deleted_cron_lines=0

get_request 'execute/Tokens/list' 'API token inventory'
while IFS= read -r token_name; do
  if [[ -z "${token_name}" ]]; then
    continue
  fi
  uapi_post 'Tokens' 'revoke' "Revoke test API token ${token_name}" "name=${token_name}"
  deleted_api_tokens=$((deleted_api_tokens + 1))
done < <(
  jq -r \
    '.data[].name | select(startswith("tfcpaneltoken"))' \
  "${response_file}"
)

get_request \
  'execute/PassengerApps/list_applications' \
  'Passenger application inventory'
while IFS= read -r application_name; do
  if [[ -z "${application_name}" ]]; then
    continue
  fi
  uapi_post \
    'PassengerApps' \
    'unregister_application' \
    "Unregister test Passenger application ${application_name}" \
    "name=${application_name}"
  deleted_passenger_applications=$((deleted_passenger_applications + 1))
done < <(
  jq -r \
    '.data
      | to_entries[]
      | .key
      | select(startswith("tfcpanelpassenger"))' \
    "${response_file}"
)

get_request 'execute/SSL/list_csrs' 'stored SSL CSR inventory'
if ! jq -e \
  --arg prefix 'tfcpanelcsr' \
  '
    (.data | type == "array")
    and all(
      .data[];
      (
        (
          ((.friendly_name? | type) == "string"
            and (.friendly_name | startswith($prefix)))
          or
          ((.commonName? | type) == "string"
            and (.commonName | startswith($prefix)))
        )
        | not
      )
      or (
        ((.id? | type) == "string" or (.id? | type) == "number")
        and ((.id | tostring | length) > 0)
        and ((.friendly_name? | type) == "string")
        and (.friendly_name | startswith($prefix))
        and ((.commonName? | type) == "string")
        and (.commonName | startswith($prefix))
      )
    )
  ' \
  "${response_file}" >/dev/null; then
  printf 'Refusing to clean ambiguous test SSL CSR inventory\n' >&2
  exit 1
fi
ssl_csr_candidates="$(
  jq -r \
    --arg prefix 'tfcpanelcsr' \
    '
      .data[]
      | select(
          (.friendly_name? | type) == "string"
          and (.friendly_name | startswith($prefix))
          and (.commonName? | type) == "string"
          and (.commonName | startswith($prefix))
        )
      | [(.id | tostring), .friendly_name, .commonName]
      | @tsv
    ' \
    "${response_file}"
)"
while IFS=$'\t' read -r csr_id friendly_name common_name; do
  if [[ -z "${csr_id}" ]]; then
    continue
  fi

  get_request 'execute/SSL/list_csrs' \
    "Re-read test SSL CSR ${friendly_name}"
  if ! jq -e \
    --arg id "${csr_id}" \
    --arg friendly_name "${friendly_name}" \
    --arg common_name "${common_name}" \
    '
      (.data | type == "array")
      and (
        [
          .data[]
          | select(
              ((.id? | type) == "string" or (.id? | type) == "number")
              and ((.id | tostring) == $id)
            )
        ] as $matches
        | ($matches | length) == 1
        and ($matches[0].friendly_name? == $friendly_name)
        and ($matches[0].commonName? == $common_name)
      )
    ' \
    "${response_file}" >/dev/null; then
    printf 'Refusing to delete ambiguous test SSL CSR: %s\n' \
      "${friendly_name}" >&2
    exit 1
  fi

  encoded_csr_id="$(
    jq -rn --arg value "${csr_id}" '$value | @uri'
  )"
  get_request \
    "execute/SSL/show_csr?id=${encoded_csr_id}" \
    "Inspect test SSL CSR ${friendly_name}"
  if ! jq -e \
    --arg id "${csr_id}" \
    --arg friendly_name "${friendly_name}" \
    --arg common_name "${common_name}" \
    '
      (.data | type == "object")
      and ((.data.csr? | type) == "string")
      and (.data.csr | startswith("-----BEGIN CERTIFICATE REQUEST-----"))
      and ((.data.details.id | tostring) == $id)
      and (.data.details.friendly_name == $friendly_name)
      and (.data.details.commonName == $common_name)
    ' \
    "${response_file}" >/dev/null; then
    printf 'Refusing to delete unverifiable test SSL CSR: %s\n' \
      "${friendly_name}" >&2
    exit 1
  fi

  uapi_post \
    'SSL' \
    'delete_csr' \
    "Delete test SSL CSR ${friendly_name}" \
    "id=${csr_id}" \
    "friendly_name=${friendly_name}"
  deleted_ssl_csrs=$((deleted_ssl_csrs + 1))
done <<<"${ssl_csr_candidates}"

get_request \
  'execute/SSL/list_csrs' \
  'stored SSL CSR inventory after cleanup'
if ! jq -e '(.data | type) == "array"' \
  "${response_file}" >/dev/null; then
  printf 'Stored SSL CSR inventory after cleanup is incomplete\n' >&2
  exit 1
fi
if jq -e \
  --arg prefix 'tfcpanelcsr' \
  '
    .data[]
    | select(
        ((.friendly_name? | type) == "string"
          and (.friendly_name | startswith($prefix)))
        or
        ((.commonName? | type) == "string"
          and (.commonName | startswith($prefix)))
      )
  ' \
  "${response_file}" >/dev/null; then
  printf 'Test SSL CSR still exists after cleanup\n' >&2
  exit 1
fi

get_request 'execute/SSL/list_certs' 'stored SSL certificate inventory'
if ! jq -e \
  --arg prefix 'tfcpanelsslcert' \
  '
    (.data | type == "array")
    and all(
      .data[];
      (
        (.friendly_name? | type) != "string"
        or (.friendly_name | startswith($prefix) | not)
      )
      or (
        ((.id? | type) == "string" or (.id? | type) == "number")
        and ((.id | tostring | length) > 0)
      )
    )
  ' \
  "${response_file}" >/dev/null; then
  printf 'Stored SSL certificate inventory is incomplete\n' >&2
  exit 1
fi
ssl_certificate_candidates="$(
  jq -r \
    '
      .data[]
      | select(
          (.friendly_name? | type) == "string"
          and (.friendly_name | startswith("tfcpanelsslcert"))
        )
      | [(.id | tostring), .friendly_name]
      | @tsv
    ' \
    "${response_file}"
)"
while IFS=$'\t' read -r certificate_id friendly_name; do
  if [[ -z "${certificate_id}" ]]; then
    continue
  fi
  get_request 'execute/SSL/list_certs' \
    "Re-read test SSL certificate ${friendly_name}"
  if ! jq -e \
    --arg id "${certificate_id}" \
    --arg friendly_name "${friendly_name}" \
    '
      (.data | type == "array")
      and (
        [
          .data[]
          | select(
              ((.id? | type) == "string" or (.id? | type) == "number")
              and ((.id | tostring) == $id)
            )
        ] as $matches
        | ($matches | length) == 1
        and ($matches[0].friendly_name? == $friendly_name)
        and (
          $matches[0].domain_is_configured? == 0
          or $matches[0].domain_is_configured? == "0"
          or $matches[0].domain_is_configured? == false
          or $matches[0].domain_is_configured? == "false"
        )
      )
    ' \
    "${response_file}" >/dev/null; then
    printf 'Refusing to delete ambiguous or configured test SSL certificate: %s\n' \
      "${friendly_name}" >&2
    exit 1
  fi

  get_request 'execute/SSL/installed_hosts' \
    "Re-read installed SSL hosts before deleting ${friendly_name}"
  if ! jq -e \
    '
      (.data | type == "array")
      and all(
        .data[];
        (.certificate? | type) == "object"
        and (
          (.certificate.id? | type) == "string"
          or (.certificate.id? | type) == "number"
        )
        and ((.certificate.id | tostring | length) > 0)
      )
    ' \
    "${response_file}" >/dev/null; then
    printf 'Installed SSL host inventory is incomplete\n' >&2
    exit 1
  fi
  if jq -e \
    --arg id "${certificate_id}" \
    '.data[] | select((.certificate.id | tostring) == $id)' \
    "${response_file}" >/dev/null; then
    printf 'Refusing to delete installed test SSL certificate: %s\n' \
      "${friendly_name}" >&2
    exit 1
  fi

  uapi_post \
    'SSL' \
    'delete_cert' \
    "Delete test SSL certificate ${friendly_name}" \
    "id=${certificate_id}"
  deleted_ssl_certificates=$((deleted_ssl_certificates + 1))
done <<<"${ssl_certificate_candidates}"

get_request \
  'execute/SSL/list_certs' \
  'stored SSL certificate inventory after cleanup'
if ! jq -e '(.data | type) == "array"' \
  "${response_file}" >/dev/null; then
  printf 'Stored SSL certificate inventory after cleanup is incomplete\n' >&2
  exit 1
fi
if jq -e \
  '.data[]
    | .friendly_name
    | select(startswith("tfcpanelsslcert"))' \
  "${response_file}" >/dev/null; then
  printf 'Test SSL certificate still exists after cleanup\n' >&2
  exit 1
fi

get_request 'execute/VersionControl/retrieve' 'Git repository inventory'
while IFS= read -r repository_root; do
  if [[ -z "${repository_root}" ]]; then
    continue
  fi
  uapi_post \
    'VersionControl' \
    'delete' \
    "Delete test Git repository ${repository_root}" \
    "repository_root=${repository_root}"
  deleted_git_repositories=$((deleted_git_repositories + 1))
done < <(
  jq -r \
    '.data[]
      | select(
          .repository_root
          | split("/")
          | last
          | startswith("tfcpanel-git-")
        )
      | .repository_root' \
    "${response_file}"
)

get_request 'execute/DynamicDNS/list' 'Dynamic DNS inventory'
while IFS=$'\t' read -r domain id; do
  if [[ -z "${domain}" || -z "${id}" ]]; then
    continue
  fi
  uapi_post \
    'DynamicDNS' \
    'delete' \
    "Delete test Dynamic DNS domain ${domain}" \
    "id=${id}"
  deleted_dynamic_dns_domains=$((deleted_dynamic_dns_domains + 1))
done < <(
  jq -r \
    '.data[]
      | select(.domain | startswith("tfcpanelddns"))
      | [.domain, .id]
      | @tsv' \
    "${response_file}"
)

get_request 'execute/Mime/list_redirects' 'HTTP redirect inventory'
while IFS=$'\t' read -r domain source; do
  if [[ -z "${domain}" || -z "${source}" ]]; then
    continue
  fi
  uapi_post \
    'Mime' \
    'delete_redirect' \
    "Delete test HTTP redirect ${domain}${source}" \
    "domain=${domain}" \
    "src=${source}"
  deleted_redirects=$((deleted_redirects + 1))
done < <(
  jq -r \
    '.data[]
      | select(.source | startswith("/tfcpanelredirect-"))
      | [.domain, .source]
      | @tsv' \
    "${response_file}"
)

get_request 'execute/Mime/list_mime?type=user' 'custom MIME type inventory'
while IFS= read -r mime_type; do
  if [[ -z "${mime_type}" ]]; then
    continue
  fi
  uapi_post \
    'Mime' \
    'delete_mime' \
    "Delete test custom MIME type ${mime_type}" \
    "type=${mime_type}"
  deleted_mime_types=$((deleted_mime_types + 1))
done < <(
  jq -r \
    '.data[].type | select(startswith("application/x-tfcpanel-"))' \
    "${response_file}"
)

get_request 'execute/Mime/list_handlers?type=user' 'Apache handler inventory'
while IFS= read -r extension; do
  if [[ -z "${extension}" ]]; then
    continue
  fi
  uapi_post \
    'Mime' \
    'delete_handler' \
    "Delete test Apache handler ${extension}" \
    "extension=${extension}"
  deleted_apache_handlers=$((deleted_apache_handlers + 1))
done < <(
  jq -r \
    '.data[].extension | select(startswith(".tfcpanelhandler"))' \
    "${response_file}"
)

get_request 'execute/Postgresql/list_databases' 'PostgreSQL database inventory'
while IFS= read -r database; do
  if [[ -z "${database}" ]]; then
    continue
  fi
  uapi_post 'Postgresql' 'delete_database' "Delete test database ${database}" "name=${database}"
  deleted_databases=$((deleted_databases + 1))
done < <(
  jq -r \
    --arg prefix "${test_prefix}" \
    '.data[].database | select(startswith($prefix))' \
    "${response_file}"
)

get_request 'execute/Postgresql/list_users' 'PostgreSQL user inventory'
while IFS= read -r user; do
  if [[ -z "${user}" ]]; then
    continue
  fi
  uapi_post 'Postgresql' 'delete_user' "Delete test user ${user}" "name=${user}"
  deleted_users=$((deleted_users + 1))
done < <(
  jq -r \
    --arg prefix "${test_prefix}" \
    '.data[] | select(startswith($prefix))' \
    "${response_file}"
)

get_request 'execute/Mysql/list_databases' 'MySQL database inventory'
while IFS= read -r database; do
  if [[ -z "${database}" ]]; then
    continue
  fi
  uapi_post 'Mysql' 'delete_database' "Delete test MySQL database ${database}" "name=${database}"
  deleted_mysql_databases=$((deleted_mysql_databases + 1))
done < <(
  jq -r \
    --arg prefix "${test_prefix}" \
    '.data[].database | select(startswith($prefix))' \
    "${response_file}"
)

get_request 'execute/Mysql/list_users' 'MySQL user inventory'
while IFS= read -r user; do
  if [[ -z "${user}" ]]; then
    continue
  fi
  uapi_post 'Mysql' 'delete_user' "Delete test MySQL user ${user}" "name=${user}"
  deleted_mysql_users=$((deleted_mysql_users + 1))
done < <(
  jq -r \
    --arg prefix "${test_prefix}" \
    '.data[].user | select(startswith($prefix))' \
    "${response_file}"
)

get_request \
  "json-api/cpanel?cpanel_jsonapi_user=${CPANEL_USERNAME}&cpanel_jsonapi_apiversion=2&cpanel_jsonapi_module=MysqlFE&cpanel_jsonapi_func=listhosts" \
  'remote MySQL host inventory'
while IFS= read -r remote_host; do
  if [[ -z "${remote_host}" ]]; then
    continue
  fi
  uapi_post \
    'Mysql' \
    'delete_host' \
    "Delete test remote MySQL host ${remote_host}" \
    "host=${remote_host}"
  deleted_mysql_remote_hosts=$((deleted_mysql_remote_hosts + 1))
done < <(
  jq -r \
    '.cpanelresult.data[].host
      | select(
          . == "198.51.100.245"
          or . == "198.51.100.246"
          or . == "198.51.100.247"
          or . == "198.51.100.248"
          or . == "198.51.100.249"
          or . == "198.51.100.250"
        )' \
    "${response_file}"
)

get_request \
  'execute/Email/list_pops_with_disk?skip_main=1&no_disk=1&get_restrictions=1' \
  'Email account inventory'
email_account_candidates="$(
  jq -r \
    '.data[]
      | select(.email | startswith("tfcpanel"))
      | [
          .email,
          (.suspended_login // 0),
          (.suspended_incoming // 0),
          (.suspended_outgoing // 0),
          (.hold_outgoing // 0)
        ]
      | @tsv' \
    "${response_file}"
)"
email_filter_accounts="$(
  jq -r \
    '.data[].email | select(startswith("tfcpanelfilter"))' \
    "${response_file}"
)"

get_request \
  'execute/CPDAVD/list_delegates' \
  'Calendar delegate inventory'
if ! jq -e \
  '
    (.data | type == "array")
    and all(
      .data[];
      (.delegator? | type) == "string"
      and (.delegatee? | type) == "string"
      and (.calendar? | type) == "string"
      and (
        .readonly? == 0
        or .readonly? == "0"
        or .readonly? == false
        or .readonly? == "false"
        or .readonly? == 1
        or .readonly? == "1"
        or .readonly? == true
        or .readonly? == "true"
      )
    )
  ' \
  "${response_file}" >/dev/null; then
  printf 'Calendar delegate inventory is incomplete\n' >&2
  exit 1
fi
calendar_delegate_candidates="$(
  jq -r \
    '
      .data[]
      | select(
          (.delegator | split("@")[0] | startswith("tfcpanelcal"))
          or (.delegatee | split("@")[0] | startswith("tfcpanelcal"))
        )
      | [.delegator, .calendar, .delegatee]
      | @tsv
    ' \
    "${response_file}"
)"
while IFS=$'\t' read -r delegator calendar delegatee; do
  if [[
    -z "${delegator}"
    || -z "${calendar}"
    || -z "${delegatee}"
  ]]; then
    continue
  fi
  uapi_post \
    'CPDAVD' \
    'remove_delegate' \
    "Delete test calendar delegate ${delegator} to ${delegatee}" \
    "delegator=${delegator}" \
    "calendar=${calendar}" \
    "delegatee=${delegatee}"
  deleted_calendar_delegates=$((deleted_calendar_delegates + 1))
done <<<"${calendar_delegate_candidates}"

get_request \
  'execute/CPDAVD/list_delegates' \
  'Calendar delegate inventory after cleanup'
if jq -e \
  '
    .data[]
    | select(
        (.delegator | split("@")[0] | startswith("tfcpanelcal"))
        or (.delegatee | split("@")[0] | startswith("tfcpanelcal"))
      )
  ' \
  "${response_file}" >/dev/null; then
  printf 'Test calendar delegate still exists\n' >&2
  exit 1
fi

while IFS= read -r address; do
  if [[ -z "${address}" ]]; then
    continue
  fi

  get_request \
    "execute/Email/list_filters?account=${address}" \
    "Email filter inventory for ${address}"
  if ! jq -e \
    '
      (.data | type == "array")
      and all(
        .data[];
        (.filtername? | type) == "string"
        and (.rules? | type) == "array"
        and (.actions? | type) == "array"
        and (
          .enabled? == 0
          or .enabled? == "0"
          or .enabled? == false
          or .enabled? == "false"
          or .enabled? == 1
          or .enabled? == "1"
          or .enabled? == true
          or .enabled? == "true"
        )
      )
    ' \
    "${response_file}" >/dev/null; then
    printf 'Email filter inventory is incomplete for %s\n' \
      "${address}" >&2
    exit 1
  fi

  test_filter_names="$(
    jq -r \
      '.data[].filtername | select(startswith("tfcpanelfilter"))' \
      "${response_file}"
  )"
  while IFS= read -r filter_name; do
    if [[ -z "${filter_name}" ]]; then
      continue
    fi
    uapi_post \
      'Email' \
      'delete_filter' \
      "Delete test email filter ${filter_name} for ${address}" \
      "account=${address}" \
      "filtername=${filter_name}"
    deleted_email_filters=$((deleted_email_filters + 1))
  done <<<"${test_filter_names}"

  get_request \
    "execute/Email/list_filters?account=${address}" \
    "Email filter inventory after cleanup for ${address}"
  if jq -e \
    '.data[].filtername | select(startswith("tfcpanelfilter"))' \
    "${response_file}" >/dev/null; then
    printf 'Test email filter still exists for %s\n' "${address}" >&2
    exit 1
  fi
done <<<"${email_filter_accounts}"

while IFS=$'\t' read -r address login incoming outgoing held; do
  if [[ -z "${address}" ]]; then
    continue
  fi
  if [[ "${held}" == "1" ]]; then
    printf 'Refusing to delete test email account with held outgoing mail: %s\n' \
      "${address}" >&2
    exit 1
  fi
  if [[ "${login}" == "1" ]]; then
    uapi_post \
      'Email' \
      'unsuspend_login' \
      "Unsuspend login for test email account ${address}" \
      "email=${address}"
    unsuspended_email_restrictions=$((unsuspended_email_restrictions + 1))
  fi
  if [[ "${incoming}" == "1" ]]; then
    uapi_post \
      'Email' \
      'unsuspend_incoming' \
      "Unsuspend incoming mail for test email account ${address}" \
      "email=${address}"
    unsuspended_email_restrictions=$((unsuspended_email_restrictions + 1))
  fi
  if [[ "${outgoing}" == "1" ]]; then
    uapi_post \
      'Email' \
      'unsuspend_outgoing' \
      "Unsuspend outgoing mail for test email account ${address}" \
      "email=${address}"
    unsuspended_email_restrictions=$((unsuspended_email_restrictions + 1))
  fi
  uapi_post 'Email' 'delete_pop' "Delete test email account ${address}" "email=${address}"
  deleted_email_accounts=$((deleted_email_accounts + 1))
done <<<"${email_account_candidates}"

get_request 'execute/Email/list_mail_domains' 'Email forwarder domain inventory'
mail_domains="$(jq -r '.data[].domain' "${response_file}")"
while IFS= read -r domain; do
  if [[ -z "${domain}" ]]; then
    continue
  fi

  get_request \
    "execute/Email/list_forwarders?domain=${domain}" \
    "Email forwarder inventory for ${domain}"
  while IFS=$'\t' read -r address destination; do
    if [[ -z "${address}" || -z "${destination}" ]]; then
      continue
    fi
    uapi_post \
      'Email' \
      'delete_forwarder' \
      "Delete test email forwarder ${address}" \
      "address=${address}" \
      "forwarder=${destination}"
    deleted_email_forwarders=$((deleted_email_forwarders + 1))
  done < <(
    jq -r \
      '.data[]
        | select(.dest | startswith("tfcpanelfwd"))
        | [.dest, .forward]
        | @tsv' \
      "${response_file}"
  )
done <<<"${mail_domains}"

while IFS= read -r domain; do
  if [[ -z "${domain}" ]]; then
    continue
  fi

  get_request \
    "execute/Email/list_auto_responders?domain=${domain}" \
    "Email autoresponder inventory for ${domain}"
  while IFS= read -r address; do
    if [[ -z "${address}" ]]; then
      continue
    fi
    uapi_post \
      'Email' \
      'delete_auto_responder' \
      "Delete test email autoresponder ${address}" \
      "email=${address}"
    deleted_email_auto_responders=$((deleted_email_auto_responders + 1))
  done < <(
    jq -r \
      '.data[].email | select(startswith("tfcpanelauto"))' \
      "${response_file}"
  )
done <<<"${mail_domains}"

get_request 'execute/Email/list_domain_forwarders' 'Email domain forwarder inventory'
while IFS= read -r domain; do
  if [[ -z "${domain}" ]]; then
    continue
  fi
  uapi_post \
    'Email' \
    'delete_domain_forwarder' \
    "Delete test email domain forwarder ${domain}" \
    "domain=${domain}"
  deleted_email_domain_forwarders=$((deleted_email_domain_forwarders + 1))
done < <(
  jq -r \
    '.data[]
      | select(.forward | startswith("tfcpaneldomainfwd"))
      | .dest' \
    "${response_file}"
)

get_request 'execute/Ftp/list_ftp_with_disk?include_acct_types=sub' 'FTP account inventory'
while IFS= read -r login; do
  if [[ -z "${login}" ]]; then
    continue
  fi
  user="${login%@*}"
  domain="${login#*@}"
  uapi_post \
    'Ftp' \
    'delete_ftp' \
    "Delete test FTP account ${login}" \
    "user=${user}" \
    "domain=${domain}" \
    'destroy=1'
  deleted_ftp_accounts=$((deleted_ftp_accounts + 1))
done < <(
  jq -r \
    '.data[].login | select(startswith("tfcpanelftp"))' \
    "${response_file}"
)

get_request \
  "json-api/cpanel?cpanel_jsonapi_user=${CPANEL_USERNAME}&cpanel_jsonapi_apiversion=2&cpanel_jsonapi_module=DenyIp&cpanel_jsonapi_func=listdenyips" \
  'IP block inventory'
while IFS= read -r address; do
  if [[ -z "${address}" ]]; then
    continue
  fi
  uapi_post \
    'BlockIP' \
    'remove_ip' \
    "Delete test IP block ${address}" \
    "ip=${address}"
  deleted_ip_blocks=$((deleted_ip_blocks + 1))
done < <(
  jq -r \
    '.cpanelresult.data[]
      | select(
          .ip == "198.51.100.253"
          or .ip == "198.51.100.254"
          or .ip == "198.51.100.240-198.51.100.242"
          or .ip == "198.51.100.240/31"
          or .ip == "198.51.100.242"
          or .ip == "203.0.113.248/30"
          or (.ip | startswith("2001:0db8:ffff:"))
        )
      | .ip' \
    "${response_file}"
)

get_request 'execute/DomainInfo/list_domains' 'DNS zone inventory'
dns_zones="$(
  jq -r \
    '[.data.main_domain] + (.data.addon_domains // []) | .[]' \
    "${response_file}"
)"
while IFS= read -r zone; do
  if [[ -z "${zone}" ]]; then
    continue
  fi

  while true; do
    get_request "execute/DNS/parse_zone?zone=${zone}" "DNS record inventory for ${zone}"
    line_index="$(
      jq -r \
        'first(
          .data[]
          | select(.record_type != null)
          | select(
              (try (.dname_b64 | @base64d) catch "")
              | startswith("tfcpaneldns")
            )
          | .line_index
        ) // empty' \
        "${response_file}"
    )"
    if [[ -z "${line_index}" ]]; then
      break
    fi

    serial="$(
      jq -r \
        '.data[]
          | select(.record_type == "SOA")
          | .data_b64[2]
          | @base64d' \
        "${response_file}"
    )"
    if [[ -z "${serial}" ]]; then
      printf 'DNS zone %s does not contain a readable SOA serial\n' "${zone}" >&2
      exit 1
    fi

    mutation_label="Delete test DNS record at line ${line_index} in ${zone}"
    uapi_post \
      'DNS' \
      'mass_edit_zone' \
      "${mutation_label}" \
      "zone=${zone}" \
      "serial=${serial}" \
      "remove=${line_index}"
    deleted_dns_records=$((deleted_dns_records + 1))
  done
done <<<"${dns_zones}"

get_request \
  "json-api/cpanel?cpanel_jsonapi_user=${CPANEL_USERNAME}&cpanel_jsonapi_apiversion=2&cpanel_jsonapi_module=Park&cpanel_jsonapi_func=listparkeddomains" \
  'Domain alias inventory'
while IFS= read -r domain; do
  if [[ -z "${domain}" ]]; then
    continue
  fi
  api2_post \
    'Park' \
    'unpark' \
    "Delete test domain alias ${domain}" \
    "domain=${domain}"
  deleted_domain_aliases=$((deleted_domain_aliases + 1))
done < <(
  jq -r \
    '.cpanelresult.data[].domain | select(startswith("tfcpanelalias"))' \
    "${response_file}"
)

get_request \
  'execute/ModSecurity/list_domains' \
  'ModSecurity domain inventory'
while IFS= read -r domain; do
  if [[ -z "${domain}" ]]; then
    continue
  fi
  uapi_post \
    'ModSecurity' \
    'enable_domains' \
    "Enable test ModSecurity domain ${domain}" \
    "domains=${domain}"
  if ! jq -e --arg domain "${domain}" \
    '(.data | type) == "array"
      and (.data | length) > 0
      and all(.data[]; (.exception // "") == "" and .enabled == 1)
      and any(.data[]; .domain == $domain)' \
    "${response_file}" >/dev/null; then
    printf 'Enable test ModSecurity domain %s failed: %s\n' \
      "${domain}" \
      "$(jq -c '.data' "${response_file}")" >&2
    exit 1
  fi
  enabled_modsecurity_domains=$((enabled_modsecurity_domains + 1))
done < <(
  jq -r \
    '.data[]
      | select(.enabled == 0)
      | .domain
      | select(startswith("tfcpanelsubmodsecurity"))' \
    "${response_file}"
)

get_request \
  "json-api/cpanel?cpanel_jsonapi_user=${CPANEL_USERNAME}&cpanel_jsonapi_apiversion=2&cpanel_jsonapi_module=SubDomain&cpanel_jsonapi_func=listsubdomains" \
  'Subdomain inventory'
test_subdomains="$(
  jq -r \
    '.cpanelresult.data[].domain | select(startswith("tfcpanelsub"))' \
    "${response_file}"
)"
while IFS= read -r domain; do
  if [[ -z "${domain}" ]]; then
    continue
  fi
  api2_post \
    'SubDomain' \
    'delsubdomain' \
    "Delete test subdomain ${domain}" \
    "domain=${domain}"
  deleted_subdomains=$((deleted_subdomains + 1))
done <<<"${test_subdomains}"

get_request \
  "json-api/cpanel?cpanel_jsonapi_user=${CPANEL_USERNAME}&cpanel_jsonapi_apiversion=2&cpanel_jsonapi_module=AddonDomain&cpanel_jsonapi_func=listaddondomains" \
  'Addon domain inventory'
test_addon_domains="$(
  jq -r \
    '.cpanelresult.data[]
      | select(.domain | startswith("tfcpaneladdon"))
      | [.domain, .domainkey]
      | @tsv' \
    "${response_file}"
)"
while IFS=$'\t' read -r domain domain_key; do
  if [[ -z "${domain}" || -z "${domain_key}" ]]; then
    continue
  fi
  api2_post \
    'AddonDomain' \
    'deladdondomain' \
    "Delete test addon domain ${domain}" \
    "domain=${domain}" \
    "subdomain=${domain_key}"
  deleted_addon_domains=$((deleted_addon_domains + 1))
done <<<"${test_addon_domains}"

get_request \
  'execute/Fileman/list_files?dir=public_html&show_hidden=1&limit=1000' \
  'Test domain directory inventory'
test_domain_directories="$(
  jq -r \
    '.data[]
      | select(.type == "dir")
      | .file
      | select(startswith("tfcpanel-"))' \
    "${response_file}"
)"
while IFS= read -r directory; do
  if [[ -z "${directory}" ]]; then
    continue
  fi
  api2_post \
    'Fileman' \
    'fileop' \
    "Delete test domain directory public_html/${directory}" \
    'op=unlink' \
    "sourcefiles=public_html/${directory}" \
    'doubledecode=0'
  deleted_domain_directories=$((deleted_domain_directories + 1))
done <<<"${test_domain_directories}"

get_request \
  'execute/Fileman/list_files?dir=&show_hidden=1&limit=1000' \
  'cPanel home directory inventory'
test_git_repository_directories="$(
  jq -r \
    '.data[]
      | select(.type == "dir")
      | .file
      | select(
          startswith("tfcpanel-git-")
          or startswith(".terraform-cpanel-git-delete-")
        )' \
    "${response_file}"
)"
home_has_trash="$(
  jq -r \
    '[.data[] | select(.type == "dir" and .file == ".trash")] | length' \
    "${response_file}"
)"
if [[ "${home_has_trash}" != "0" ]]; then
  get_request \
    'execute/Fileman/list_files?dir=.trash&show_hidden=1&limit=1000' \
    'cPanel trash inventory'
  test_git_repository_trash_entries="$(
    jq -r \
      '.data[]
        | select(.type == "dir")
        | .file
        | select(
            startswith("tfcpanel-git-")
            or startswith(".terraform-cpanel-git-delete-")
          )' \
      "${response_file}"
  )"
  while IFS= read -r trash_entry; do
    if [[ -z "${trash_entry}" ]]; then
      continue
    fi
    uapi_post \
      'Fileman' \
      'empty_trash' \
      "Permanently delete stale test Git trash entry ${trash_entry}" \
      "only_these_files=${trash_entry}"
    deleted_git_repository_trash_entries=$((deleted_git_repository_trash_entries + 1))
  done <<<"${test_git_repository_trash_entries}"
fi
while IFS= read -r directory; do
  if [[ -z "${directory}" ]]; then
    continue
  fi
  purge_test_git_directory "${directory}"
  deleted_git_repository_directories=$((deleted_git_repository_directories + 1))
done <<<"${test_git_repository_directories}"

get_request \
  'execute/Fileman/list_files?dir=&show_hidden=1&limit=1000' \
  'cPanel home directory inventory after Git cleanup'
if jq -e \
  '.data[] | select(.type == "dir" and .file == ".htpasswds")' \
  "${response_file}" >/dev/null; then
  get_request \
    'execute/Fileman/list_files?dir=.htpasswds&show_hidden=1&limit=1000' \
    'Directory Privacy password root inventory'
  if jq -e \
    '.data[] | select(.type == "dir" and .file == "public_html")' \
    "${response_file}" >/dev/null; then
    get_request \
      'execute/Fileman/list_files?dir=.htpasswds/public_html&show_hidden=1&limit=1000' \
      'Directory Privacy test password directory inventory'
    test_directory_privacy_password_directories="$(
      jq -r \
        '.data[]
          | select(.type == "dir")
          | .file
          | select(startswith("tfcpanel-privacy-"))' \
        "${response_file}"
    )"
    while IFS= read -r directory; do
      if [[ -z "${directory}" ]]; then
        continue
      fi
      api2_post \
        'Fileman' \
        'fileop' \
        "Delete Directory Privacy test password directory ${directory}" \
        'op=unlink' \
        "sourcefiles=.htpasswds/public_html/${directory}" \
        'doubledecode=0'
      deleted_directory_privacy_password_directories=$((deleted_directory_privacy_password_directories + 1))
    done <<<"${test_directory_privacy_password_directories}"
  fi
fi

get_request \
  "json-api/cpanel?cpanel_jsonapi_user=${CPANEL_USERNAME}&cpanel_jsonapi_apiversion=2&cpanel_jsonapi_module=Cron&cpanel_jsonapi_func=fetchcron" \
  'Cron inventory'
while IFS= read -r linekey; do
  if [[ -z "${linekey}" ]]; then
    continue
  fi
  remove_cron_line "${linekey}" "Delete test cron line ${linekey}"
  deleted_cron_lines=$((deleted_cron_lines + 1))
done < <(
  jq -r \
    '.cpanelresult.data[]
      | select(.type == "command")
      | select((.command // "") | contains("# terraform-provider-cpanel-"))
      | .linekey' \
    "${response_file}"
)

get_request \
  "json-api/cpanel?cpanel_jsonapi_user=${CPANEL_USERNAME}&cpanel_jsonapi_apiversion=2&cpanel_jsonapi_module=Cron&cpanel_jsonapi_func=fetchcron" \
  'Cron inventory after test cleanup'
cron_command_count="$(
  jq -r '[.cpanelresult.data[] | select(.type == "command")] | length' "${response_file}"
)"

if [[ "${cron_command_count}" == "0" ]]; then
  while IFS= read -r linekey; do
    if [[ -z "${linekey}" ]]; then
      continue
    fi
    remove_cron_line "${linekey}" "Delete generated cron variable ${linekey}"
    deleted_cron_lines=$((deleted_cron_lines + 1))
  done < <(
    jq -r \
      '.cpanelresult.data[]
        | select(.type == "variable")
        | select(
            (.key == "MAILTO" and (.value // "") == "")
            or (.key == "SHELL" and (.value // "") == "/bin/bash")
          )
        | .linekey' \
      "${response_file}"
  )
fi

printf 'cPanel test cleanup passed\n'
printf '  API tokens revoked: %d\n' "${deleted_api_tokens}"
printf '  Dynamic DNS domains deleted: %d\n' \
  "${deleted_dynamic_dns_domains}"
printf '  HTTP redirects deleted: %d\n' "${deleted_redirects}"
printf '  custom MIME types deleted: %d\n' "${deleted_mime_types}"
printf '  Apache handlers deleted: %d\n' "${deleted_apache_handlers}"
printf '  Passenger applications unregistered: %d\n' \
  "${deleted_passenger_applications}"
printf '  stored SSL CSRs deleted: %d\n' "${deleted_ssl_csrs}"
printf '  stored SSL certificates deleted: %d\n' \
  "${deleted_ssl_certificates}"
printf '  Git repositories deleted: %d\n' "${deleted_git_repositories}"
printf '  Git repository directories deleted: %d\n' \
  "${deleted_git_repository_directories}"
printf '  stale Git trash entries deleted: %d\n' \
  "${deleted_git_repository_trash_entries}"
printf '  PostgreSQL databases deleted: %d\n' "${deleted_databases}"
printf '  PostgreSQL users deleted: %d\n' "${deleted_users}"
printf '  MySQL databases deleted: %d\n' "${deleted_mysql_databases}"
printf '  MySQL users deleted: %d\n' "${deleted_mysql_users}"
printf '  remote MySQL hosts deleted: %d\n' "${deleted_mysql_remote_hosts}"
printf '  email account restrictions unsuspended: %d\n' \
  "${unsuspended_email_restrictions}"
printf '  calendar delegates deleted: %d\n' \
  "${deleted_calendar_delegates}"
printf '  email filters deleted: %d\n' "${deleted_email_filters}"
printf '  email accounts deleted: %d\n' "${deleted_email_accounts}"
printf '  email forwarders deleted: %d\n' "${deleted_email_forwarders}"
printf '  email domain forwarders deleted: %d\n' \
  "${deleted_email_domain_forwarders}"
printf '  email autoresponders deleted: %d\n' \
  "${deleted_email_auto_responders}"
printf '  FTP accounts deleted: %d\n' "${deleted_ftp_accounts}"
printf '  IP blocks deleted: %d\n' "${deleted_ip_blocks}"
printf '  DNS records deleted: %d\n' "${deleted_dns_records}"
printf '  addon domains deleted: %d\n' "${deleted_addon_domains}"
printf '  domain aliases deleted: %d\n' "${deleted_domain_aliases}"
printf '  test ModSecurity domains enabled: %d\n' \
  "${enabled_modsecurity_domains}"
printf '  subdomains deleted: %d\n' "${deleted_subdomains}"
printf '  test domain directories deleted: %d\n' "${deleted_domain_directories}"
printf '  Directory Privacy test password directories deleted: %d\n' \
  "${deleted_directory_privacy_password_directories}"
printf '  cron lines deleted: %d\n' "${deleted_cron_lines}"
