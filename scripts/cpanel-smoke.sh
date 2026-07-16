#!/usr/bin/env bash

set -euo pipefail

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

request \
  'execute/StatsBar/get_stats?display=cpanelversion' \
  'cPanel version check'
cpanel_version="$(
  jq -r '.data[] | select(.name == "cpanelversion") | .value' "${response_file}"
)"

request 'execute/Locale/get_attributes' 'cPanel locale check'
account_locale="$(jq -r '.data.locale // empty' "${response_file}")"
account_locale_direction="$(jq -r '.data.direction // empty' "${response_file}")"
account_locale_encoding="$(jq -r '.data.encoding // empty' "${response_file}")"
if [[
  -z "${account_locale}"
  || -z "${account_locale_direction}"
  || -z "${account_locale_encoding}"
 ]]; then
  printf 'cPanel returned incomplete locale attributes\n' >&2
  exit 1
fi

request 'execute/Locale/list_locales' 'cPanel locale inventory'
locale_count="$(jq -r '.data | length' "${response_file}")"
if ! jq -e --arg locale "${account_locale}" \
  '.data[] | select(.locale == $locale)' \
  "${response_file}" >/dev/null; then
  printf 'Current cPanel locale is absent from the locale inventory: %s\n' \
    "${account_locale}" >&2
  exit 1
fi
if [[
  -n "${CPANEL_EXPECTED_LOCALE:-}"
  && "${account_locale}" != "${CPANEL_EXPECTED_LOCALE}"
 ]]; then
  printf 'cPanel locale is %s; expected restored locale %s\n' \
    "${account_locale}" \
    "${CPANEL_EXPECTED_LOCALE}" >&2
  exit 1
fi

request 'execute/LogManager/get_settings' 'cPanel log settings check'
if ! jq -e \
  '
    ((.data.archive_logs | tostring) | test("^[01]$"))
    and ((.data.prune_archive | tostring) | test("^[01]$"))
    and ((.data.using_default | tostring) | test("^[01]$"))
    and ((.data.retention_days | tostring) | test("^[0-9]+$"))
  ' \
  "${response_file}" >/dev/null; then
  printf 'cPanel returned incomplete log archival settings\n' >&2
  exit 1
fi
log_archive_logs="$(jq -r '.data.archive_logs | tostring' "${response_file}")"
log_prune_archive="$(jq -r '.data.prune_archive | tostring' "${response_file}")"
log_using_default="$(jq -r '.data.using_default | tostring' "${response_file}")"
log_effective_retention="$(
  jq -r '.data.retention_days | tostring' "${response_file}"
)"
log_configured_retention="${log_effective_retention}"
if [[ "${log_using_default}" == "1" ]]; then
  log_configured_retention="-1"
fi
if [[
  -n "${CPANEL_EXPECTED_LOG_ARCHIVE:-}"
  && "${log_archive_logs}" != "${CPANEL_EXPECTED_LOG_ARCHIVE}"
 ]]; then
  printf 'cPanel archive_logs is %s; expected restored value %s\n' \
    "${log_archive_logs}" \
    "${CPANEL_EXPECTED_LOG_ARCHIVE}" >&2
  exit 1
fi
if [[
  -n "${CPANEL_EXPECTED_LOG_PRUNE:-}"
  && "${log_prune_archive}" != "${CPANEL_EXPECTED_LOG_PRUNE}"
 ]]; then
  printf 'cPanel prune_archive is %s; expected restored value %s\n' \
    "${log_prune_archive}" \
    "${CPANEL_EXPECTED_LOG_PRUNE}" >&2
  exit 1
fi
if [[
  -n "${CPANEL_EXPECTED_LOG_RETENTION:-}"
  && "${log_configured_retention}" != "${CPANEL_EXPECTED_LOG_RETENTION}"
 ]]; then
  printf 'cPanel log retention is %s; expected restored value %s\n' \
    "${log_configured_retention}" \
    "${CPANEL_EXPECTED_LOG_RETENTION}" >&2
  exit 1
fi

request \
  'execute/ContactInformation/get_notification_preferences' \
  'cPanel notification preferences check'
if ! jq -e \
  '
    (.data | type == "array" and length > 0)
    and (
      [.data[].name] as $names
      | ($names | length) == ($names | unique | length)
    )
    and all(
      .data[];
      (.name | test("^notify_[a-z0-9_]+$"))
      and ((.enabled | tostring) | test("^[01]$"))
      and (.descp | type == "string")
    )
  ' \
  "${response_file}" >/dev/null; then
  printf 'cPanel returned invalid notification preferences\n' >&2
  exit 1
fi
notification_preference_count="$(jq -r '.data | length' "${response_file}")"
notification_preferences="$(
  jq -S -c \
    '
      .data
      | map({
          key: .name,
          value: ((.enabled | tostring) == "1")
        })
      | from_entries
    ' \
    "${response_file}"
)"
if [[ -n "${CPANEL_EXPECTED_NOTIFICATION_PREFERENCES:-}" ]]; then
  if ! jq -e \
    '
      type == "object"
      and length > 0
      and all(
        to_entries[];
        (.key | test("^notify_[a-z0-9_]+$"))
        and (.value | type == "boolean")
      )
    ' <<<"${CPANEL_EXPECTED_NOTIFICATION_PREFERENCES}" >/dev/null; then
    printf 'Invalid expected cPanel notification preferences JSON\n' >&2
    exit 1
  fi
  expected_notification_preferences="$(
    jq -S -c . <<<"${CPANEL_EXPECTED_NOTIFICATION_PREFERENCES}"
  )"
  if [[ "${notification_preferences}" != "${expected_notification_preferences}" ]]; then
    printf 'cPanel notification preferences do not match the restored baseline\n' \
      >&2
    exit 1
  fi
fi

request \
  'execute/SpamAssassin/get_user_preferences' \
  'cPanel SpamAssassin preferences check'
if ! jq -e \
  '
    (.data | type == "object")
    and (.data as $data
    | all(
      [
        "blacklist_from",
        "required_score",
        "score",
        "whitelist_from"
      ][];
      (. as $name
        | ($data[$name]? // null) as $values
        | $values == null
          or (
            $values
            | type == "array"
            and length > 0
            and all(
              .[];
              type == "string"
              and length > 0
              and (test("[\\r\\n\\u0000]") | not)
            )
          )
      )
    ))
  ' \
  "${response_file}" >/dev/null; then
  printf 'cPanel returned invalid SpamAssassin preferences\n' >&2
  exit 1
fi
spam_preferences="$(
  jq -S -c \
    '
      .data as $data
      | reduce [
          "blacklist_from",
          "required_score",
          "score",
          "whitelist_from"
        ][] as $name (
          {};
          if ($data | has($name))
          then .[$name] = ($data[$name] | sort)
          else .
          end
        )
    ' \
    "${response_file}"
)"
spam_preference_count="$(jq -r 'length' <<<"${spam_preferences}")"
if [[ -n "${CPANEL_EXPECTED_SPAM_PREFERENCES:-}" ]]; then
  if ! jq -e \
    '
      type == "object"
      and all(
        to_entries[];
        (.key as $key
          | (
            [
            "blacklist_from",
            "required_score",
            "score",
            "whitelist_from"
            ]
            | index($key)
          ) != null
          and (
            .value
            | type == "array"
            and length > 0
            and all(
              .[];
              type == "string"
              and length > 0
              and (test("[\\r\\n\\u0000]") | not)
            )
          )
        )
      )
    ' <<<"${CPANEL_EXPECTED_SPAM_PREFERENCES}" >/dev/null; then
    printf 'Invalid expected cPanel SpamAssassin preferences JSON\n' >&2
    exit 1
  fi
  expected_spam_preferences="$(
    jq -S -c \
      'with_entries(.value |= sort)' \
      <<<"${CPANEL_EXPECTED_SPAM_PREFERENCES}"
  )"
  if [[ "${spam_preferences}" != "${expected_spam_preferences}" ]]; then
    printf 'cPanel SpamAssassin preferences do not match the restored baseline\n' \
      >&2
    exit 1
  fi
fi

request 'execute/Features/list_features' 'feature check'
for feature in addondomains apitokens blockers boxtrapper changemx cron dynamicdns ftpaccts handlers indexmanager lists mime modsecurity mysql parkeddomains passengerapps pgp popaccts postgres redirects sslmanager subdomains version_control webprotect zoneedit; do
  if [[ "$(jq -r --arg feature "${feature}" '.data[$feature] // 0' "${response_file}")" != "1" ]]; then
    printf 'Required cPanel feature is disabled: %s\n' "${feature}" >&2
    exit 1
  fi
done

request 'execute/Variables/get_user_information' 'cPanel account home check'
account_home="$(jq -r '.data.home' "${response_file}")"
if [[ -z "${account_home}" || "${account_home}" != /* ]]; then
  printf 'cPanel returned an invalid account home directory\n' >&2
  exit 1
fi

request \
  "execute/DirectoryIndexes/get_indexing?dir=${account_home}/public_html" \
  'Directory Indexes check'
public_html_index_type="$(jq -r '.data' "${response_file}")"

request \
  "execute/DirectoryPrivacy/is_directory_protected?dir=${account_home}/public_html" \
  'Directory Privacy check'
public_html_protected="$(jq -r '.data.protected' "${response_file}")"

request 'execute/Tokens/list' 'API token check'
api_token_count="$(jq -r '.data | length' "${response_file}")"
api_test_token_count="$(
  jq -r \
    '[.data[].name | select(startswith("tfcpaneltoken"))] | length' \
    "${response_file}"
)"

request 'execute/DynamicDNS/list' 'Dynamic DNS check'
dynamic_dns_count="$(jq -r '.data | length' "${response_file}")"
dynamic_dns_test_count="$(
  jq -r \
    '[.data[].domain | select(startswith("tfcpanelddns"))] | length' \
    "${response_file}"
)"

request 'execute/Mime/list_redirects' 'HTTP redirect check'
redirect_count="$(jq -r '.data | length' "${response_file}")"
redirect_test_count="$(
  jq -r \
    '[.data[].source | select(startswith("/tfcpanelredirect-"))] | length' \
    "${response_file}"
)"

request 'execute/Mime/list_mime?type=user' 'custom MIME type check'
mime_type_count="$(jq -r '.data | length' "${response_file}")"
mime_type_test_count="$(
  jq -r \
    '[.data[].type | select(startswith("application/x-tfcpanel-"))] | length' \
    "${response_file}"
)"

request 'execute/Mime/list_handlers?type=user' 'Apache handler check'
apache_handler_count="$(jq -r '.data | length' "${response_file}")"
apache_handler_test_count="$(
  jq -r \
    '[.data[].extension | select(startswith(".tfcpanelhandler"))] | length' \
    "${response_file}"
)"

request 'execute/VersionControl/retrieve' 'Git repository check'
git_repository_count="$(jq -r '.data | length' "${response_file}")"
git_repository_test_count="$(
  jq -r \
    '[.data[]
      | .repository_root
      | split("/")
      | last
      | select(startswith("tfcpanel-git-"))] | length' \
    "${response_file}"
)"

request 'execute/PassengerApps/list_applications' 'Passenger application check'
passenger_application_count="$(jq -r '.data | length' "${response_file}")"
passenger_application_test_count="$(
  jq -r \
    '[.data
      | to_entries[]
      | .key
      | select(startswith("tfcpanelpassenger"))] | length' \
    "${response_file}"
)"

request 'execute/SSL/list_csrs' 'stored SSL CSR check'
if ! jq -e \
  '
    (.data | type == "array")
    and all(
      .data[];
      ((.id? | type) == "string" or (.id? | type) == "number")
      and ((.id | tostring | length) > 0)
      and ((.friendly_name? | type) == "string")
      and ((.commonName? | type) == "string")
      and ((.created? | type) == "string"
        or (.created? | type) == "number")
      and ((.domains? | type) == "array")
      and ((.key_algorithm? | type) == "string")
    )
  ' \
  "${response_file}" >/dev/null; then
  printf 'Stored SSL CSR inventory is incomplete\n' >&2
  exit 1
fi
ssl_csr_count="$(jq -r '.data | length' "${response_file}")"
ssl_csr_test_count="$(
  jq -r \
    --arg prefix 'tfcpanelcsr' \
    '
      [.data[]
        | select(
            ((.friendly_name? | type) == "string"
              and (.friendly_name | startswith($prefix)))
            or
            ((.commonName? | type) == "string"
              and (.commonName | startswith($prefix)))
          )]
      | length
    ' \
    "${response_file}"
)"

request 'execute/SSL/list_certs' 'stored SSL certificate check'
if ! jq -e \
  '
    (.data | type == "array")
    and all(
      .data[];
      (
        ((.id? | type) == "string" or (.id? | type) == "number")
        and ((.id | tostring | length) > 0)
        and (
          .domain_is_configured? == 0
          or .domain_is_configured? == "0"
          or .domain_is_configured? == false
          or .domain_is_configured? == "false"
          or .domain_is_configured? == 1
          or .domain_is_configured? == "1"
          or .domain_is_configured? == true
          or .domain_is_configured? == "true"
        )
      )
    )
  ' \
  "${response_file}" >/dev/null; then
  printf 'Stored SSL certificate inventory is incomplete\n' >&2
  exit 1
fi
ssl_certificate_count="$(jq -r '.data | length' "${response_file}")"
ssl_certificate_test_count="$(
  jq -r \
    '
      [.data[]
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
      ]
      | length
    ' \
    "${response_file}"
)"

request 'execute/GPG/list_public_keys' 'GPG public-key inventory'
if ! jq -e \
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
      and (
        .expires? == null
        or (
          (
            (.expires? | type) == "string"
            or (.expires? | type) == "number"
          )
          and (
            (.expires | tostring) == ""
            or ((.expires | tostring) | test("^[0-9]+$"))
          )
        )
      )
      and (.id? | type) == "string"
      and (.id | test("^[0-9A-Fa-f]{16}$"))
      and .type? == "pub"
      and (.user_id? | type) == "string"
      and ((.user_id | length) > 0)
    )
  ' \
  "${response_file}" >/dev/null; then
  printf 'GPG public-key inventory is incomplete\n' >&2
  exit 1
fi
gpg_public_key_count="$(jq -r '.data | length' "${response_file}")"
gpg_public_test_key_count="$(
  jq -r \
    '
      [.data[]
        | select(
            .user_id
            | startswith(
                "Terraform cPanel acceptance <tfcpanelgpg-"
              )
          )]
      | length
    ' \
    "${response_file}"
)"

request 'execute/GPG/list_secret_keys' 'GPG secret-key inventory'
if ! jq -e \
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
      and (
        .expires? == null
        or (
          (
            (.expires? | type) == "string"
            or (.expires? | type) == "number"
          )
          and (
            (.expires | tostring) == ""
            or ((.expires | tostring) | test("^[0-9]+$"))
          )
        )
      )
      and (.id? | type) == "string"
      and (.id | test("^([0-9A-Fa-f]{8}|[0-9A-Fa-f]{16})$"))
      and .type? == "sec"
      and (.user_id? | type) == "string"
      and ((.user_id | length) > 0)
    )
  ' \
  "${response_file}" >/dev/null; then
  printf 'GPG secret-key inventory is incomplete\n' >&2
  exit 1
fi
gpg_secret_key_count="$(jq -r '.data | length' "${response_file}")"
gpg_secret_test_key_count="$(
  jq -r \
    '
      [.data[]
        | select(
            .user_id
            | startswith(
                "Terraform cPanel acceptance <tfcpanelgpg-"
              )
          )]
      | length
    ' \
    "${response_file}"
)"
if [[
  -n "${CPANEL_EXPECTED_GPG_PUBLIC_COUNT:-}"
  && "${gpg_public_key_count}" != "${CPANEL_EXPECTED_GPG_PUBLIC_COUNT}"
 ]]; then
  printf 'GPG public-key count is %s; expected restored count %s\n' \
    "${gpg_public_key_count}" \
    "${CPANEL_EXPECTED_GPG_PUBLIC_COUNT}" >&2
  exit 1
fi
if [[
  -n "${CPANEL_EXPECTED_GPG_SECRET_COUNT:-}"
  && "${gpg_secret_key_count}" != "${CPANEL_EXPECTED_GPG_SECRET_COUNT}"
 ]]; then
  printf 'GPG secret-key count is %s; expected restored count %s\n' \
    "${gpg_secret_key_count}" \
    "${CPANEL_EXPECTED_GPG_SECRET_COUNT}" >&2
  exit 1
fi

request \
  "json-api/cpanel?cpanel_jsonapi_user=${CPANEL_USERNAME}&cpanel_jsonapi_apiversion=2&cpanel_jsonapi_module=SSH&cpanel_jsonapi_func=listkeys&pub=1" \
  'SSH public-key inventory'
if ! jq -e \
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
  "${response_file}" >/dev/null; then
  printf 'SSH public-key inventory is incomplete\n' >&2
  exit 1
fi
ssh_public_key_count="$(
  jq -r '.cpanelresult.data | length' "${response_file}"
)"
ssh_public_test_key_count="$(
  jq -r \
    '
      [
        .cpanelresult.data[]
        | select(.name | startswith("tfcpanelssh"))
      ]
      | length
    ' \
    "${response_file}"
)"
if [[
  -n "${CPANEL_EXPECTED_SSH_PUBLIC_COUNT:-}"
  && "${ssh_public_key_count}" != "${CPANEL_EXPECTED_SSH_PUBLIC_COUNT}"
 ]]; then
  printf 'SSH public-key count is %s; expected restored count %s\n' \
    "${ssh_public_key_count}" \
    "${CPANEL_EXPECTED_SSH_PUBLIC_COUNT}" >&2
  exit 1
fi

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
  "json-api/cpanel?cpanel_jsonapi_user=${CPANEL_USERNAME}&cpanel_jsonapi_apiversion=2&cpanel_jsonapi_module=MysqlFE&cpanel_jsonapi_func=listhosts" \
  'remote MySQL host check'
mysql_remote_host_count="$(jq -r '.cpanelresult.data | length' "${response_file}")"
mysql_test_remote_host_count="$(
  jq -r \
    '[.cpanelresult.data[].host
      | select(
          . == "198.51.100.245"
          or . == "198.51.100.246"
          or . == "198.51.100.247"
          or . == "198.51.100.248"
          or . == "198.51.100.249"
          or . == "198.51.100.250"
        )] | length' \
    "${response_file}"
)"

request 'execute/CPDAVD/list_delegates' 'Calendar delegate check'
if ! jq -e \
  '
    (.data | type == "array")
    and all(
      .data[];
      (.delegator? | type) == "string"
      and (.delegatee? | type) == "string"
      and (.calendar? | type) == "string"
      and (.calname? | type) == "string"
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
calendar_delegate_count="$(jq -r '.data | length' "${response_file}")"
calendar_test_delegate_count="$(
  jq -r \
    '
      [
        .data[]
        | select(
            (.delegator | split("@")[0] | startswith("tfcpanelcal"))
            or (.delegatee | split("@")[0] | startswith("tfcpanelcal"))
          )
      ]
      | length
    ' \
    "${response_file}"
)"

request \
  'execute/Email/list_pops_with_disk?skip_main=1&no_disk=1&get_restrictions=1' \
  'Email account check'
email_account_count="$(jq -r '.data | length' "${response_file}")"
email_test_account_count="$(
  jq -r \
    '[.data[].email | select(startswith("tfcpanel"))] | length' \
    "${response_file}"
)"
email_suspended_count="$(
  jq -r '[.data[] | select(.has_suspended == 1)] | length' "${response_file}"
)"
email_test_suspended_count="$(
  jq -r \
    '[.data[]
      | select(.email | startswith("tfcpanel"))
      | select(.has_suspended == 1)] | length' \
    "${response_file}"
)"
email_accounts="$(jq -r '.data[].email' "${response_file}")"

request \
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
      and (.accounturi | type) == "string"
      and (.accounturi | length) > 0
      and (.bg | type) == "string"
      and (.bg | length) > 0
      and (
        .enabled == 0
        or .enabled == "0"
        or .enabled == 1
        or .enabled == "1"
      )
      and (.status | type) == "string"
      and (.status | length) > 0
    )
  ' \
  "${response_file}" >/dev/null; then
  printf 'BoxTrapper account inventory is incomplete\n' >&2
  exit 1
fi
boxtrapper_account_count="$(
  jq -r '.cpanelresult.data | length' "${response_file}"
)"
boxtrapper_enabled_count="$(
  jq -r \
    '[.cpanelresult.data[] | select((.enabled | tostring) == "1")] | length' \
    "${response_file}"
)"
boxtrapper_test_account_count="$(
  jq -r \
    '[
      .cpanelresult.data[].account
      | select((split("@")[0]) | startswith("tfcpanelboxtrapper"))
    ] | length' \
    "${response_file}"
)"
boxtrapper_test_enabled_count="$(
  jq -r \
    '[
      .cpanelresult.data[]
      | select((.account | split("@")[0]) | startswith("tfcpanelboxtrapper"))
      | select((.enabled | tostring) == "1")
    ] | length' \
    "${response_file}"
)"
boxtrapper_accounts="$(
  jq -r \
    '.cpanelresult.data[] | [.account, (.enabled | tostring)] | @tsv' \
    "${response_file}"
)"
boxtrapper_test_queued_message_count=0
while IFS=$'\t' read -r address inventory_enabled; do
  if [[ -z "${address}" ]]; then
    continue
  fi

  request \
    "execute/BoxTrapper/get_status?email=${address}" \
    "BoxTrapper status check for ${address}"
  if ! jq -e \
    '
      .data == 0
      or .data == "0"
      or .data == 1
      or .data == "1"
    ' \
    "${response_file}" >/dev/null; then
    printf 'BoxTrapper status is invalid for %s\n' "${address}" >&2
    exit 1
  fi
  if [[ "$(jq -r '.data | tostring' "${response_file}")" != "${inventory_enabled}" ]]; then
    printf 'BoxTrapper status disagrees with the account inventory for %s\n' \
      "${address}" >&2
    exit 1
  fi

  request \
    "execute/BoxTrapper/get_configuration?email=${address}" \
    "BoxTrapper configuration check for ${address}"
  if ! jq -e \
    '
      def flag:
        . == 0 or . == "0" or . == 1 or . == "1";
      def positive_integer:
        (
          type == "number"
          and floor == .
          and . >= 1
        )
        or (
          type == "string"
          and test("^[1-9][0-9]*$")
        );
      def decimal:
        (type == "number")
        or (
          type == "string"
          and test("^-?[0-9]+([.][0-9]+)?$")
        );
      (.data | type) == "object"
      and ((.data | keys | sort) == [
        "enable_auto_whitelist",
        "from_addresses",
        "from_name",
        "queue_days",
        "spam_score",
        "whitelist_by_association"
      ])
      and (.data.enable_auto_whitelist | flag)
      and (.data.from_addresses | type) == "string"
      and (
        .data.from_name == null
        or (.data.from_name | type) == "string"
      )
      and (.data.queue_days | positive_integer)
      and (.data.spam_score | decimal)
      and (.data.whitelist_by_association | flag)
    ' \
    "${response_file}" >/dev/null; then
    printf 'BoxTrapper configuration is incomplete for %s\n' \
      "${address}" >&2
    exit 1
  fi

  if [[ "${address%%@*}" == tfcpanelboxtrapper* ]]; then
    request \
      "execute/BoxTrapper/list_queued_messages?email=${address}" \
      "BoxTrapper queue check for ${address}"
    if ! jq -e '(.data | type) == "array"' \
      "${response_file}" >/dev/null; then
      printf 'BoxTrapper queue inventory is invalid for %s\n' \
        "${address}" >&2
      exit 1
    fi
    account_queue_count="$(jq -r '.data | length' "${response_file}")"
    boxtrapper_test_queued_message_count=$((boxtrapper_test_queued_message_count + account_queue_count))
  fi
done <<<"${boxtrapper_accounts}"

request 'execute/Email/list_lists' 'Email mailing list check'
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
      and (.list | contains("@"))
      and (.listid? | type) == "string"
      and (.listid | length > 0)
      and (.listadmin? | type) == "string"
      and (.humandiskused? | type) == "string"
      and (
        (
          (
            ((.advertised? | type) == "number")
            and (.advertised? == 0)
          ) as $not_advertised
          | (
            ((.archive_private? | type) == "number")
            and (.archive_private? == 1)
          ) as $private_archive
          | (
            ((.subscribe_policy? | type) == "number")
            and (
              (.subscribe_policy? == 2)
              or (.subscribe_policy? == 3)
            )
          ) as $private_subscription
          | (
              $not_advertised
              and $private_archive
              and $private_subscription
            ) as $private
          | (
              (.accesstype? == "private" and $private)
              or
              (.accesstype? == "public" and ($private | not))
            )
        )
      )
      and all(
        .data[];
        (.advertised? == 0 or .advertised? == 1)
        and (.archive_private? == 0 or .archive_private? == 1)
        and (
          .subscribe_policy? == 1
          or .subscribe_policy? == 2
          or .subscribe_policy? == 3
        )
      )
    )
  ' \
  "${response_file}" >/dev/null; then
  printf 'Email mailing list inventory is incomplete\n' >&2
  exit 1
fi
email_mailing_list_count="$(jq -r '.data | length' "${response_file}")"
email_test_mailing_list_count="$(
  jq -r \
    '[
      .data[].list
      | select((split("@")[0]) | startswith("tfcpanellist"))
    ] | length' \
    "${response_file}"
)"

request \
  'execute/Email/list_filters' \
  'Account-level email filter check'
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
  printf 'Account-level email filter inventory is incomplete\n' >&2
  exit 1
fi
account_email_filter_count="$(jq -r '.data | length' "${response_file}")"
account_email_test_filter_count="$(
  jq -r \
    '[.data[].filtername
      | select(startswith("tfcpanelfilter"))] | length' \
    "${response_file}"
)"
email_filter_count="${account_email_filter_count}"
email_test_filter_count="${account_email_test_filter_count}"
while IFS= read -r address; do
  if [[ -z "${address}" ]]; then
    continue
  fi

  request \
    "execute/Email/list_filters?account=${address}" \
    "Email filter check for ${address}"
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
  account_filter_count="$(jq -r '.data | length' "${response_file}")"
  account_test_filter_count="$(
    jq -r \
      '[.data[].filtername
        | select(startswith("tfcpanelfilter"))] | length' \
      "${response_file}"
  )"
  email_filter_count=$((email_filter_count + account_filter_count))
  email_test_filter_count=$((email_test_filter_count + account_test_filter_count))
done <<<"${email_accounts}"

request 'execute/Email/list_mxs' 'Email routing inventory'
if ! jq -e \
  '
    def flag:
      . == 0 or . == 1 or . == "0" or . == "1";
    def mode:
      . == "auto"
      or . == "local"
      or . == "remote"
      or . == "secondary";
    def nonnegative_integer:
      (
        type == "number"
        and floor == .
        and . >= 0
      )
      or (
        type == "string"
        and test("^(0|[1-9][0-9]*)$")
      );
    def positive_integer:
      (
        type == "number"
        and floor == .
        and . >= 1
      )
      or (
        type == "string"
        and test("^[1-9][0-9]*$")
      );
    (.data | type) == "array"
    and all(
      .data[];
      (.domain? | type) == "string"
      and (.domain | length) > 0
      and (.mxcheck | mode)
      and (.detected | mode)
      and (
        .mx == null
        or (
          (.mx | type) == "string"
          and (.mx | length) > 0
        )
      )
      and (.alwaysaccept | flag)
      and (.local | flag)
      and (.remote | flag)
      and (.secondary | flag)
      and (
        .status == 1
        or .status == "1"
      )
      and (.statusmsg? | type) == "string"
      and (.statusmsg | length) > 0
      and (.entries? | type) == "array"
      and all(
        .entries[];
        (.domain? | type) == "string"
        and (.domain | length) > 0
        and (.entrycount | positive_integer)
        and (.mx? | type) == "string"
        and (.mx | length) > 0
        and (.priority | nonnegative_integer)
        and (
          .row == "odd"
          or .row == "even"
        )
      )
    )
  ' \
  "${response_file}" >/dev/null; then
  printf 'Email routing inventory is incomplete\n' >&2
  exit 1
fi
email_routing_count="$(jq -r '.data | length' "${response_file}")"
email_test_non_auto_routing_count="$(
  jq -r \
    '
      [
        .data[]
        | select(
            (.domain | startswith("tfcpanelsubemailrouting"))
            and .mxcheck != "auto"
          )
      ]
      | length
    ' \
    "${response_file}"
)"

request 'execute/Email/list_mail_domains' 'Email forwarder domain inventory'
mail_domains="$(jq -r '.data[].domain' "${response_file}")"
email_forwarder_count=0
email_test_forwarder_count=0
email_auto_responder_count=0
email_test_auto_responder_count=0
while IFS= read -r domain; do
  if [[ -z "${domain}" ]]; then
    continue
  fi

  request \
    "execute/Email/list_forwarders?domain=${domain}" \
    "Email forwarder check for ${domain}"
  domain_forwarder_count="$(jq -r '.data | length' "${response_file}")"
  domain_test_forwarder_count="$(
    jq -r \
      '[.data[].dest | select(startswith("tfcpanelfwd"))] | length' \
      "${response_file}"
  )"
  email_forwarder_count=$((email_forwarder_count + domain_forwarder_count))
  email_test_forwarder_count=$((email_test_forwarder_count + domain_test_forwarder_count))

  request \
    "execute/Email/list_auto_responders?domain=${domain}" \
    "Email autoresponder check for ${domain}"
  domain_auto_responder_count="$(jq -r '.data | length' "${response_file}")"
  domain_test_auto_responder_count="$(
    jq -r \
      '[.data[].email | select(startswith("tfcpanelauto"))] | length' \
      "${response_file}"
  )"
  email_auto_responder_count=$((email_auto_responder_count + domain_auto_responder_count))
  email_test_auto_responder_count=$((email_test_auto_responder_count + domain_test_auto_responder_count))
done <<<"${mail_domains}"

request 'execute/Email/list_domain_forwarders' 'Email domain forwarder check'
email_domain_forwarder_count="$(jq -r '.data | length' "${response_file}")"
email_test_domain_forwarder_count="$(
  jq -r \
    '[.data[].forward | select(startswith("tfcpaneldomainfwd"))] | length' \
    "${response_file}"
)"

request 'execute/Ftp/list_ftp_with_disk?include_acct_types=sub' 'FTP account check'
ftp_account_count="$(jq -r '.data | length' "${response_file}")"
ftp_test_account_count="$(
  jq -r \
    '[.data[].login | select(startswith("tfcpanelftp"))] | length' \
  "${response_file}"
)"

request \
  "json-api/cpanel?cpanel_jsonapi_user=${CPANEL_USERNAME}&cpanel_jsonapi_apiversion=2&cpanel_jsonapi_module=DenyIp&cpanel_jsonapi_func=listdenyips" \
  'IP block check'
ip_block_count="$(jq -r '.cpanelresult.data | length' "${response_file}")"
ip_test_block_count="$(
  jq -r \
    '[.cpanelresult.data[]
      | select(
          .ip == "198.51.100.253"
          or .ip == "198.51.100.254"
          or .ip == "198.51.100.240-198.51.100.242"
          or .ip == "198.51.100.240/31"
          or .ip == "198.51.100.242"
          or .ip == "203.0.113.248/30"
          or (.ip | startswith("2001:0db8:ffff:"))
        )] | length' \
    "${response_file}"
)"

request 'execute/DomainInfo/list_domains' 'DNS zone inventory'
dns_zones="$(
  jq -r \
    '[.data.main_domain] + (.data.addon_domains // []) | .[]' \
    "${response_file}"
)"
dns_record_count=0
dns_test_record_count=0
while IFS= read -r zone; do
  if [[ -z "${zone}" ]]; then
    continue
  fi

  request "execute/DNS/parse_zone?zone=${zone}" "DNS record check for ${zone}"
  zone_record_count="$(
    jq -r \
      '[.data[] | select(.record_type != null)] | length' \
      "${response_file}"
  )"
  zone_test_record_count="$(
    jq -r \
      '[.data[]
        | select(.record_type != null)
        | select(
            (try (.dname_b64 | @base64d) catch "")
            | startswith("tfcpaneldns")
          )] | length' \
      "${response_file}"
  )"
  dns_record_count=$((dns_record_count + zone_record_count))
  dns_test_record_count=$((dns_test_record_count + zone_test_record_count))
done <<<"${dns_zones}"

request \
  "json-api/cpanel?cpanel_jsonapi_user=${CPANEL_USERNAME}&cpanel_jsonapi_apiversion=2&cpanel_jsonapi_module=AddonDomain&cpanel_jsonapi_func=listaddondomains" \
  'addon domain check'
addon_domain_count="$(jq -r '.cpanelresult.data | length' "${response_file}")"
addon_domain_test_count="$(
  jq -r \
    '[.cpanelresult.data[].domain | select(startswith("tfcpaneladdon"))] | length' \
    "${response_file}"
)"

request \
  "json-api/cpanel?cpanel_jsonapi_user=${CPANEL_USERNAME}&cpanel_jsonapi_apiversion=2&cpanel_jsonapi_module=Park&cpanel_jsonapi_func=listparkeddomains" \
  'domain alias check'
domain_alias_count="$(jq -r '.cpanelresult.data | length' "${response_file}")"
domain_alias_test_count="$(
  jq -r \
    '[.cpanelresult.data[].domain | select(startswith("tfcpanelalias"))] | length' \
    "${response_file}"
)"

request \
  "json-api/cpanel?cpanel_jsonapi_user=${CPANEL_USERNAME}&cpanel_jsonapi_apiversion=2&cpanel_jsonapi_module=SubDomain&cpanel_jsonapi_func=listsubdomains" \
  'subdomain check'
subdomain_count="$(jq -r '.cpanelresult.data | length' "${response_file}")"
subdomain_test_count="$(
  jq -r \
    '[.cpanelresult.data[].domain | select(startswith("tfcpanelsub"))] | length' \
    "${response_file}"
)"

request 'execute/ModSecurity/list_domains' 'ModSecurity domain check'
modsecurity_domain_count="$(jq -r '.data | length' "${response_file}")"
modsecurity_disabled_count="$(
  jq -r '[.data[] | select(.enabled == 0)] | length' "${response_file}"
)"
modsecurity_test_disabled_count="$(
  jq -r \
    '[.data[]
      | select(.enabled == 0)
      | .domain
      | select(startswith("tfcpanelsubmodsecurity"))] | length' \
    "${response_file}"
)"

request \
  'execute/Fileman/list_files?dir=public_html&show_hidden=1&limit=1000' \
  'test domain directory check'
domain_test_directory_count="$(
  jq -r \
    '[.data[]
      | select(.type == "dir")
      | .file
      | select(startswith("tfcpanel-"))] | length' \
    "${response_file}"
)"
filesystem_text_file_test_count="$(
  jq -r \
    '[.data[]
      | select(.type == "file")
      | .file
      | select(startswith("tfcpanel-fs-file-"))] | length' \
    "${response_file}"
)"
filesystem_text_file_marker_count="$(
  jq -r \
    '[.data[]
      | select(.type == "file")
      | .file
      | select(startswith(".terraform-cpanel-text-file-"))] | length' \
    "${response_file}"
)"

directory_privacy_test_directory_count=0
request \
  'execute/Fileman/list_files?dir=&show_hidden=1&limit=1000' \
  'cPanel home directory check'
git_repository_test_directory_count="$(
  jq -r \
    '[.data[]
      | select(.type == "dir")
      | .file
      | select(
          startswith("tfcpanel-git-")
          or startswith(".terraform-cpanel-git-delete-")
        )] | length' \
    "${response_file}"
)"
home_has_trash="$(
  jq -r \
    '[.data[] | select(.type == "dir" and .file == ".trash")] | length' \
    "${response_file}"
)"
home_has_password_root="$(
  jq -r \
    '[.data[] | select(.type == "dir" and .file == ".htpasswds")] | length' \
    "${response_file}"
)"
git_repository_test_trash_count=0
if [[ "${home_has_trash}" != "0" ]]; then
  request \
    'execute/Fileman/list_files?dir=.trash&show_hidden=1&limit=1000' \
    'cPanel trash check'
  git_repository_test_trash_count="$(
    jq -r \
      '[.data[]
        | select(.type == "dir")
        | .file
        | select(
            startswith("tfcpanel-git-")
            or startswith(".terraform-cpanel-git-delete-")
          )] | length' \
      "${response_file}"
  )"
fi
if [[ "${home_has_password_root}" != "0" ]]; then
  request \
    'execute/Fileman/list_files?dir=.htpasswds&show_hidden=1&limit=1000' \
    'Directory Privacy password root check'
  if jq -e \
    '.data[] | select(.type == "dir" and .file == "public_html")' \
    "${response_file}" >/dev/null; then
    request \
      'execute/Fileman/list_files?dir=.htpasswds/public_html&show_hidden=1&limit=1000' \
      'Directory Privacy test directory check'
    directory_privacy_test_directory_count="$(
      jq -r \
        '[.data[]
          | select(.type == "dir")
          | .file
          | select(startswith("tfcpanel-privacy-"))] | length' \
        "${response_file}"
    )"
  fi
fi

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
    "${api_test_token_count}" != "0"
    || "${dynamic_dns_test_count}" != "0"
    || "${redirect_test_count}" != "0"
    || "${mime_type_test_count}" != "0"
    || "${apache_handler_test_count}" != "0"
    || "${git_repository_test_count}" != "0"
    || "${passenger_application_test_count}" != "0"
    || "${ssl_csr_test_count}" != "0"
    || "${ssl_certificate_test_count}" != "0"
    || "${gpg_public_test_key_count}" != "0"
    || "${gpg_secret_test_key_count}" != "0"
    || "${ssh_public_test_key_count}" != "0"
    || "${database_count}" != "0"
    || "${user_count}" != "0"
    || "${mysql_test_database_count}" != "0"
    || "${mysql_test_user_count}" != "0"
    || "${mysql_test_remote_host_count}" != "0"
    || "${calendar_test_delegate_count}" != "0"
    || "${email_test_account_count}" != "0"
    || "${email_test_suspended_count}" != "0"
    || "${boxtrapper_test_account_count}" != "0"
    || "${boxtrapper_test_enabled_count}" != "0"
    || "${boxtrapper_test_queued_message_count}" != "0"
    || "${email_test_filter_count}" != "0"
    || "${email_test_mailing_list_count}" != "0"
    || "${email_test_non_auto_routing_count}" != "0"
    || "${email_test_forwarder_count}" != "0"
    || "${email_test_domain_forwarder_count}" != "0"
    || "${email_test_auto_responder_count}" != "0"
    || "${ftp_test_account_count}" != "0"
    || "${ip_test_block_count}" != "0"
    || "${dns_test_record_count}" != "0"
    || "${addon_domain_test_count}" != "0"
    || "${domain_alias_test_count}" != "0"
    || "${subdomain_test_count}" != "0"
    || "${modsecurity_test_disabled_count}" != "0"
    || "${domain_test_directory_count}" != "0"
    || "${filesystem_text_file_test_count}" != "0"
    || "${filesystem_text_file_marker_count}" != "0"
    || "${git_repository_test_directory_count}" != "0"
    || "${git_repository_test_trash_count}" != "0"
    || "${directory_privacy_test_directory_count}" != "0"
    || "${cron_count}" != "0"
  ]]; then
    printf 'cPanel test-managed inventory is not empty\n' >&2
    printf '  test API tokens: %s\n' "${api_test_token_count}" >&2
    printf '  test Dynamic DNS domains: %s\n' \
      "${dynamic_dns_test_count}" >&2
    printf '  test HTTP redirects: %s\n' "${redirect_test_count}" >&2
    printf '  test custom MIME types: %s\n' "${mime_type_test_count}" >&2
    printf '  test Apache handlers: %s\n' \
      "${apache_handler_test_count}" >&2
    printf '  test Git repositories: %s\n' \
      "${git_repository_test_count}" >&2
    printf '  test Passenger applications: %s\n' \
      "${passenger_application_test_count}" >&2
    printf '  test stored SSL CSRs: %s\n' \
      "${ssl_csr_test_count}" >&2
    printf '  test stored SSL certificates: %s\n' \
      "${ssl_certificate_test_count}" >&2
    printf '  test GPG public keys: %s\n' \
      "${gpg_public_test_key_count}" >&2
    printf '  test GPG secret keys: %s\n' \
      "${gpg_secret_test_key_count}" >&2
    printf '  test SSH public keys: %s\n' \
      "${ssh_public_test_key_count}" >&2
    printf '  PostgreSQL databases: %s\n' "${database_count}" >&2
    printf '  PostgreSQL users: %s\n' "${user_count}" >&2
    printf '  MySQL test databases: %s\n' "${mysql_test_database_count}" >&2
    printf '  MySQL test users: %s\n' "${mysql_test_user_count}" >&2
    printf '  test remote MySQL hosts: %s\n' \
      "${mysql_test_remote_host_count}" >&2
    printf '  test calendar delegates: %s\n' \
      "${calendar_test_delegate_count}" >&2
    printf '  email test accounts: %s\n' "${email_test_account_count}" >&2
    printf '  suspended email test accounts: %s\n' \
      "${email_test_suspended_count}" >&2
    printf '  test BoxTrapper accounts: %s\n' \
      "${boxtrapper_test_account_count}" >&2
    printf '  enabled test BoxTrapper accounts: %s\n' \
      "${boxtrapper_test_enabled_count}" >&2
    printf '  queued test BoxTrapper messages: %s\n' \
      "${boxtrapper_test_queued_message_count}" >&2
    printf '  test email filters: %s\n' "${email_test_filter_count}" >&2
    printf '  test account-level email filters: %s\n' \
      "${account_email_test_filter_count}" >&2
    printf '  test email mailing lists: %s\n' \
      "${email_test_mailing_list_count}" >&2
    printf '  test non-auto email routing domains: %s\n' \
      "${email_test_non_auto_routing_count}" >&2
    printf '  test email forwarders: %s\n' "${email_test_forwarder_count}" >&2
    printf '  test email domain forwarders: %s\n' \
      "${email_test_domain_forwarder_count}" >&2
    printf '  test email autoresponders: %s\n' \
      "${email_test_auto_responder_count}" >&2
    printf '  FTP test accounts: %s\n' "${ftp_test_account_count}" >&2
    printf '  test IP blocks: %s\n' "${ip_test_block_count}" >&2
    printf '  test DNS records: %s\n' "${dns_test_record_count}" >&2
    printf '  test addon domains: %s\n' "${addon_domain_test_count}" >&2
    printf '  test domain aliases: %s\n' "${domain_alias_test_count}" >&2
    printf '  test subdomains: %s\n' "${subdomain_test_count}" >&2
    printf '  disabled test ModSecurity domains: %s\n' \
      "${modsecurity_test_disabled_count}" >&2
    printf '  test domain directories: %s\n' "${domain_test_directory_count}" >&2
    printf '  test filesystem text files: %s\n' \
      "${filesystem_text_file_test_count}" >&2
    printf '  filesystem text file markers: %s\n' \
      "${filesystem_text_file_marker_count}" >&2
    printf '  test Git repository directories: %s\n' \
      "${git_repository_test_directory_count}" >&2
    printf '  test Git trash entries: %s\n' \
      "${git_repository_test_trash_count}" >&2
    printf '  test Directory Privacy password directories: %s\n' \
      "${directory_privacy_test_directory_count}" >&2
    printf '  cron commands: %s\n' "${cron_command_count}" >&2
    printf '  cron variables: %s\n' "${cron_variable_count}" >&2
    exit 1
  fi
fi

printf 'cPanel smoke test passed\n'
printf '  account: %s\n' "${CPANEL_USERNAME}"
printf '  host: %s\n' "${host}"
printf '  version: %s\n' "${cpanel_version}"
printf '  locale: %s (%s, %s; %s available)\n' \
  "${account_locale}" \
  "${account_locale_direction}" \
  "${account_locale_encoding}" \
  "${locale_count}"
printf '  log archival: archive=%s prune=%s retention=%s (effective %s)\n' \
  "${log_archive_logs}" \
  "${log_prune_archive}" \
  "${log_configured_retention}" \
  "${log_effective_retention}"
printf '  notification preferences: %s\n' \
  "${notification_preference_count}"
printf '  SpamAssassin preferences: %s configured\n' \
  "${spam_preference_count}"
printf '  API tokens: %s (%s test-managed)\n' \
  "${api_token_count}" \
  "${api_test_token_count}"
printf '  Dynamic DNS domains: %s (%s test-managed)\n' \
  "${dynamic_dns_count}" \
  "${dynamic_dns_test_count}"
printf '  HTTP redirects: %s (%s test-managed)\n' \
  "${redirect_count}" \
  "${redirect_test_count}"
printf '  custom MIME types: %s (%s test-managed)\n' \
  "${mime_type_count}" \
  "${mime_type_test_count}"
printf '  Apache handlers: %s (%s test-managed)\n' \
  "${apache_handler_count}" \
  "${apache_handler_test_count}"
printf '  Git repositories: %s (%s test-managed)\n' \
  "${git_repository_count}" \
  "${git_repository_test_count}"
printf '  Passenger applications: %s (%s test-managed)\n' \
  "${passenger_application_count}" \
  "${passenger_application_test_count}"
printf '  stored SSL CSRs: %s (%s test-managed)\n' \
  "${ssl_csr_count}" \
  "${ssl_csr_test_count}"
printf '  stored SSL certificates: %s (%s test-managed)\n' \
  "${ssl_certificate_count}" \
  "${ssl_certificate_test_count}"
printf '  GPG public keys: %s (%s test-managed)\n' \
  "${gpg_public_key_count}" \
  "${gpg_public_test_key_count}"
printf '  GPG secret keys: %s (%s test-managed)\n' \
  "${gpg_secret_key_count}" \
  "${gpg_secret_test_key_count}"
printf '  SSH public keys: %s (%s test-managed)\n' \
  "${ssh_public_key_count}" \
  "${ssh_public_test_key_count}"
printf '  public_html directory index: %s\n' "${public_html_index_type}"
printf '  public_html directory protected: %s\n' "${public_html_protected}"
printf '  PostgreSQL databases: %s\n' "${database_count}"
printf '  PostgreSQL users: %s\n' "${user_count}"
printf '  MySQL databases: %s (%s test-managed)\n' \
  "${mysql_database_count}" \
  "${mysql_test_database_count}"
printf '  MySQL users: %s (%s test-managed)\n' \
  "${mysql_user_count}" \
  "${mysql_test_user_count}"
printf '  remote MySQL hosts: %s (%s test-managed)\n' \
  "${mysql_remote_host_count}" \
  "${mysql_test_remote_host_count}"
printf '  calendar delegates: %s (%s test-managed)\n' \
  "${calendar_delegate_count}" \
  "${calendar_test_delegate_count}"
printf '  email accounts: %s (%s suspended, %s test-managed, %s test-suspended)\n' \
  "${email_account_count}" \
  "${email_suspended_count}" \
  "${email_test_account_count}" \
  "${email_test_suspended_count}"
printf '  BoxTrapper accounts: %s (%s enabled, %s test-managed, %s test-enabled, %s test-queued messages)\n' \
  "${boxtrapper_account_count}" \
  "${boxtrapper_enabled_count}" \
  "${boxtrapper_test_account_count}" \
  "${boxtrapper_test_enabled_count}" \
  "${boxtrapper_test_queued_message_count}"
printf '  email filters: %s (%s test-managed)\n' \
  "${email_filter_count}" \
  "${email_test_filter_count}"
printf '  account-level email filters: %s (%s test-managed)\n' \
  "${account_email_filter_count}" \
  "${account_email_test_filter_count}"
printf '  email mailing lists: %s (%s test-managed)\n' \
  "${email_mailing_list_count}" \
  "${email_test_mailing_list_count}"
printf '  email routing domains: %s (%s test non-auto)\n' \
  "${email_routing_count}" \
  "${email_test_non_auto_routing_count}"
printf '  email forwarders: %s (%s test-managed)\n' \
  "${email_forwarder_count}" \
  "${email_test_forwarder_count}"
printf '  email domain forwarders: %s (%s test-managed)\n' \
  "${email_domain_forwarder_count}" \
  "${email_test_domain_forwarder_count}"
printf '  email autoresponders: %s (%s test-managed)\n' \
  "${email_auto_responder_count}" \
  "${email_test_auto_responder_count}"
printf '  FTP accounts: %s (%s test-managed)\n' \
  "${ftp_account_count}" \
  "${ftp_test_account_count}"
printf '  IP blocks: %s (%s test-managed)\n' \
  "${ip_block_count}" \
  "${ip_test_block_count}"
printf '  DNS records: %s (%s test-managed)\n' \
  "${dns_record_count}" \
  "${dns_test_record_count}"
printf '  addon domains: %s (%s test-managed)\n' \
  "${addon_domain_count}" \
  "${addon_domain_test_count}"
printf '  domain aliases: %s (%s test-managed)\n' \
  "${domain_alias_count}" \
  "${domain_alias_test_count}"
printf '  subdomains: %s (%s test-managed)\n' \
  "${subdomain_count}" \
  "${subdomain_test_count}"
printf '  ModSecurity domains: %s (%s disabled, %s test-disabled)\n' \
  "${modsecurity_domain_count}" \
  "${modsecurity_disabled_count}" \
  "${modsecurity_test_disabled_count}"
printf '  test domain directories: %s\n' "${domain_test_directory_count}"
printf '  test filesystem text files: %s\n' \
  "${filesystem_text_file_test_count}"
printf '  filesystem text file markers: %s\n' \
  "${filesystem_text_file_marker_count}"
printf '  test Git repository directories: %s\n' \
  "${git_repository_test_directory_count}"
printf '  test Git trash entries: %s\n' \
  "${git_repository_test_trash_count}"
printf '  test Directory Privacy password directories: %s\n' \
  "${directory_privacy_test_directory_count}"
printf '  cron commands: %s\n' "${cron_command_count}"
printf '  cron variables: %s\n' "${cron_variable_count}"
