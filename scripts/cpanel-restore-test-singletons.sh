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

for variable in \
  CPANEL_HOST \
  CPANEL_USERNAME \
  CPANEL_API_TOKEN \
  CPANEL_EXPECTED_LOCALE \
  CPANEL_EXPECTED_LOG_ARCHIVE \
  CPANEL_EXPECTED_LOG_PRUNE \
  CPANEL_EXPECTED_LOG_RETENTION \
  CPANEL_EXPECTED_NOTIFICATION_PREFERENCES \
  CPANEL_EXPECTED_SPAM_PREFERENCES; do
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

if [[ ! "${CPANEL_EXPECTED_LOCALE}" =~ ^[a-z][a-z0-9_]{0,63}$ ]]; then
  printf 'Invalid expected cPanel locale: %s\n' \
    "${CPANEL_EXPECTED_LOCALE}" >&2
  exit 1
fi
for variable in CPANEL_EXPECTED_LOG_ARCHIVE CPANEL_EXPECTED_LOG_PRUNE; do
  if [[ ! "${!variable}" =~ ^[01]$ ]]; then
    printf 'Invalid expected cPanel log flag %s: %s\n' \
      "${variable}" \
      "${!variable}" >&2
    exit 1
  fi
done
if [[
  ! "${CPANEL_EXPECTED_LOG_RETENTION}" =~ ^(-1|0|[1-9][0-9]*)$
 ]]; then
  printf 'Invalid expected cPanel log retention: %s\n' \
    "${CPANEL_EXPECTED_LOG_RETENTION}" >&2
  exit 1
fi
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
    '
      with_entries(.value |= sort)
    ' <<<"${CPANEL_EXPECTED_SPAM_PREFERENCES}"
)"

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
    return 1
  fi

  status="$(jq -r '.status // empty' "${response_file}")"
  if [[ "${status}" != "1" ]]; then
    printf '%s failed: %s\n' \
      "${label}" \
      "$(jq -c '{errors, messages}' "${response_file}")" >&2
    return 1
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
    return 1
  fi

  status="$(jq -r '.status // empty' "${response_file}")"
  if [[ "${status}" != "1" ]]; then
    printf '%s failed: %s\n' \
      "${label}" \
      "$(jq -c '{errors, messages}' "${response_file}")" >&2
    return 1
  fi
}

uapi_json_post() {
  local module="$1"
  local function="$2"
  local label="$3"
  local payload="$4"
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
      --header 'Content-Type: application/json' \
      --data-binary "${payload}" \
      "${host}/execute/${module}/${function}"
  )"

  if [[ "${http_code}" != "200" ]]; then
    printf '%s failed with HTTP %s\n' "${label}" "${http_code}" >&2
    return 1
  fi

  status="$(jq -r '.status // empty' "${response_file}")"
  if [[ "${status}" != "1" ]]; then
    printf '%s failed: %s\n' \
      "${label}" \
      "$(jq -c '{errors, messages}' "${response_file}")" >&2
    return 1
  fi
}

read_log_settings() {
  if ! get_request \
    'execute/LogManager/get_settings' \
    'cPanel log settings inventory'; then
    return 1
  fi
  if ! jq -e \
    '
      ((.data.archive_logs | tostring) | test("^[01]$"))
      and ((.data.prune_archive | tostring) | test("^[01]$"))
      and ((.data.using_default | tostring) | test("^[01]$"))
      and ((.data.retention_days | tostring) | test("^[0-9]+$"))
    ' \
    "${response_file}" >/dev/null; then
    printf 'cPanel returned invalid log settings\n' >&2
    return 1
  fi

  current_log_archive="$(jq -r '.data.archive_logs | tostring' "${response_file}")"
  current_log_prune="$(jq -r '.data.prune_archive | tostring' "${response_file}")"
  current_log_retention="$(jq -r '.data.retention_days | tostring' "${response_file}")"
  if [[ "$(jq -r '.data.using_default | tostring' "${response_file}")" == "1" ]]; then
    current_log_retention="-1"
  fi
}

read_notification_preferences() {
  if ! get_request \
    'execute/ContactInformation/get_notification_preferences' \
    'cPanel notification preferences inventory'; then
    return 1
  fi
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
      )
    ' \
    "${response_file}" >/dev/null; then
    printf 'cPanel returned invalid notification preferences\n' >&2
    return 1
  fi

  current_notification_preferences="$(
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
}

read_spam_preferences() {
  if ! get_request \
    'execute/SpamAssassin/get_user_preferences' \
    'cPanel SpamAssassin preferences inventory'; then
    return 1
  fi
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
    return 1
  fi

  current_spam_preferences="$(
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
}

restored_locale=0
restored_log_settings=0
restored_notification_preferences=0
restored_spam_preferences=0

restore_locale() {
  local current_locale

  if ! get_request \
    'execute/Locale/get_attributes' \
    'cPanel locale inventory'; then
    return 1
  fi
  current_locale="$(jq -r '.data.locale // empty' "${response_file}")"
  if [[ -z "${current_locale}" ]]; then
    printf 'cPanel returned an empty locale\n' >&2
    return 1
  fi
  if [[ "${current_locale}" != "${CPANEL_EXPECTED_LOCALE}" ]]; then
    if ! uapi_post \
      'Locale' \
      'set_locale' \
      'Restore cPanel locale' \
      "locale=${CPANEL_EXPECTED_LOCALE}"; then
      return 1
    fi
    restored_locale=1
  fi

  if ! get_request \
    'execute/Locale/get_attributes' \
    'cPanel locale verification'; then
    return 1
  fi
  if [[ "$(jq -r '.data.locale // empty' "${response_file}")" != "${CPANEL_EXPECTED_LOCALE}" ]]; then
    printf 'cPanel locale was not restored exactly\n' >&2
    return 1
  fi
}

restore_log_settings() {
  if ! read_log_settings; then
    return 1
  fi
  if [[
    "${current_log_archive}" != "${CPANEL_EXPECTED_LOG_ARCHIVE}"
    || "${current_log_prune}" != "${CPANEL_EXPECTED_LOG_PRUNE}"
    || "${current_log_retention}" != "${CPANEL_EXPECTED_LOG_RETENTION}"
   ]]; then
    if ! uapi_post \
      'LogManager' \
      'set_settings' \
      'Restore cPanel log settings' \
      "archive_logs=${CPANEL_EXPECTED_LOG_ARCHIVE}" \
      "prune_archive=${CPANEL_EXPECTED_LOG_PRUNE}" \
      "retention_days=${CPANEL_EXPECTED_LOG_RETENTION}"; then
      return 1
    fi
    restored_log_settings=1
  fi

  if ! read_log_settings; then
    return 1
  fi
  if [[
    "${current_log_archive}" != "${CPANEL_EXPECTED_LOG_ARCHIVE}"
    || "${current_log_prune}" != "${CPANEL_EXPECTED_LOG_PRUNE}"
    || "${current_log_retention}" != "${CPANEL_EXPECTED_LOG_RETENTION}"
   ]]; then
    printf 'cPanel log settings were not restored exactly\n' >&2
    return 1
  fi
}

restore_notification_preferences() {
  local payload

  if ! read_notification_preferences; then
    return 1
  fi
  if [[ "${current_notification_preferences}" != "${expected_notification_preferences}" ]]; then
    payload="$(
      jq -c -n \
        --argjson preferences "${expected_notification_preferences}" \
        '
          {
            preferences: (
              $preferences
              | with_entries(.value = if .value then 1 else 0 end)
            )
          }
        '
    )"
    if ! uapi_json_post \
      'ContactInformation' \
      'set_notification_preferences' \
      'Restore cPanel notification preferences' \
      "${payload}"; then
      return 1
    fi
    restored_notification_preferences=1
  fi

  if ! read_notification_preferences; then
    return 1
  fi
  if [[ "${current_notification_preferences}" != "${expected_notification_preferences}" ]]; then
    printf 'cPanel notification preferences were not restored exactly\n' >&2
    return 1
  fi
}

restore_spam_preferences() {
  local current_present
  local expected_present
  local index
  local name
  local parameter
  local -a parameters
  local value
  local -a values

  if ! read_spam_preferences; then
    return 1
  fi
  for name in \
    blacklist_from \
    required_score \
    score \
    whitelist_from; do
    current_present="$(
      jq -r --arg name "${name}" 'has($name)' \
        <<<"${current_spam_preferences}"
    )"
    expected_present="$(
      jq -r --arg name "${name}" 'has($name)' \
        <<<"${expected_spam_preferences}"
    )"
    if [[
      "${current_present}" == "${expected_present}"
      && "$(
        jq -S -c --arg name "${name}" '.[$name] // []' \
          <<<"${current_spam_preferences}"
      )" == "$(
        jq -S -c --arg name "${name}" '.[$name] // []' \
          <<<"${expected_spam_preferences}"
      )"
     ]]; then
      continue
    fi

    parameters=("preference=${name}")
    values=()
    while IFS= read -r value; do
      values+=("${value}")
    done < <(
      jq -r --arg name "${name}" '.[$name][]?' \
        <<<"${expected_spam_preferences}"
    )
    if [[ "${#values[@]}" == "1" ]]; then
      parameters+=("value=${values[0]}")
    elif [[ "${#values[@]}" -gt "1" ]]; then
      for index in "${!values[@]}"; do
        parameter="value-${index}=${values[index]}"
        parameters+=("${parameter}")
      done
    fi

    if ! uapi_post \
      'SpamAssassin' \
      'update_user_preference' \
      "Restore cPanel SpamAssassin preference ${name}" \
      "${parameters[@]}"; then
      return 1
    fi
    restored_spam_preferences=1
  done

  if ! read_spam_preferences; then
    return 1
  fi
  if [[ "${current_spam_preferences}" != "${expected_spam_preferences}" ]]; then
    printf 'cPanel SpamAssassin preferences were not restored exactly\n' >&2
    return 1
  fi
}

set +e
restore_locale
locale_status=$?
restore_log_settings
log_settings_status=$?
restore_notification_preferences
notification_preferences_status=$?
restore_spam_preferences
spam_preferences_status=$?
set -e

if [[
  "${locale_status}" != "0"
  || "${log_settings_status}" != "0"
  || "${notification_preferences_status}" != "0"
  || "${spam_preferences_status}" != "0"
 ]]; then
  printf 'cPanel singleton restoration failed: locale=%d log_settings=%d notification_preferences=%d spam_preferences=%d\n' \
    "${locale_status}" \
    "${log_settings_status}" \
    "${notification_preferences_status}" \
    "${spam_preferences_status}" >&2
  exit 1
fi

printf 'cPanel singleton restoration passed\n'
printf '  locale restored: %d\n' "${restored_locale}"
printf '  log settings restored: %d\n' "${restored_log_settings}"
printf '  notification preferences restored: %d\n' \
  "${restored_notification_preferences}"
printf '  SpamAssassin preferences restored: %d\n' \
  "${restored_spam_preferences}"
