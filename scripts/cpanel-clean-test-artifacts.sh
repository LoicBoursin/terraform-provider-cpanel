#!/usr/bin/env bash

set -euo pipefail

script_directory="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
env_file="${CPANEL_ENV_FILE:-${HOME}/.config/terraform-provider-cpanel/acceptance.env}"
artifact_manifest="${CPANEL_TEST_ARTIFACT_MANIFEST:-${env_file%.env}-artifacts.txt}"
remote_artifact_manifest="${CPANEL_TEST_REMOTE_ARTIFACT_MANIFEST:-.terraform-provider-cpanel-acceptance-artifacts}"

source "${script_directory}/cpanel-common.sh"

if [[ -f "${env_file}" ]]; then
  cpanel_load_environment_file \
    "${env_file}" \
    'Acceptance environment file' \
    CPANEL_HOST \
    CPANEL_USERNAME \
    CPANEL_API_TOKEN \
    CPANEL_API_TOKEN_NAME \
    CPANEL_EXPECTED_TEST_HOST \
    CPANEL_EXPECTED_TEST_USERNAME \
    CPANEL_EXPECTED_VERSION \
    CPANEL_ACCEPT_DESTRUCTIVE \
    CPANEL_TEST_SSL_KEY_ID
fi

for variable in \
  CPANEL_HOST \
  CPANEL_USERNAME \
  CPANEL_API_TOKEN \
  CPANEL_API_TOKEN_NAME; do
  if [[ -z "${!variable:-}" ]]; then
    printf 'Missing required environment variable: %s\n' "${variable}" >&2
    exit 1
  fi
done

"${script_directory}/cpanel-verify-test-account.sh"

if [[ "${CPANEL_API_TOKEN_NAME}" == tfcpaneltoken* ]]; then
  printf '%s\n' \
    'Refusing cleanup: the active provider token uses the reserved tfcpaneltoken prefix.' >&2
  exit 1
fi
if [[ "${remote_artifact_manifest}" != ".terraform-provider-cpanel-acceptance-artifacts" ]]; then
  printf 'Invalid remote acceptance artifact manifest name: %s\n' \
    "${remote_artifact_manifest}" >&2
  exit 1
fi

for command in curl grep jq; do
  if ! command -v "${command}" >/dev/null 2>&1; then
    printf 'Missing required command: %s\n' "${command}" >&2
    exit 1
  fi
done

if command -v sha256sum >/dev/null 2>&1; then
  sha256_utility='sha256sum'
elif command -v shasum >/dev/null 2>&1; then
  sha256_utility='shasum'
else
  printf 'Missing required command: sha256sum or shasum\n' >&2
  exit 1
fi

host="${CPANEL_HOST%/}"
response_file="$(mktemp)"

cleanup() {
  rm -f "${response_file}"
}

trap cleanup EXIT

artifact_registered() {
  local value="$1"

  cpanel_artifact_registered "${artifact_manifest}" "${value}"
}

require_registered_artifact() {
  local value="$1"

  if ! artifact_registered "${value}"; then
    printf 'Refusing cleanup of unregistered test artifact: %s\n' \
      "${value}" >&2
    exit 1
  fi
}

require_registered_dns_record() {
  local record_identity="$1"

  if ! cpanel_dns_record_artifact_registered \
    "${artifact_manifest}" \
    "${record_identity}"; then
    printf 'Refusing cleanup of unregistered test DNS record: %s\n' \
      "$(jq -r '.name' <<<"${record_identity}")" >&2
    exit 1
  fi
}

get_request() {
  local endpoint="$1"
  local label="$2"
  local http_code
  local status

  http_code="$(
    cpanel_curl \
      --silent \
      --show-error \
      --max-time 90 \
      --output "${response_file}" \
      --write-out '%{http_code}' \
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
    --header 'Content-Type: application/x-www-form-urlencoded'
  )

  shift 3
  for parameter in "$@"; do
    curl_arguments+=(--data-urlencode "${parameter}")
  done

  http_code="$(cpanel_curl "${curl_arguments[@]}" "${host}/execute/${module}/${function}")"

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

  http_code="$(cpanel_curl "${curl_arguments[@]}" "${host}/json-api/cpanel")"

  if [[ "${http_code}" != "200" ]]; then
    printf '%s failed with HTTP %s\n' "${label}" "${http_code}" >&2
    exit 1
  fi

  status="$(jq -r '.cpanelresult.event.result // empty' "${response_file}")"
  if [[ "${status}" != "1" ]] || ! jq -e \
    '
      (.cpanelresult.data // []) as $data
      | ($data | type) == "array"
      and all(
        $data[]?;
        ((has("result") | not) or ((.result | tostring) == "1"))
        and ((has("status") | not) or ((.status | tostring) == "1"))
        and (
          (has("reason") | not)
          or has("result")
          or has("status")
        )
      )
    ' \
    "${response_file}" >/dev/null; then
    printf '%s failed: %s\n' \
      "${label}" \
      "$(jq -c '{cpanelresult}' "${response_file}")" >&2
    exit 1
  fi
}

delete_test_git_repository() {
  local repository_root="$1"
  local attempt
  local http_code
  local status

  for ((attempt = 1; attempt <= 31; attempt++)); do
    http_code="$(
      cpanel_curl \
        --silent \
        --show-error \
        --max-time 90 \
        --request POST \
        --output "${response_file}" \
        --write-out '%{http_code}' \
        --header 'Content-Type: application/x-www-form-urlencoded' \
        --data-urlencode "repository_root=${repository_root}" \
        "${host}/execute/VersionControl/delete"
    )"

    if [[ "${http_code}" != "200" ]]; then
      printf 'Delete test Git repository failed with HTTP %s\n' \
        "${http_code}" >&2
      exit 1
    fi

    status="$(jq -r '.status // empty' "${response_file}")"
    if [[ "${status}" == "1" ]]; then
      return
    fi
    if ((attempt < 31)) &&
      jq -e \
        '
          [.errors[]?, .messages[]?] as $messages
          | ($messages | length) > 0
          and all(
            $messages[];
            type == "string"
            and endswith(
              " can not be deleted because there are tasks pending."
            )
          )
        ' \
        "${response_file}" >/dev/null; then
      sleep 2
      continue
    fi

    printf 'Delete test Git repository failed: %s\n' \
      "$(jq -c '{errors, messages}' "${response_file}")" >&2
    exit 1
  done
}

sha256_stream() {
  local output

  if [[ "${sha256_utility}" == "sha256sum" ]]; then
    output="$(sha256sum)"
  else
    output="$(shasum -a 256)"
  fi

  printf '%s\n' "${output%% *}"
}

delete_test_filesystem_path() {
  local managed_path="$1"
  local label="$2"
  local parent_directory="${managed_path%/*}"
  local entry_name="${managed_path##*/}"

  if [[ "${parent_directory}" == "${managed_path}" ]]; then
    parent_directory=""
  fi

  api2_post \
    'Fileman' \
    'fileop' \
    "${label}" \
    'op=unlink' \
    "sourcefiles=${managed_path}" \
    'doubledecode=0'
  if ! jq -e \
    '
      (.cpanelresult.data | type) == "array"
      and (.cpanelresult.data | length) == 1
      and ((.cpanelresult.data[0].result | tostring) == "1")
      and (.cpanelresult.data[0].dest == null)
    ' \
    "${response_file}" >/dev/null; then
    printf '%s failed: %s\n' \
      "${label}" \
      "$(jq -c '.cpanelresult.data' "${response_file}")" >&2
    exit 1
  fi

  get_request \
    "execute/Fileman/list_files?dir=${parent_directory}&show_hidden=1&limit=1000" \
    "Verify ${label}"
  if jq -e \
    --arg name "${entry_name}" \
    '.data[] | select(.file == $name)' \
    "${response_file}" >/dev/null; then
    printf '%s still exists after deletion: %s\n' \
      "${label}" \
      "${managed_path}" >&2
    exit 1
  fi
}

delete_safe_test_ftp_home_directory() {
  local home_directory="$1"

  get_request \
    "execute/Fileman/list_files?dir=${home_directory}&show_hidden=1&limit=1000" \
    "Verify FTP home directory ${home_directory} before deletion"
  if ! cpanel_ftp_home_inventory_is_safe_to_delete "${response_file}"; then
    printf 'Refusing to delete FTP home directory with user content: %s\n' \
      "${home_directory}" >&2
    exit 1
  fi
  if jq -e '.data | length == 1' "${response_file}" >/dev/null; then
    get_request \
      "execute/Fileman/get_file_content?dir=${home_directory}&file=.ftpquota&from_charset=UTF-8&to_charset=UTF-8&update_html_document_encoding=0" \
      "Verify FTP quota file ${home_directory}/.ftpquota before deletion"
    if ! cpanel_ftp_quota_file_is_empty "${response_file}"; then
      printf 'Refusing to delete FTP home directory with a non-empty quota file: %s\n' \
        "${home_directory}" >&2
      exit 1
    fi
  fi
  delete_test_filesystem_path \
    "${home_directory}" \
    "Delete safe test FTP home directory ${home_directory}"
  deleted_ftp_home_directories=$((deleted_ftp_home_directories + 1))
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
  local expected_job="$1"
  local label="$2"
  local command_number
  local linekey
  local observed_job

  linekey="$(jq -r '.linekey' <<<"${expected_job}")"
  command_number="$(jq -r '.commandnumber' <<<"${expected_job}")"

  if [[ ! "${command_number}" =~ ^[1-9][0-9]*$ ]]; then
    printf 'Refusing to delete cron entry %s with invalid command number %s\n' \
      "${linekey}" \
      "${command_number}" >&2
    exit 1
  fi

  get_request \
    "json-api/cpanel?cpanel_jsonapi_user=${CPANEL_USERNAME}&cpanel_jsonapi_apiversion=2&cpanel_jsonapi_module=Cron&cpanel_jsonapi_func=fetchcron" \
    "Revalidate cron entry ${linekey} before deletion"
  observed_job="$(
    jq -c \
      --arg linekey "${linekey}" \
      '
        [
          .cpanelresult.data[]
          | select(
              .type == "command"
              and (.linekey | tostring) == $linekey
            )
          | {
              linekey: (.linekey | tostring),
              line: (.line | tonumber),
              commandnumber: (.commandnumber | tonumber),
              command: (.command | tostring),
              minute: (.minute | tostring),
              hour: (.hour | tostring),
              day: (.day | tostring),
              weekday: (.weekday | tostring),
              month: (.month | tostring)
            }
        ]
        | if length == 1 then .[0] else null end
      ' \
      "${response_file}"
  )"
  if [[ "${observed_job}" != "${expected_job}" ]]; then
    printf 'Refusing to delete changed cron entry: %s\n' "${linekey}" >&2
    exit 1
  fi

  api2_post \
    'Cron' \
    'remove_line' \
    "${label}" \
    "line=${command_number}"

  get_request \
    "json-api/cpanel?cpanel_jsonapi_user=${CPANEL_USERNAME}&cpanel_jsonapi_apiversion=2&cpanel_jsonapi_module=Cron&cpanel_jsonapi_func=fetchcron" \
    "Verify cron deletion ${linekey}"
  if jq -e \
    --arg linekey "${linekey}" \
    '.cpanelresult.data[] | select((.linekey | tostring) == $linekey)' \
    "${response_file}" >/dev/null; then
    printf 'Cron entry still exists after deletion: %s\n' "${linekey}" >&2
    exit 1
  fi
}

recover_remote_artifact_manifest() {
  local content
  local manifest_directory
  local merged_manifest
  local remote_manifest

  get_request \
    'execute/Fileman/list_files?dir=&show_hidden=1&limit=1000' \
    'Remote acceptance artifact manifest inventory'
  if ! jq -e \
    --arg file "${remote_artifact_manifest}" \
    '.data[] | select(.type == "file" and .file == $file)' \
    "${response_file}" >/dev/null; then
    return
  fi

  get_request \
    "execute/Fileman/get_file_content?dir=&file=${remote_artifact_manifest}&from_charset=UTF-8&to_charset=UTF-8&update_html_document_encoding=0" \
    'Read remote acceptance artifact manifest'
  if ! jq -e \
    '.data.content | type == "string" and utf8bytelength <= 1048576' \
    "${response_file}" >/dev/null; then
    printf 'Remote acceptance artifact manifest content is invalid\n' >&2
    exit 1
  fi
  content="$(jq -r '.data.content' "${response_file}")"

  manifest_directory="$(dirname "${artifact_manifest}")"
  umask 077
  mkdir -p "${manifest_directory}"
  merged_manifest="$(mktemp "${manifest_directory}/acceptance-artifacts.XXXXXX")"
  if [[ -f "${artifact_manifest}" ]]; then
    cpanel_require_private_file \
      "${artifact_manifest}" \
      'Acceptance artifact manifest'
    cpanel_validate_artifact_manifest \
      "${artifact_manifest}" \
      'Local acceptance artifact manifest'
    cat "${artifact_manifest}" >>"${merged_manifest}"
  fi
  if [[ -n "${content}" ]]; then
    remote_manifest="$(mktemp "${manifest_directory}/remote-acceptance-artifacts.XXXXXX")"
    printf '%s\n' "${content}" >"${remote_manifest}"
    chmod 0600 "${remote_manifest}"
    cpanel_validate_artifact_manifest \
      "${remote_manifest}" \
      'Remote acceptance artifact manifest'
    cat "${remote_manifest}" >>"${merged_manifest}"
    rm -f "${remote_manifest}"
  fi
  awk 'length($0) > 0 && !seen[$0]++' \
    "${merged_manifest}" >"${merged_manifest}.deduplicated"
  chmod 0600 "${merged_manifest}.deduplicated"
  cpanel_validate_artifact_manifest \
    "${merged_manifest}.deduplicated" \
    'Merged acceptance artifact manifest'
  mv "${merged_manifest}.deduplicated" "${artifact_manifest}"
  rm -f "${merged_manifest}"
}

recover_remote_artifact_manifest

test_prefix="${CPANEL_USERNAME}_tf"
deleted_api_tokens=0
deleted_dynamic_dns_domains=0
deleted_redirects=0
deleted_mime_types=0
deleted_apache_handlers=0
deleted_passenger_applications=0
deleted_ssl_certificates=0
deleted_ssl_csrs=0
deleted_ssl_keys=0
deleted_gpg_public_keys=0
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
deleted_account_email_filters=0
deleted_email_filters=0
deleted_email_mailing_lists=0
unsuspended_email_restrictions=0
reset_boxtrapper_accounts=0
deleted_email_forwarders=0
deleted_email_domain_forwarders=0
deleted_email_auto_responders=0
reset_email_routings=0
deleted_ftp_accounts=0
deleted_ftp_home_directories=0
deleted_ip_blocks=0
deleted_dns_records=0
deleted_addon_domains=0
deleted_domain_aliases=0
enabled_modsecurity_domains=0
deleted_subdomains=0
deleted_domain_directories=0
deleted_filesystem_text_files=0
deleted_filesystem_text_file_markers=0
deleted_directory_privacy_password_directories=0
deleted_cron_lines=0

get_request 'execute/Email/list_mxs' 'Email routing inventory'
test_non_auto_routing_domains="$(
  jq -r \
    '
      .data[]
      | select(
          (.domain | startswith("tfcpanelsubemailrouting"))
          and .mxcheck != "auto"
        )
      | .domain
    ' \
    "${response_file}"
)"
while IFS= read -r domain; do
  if [[ -z "${domain}" ]]; then
    continue
  fi
  require_registered_artifact "${domain}"
  uapi_post \
    'Email' \
    'set_always_accept' \
    "Reset email routing for test subdomain ${domain}" \
    "domain=${domain}" \
    'mxcheck=auto'
  get_request \
    'execute/Email/list_mxs' \
    "Verify email routing reset for ${domain}"
  if ! jq -e \
    --arg domain "${domain}" \
    '
      [
        .data[]
        | select(
            .domain == $domain
            and .mxcheck == "auto"
          )
      ]
      | length == 1
    ' \
    "${response_file}" >/dev/null; then
    printf 'Email routing reset failed for test subdomain %s\n' \
      "${domain}" >&2
    exit 1
  fi
  reset_email_routings=$((reset_email_routings + 1))
done <<<"${test_non_auto_routing_domains}"

deleted_api_tokens="$(cpanel_cleanup_test_api_tokens \
  "${artifact_manifest}" \
  "${CPANEL_API_TOKEN_NAME}" \
  "${response_file}")"

get_request \
  'execute/PassengerApps/list_applications' \
  'Passenger application inventory'
while IFS= read -r application_name; do
  if [[ -z "${application_name}" ]]; then
    continue
  fi
  require_registered_artifact "${application_name}"
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
  require_registered_artifact "${friendly_name}"

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

cleanup_test_ssl_certificates() {
  local require_empty="${1:-0}"
  local cleanup_artifact

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
            (
              (.friendly_name? | type) == "string"
              and (.friendly_name | startswith("tfcpanelsslcert"))
            )
            or (
              (
                (.id? | type) == "string"
                or (.id? | type) == "number"
              )
              and (.id | tostring | startswith("tfcpanel"))
            )
            or (
              (.domains? | type) == "array"
              and any(
                .domains[];
                (type == "string" and startswith("tfcpanel"))
              )
            )
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
    if ! cleanup_artifact="$(
      cpanel_ssl_certificate_cleanup_artifact \
        "${response_file}" \
        "${certificate_id}"
    )"; then
      printf 'Refusing to attribute test SSL certificate safely: %s\n' \
        "${friendly_name}" >&2
      exit 1
    fi
    require_registered_artifact "${cleanup_artifact}"
    if jq -e \
      --arg id "${certificate_id}" \
      '
        [
          .data[]
          | select(
              ((.id? | type) == "string" or (.id? | type) == "number")
              and ((.id | tostring) == $id)
            )
        ]
        | length == 1
        and (
          .[0].domain_is_configured? == 1
          or .[0].domain_is_configured? == "1"
          or .[0].domain_is_configured? == true
          or .[0].domain_is_configured? == "true"
        )
      ' \
      "${response_file}" >/dev/null; then
      continue
    fi
    if ! jq -e \
      --arg id "${certificate_id}" \
      --arg cleanup_artifact "${cleanup_artifact}" \
      --arg friendly_name "${friendly_name}" \
      '
        def automatic_test_certificate:
          .friendly_name
          | try capture(
              "^Cert for \u201c(?<domain>tfcpanel(?:sub|addon)[a-z0-9.-]+)\u201d$"
            )
            catch null;
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
            (
              ($matches[0].friendly_name? | type) == "string"
              and ($matches[0].friendly_name == $cleanup_artifact)
            )
            or (
              (($matches[0] | automatic_test_certificate) | type) == "object"
              and (
                ($matches[0] | automatic_test_certificate).domain
                == $cleanup_artifact
              )
            )
            or (($matches[0].id | tostring) == $cleanup_artifact)
          )
          and (
            (
              ($matches[0].friendly_name? | type) == "string"
              and (
                $matches[0].friendly_name
                | startswith("tfcpanelsslcert")
              )
            )
            or (
              (
                ($matches[0].id? | type) == "string"
                or ($matches[0].id? | type) == "number"
              )
              and (
                $matches[0].id
                | tostring
                | startswith("tfcpanel")
              )
            )
            or (
              ($matches[0].domains? | type) == "array"
              and any(
                $matches[0].domains[];
                (type == "string" and startswith("tfcpanel"))
              )
            )
          )
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
    --arg require_empty "${require_empty}" \
    '
      .data[]
      | select(
          (
            (.friendly_name? | type) == "string"
            and (.friendly_name | startswith("tfcpanelsslcert"))
          )
          or (
            (
              (.id? | type) == "string"
              or (.id? | type) == "number"
            )
            and (.id | tostring | startswith("tfcpanel"))
          )
          or (
            (.domains? | type) == "array"
            and any(
              .domains[];
              (type == "string" and startswith("tfcpanel"))
            )
          )
        )
      | select(
          $require_empty == "1"
          or (
            .domain_is_configured? == 0
            or .domain_is_configured? == "0"
            or .domain_is_configured? == false
            or .domain_is_configured? == "false"
          )
        )
    ' \
    "${response_file}" >/dev/null; then
    printf 'Test SSL certificate still exists after cleanup\n' >&2
    exit 1
  fi
}

cleanup_test_ssl_keys() {
  local configured_domain
  local configured_domains
  local domain
  local domain_is_configured
  local friendly_name
  local key_id
  local key_modulus
  local main_domain
  local ssl_key_candidates

  get_request \
    'execute/DomainInfo/list_domains' \
    'Configured domain inventory before SSL key cleanup'
  if ! jq -e \
    '
      ((.warnings // []) | length) == 0
      and (.data | type) == "object"
      and (.data.main_domain | type) == "string"
      and (.data.main_domain | length) > 0
      and (.data.sub_domains | type) == "array"
      and all(.data.sub_domains[]; type == "string" and length > 0)
      and (.data.addon_domains | type) == "array"
      and all(.data.addon_domains[]; type == "string" and length > 0)
      and (.data.parked_domains | type) == "array"
      and all(.data.parked_domains[]; type == "string" and length > 0)
    ' \
    "${response_file}" >/dev/null; then
    printf 'Configured domain inventory is incomplete before SSL key cleanup\n' >&2
    exit 1
  fi
  main_domain="$(jq -r '.data.main_domain' "${response_file}")"
  configured_domains="$(
    jq -r \
      '
        (
          [.data.main_domain]
          + .data.sub_domains
          + .data.addon_domains
          + .data.parked_domains
        )[]
      ' \
      "${response_file}"
  )"

  get_request \
    "json-api/cpanel?cpanel_jsonapi_user=${CPANEL_USERNAME}&cpanel_jsonapi_apiversion=2&cpanel_jsonapi_module=SubDomain&cpanel_jsonapi_func=listsubdomains" \
    'Subdomain inventory before SSL key cleanup'
  if ! jq -e \
    '
      (.cpanelresult.data | type) == "array"
      and all(
        .cpanelresult.data[];
        (.domain | type) == "string"
        and (.domain | length) > 0
      )
    ' \
    "${response_file}" >/dev/null; then
    printf 'Subdomain inventory is incomplete before SSL key cleanup\n' >&2
    exit 1
  fi
  configured_domains="$(
    printf '%s\n%s\n' \
      "${configured_domains}" \
      "$(jq -r '.cpanelresult.data[].domain' "${response_file}")"
  )"

  get_request \
    "json-api/cpanel?cpanel_jsonapi_user=${CPANEL_USERNAME}&cpanel_jsonapi_apiversion=2&cpanel_jsonapi_module=AddonDomain&cpanel_jsonapi_func=listaddondomains" \
    'Addon domain inventory before SSL key cleanup'
  if ! jq -e \
    '
      (.cpanelresult.data | type) == "array"
      and all(
        .cpanelresult.data[];
        (.domain | type) == "string"
        and (.domain | length) > 0
        and (.fullsubdomain | type) == "string"
        and (.fullsubdomain | length) > 0
      )
    ' \
    "${response_file}" >/dev/null; then
    printf 'Addon domain inventory is incomplete before SSL key cleanup\n' >&2
    exit 1
  fi
  configured_domains="$(
    printf '%s\n%s\n' \
      "${configured_domains}" \
      "$(
        jq -r \
          '.cpanelresult.data[] | .domain, .fullsubdomain' \
          "${response_file}"
      )"
  )"

  get_request \
    "json-api/cpanel?cpanel_jsonapi_user=${CPANEL_USERNAME}&cpanel_jsonapi_apiversion=2&cpanel_jsonapi_module=Park&cpanel_jsonapi_func=listparkeddomains" \
    'Domain alias inventory before SSL key cleanup'
  if ! jq -e \
    '
      (.cpanelresult.data | type) == "array"
      and all(
        .cpanelresult.data[];
        (.domain | type) == "string"
        and (.domain | length) > 0
      )
    ' \
    "${response_file}" >/dev/null; then
    printf 'Domain alias inventory is incomplete before SSL key cleanup\n' >&2
    exit 1
  fi
  configured_domains="$(
    printf '%s\n%s\n' \
      "${configured_domains}" \
      "$(jq -r '.cpanelresult.data[].domain' "${response_file}")"
  )"

  get_request 'execute/SSL/list_keys' 'stored SSL key inventory'
  if ! jq -e \
    --arg main_domain "${main_domain}" \
    '
      def test_key_match:
        .friendly_name
        | try capture(
            "^Key for \u201c(?<domain>tfcpanel(?:sub|addon)[a-z0-9.-]+)\u201d$"
          )
          catch null;
      (.data | type) == "array"
      and (
        [.data[] | .id | tostring]
        | length == (unique | length)
      )
      and all(
        .data[];
        ((.id? | type) == "string" or (.id? | type) == "number")
        and ((.id | tostring | length) > 0)
        and ((.friendly_name? | type) == "string")
        and (
          (
            .friendly_name
            | startswith("Key for \u201ctfcpanel")
            | not
          )
          or (
            (test_key_match | type) == "object"
            and (
              (test_key_match).domain
              | endswith("." + $main_domain)
            )
            and .key_algorithm? == "rsaEncryption"
            and ((.modulus_length? | tostring) == "2048")
            and (.modulus? | type) == "string"
            and (.modulus | test("^[0-9a-fA-F]{512}$"))
          )
        )
      )
    ' \
    "${response_file}" >/dev/null; then
    printf 'Refusing to clean ambiguous test SSL key inventory\n' >&2
    exit 1
  fi
  ssl_key_candidates="$(
    jq -r \
      --arg main_domain "${main_domain}" \
      '
        def test_key_match:
          .friendly_name
          | try capture(
              "^Key for \u201c(?<domain>tfcpanel(?:sub|addon)[a-z0-9.-]+)\u201d$"
            )
            catch null;
        .data[]
        | (test_key_match) as $match
        | select(
            ($match | type) == "object"
            and ($match.domain | endswith("." + $main_domain))
          )
        | [
            (.id | tostring),
            .friendly_name,
            $match.domain,
            .modulus
          ]
        | @tsv
      ' \
      "${response_file}"
  )"

  while IFS=$'\t' read -r key_id friendly_name domain key_modulus; do
    if [[ -z "${key_id}" ]]; then
      continue
    fi
    require_registered_artifact "${domain}"

    domain_is_configured=0
    while IFS= read -r configured_domain; do
      if [[
        -n "${configured_domain}"
        && "${configured_domain}" == "${domain}"
      ]]; then
        domain_is_configured=1
        break
      fi
    done <<<"${configured_domains}"
    if [[ "${domain_is_configured}" == "1" ]]; then
      printf 'Refusing to delete SSL key for configured test domain: %s\n' \
        "${domain}" >&2
      exit 1
    fi

    get_request 'execute/SSL/list_certs' \
      "Inspect certificates before deleting ${friendly_name}"
    if ! jq -e \
      '(.data | type) == "array"' \
      "${response_file}" >/dev/null; then
      printf 'Stored SSL certificate inventory is incomplete\n' >&2
      exit 1
    fi
    if jq -e \
      --arg domain "${domain}" \
      --arg modulus "${key_modulus}" \
      '
        .data[]
        | select(
            .modulus? == $modulus
            or .["subject.commonName"]? == $domain
            or (
              (.domains? | type) == "array"
              and any(.domains[]; . == $domain)
            )
          )
      ' \
      "${response_file}" >/dev/null; then
      printf 'Refusing to delete SSL key referenced by a certificate: %s\n' \
        "${friendly_name}" >&2
      exit 1
    fi

    get_request 'execute/SSL/list_csrs' \
      "Inspect CSRs before deleting ${friendly_name}"
    if ! jq -e \
      '(.data | type) == "array"' \
      "${response_file}" >/dev/null; then
      printf 'Stored SSL CSR inventory is incomplete\n' >&2
      exit 1
    fi
    if jq -e \
      --arg domain "${domain}" \
      --arg modulus "${key_modulus}" \
      '
        .data[]
        | select(
            .modulus? == $modulus
            or .commonName? == $domain
            or (
              (.domains? | type) == "array"
              and any(.domains[]; . == $domain)
            )
          )
      ' \
      "${response_file}" >/dev/null; then
      printf 'Refusing to delete SSL key referenced by a CSR: %s\n' \
        "${friendly_name}" >&2
      exit 1
    fi

    get_request 'execute/SSL/installed_hosts' \
      "Inspect installed hosts before deleting ${friendly_name}"
    if ! jq -e \
      '
        (.data | type) == "array"
        and all(
          .data[];
          (.servername | type) == "string"
          and (.domains | type) == "array"
          and (.fqdns | type) == "array"
          and (.certificate | type) == "object"
        )
      ' \
      "${response_file}" >/dev/null; then
      printf 'Installed SSL host inventory is incomplete\n' >&2
      exit 1
    fi
    if jq -e \
      --arg domain "${domain}" \
      --arg modulus "${key_modulus}" \
      '
        .data[]
        | select(
            .servername == $domain
            or .certificate.modulus? == $modulus
            or any(.domains[]?; . == $domain)
            or any(.fqdns[]?; . == $domain)
            or any(.certificate.domains[]?; . == $domain)
          )
      ' \
      "${response_file}" >/dev/null; then
      printf 'Refusing to delete SSL key referenced by an installed host: %s\n' \
        "${friendly_name}" >&2
      exit 1
    fi

    get_request 'execute/SSL/installed_host' \
      "Inspect dedicated SSL host before deleting ${friendly_name}"
    if ! jq -e \
      '
        (.data | type) == "object"
        and (.data.host | type) == "string"
        and (.data.certificate | type) == "object"
      ' \
      "${response_file}" >/dev/null; then
      printf 'Dedicated SSL host inventory is incomplete\n' >&2
      exit 1
    fi
    if jq -e \
      --arg domain "${domain}" \
      --arg modulus "${key_modulus}" \
      '
        .data
        | select(
            .host == $domain
            or .certificate.modulus? == $modulus
            or any(.certificate.domains[]?; . == $domain)
          )
      ' \
      "${response_file}" >/dev/null; then
      printf 'Refusing to delete SSL key referenced by the dedicated host: %s\n' \
        "${friendly_name}" >&2
      exit 1
    fi

    get_request \
      'execute/DomainInfo/list_domains' \
      "Re-read configured domains before deleting ${friendly_name}"
    if ! jq -e \
      --arg domain "${domain}" \
      '
        ((.warnings // []) | length) == 0
        and (.data | type) == "object"
        and (.data.main_domain | type) == "string"
        and (.data.main_domain | length) > 0
        and (.data.sub_domains | type) == "array"
        and all(.data.sub_domains[]; type == "string" and length > 0)
        and (.data.addon_domains | type) == "array"
        and all(.data.addon_domains[]; type == "string" and length > 0)
        and (.data.parked_domains | type) == "array"
        and all(.data.parked_domains[]; type == "string" and length > 0)
        and (
          (
            [.data.main_domain]
            + .data.sub_domains
            + .data.addon_domains
            + .data.parked_domains
          )
          | index($domain)
        ) == null
      ' \
      "${response_file}" >/dev/null; then
      printf 'Refusing to delete SSL key for a newly configured domain: %s\n' \
        "${domain}" >&2
      exit 1
    fi

    get_request 'execute/SSL/list_keys' \
      "Re-read test SSL key before deleting ${friendly_name}"
    if ! jq -e \
      --arg id "${key_id}" \
      --arg friendly_name "${friendly_name}" \
      --arg domain "${domain}" \
      --arg modulus "${key_modulus}" \
      '
        def test_key_match:
          .friendly_name
          | try capture(
              "^Key for \u201c(?<domain>tfcpanel(?:sub|addon)[a-z0-9.-]+)\u201d$"
            )
            catch null;
        (.data | type) == "array"
        and (
          [
            .data[]
            | select((.id | tostring) == $id)
          ] as $matches
          | ($matches | length) == 1
          and ($matches[0].friendly_name == $friendly_name)
          and ($matches[0].modulus == $modulus)
          and (($matches[0] | test_key_match).domain == $domain)
        )
      ' \
      "${response_file}" >/dev/null; then
      printf 'Refusing to delete changed or ambiguous test SSL key: %s\n' \
        "${friendly_name}" >&2
      exit 1
    fi

    uapi_post \
      'SSL' \
      'delete_key' \
      "Delete orphaned test SSL key ${friendly_name}" \
      "id=${key_id}" \
      "friendly_name=${friendly_name}"
    deleted_ssl_keys=$((deleted_ssl_keys + 1))

    get_request 'execute/SSL/list_keys' \
      "Verify deletion of test SSL key ${friendly_name}"
    if jq -e \
      --arg id "${key_id}" \
      '.data[] | select((.id | tostring) == $id)' \
      "${response_file}" >/dev/null; then
      printf 'Test SSL key still exists after deletion: %s\n' \
        "${friendly_name}" >&2
      exit 1
    fi
  done <<<"${ssl_key_candidates}"

  get_request \
    'execute/SSL/list_keys' \
    'stored SSL key inventory after cleanup'
  if ! jq -e \
    '
      (.data | type) == "array"
      and all(
        .data[];
        (
          (.friendly_name? | type) != "string"
          or (
            .friendly_name
            | startswith("Key for \u201ctfcpanel")
            | not
          )
        )
      )
    ' \
    "${response_file}" >/dev/null; then
    printf 'Test SSL key still exists after cleanup\n' >&2
    exit 1
  fi
}

validate_gpg_public_inventory() {
  jq -e \
    '
      (.data | type == "array")
      and all(
        .data[];
        (.algorithm? | type) == "string"
        and (
          (.bits? | type) == "string"
          or (.bits? | type) == "number"
        )
        and ((.bits | tostring) | test("^[1-9][0-9]*$"))
        and (
          (.created? | type) == "string"
          or (.created? | type) == "number"
        )
        and ((.created | tostring) | test("^[0-9]+$"))
        and (.id? | type) == "string"
        and (.id | test("^[0-9A-Fa-f]{16}$"))
        and .type? == "pub"
        and (.user_id? | type) == "string"
        and ((.user_id | length) > 0)
      )
    ' \
    "${response_file}" >/dev/null
}

validate_gpg_secret_inventory() {
  jq -e \
    '
      (.data | type == "array")
      and all(
        .data[];
        (.algorithm? | type) == "string"
        and (
          (.bits? | type) == "string"
          or (.bits? | type) == "number"
        )
        and ((.bits | tostring) | test("^[1-9][0-9]*$"))
        and (
          (.created? | type) == "string"
          or (.created? | type) == "number"
        )
        and ((.created | tostring) | test("^[0-9]+$"))
        and (.id? | type) == "string"
        and (.id | test("^([0-9A-Fa-f]{8}|[0-9A-Fa-f]{16})$"))
        and .type? == "sec"
        and (.user_id? | type) == "string"
        and ((.user_id | length) > 0)
      )
    ' \
    "${response_file}" >/dev/null
}

gpg_secret_inventory_matches_public_id() {
  local public_id="$1"

  jq -e \
    --arg public_id "${public_id}" \
    '
      ($public_id | ascii_upcase) as $normalized_public_id
      |
      any(
        .data[];
        (.id | ascii_upcase) as $secret_id
        | (
            (
              ($secret_id | length) == 8
              or ($secret_id | length) == 16
            )
            and ($normalized_public_id | endswith($secret_id))
          )
      )
    ' \
    "${response_file}" >/dev/null
}

delete_test_gpg_keypair_once() {
  local key_id="$1"
  local curl_status
  local http_code
  local status
  local curl_arguments=(
    --silent
    --show-error
    --max-time 90
    --request POST
    --output "${response_file}"
    --write-out '%{http_code}'
    --header 'Content-Type: application/x-www-form-urlencoded'
    --data-urlencode "key_id=${key_id}"
  )

  if http_code="$(
    cpanel_curl \
      "${curl_arguments[@]}" \
      "${host}/execute/GPG/delete_keypair"
  )"; then
    curl_status=0
  else
    curl_status=$?
  fi
  if [[ "${curl_status}" != "0" || "${http_code}" != "200" ]]; then
    return 1
  fi
  if ! jq -e '.' "${response_file}" >/dev/null 2>&1; then
    return 1
  fi

  status="$(jq -r '.status // empty' "${response_file}")"
  if [[ "${status}" == "1" ]]; then
    return 0
  fi
  if [[ "${status}" == "0" ]]; then
    printf 'Delete test GPG key pair %s failed: %s\n' \
      "${key_id}" \
      "$(jq -c '{errors, messages}' "${response_file}")" >&2
    return 2
  fi

  return 1
}

cleanup_test_gpg_public_keys() {
  local delete_ambiguous
  local delete_status
  local export_hash
  local key_id
  local second_export_hash
  local user_id

  get_request 'execute/GPG/list_public_keys' 'GPG public-key inventory'
  if ! validate_gpg_public_inventory; then
    printf 'GPG public-key inventory is incomplete\n' >&2
    exit 1
  fi
  gpg_public_candidates="$(
    jq -r \
      '
        .data[]
        | select(
            .user_id
            | startswith(
                "Terraform cPanel acceptance <tfcpanelgpg-"
              )
          )
        | [.id, .user_id]
        | @tsv
      ' \
      "${response_file}"
  )"
  if [[ -n "${gpg_public_candidates}" ]]; then
    if [[ "${CPANEL_ALLOW_GPG_KEYPAIR_DELETE:-0}" != "1" ]]; then
      printf '%s\n' \
        'Refusing GPG cleanup: set CPANEL_ALLOW_GPG_KEYPAIR_DELETE=1 only for a dedicated disposable test account.' >&2
      exit 1
    fi
    if [[ "${CPANEL_EXPECTED_GPG_SECRET_COUNT:-}" != "0" ]]; then
      printf '%s\n' \
        'Refusing GPG cleanup: CPANEL_EXPECTED_GPG_SECRET_COUNT must explicitly be 0.' >&2
      exit 1
    fi
  fi

  while IFS=$'\t' read -r key_id user_id; do
    if [[ -z "${key_id}" || -z "${user_id}" ]]; then
      continue
    fi
    require_registered_artifact "${user_id}"
    key_id="$(
      printf '%s' "${key_id}" | tr '[:lower:]' '[:upper:]'
    )"
    if [[ ! "${key_id}" =~ ^[0-9A-F]{16}$ ]]; then
      printf 'Refusing to delete invalid test GPG public-key ID: %s\n' \
        "${key_id}" >&2
      exit 1
    fi

    get_request 'execute/GPG/list_secret_keys' \
      "Re-read GPG secret inventory before deleting ${key_id}"
    if ! validate_gpg_secret_inventory; then
      printf 'GPG secret-key inventory is incomplete\n' >&2
      exit 1
    fi
    if gpg_secret_inventory_matches_public_id "${key_id}"; then
      printf 'Refusing to delete GPG public key with matching secret key: %s\n' \
        "${key_id}" >&2
      exit 1
    fi

    get_request 'execute/GPG/list_public_keys' \
      "Re-read test GPG public key ${key_id}"
    if ! validate_gpg_public_inventory; then
      printf 'GPG public-key inventory is incomplete\n' >&2
      exit 1
    fi
    if ! jq -e \
      --arg id "${key_id}" \
      --arg user_id "${user_id}" \
      '
        [
          .data[]
          | select((.id | ascii_upcase) == $id)
        ]
        | length == 1
        and .[0].user_id == $user_id
        and (
          .[0].user_id
          | startswith(
              "Terraform cPanel acceptance <tfcpanelgpg-"
            )
        )
      ' \
      "${response_file}" >/dev/null; then
      printf 'Refusing to delete ambiguous test GPG public key: %s\n' \
        "${key_id}" >&2
      exit 1
    fi

    get_request \
      "execute/GPG/export_public_key?key_id=${key_id}" \
      "Export test GPG public key ${key_id}"
    if ! jq -e \
      '
        (.data | type) == "object"
        and (.data.key_data? | type) == "string"
        and (.data.key_data | startswith(
          "-----BEGIN PGP PUBLIC KEY BLOCK-----"
        ))
        and (.data.key_data | contains(
          "-----END PGP PUBLIC KEY BLOCK-----"
        ))
        and (
          .data.key_data
          | contains("PGP PRIVATE KEY BLOCK")
          | not
        )
      ' \
      "${response_file}" >/dev/null; then
      printf 'GPG public-key export is invalid: %s\n' "${key_id}" >&2
      exit 1
    fi
    export_hash="$(jq -j '.data.key_data' "${response_file}" | sha256_stream)"

    get_request 'execute/GPG/list_public_keys' \
      "Final GPG public inventory before deleting ${key_id}"
    if ! validate_gpg_public_inventory ||
      ! jq -e \
        --arg id "${key_id}" \
        --arg user_id "${user_id}" \
        '
          [
            .data[]
            | select(
                (.id | ascii_upcase) == $id
                and .user_id == $user_id
              )
          ]
          | length == 1
        ' \
        "${response_file}" >/dev/null; then
      printf 'Test GPG public key changed before deletion: %s\n' \
        "${key_id}" >&2
      exit 1
    fi
    get_request \
      "execute/GPG/export_public_key?key_id=${key_id}" \
      "Re-export test GPG public key ${key_id}"
    second_export_hash="$(
      jq -j '.data.key_data // empty' "${response_file}" | sha256_stream
    )"
    if [[
      -z "${second_export_hash}"
      || "${second_export_hash}" != "${export_hash}"
     ]]; then
      printf 'Test GPG public-key export changed before deletion: %s\n' \
        "${key_id}" >&2
      exit 1
    fi

    get_request 'execute/GPG/list_secret_keys' \
      "Final GPG secret inventory before deleting ${key_id}"
    if ! validate_gpg_secret_inventory; then
      printf 'GPG secret-key inventory is incomplete\n' >&2
      exit 1
    fi
    if [[ "$(jq -r '.data | length' "${response_file}")" != "0" ]]; then
      printf 'Refusing GPG pair deletion while any secret key exists: %s\n' \
        "${key_id}" >&2
      exit 1
    fi

    delete_ambiguous=0
    if delete_test_gpg_keypair_once "${key_id}"; then
      delete_status=0
    else
      delete_status=$?
      if [[ "${delete_status}" == "2" ]]; then
        exit 1
      fi
      delete_ambiguous=1
    fi

    get_request 'execute/GPG/list_public_keys' \
      "Verify deletion of test GPG public key ${key_id}"
    if ! validate_gpg_public_inventory; then
      printf 'GPG public-key inventory is incomplete\n' >&2
      exit 1
    fi
    if jq -e \
      --arg id "${key_id}" \
      '.data[] | select((.id | ascii_upcase) == $id)' \
      "${response_file}" >/dev/null; then
      if [[ "${delete_ambiguous}" == "1" ]]; then
        printf 'Ambiguous GPG pair deletion could not be reconciled: %s\n' \
          "${key_id}" >&2
      else
        printf 'Test GPG public key still exists after deletion: %s\n' \
          "${key_id}" >&2
      fi
      exit 1
    fi
    get_request 'execute/GPG/list_secret_keys' \
      "Verify secret inventory after deleting ${key_id}"
    if ! validate_gpg_secret_inventory ||
      [[ "$(jq -r '.data | length' "${response_file}")" != "0" ]]; then
      printf 'GPG secret inventory changed during pair deletion: %s\n' \
        "${key_id}" >&2
      exit 1
    fi
    if [[ "${delete_ambiguous}" == "1" ]]; then
      printf 'Recovered ambiguous GPG pair deletion by read-only inventory: %s\n' \
        "${key_id}"
    fi
    deleted_gpg_public_keys=$((deleted_gpg_public_keys + 1))
  done <<<"${gpg_public_candidates}"

  get_request 'execute/GPG/list_public_keys' \
    'GPG public-key inventory after cleanup'
  if ! validate_gpg_public_inventory; then
    printf 'GPG public-key inventory is incomplete\n' >&2
    exit 1
  fi
  if jq -e \
    '
      .data[]
      | select(
          .user_id
          | startswith(
              "Terraform cPanel acceptance <tfcpanelgpg-"
            )
        )
    ' \
    "${response_file}" >/dev/null; then
    printf 'Test GPG public key still exists after cleanup\n' >&2
    exit 1
  fi

  get_request 'execute/GPG/list_secret_keys' \
    'GPG secret-key inventory after cleanup'
  if ! validate_gpg_secret_inventory; then
    printf 'GPG secret-key inventory is incomplete\n' >&2
    exit 1
  fi
  if jq -e \
    '
      .data[]
      | select(
          .user_id
          | startswith(
              "Terraform cPanel acceptance <tfcpanelgpg-"
            )
        )
    ' \
    "${response_file}" >/dev/null; then
    printf 'Test GPG secret key exists; cleanup refuses private material\n' >&2
    exit 1
  fi
}

validate_ssh_public_inventory() {
  jq -e \
    '
      def ssh_authorization:
        if (
          . == true
          or . == 1
          or . == "1"
          or . == "true"
          or . == "yes"
          or . == "authorized"
        ) then true
        elif (
          . == false
          or . == 0
          or . == "0"
          or . == "false"
          or . == "no"
          or . == "deauthorized"
          or . == "unauthorized"
          or . == "not authorized"
        ) then false
        else error("invalid SSH authorization value")
        end;
      (.cpanelresult.data | type == "array")
      and all(
        .cpanelresult.data[];
        (.name? | type) == "string"
        and (.name | test("^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$"))
        and (.name | endswith(".pub") | not)
        and (.key? == (.name + ".pub"))
        and (.file? | type) == "string"
        and (
          .name as $name
          | .file
          | endswith("/" + $name + ".pub")
        )
        and (
          [
            (
              .auth?,
              .authstatus?
            )
            | select(. != null and . != "")
            | if type == "string" then ascii_downcase else . end
            | ssh_authorization
          ] as $authorization
          | ($authorization | length) > 0
          and ($authorization | unique | length) == 1
        )
        and ((.ctime | tostring) | test("^[0-9]+$"))
        and ((.mtime | tostring) | test("^[0-9]+$"))
      )
    ' \
    "${response_file}" >/dev/null
}

cleanup_test_ssh_public_keys() {
  get_request \
    "json-api/cpanel?cpanel_jsonapi_user=${CPANEL_USERNAME}&cpanel_jsonapi_apiversion=2&cpanel_jsonapi_module=SSH&cpanel_jsonapi_func=listkeys&pub=1" \
    'SSH public-key inventory'
  if ! validate_ssh_public_inventory; then
    printf 'SSH public-key inventory is incomplete\n' >&2
    exit 1
  fi
  if jq -e \
    '.cpanelresult.data[] | select(.name | startswith("tfcpanelssh"))' \
    "${response_file}" >/dev/null; then
    printf '%s\n' \
      'Test SSH public key exists; automatic cleanup is disabled because cPanel SSH mutations are name-only and non-atomic' >&2
    exit 1
  fi
}

cleanup_test_ssl_certificates 0
cleanup_test_gpg_public_keys
cleanup_test_ssh_public_keys

get_request \
  'execute/Variables/get_user_information' \
  'cPanel account home inventory'
account_home="$(jq -r '.data.home // empty' "${response_file}")"
if [[ -z "${account_home}" || "${account_home}" != /* ]]; then
  printf 'cPanel account home inventory is invalid: %s\n' \
    "${account_home}" >&2
  exit 1
fi

get_request 'execute/VersionControl/retrieve' 'Git repository inventory'
while IFS= read -r repository_root; do
  if [[ -z "${repository_root}" ]]; then
    continue
  fi
  if [[ "${repository_root}" != "${account_home}/"* ]]; then
    printf 'Refusing to delete test Git repository outside the account home root: %s\n' \
      "${repository_root}" >&2
    exit 1
  fi
  repository_relative="${repository_root#"${account_home}/"}"
  if [[ -z "${repository_relative}" || "${repository_relative}" == *'/../'* ]]; then
    printf 'Refusing to delete test Git repository with invalid relative path: %s\n' \
      "${repository_root}" >&2
    exit 1
  fi
  require_registered_artifact "${repository_relative}"
  delete_test_git_repository "${repository_root}"
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
  require_registered_artifact "${domain}"
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
  require_registered_artifact "${source}"
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
  require_registered_artifact "${mime_type}"
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
  require_registered_artifact "${extension}"
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
  require_registered_artifact "${database}"
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
  require_registered_artifact "${user}"
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
  require_registered_artifact "${database}"
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
  require_registered_artifact "${user}"
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
  require_registered_artifact "${remote_host}"
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
          . == "198.51.100.244"
          or . == "198.51.100.245"
          or . == "198.51.100.246"
          or . == "198.51.100.247"
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
  "json-api/cpanel?cpanel_jsonapi_user=${CPANEL_USERNAME}&cpanel_jsonapi_apiversion=2&cpanel_jsonapi_module=BoxTrapper&cpanel_jsonapi_func=accountmanagelist" \
  'BoxTrapper account inventory'
if ! jq -e \
  '
    (.cpanelresult | type) == "object"
    and (.cpanelresult | has("apiversion"))
    and (.cpanelresult | has("data"))
    and (.cpanelresult | has("event"))
    and (.cpanelresult | has("func"))
    and (.cpanelresult | has("module"))
    and (
      (
        (.cpanelresult | keys)
        - [
            "apiversion",
            "data",
            "event",
            "func",
            "module",
            "postevent",
            "preevent"
          ]
      )
      | length
    ) == 0
    and .cpanelresult.apiversion == 2
    and .cpanelresult.func == "accountmanagelist"
    and .cpanelresult.module == "BoxTrapper"
    and ((.cpanelresult.event | keys) == ["result"])
    and .cpanelresult.event.result == 1
    and (
      (.cpanelresult | has("preevent") | not)
      or .cpanelresult.preevent == null
      or (.cpanelresult.preevent | type) == "object"
    )
    and (
      (.cpanelresult | has("postevent") | not)
      or .cpanelresult.postevent == null
      or (.cpanelresult.postevent | type) == "object"
    )
    and (.cpanelresult.data | type) == "array"
    and (
      ([.cpanelresult.data[].account] | length)
      == ([.cpanelresult.data[].account] | unique | length)
    )
    and all(
      .cpanelresult.data[];
      ((. | keys | sort) == [
        "account",
        "accounturi",
        "bg",
        "enabled",
        "status"
      ])
      and (.account | type) == "string"
      and (.account | length) > 0
      and (
        .enabled == 0
        or .enabled == "0"
        or .enabled == 1
        or .enabled == "1"
      )
    )
  ' \
  "${response_file}" >/dev/null; then
  printf 'Refusing to clean ambiguous BoxTrapper account inventory\n' >&2
  exit 1
fi
boxtrapper_test_accounts="$(
  jq -r \
    '
      .cpanelresult.data[]
      | select(
          (.account | split("@")[0])
          | startswith("tfcpanelboxtrapper")
        )
      | [.account, (.enabled | tostring)]
      | @tsv
    ' \
    "${response_file}"
)"
while IFS=$'\t' read -r address enabled; do
  if [[ -z "${address}" ]]; then
    continue
  fi
  require_registered_artifact "${address}"
  account_candidate_found=0
  while IFS=$'\t' read -r candidate_address _; do
    if [[ "${candidate_address}" == "${address}" ]]; then
      account_candidate_found=1
      break
    fi
  done <<<"${email_account_candidates}"
  if [[ "${account_candidate_found}" != "1" ]]; then
    printf 'Refusing to alter orphaned BoxTrapper test account: %s\n' \
      "${address}" >&2
    exit 1
  fi

  get_request \
    "execute/BoxTrapper/list_queued_messages?email=${address}" \
    "BoxTrapper queue inventory for ${address}"
  if ! jq -e '(.data | type) == "array"' \
    "${response_file}" >/dev/null; then
    printf 'Refusing to clean invalid BoxTrapper queue inventory for %s\n' \
      "${address}" >&2
    exit 1
  fi
  queue_count="$(jq -r '.data | length' "${response_file}")"
  if [[ "${queue_count}" != "0" ]]; then
    printf 'Refusing to delete BoxTrapper test account with %s queued message(s): %s\n' \
      "${queue_count}" \
      "${address}" >&2
    exit 1
  fi

  if [[ "${enabled}" == "1" ]]; then
    uapi_post \
      'BoxTrapper' \
      'set_status' \
      "Disable BoxTrapper for test account ${address}" \
      "email=${address}" \
      'enabled=0'
    reset_boxtrapper_accounts=$((reset_boxtrapper_accounts + 1))
  fi

  get_request \
    "execute/BoxTrapper/get_status?email=${address}" \
    "BoxTrapper status after reset for ${address}"
  if [[ "$(jq -r '.data | tostring' "${response_file}")" != "0" ]]; then
    printf 'BoxTrapper remains enabled for test account: %s\n' \
      "${address}" >&2
    exit 1
  fi
done <<<"${boxtrapper_test_accounts}"

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
  require_registered_artifact "${delegator}"
  require_registered_artifact "${delegatee}"
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

get_request 'execute/Email/list_lists' 'Email mailing list inventory'
if ! jq -e \
  '
    (.data | type == "array")
    and (
      ([.data[].list] | length)
      == ([.data[].list] | unique | length)
    )
    and all(
      .data[];
      (.list? | type) == "string"
      and (
        (((.list | split("@")[0]) | startswith("tfcpanellist")) | not)
        or (
          (.list | contains("@"))
          and (.listid? | type) == "string"
          and (.listid | length > 0)
        )
      )
    )
  ' \
  "${response_file}" >/dev/null; then
  printf 'Refusing to clean ambiguous test email mailing list inventory\n' >&2
  exit 1
fi
email_mailing_list_candidates="$(
  jq -r \
    '
      .data[]
      | select((.list | split("@")[0]) | startswith("tfcpanellist"))
      | [.list, .listid]
      | @tsv
    ' \
    "${response_file}"
)"
while IFS=$'\t' read -r address list_id; do
  if [[ -z "${address}" || -z "${list_id}" ]]; then
    continue
  fi
  require_registered_artifact "${address}"

  get_request 'execute/Email/list_lists' \
    "Re-read test email mailing list ${address}"
  if ! jq -e \
    --arg address "${address}" \
    --arg list_id "${list_id}" \
    '
      [
        .data[]
        | select(.list? == $address)
      ] as $matches
      | ($matches | length) == 1
      and ($matches[0].listid? == $list_id)
    ' \
    "${response_file}" >/dev/null; then
    printf 'Refusing to delete ambiguous test email mailing list: %s\n' \
      "${address}" >&2
    exit 1
  fi

  uapi_post \
    'Email' \
    'delete_list' \
    "Delete test email mailing list ${address}" \
    "list=${address}"
  deleted_email_mailing_lists=$((deleted_email_mailing_lists + 1))
done <<<"${email_mailing_list_candidates}"

get_request \
  'execute/Email/list_lists' \
  'Email mailing list inventory after cleanup'
if jq -e \
  '
    .data[].list
    | select((split("@")[0]) | startswith("tfcpanellist"))
  ' \
  "${response_file}" >/dev/null; then
  printf 'Test email mailing list still exists after cleanup\n' >&2
  exit 1
fi

get_request \
  'execute/Email/list_filters' \
  'Account-level email filter inventory'
if ! jq -e \
  '
    (.data | type) == "array"
    and (
      ([.data[].filtername] | length)
      == ([.data[].filtername] | unique | length)
    )
    and all(
      .data[];
      (.filtername? | type) == "string"
      and (.rules? | type) == "array"
      and (.actions? | type) == "array"
    )
  ' \
  "${response_file}" >/dev/null; then
  printf 'Refusing to clean ambiguous account-level email filter inventory\n' >&2
  exit 1
fi
account_test_filter_names="$(
  jq -r \
    '.data[].filtername | select(startswith("tfcpanelfilter"))' \
    "${response_file}"
)"
while IFS= read -r filter_name; do
  if [[ -z "${filter_name}" ]]; then
    continue
  fi
  require_registered_artifact "${filter_name}"
  uapi_post \
    'Email' \
    'delete_filter' \
    "Delete test account-level email filter ${filter_name}" \
    "filtername=${filter_name}"
  deleted_account_email_filters=$((deleted_account_email_filters + 1))
done <<<"${account_test_filter_names}"

get_request \
  'execute/Email/list_filters' \
  'Account-level email filter inventory after cleanup'
if jq -e \
  '.data[].filtername | select(startswith("tfcpanelfilter"))' \
  "${response_file}" >/dev/null; then
  printf 'Test account-level email filter still exists after cleanup\n' >&2
  exit 1
fi

while IFS= read -r address; do
  if [[ -z "${address}" ]]; then
    continue
  fi
  require_registered_artifact "${address}"

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
    require_registered_artifact "${filter_name}"
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
  require_registered_artifact "${address}"
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
  if [[ "${address%%@*}" == tfcpanelboxtrapper* ]]; then
    get_request \
      "execute/BoxTrapper/list_queued_messages?email=${address}" \
      "Final BoxTrapper queue inventory for ${address}"
    if ! jq -e '(.data | type) == "array"' \
      "${response_file}" >/dev/null; then
      printf 'Refusing to delete test account with invalid final BoxTrapper queue inventory: %s\n' \
        "${address}" >&2
      exit 1
    fi
    queue_count="$(jq -r '.data | length' "${response_file}")"
    if [[ "${queue_count}" != "0" ]]; then
      printf 'Refusing to delete BoxTrapper test account after final queue recheck found %s message(s): %s\n' \
        "${queue_count}" \
        "${address}" >&2
      exit 1
    fi
  fi
  uapi_post 'Email' 'delete_pop' "Delete test email account ${address}" "email=${address}"
  deleted_email_accounts=$((deleted_email_accounts + 1))
done <<<"${email_account_candidates}"

get_request \
  "json-api/cpanel?cpanel_jsonapi_user=${CPANEL_USERNAME}&cpanel_jsonapi_apiversion=2&cpanel_jsonapi_module=BoxTrapper&cpanel_jsonapi_func=accountmanagelist" \
  'BoxTrapper account inventory after cleanup'
if jq -e \
  '
    .cpanelresult.data[].account
    | select((split("@")[0]) | startswith("tfcpanelboxtrapper"))
  ' \
  "${response_file}" >/dev/null; then
  printf 'Test BoxTrapper account still exists after cleanup\n' >&2
  exit 1
fi

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
    require_registered_artifact "${address}"
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
    require_registered_artifact "${address}"
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
while IFS=$'\t' read -r domain destination; do
  if [[ -z "${domain}" || -z "${destination}" ]]; then
    continue
  fi
  require_registered_artifact "${destination}"
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
      | [.dest, .forward]
      | @tsv' \
    "${response_file}"
)

get_request 'execute/Ftp/list_ftp_with_disk?include_acct_types=sub' 'FTP account inventory'
test_ftp_accounts="$(
  jq -r \
    '
      .data[]
      | select(.accttype == "sub")
      | (.login // "") as $login
      | (.reldir // "") as $home
      | (
          try (
            ($login | split("@")[0])
            | capture(
                "^tfcpanelftp(?<kind>account|replacement|datasource)[a-z0-9]{6}$"
              )
          ) catch null
        ) as $identity
      | (
          try (
            $home
            | capture(
                "^tfcpanel-ftp-(?<kind>account|replacement|datasource)-[a-z0-9]{8}(-updated)?$"
              )
          ) catch null
        ) as $directory
      | select(
          ($identity | type) == "object"
          and ($directory | type) == "object"
          and $identity.kind == $directory.kind
        )
      | [$login, $home]
      | @tsv
    ' \
    "${response_file}"
)"
while IFS=$'\t' read -r login home_directory; do
  if [[ -z "${login}" || -z "${home_directory}" ]]; then
    continue
  fi
  require_registered_artifact "${login}"
  require_registered_artifact "${home_directory}"
  get_request \
    'execute/Ftp/list_ftp_with_disk?include_acct_types=sub' \
    "Revalidate FTP account ${login} before deletion"
  if ! jq -e \
    --arg login "${login}" \
    --arg home "${home_directory}" \
    '
      [.data[] | select(.login == $login and .reldir == $home)]
      | length == 1
    ' \
    "${response_file}" >/dev/null; then
    printf 'Refusing to delete changed FTP account: %s\n' "${login}" >&2
    exit 1
  fi
  user="${login%@*}"
  domain="${login#*@}"
  uapi_post \
    'Ftp' \
    'delete_ftp' \
    "Delete test FTP account ${login}" \
    "user=${user}" \
    "domain=${domain}" \
    'destroy=0'
  deleted_ftp_accounts=$((deleted_ftp_accounts + 1))

  get_request \
    'execute/Ftp/list_ftp_with_disk?include_acct_types=sub' \
    "Verify FTP account deletion ${login}"
  if jq -e \
    --arg login "${login}" \
    '.data[] | select(.login == $login)' \
    "${response_file}" >/dev/null; then
    printf 'Test FTP account still exists after deletion: %s\n' \
      "${login}" >&2
    exit 1
  fi

  delete_safe_test_ftp_home_directory "${home_directory}"
done <<<"${test_ftp_accounts}"

get_request \
  'execute/Fileman/list_files?dir=&show_hidden=1&limit=1000' \
  'Orphaned FTP home directory inventory'
orphaned_ftp_home_directories="$(
  jq -r \
    '
      .data[]
      | select(.type == "dir")
      | .file
      | select(
          test(
            "^tfcpanel-ftp-(account|replacement|datasource)-[a-z0-9]{8}(-updated)?$"
          )
        )
    ' \
    "${response_file}"
)"
while IFS= read -r home_directory; do
  if [[ -z "${home_directory}" ]]; then
    continue
  fi
  require_registered_artifact "${home_directory}"
  get_request \
    'execute/Ftp/list_ftp_with_disk?include_acct_types=sub' \
    "Verify no FTP account owns orphaned home ${home_directory}"
  if jq -e \
    --arg home "${home_directory}" \
    '.data[] | select(.accttype == "sub" and .reldir == $home)' \
    "${response_file}" >/dev/null; then
    printf 'Refusing to delete FTP home directory still used by an account: %s\n' \
      "${home_directory}" >&2
    exit 1
  fi
  delete_safe_test_ftp_home_directory "${home_directory}"
done <<<"${orphaned_ftp_home_directories}"

get_request \
  "json-api/cpanel?cpanel_jsonapi_user=${CPANEL_USERNAME}&cpanel_jsonapi_apiversion=2&cpanel_jsonapi_module=DenyIp&cpanel_jsonapi_func=listdenyips" \
  'IP block inventory'
while IFS= read -r address; do
  if [[ -z "${address}" ]]; then
    continue
  fi
  require_registered_artifact "${address}"
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
          or (.ip | startswith("2001:db8:ffff:"))
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
    dns_record_candidate="$(
      jq -r \
        --arg zone "${zone}" \
        'first(
          .data[]
          | select(.record_type != null)
          | (try (.dname_b64 | @base64d) catch "") as $raw_name
          | (
              $raw_name
              | gsub("^\\s+|\\s+$"; "")
              | rtrimstr(".")
            ) as $absolute_name
          | (
              if $absolute_name == $zone then
                "@"
              elif ($absolute_name | endswith("." + $zone)) then
                ($absolute_name | rtrimstr("." + $zone))
              else
                $absolute_name
              end
            ) as $record_name
          | select($record_name | startswith("tfcpaneldns"))
          | [
              ({
                version: 1,
                kind: "dns_record",
                zone: $zone,
                name: $record_name,
                type: (.record_type | ascii_upcase),
                ttl: .ttl,
                data: [.data_b64[] | @base64d]
              } | tojson),
              (.line_index | tostring)
            ]
          | @tsv
        ) // empty' \
        "${response_file}"
    )"
    record_identity="${dns_record_candidate%%$'\t'*}"
    line_index="${dns_record_candidate#*$'\t'}"
    if [[ -z "${line_index}" ]]; then
      break
    fi
    require_registered_dns_record "${record_identity}"

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
  require_registered_artifact "${domain}"
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
  require_registered_artifact "${domain}"
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
  require_registered_artifact "${domain}"
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
  require_registered_artifact "${domain}"
  require_registered_artifact "${domain_key}"
  api2_post \
    'AddonDomain' \
    'deladdondomain' \
    "Delete test addon domain ${domain}" \
    "domain=${domain}" \
    "subdomain=${domain_key}"
  deleted_addon_domains=$((deleted_addon_domains + 1))
done <<<"${test_addon_domains}"

cleanup_test_ssl_certificates 1
cleanup_test_ssl_keys

get_request \
  'execute/Fileman/list_files?dir=public_html&show_hidden=1&limit=1000' \
  'Test domain directory inventory'
test_filesystem_text_files="$(
  jq -r \
    '.data[]
      | select(.type == "file")
      | .file
      | select(startswith("tfcpanel-fs-file-"))' \
    "${response_file}"
)"
filesystem_text_file_marker_names="$(
  jq -r \
    '.data[]
      | select(.type == "file")
      | .file
      | select(startswith(".terraform-cpanel-text-file-"))' \
    "${response_file}"
)"
while IFS= read -r file_name; do
  if [[ -z "${file_name}" ]]; then
    continue
  fi

  managed_path="public_html/${file_name}"
  require_registered_artifact "${managed_path}"
  marker_digest="$(printf '%s' "${managed_path}" | sha256_stream)"
  marker_name=".terraform-cpanel-text-file-${marker_digest}"
  marker_found=0
  while IFS= read -r candidate_marker; do
    if [[ "${candidate_marker}" == "${marker_name}" ]]; then
      marker_found=1
      break
    fi
  done <<<"${filesystem_text_file_marker_names}"
  if [[ "${marker_found}" != "1" ]]; then
    delete_test_filesystem_path \
      "${managed_path}" \
      "Delete registered unmarked test filesystem text file ${managed_path}"
    deleted_filesystem_text_files=$((deleted_filesystem_text_files + 1))
    continue
  fi
  require_registered_artifact "${marker_name}"

  get_request \
    "execute/Fileman/get_file_content?dir=public_html&file=${marker_name}&from_charset=UTF-8&to_charset=UTF-8&update_html_document_encoding=0" \
    "Read ownership marker for test filesystem text file ${managed_path}"
  if ! jq -e \
    --arg managed_path "${managed_path}" \
    '
      (.data.content | type) == "string"
      and (.data.content | utf8bytelength) <= 4096
      and (
        .data.content as $raw
        | (try ($raw | fromjson) catch null) as $marker
        | ($marker | type) == "object"
        and ($marker | keys | sort) == (
          [
            "content_sha256",
            "kind",
            "path",
            "provider",
            "size_bytes",
            "token",
            "version"
          ] | sort
        )
        and $marker.provider == "terraform-provider-cpanel"
        and $marker.version == 1
        and $marker.kind == "text_file"
        and $marker.path == $managed_path
        and ($marker.token | type) == "string"
        and ($marker.token | test("^[0-9a-f]{64}$"))
        and ($marker.content_sha256 | type) == "string"
        and ($marker.content_sha256 | test("^[0-9a-f]{64}$"))
        and ($marker.size_bytes | type) == "number"
        and ($marker.size_bytes | floor) == $marker.size_bytes
        and $marker.size_bytes >= 0
        and $marker.size_bytes <= 1048576
        and $raw == ({
          provider: $marker.provider,
          version: $marker.version,
          kind: $marker.kind,
          path: $marker.path,
          token: $marker.token,
          content_sha256: $marker.content_sha256,
          size_bytes: $marker.size_bytes
        } | tojson)
      )
    ' \
    "${response_file}" >/dev/null; then
    delete_test_filesystem_path \
      "${managed_path}" \
      "Delete registered test filesystem text file with invalid marker ${managed_path}"
    deleted_filesystem_text_files=$((deleted_filesystem_text_files + 1))
    delete_test_filesystem_path \
      "public_html/${marker_name}" \
      "Delete invalid registered ownership marker for ${managed_path}"
    deleted_filesystem_text_file_markers=$((deleted_filesystem_text_file_markers + 1))
    continue
  fi
  marker_content="$(jq -r '.data.content' "${response_file}")"
  marker_content_sha256="$(
    jq -r '.data.content | fromjson | .content_sha256' "${response_file}"
  )"
  marker_size_bytes="$(
    jq -r '.data.content | fromjson | .size_bytes' "${response_file}"
  )"

  get_request \
    "execute/Fileman/get_file_content?dir=public_html&file=${file_name}&from_charset=UTF-8&to_charset=UTF-8&update_html_document_encoding=0" \
    "Read test filesystem text file ${managed_path}"
  actual_content_sha256="$(jq -j '.data.content' "${response_file}" | sha256_stream)"
  actual_size_bytes="$(jq -r '.data.content | utf8bytelength' "${response_file}")"
  if [[
    "${actual_content_sha256}" != "${marker_content_sha256}"
    || "${actual_size_bytes}" != "${marker_size_bytes}"
  ]]; then
    delete_test_filesystem_path \
      "${managed_path}" \
      "Delete registered drifted test filesystem text file ${managed_path}"
    deleted_filesystem_text_files=$((deleted_filesystem_text_files + 1))
    delete_test_filesystem_path \
      "public_html/${marker_name}" \
      "Delete ownership marker for registered drifted file ${managed_path}"
    deleted_filesystem_text_file_markers=$((deleted_filesystem_text_file_markers + 1))
    continue
  fi

  get_request \
    "execute/Fileman/get_file_content?dir=public_html&file=${marker_name}&from_charset=UTF-8&to_charset=UTF-8&update_html_document_encoding=0" \
    "Recheck ownership marker for test filesystem text file ${managed_path}"
  if [[ "$(jq -r '.data.content' "${response_file}")" != "${marker_content}" ]]; then
    printf 'Refusing to delete %s because marker %s changed\n' \
      "${managed_path}" \
      "${marker_name}" >&2
    exit 1
  fi

  get_request \
    "execute/Fileman/get_file_content?dir=public_html&file=${file_name}&from_charset=UTF-8&to_charset=UTF-8&update_html_document_encoding=0" \
    "Recheck test filesystem text file ${managed_path}"
  actual_content_sha256="$(jq -j '.data.content' "${response_file}" | sha256_stream)"
  actual_size_bytes="$(jq -r '.data.content | utf8bytelength' "${response_file}")"
  if [[
    "${actual_content_sha256}" != "${marker_content_sha256}"
    || "${actual_size_bytes}" != "${marker_size_bytes}"
  ]]; then
    printf 'Refusing to delete test filesystem text file changed during cleanup: %s\n' \
      "${managed_path}" >&2
    exit 1
  fi

  delete_test_filesystem_path \
    "${managed_path}" \
    "Delete test filesystem text file ${managed_path}"
  deleted_filesystem_text_files=$((deleted_filesystem_text_files + 1))
  delete_test_filesystem_path \
    "public_html/${marker_name}" \
    "Delete ownership marker for test filesystem text file ${managed_path}"
  deleted_filesystem_text_file_markers=$((deleted_filesystem_text_file_markers + 1))
done <<<"${test_filesystem_text_files}"

get_request \
  'execute/Fileman/list_files?dir=public_html&show_hidden=1&limit=1000' \
  'Orphaned filesystem text file marker inventory'
orphaned_filesystem_text_file_markers="$(
  jq -r \
    '.data[]
      | select(.type == "file")
      | .file
      | select(startswith(".terraform-cpanel-text-file-"))' \
    "${response_file}"
)"
remaining_filesystem_entry_names="$(jq -r '.data[].file' "${response_file}")"
while IFS= read -r marker_name; do
  if [[ -z "${marker_name}" ]]; then
    continue
  fi
  require_registered_artifact "${marker_name}"

  get_request \
    "execute/Fileman/get_file_content?dir=public_html&file=${marker_name}&from_charset=UTF-8&to_charset=UTF-8&update_html_document_encoding=0" \
    "Read orphaned filesystem text file marker ${marker_name}"
  if ! jq -e \
    '
      (.data.content | type) == "string"
      and (
        try (.data.content | fromjson) catch null
        | type == "object"
        and .provider == "terraform-provider-cpanel"
        and .version == 1
        and .kind == "text_file"
        and (.path | type) == "string"
        and (.path | startswith("public_html/tfcpanel-fs-file-"))
      )
    ' \
    "${response_file}" >/dev/null; then
    delete_test_filesystem_path \
      "public_html/${marker_name}" \
      "Delete invalid registered orphaned filesystem text file marker ${marker_name}"
    deleted_filesystem_text_file_markers=$((deleted_filesystem_text_file_markers + 1))
    continue
  fi

  orphaned_path="$(jq -r '.data.content | fromjson | .path' "${response_file}")"
  expected_marker_digest="$(printf '%s' "${orphaned_path}" | sha256_stream)"
  if [[ "${marker_name}" != ".terraform-cpanel-text-file-${expected_marker_digest}" ]]; then
    delete_test_filesystem_path \
      "public_html/${marker_name}" \
      "Delete mismatched registered filesystem text file marker ${marker_name}"
    deleted_filesystem_text_file_markers=$((deleted_filesystem_text_file_markers + 1))
    continue
  fi

  orphaned_file_name="${orphaned_path##*/}"
  orphaned_target_found=0
  while IFS= read -r candidate_name; do
    if [[ "${candidate_name}" == "${orphaned_file_name}" ]]; then
      orphaned_target_found=1
      break
    fi
  done <<<"${remaining_filesystem_entry_names}"
  if [[ "${orphaned_target_found}" == "1" ]]; then
    printf 'Refusing to delete marker %s while target %s still exists\n' \
      "${marker_name}" \
      "${orphaned_path}" >&2
    exit 1
  fi

  delete_test_filesystem_path \
    "public_html/${marker_name}" \
    "Delete orphaned test filesystem text file marker ${marker_name}"
  deleted_filesystem_text_file_markers=$((deleted_filesystem_text_file_markers + 1))
done <<<"${orphaned_filesystem_text_file_markers}"

get_request \
  'execute/Fileman/list_files?dir=public_html&show_hidden=1&limit=1000' \
  'Test domain directory inventory after filesystem text file cleanup'
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

  managed_path="public_html/${directory}"
  require_registered_artifact "${managed_path}"
  delete_test_filesystem_path \
    "${managed_path}" \
    "Delete registered test directory ${managed_path}"
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
    require_registered_artifact "${trash_entry}"
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
  require_registered_artifact "${directory}"
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
      require_registered_artifact "public_html/${directory}"
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
test_cron_jobs="$(
  jq -r \
    '.cpanelresult.data[]
      | select(.type == "command")
      | select((.command // "") | contains("# terraform-provider-cpanel-"))
      | {
          linekey: (.linekey | tostring),
          line: (.line | tonumber),
          commandnumber: (.commandnumber | tonumber),
          command: (.command | tostring),
          minute: (.minute | tostring),
          hour: (.hour | tostring),
          day: (.day | tostring),
          weekday: (.weekday | tostring),
          month: (.month | tostring)
        }
      | @json' \
    "${response_file}"
)"
while IFS= read -r cron_job; do
  if [[ -z "${cron_job}" ]]; then
    continue
  fi
  command="$(jq -r '.command' <<<"${cron_job}")"
  require_registered_artifact "${command}"
  remove_cron_line \
    "${cron_job}" \
    "Delete test cron line $(jq -r '.linekey' <<<"${cron_job}")"
  deleted_cron_lines=$((deleted_cron_lines + 1))
done <<<"${test_cron_jobs}"

get_request \
  "json-api/cpanel?cpanel_jsonapi_user=${CPANEL_USERNAME}&cpanel_jsonapi_apiversion=2&cpanel_jsonapi_module=Cron&cpanel_jsonapi_func=fetchcron" \
  'Cron inventory after test cleanup'

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
printf '  stored SSL keys deleted: %d\n' "${deleted_ssl_keys}"
printf '  GPG public keys deleted: %d\n' "${deleted_gpg_public_keys}"
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
printf '  BoxTrapper test accounts disabled: %d\n' \
  "${reset_boxtrapper_accounts}"
printf '  calendar delegates deleted: %d\n' \
  "${deleted_calendar_delegates}"
printf '  account-level email filters deleted: %d\n' \
  "${deleted_account_email_filters}"
printf '  email filters deleted: %d\n' "${deleted_email_filters}"
printf '  email mailing lists deleted: %d\n' \
  "${deleted_email_mailing_lists}"
printf '  email accounts deleted: %d\n' "${deleted_email_accounts}"
printf '  email forwarders deleted: %d\n' "${deleted_email_forwarders}"
printf '  email domain forwarders deleted: %d\n' \
  "${deleted_email_domain_forwarders}"
printf '  email autoresponders deleted: %d\n' \
  "${deleted_email_auto_responders}"
printf '  email routing domains reset: %d\n' \
  "${reset_email_routings}"
printf '  FTP accounts deleted: %d\n' "${deleted_ftp_accounts}"
printf '  FTP home directories deleted: %d\n' \
  "${deleted_ftp_home_directories}"
printf '  IP blocks deleted: %d\n' "${deleted_ip_blocks}"
printf '  DNS records deleted: %d\n' "${deleted_dns_records}"
printf '  addon domains deleted: %d\n' "${deleted_addon_domains}"
printf '  domain aliases deleted: %d\n' "${deleted_domain_aliases}"
printf '  test ModSecurity domains enabled: %d\n' \
  "${enabled_modsecurity_domains}"
printf '  subdomains deleted: %d\n' "${deleted_subdomains}"
printf '  test domain directories deleted: %d\n' "${deleted_domain_directories}"
printf '  filesystem text files deleted: %d\n' \
  "${deleted_filesystem_text_files}"
printf '  filesystem text file markers deleted: %d\n' \
  "${deleted_filesystem_text_file_markers}"
printf '  Directory Privacy test password directories deleted: %d\n' \
  "${deleted_directory_privacy_password_directories}"
printf '  cron lines deleted: %d\n' "${deleted_cron_lines}"
