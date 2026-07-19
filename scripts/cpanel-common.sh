#!/usr/bin/env bash

cpanel_require_private_file() {
  local file="${1:-}"
  local label="${2:-cPanel configuration file}"
  local mode
  local owner

  if [[ ! -f "${file}" || -L "${file}" ]]; then
    printf '%s must be a regular file and not a symbolic link: %s\n' \
      "${label}" \
      "${file}" >&2
    return 1
  fi

  if stat -f '%u' "${file}" >/dev/null 2>&1; then
    owner="$(stat -f '%u' "${file}")"
    mode="$(stat -f '%Lp' "${file}")"
  else
    owner="$(stat -c '%u' "${file}")"
    mode="$(stat -c '%a' "${file}")"
  fi

  if [[ "${owner}" != "$(id -u)" ]]; then
    printf '%s must be owned by the current user: %s\n' \
      "${label}" \
      "${file}" >&2
    return 1
  fi
  if [[ ! "${mode}" =~ ^[0-7]{3,4}$ ]] ||
    ((8#${mode} & 077)); then
    printf '%s must not be readable, writable, or executable by group or others: %s\n' \
      "${label}" \
      "${file}" >&2
    return 1
  fi
}

cpanel_load_environment_file() {
  local file="${1:-}"
  local label="${2:-cPanel configuration file}"
  local key
  local parsed_file
  local value

  shift 2
  cpanel_require_private_file "${file}" "${label}"

  if ! command -v python3 >/dev/null 2>&1; then
    printf 'python3 is required to parse %s: %s\n' \
      "${label}" \
      "${file}" >&2
    return 1
  fi

  parsed_file="$(mktemp)"
  if ! python3 - "${file}" "${label}" "$@" >"${parsed_file}" <<'PYTHON'
import re
import shlex
import sys

file_name = sys.argv[1]
label = sys.argv[2]
allowed = set(sys.argv[3:])
assignment = re.compile(r"^([A-Za-z_][A-Za-z0-9_]*)=(.*)$")

with open(file_name, "r", encoding="utf-8") as environment_file:
    for line_number, raw_line in enumerate(environment_file, start=1):
        line = raw_line.strip()
        if not line or line.startswith("#"):
            continue

        match = assignment.fullmatch(line)
        if match is None:
            raise SystemExit(
                f"{label} contains an invalid assignment at "
                f"{file_name}:{line_number}"
            )

        key, raw_value = match.groups()
        if key not in allowed:
            raise SystemExit(
                f"{label} contains an unsupported variable {key} at "
                f"{file_name}:{line_number}"
            )

        if raw_value == "":
            value = ""
        else:
            lexer = shlex.shlex(raw_value, posix=True)
            lexer.whitespace_split = True
            lexer.commenters = ""
            values = list(lexer)
            if len(values) != 1:
                raise SystemExit(
                    f"{label} contains an invalid value for {key} at "
                    f"{file_name}:{line_number}"
                )
            value = values[0]

        sys.stdout.buffer.write(key.encode("utf-8") + b"\0")
        sys.stdout.buffer.write(value.encode("utf-8") + b"\0")
PYTHON
  then
    rm -f "${parsed_file}"
    return 1
  fi

  while IFS= read -r -d '' key &&
    IFS= read -r -d '' value; do
    printf -v "${key}" '%s' "${value}"
    export "${key?}"
  done <"${parsed_file}"
  rm -f "${parsed_file}"
}

cpanel_sha256_text() {
  local value="${1:-}"

  if command -v sha256sum >/dev/null 2>&1; then
    printf '%s' "${value}" | sha256sum | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then
    printf '%s' "${value}" | shasum -a 256 | awk '{print $1}'
  else
    printf '%s\n' 'Missing required command: sha256sum or shasum' >&2
    return 1
  fi
}

cpanel_acceptance_account_marker() {
  local host="${1:-}"
  local username="${2:-}"
  local api_token="${3:-}"

  cpanel_sha256_text \
    "terraform-provider-cpanel-acceptance-account-v1
${host%/}
${username}
${api_token}"
}

cpanel_require_expected_version() {
  local host="${1:-}"
  local expected_version="${2:-}"
  local response_file="${3:-}"
  local actual_version
  local http_code

  http_code="$(
    cpanel_curl \
      --silent \
      --show-error \
      --max-time 90 \
      --output "${response_file}" \
      --write-out '%{http_code}' \
      "${host%/}/execute/StatsBar/get_stats?display=cpanelversion"
  )"
  if [[ "${http_code}" != "200" ]] ||
    ! jq -e \
      '
        .status == 1
        and (.data | type == "array")
        and ([.data[] | select(.name == "cpanelversion")] | length == 1)
      ' \
      "${response_file}" >/dev/null; then
    printf 'Unable to verify the cPanel version before destructive operations\n' \
      >&2
    return 1
  fi

  actual_version="$(
    jq -r '.data[] | select(.name == "cpanelversion") | .value' \
      "${response_file}"
  )"
  if [[ "${actual_version}" != "${expected_version}" ]]; then
    printf 'Refusing destructive cPanel operations: server version is %s; expected %s\n' \
      "${actual_version}" \
      "${expected_version}" >&2
    return 1
  fi
}

cpanel_artifact_registered() {
  local file="${1:-}"
  local value="${2:-}"

  [[ -f "${file}" ]] &&
    jq -s -e \
      --arg value "${value}" \
      'any(.[]; .version == 1 and .kind == "value" and .value == $value)' \
      "${file}" >/dev/null
}

cpanel_api_token_artifact_registered() {
  local file="${1:-}"
  local token_identity="${2:-}"

  [[ -f "${file}" ]] &&
    jq -s -e \
      --argjson token_identity "${token_identity}" \
      'any(.[]; . == $token_identity)' \
      "${file}" >/dev/null
}

cpanel_api_token_candidate_registered() {
  local file="${1:-}"
  local token_identity="${2:-}"

  [[ -f "${file}" ]] &&
    jq -s -e \
      --argjson token_identity "${token_identity}" \
      '
        any(
          .[];
          .version == 1
          and .kind == "api_token_candidate"
          and .name == $token_identity.name
          and ($token_identity.created_at >= .not_before)
          and ($token_identity.created_at <= .not_after)
          and $token_identity.has_full_access == 1
          and $token_identity.features == []
          and $token_identity.whitelist_ips == []
        )
      ' \
      "${file}" >/dev/null
}

cpanel_cleanup_test_api_tokens() {
  local artifact_manifest="${1:-}"
  local active_token_name="${2:-}"
  local response_file="${3:-}"
  local deleted=0
  local token_identity
  local token_identities
  local token_name

  get_request 'execute/Tokens/list' 'API token inventory'
  if ! token_identities="$(
    jq -c \
      '
        if (.data | type) != "array" then
          error("API token inventory data is not an array")
        else
          .data[]
          | select(.name | startswith("tfcpaneltoken"))
          | {
              version: 1,
              kind: "api_token",
              name,
              created_at: (.create_time | tonumber),
              expires_at: (
                if .expires_at == null or .expires_at == "" then
                  0
                else
                  (.expires_at | tonumber)
                end
              ),
              has_full_access: (.has_full_access | tonumber),
              features: ((.features // []) | sort),
              whitelist_ips: ((.whitelist_ips // []) | sort)
            }
        end
      ' \
      "${response_file}"
  )"; then
    printf 'API token inventory is invalid\n' >&2
    return 1
  fi
  while IFS= read -r token_identity; do
    token_name="$(jq -r '.name' <<<"${token_identity}")"
    if [[ -z "${token_name}" ]]; then
      continue
    fi
    if [[ "${token_name}" == "${active_token_name}" ]]; then
      printf 'Refusing to revoke active provider token: %s\n' \
        "${token_name}" >&2
      return 1
    fi
    if ! cpanel_api_token_artifact_registered \
        "${artifact_manifest}" \
        "${token_identity}" &&
      ! cpanel_api_token_candidate_registered \
        "${artifact_manifest}" \
        "${token_identity}"; then
      printf 'Refusing cleanup of API token with unregistered identity: %s\n' \
        "${token_name}" >&2
      return 1
    fi
    uapi_post \
      'Tokens' \
      'revoke' \
      "Revoke test API token ${token_name}" \
      "name=${token_name}"
    deleted=$((deleted + 1))
  done <<<"${token_identities}"

  printf '%d\n' "${deleted}"
}

cpanel_dns_record_artifact_registered() {
  local file="${1:-}"
  local record_identity="${2:-}"

  [[ -f "${file}" ]] &&
    jq -s -e \
      --argjson record_identity "${record_identity}" \
      'any(.[]; . == $record_identity)' \
      "${file}" >/dev/null
}

cpanel_validate_artifact_manifest() {
  local file="${1:-}"
  local label="${2:-Acceptance artifact manifest}"
  local byte_count
  local line_count

  if [[ ! -f "${file}" ]]; then
    return
  fi

  byte_count="$(wc -c <"${file}" | tr -d '[:space:]')"
  line_count="$(wc -l <"${file}" | tr -d '[:space:]')"
  if [[ ! "${byte_count}" =~ ^[0-9]+$ ]] ||
    ((10#${byte_count} > 1048576)); then
    printf '%s exceeds the 1 MiB limit\n' "${label}" >&2
    return 1
  fi
  if [[ ! "${line_count}" =~ ^[0-9]+$ ]] ||
    ((10#${line_count} > 10000)); then
    printf '%s exceeds the 10000-record limit\n' "${label}" >&2
    return 1
  fi
  if ! awk 'length($0) > 4096 {exit 1}' "${file}"; then
    printf '%s contains a record exceeding 4096 bytes\n' "${label}" >&2
    return 1
  fi
  if ! jq -s -e \
    '
      def exact_keys($expected):
        (keys | sort) == ($expected | sort);
      def valid_record:
        .version == 1
        and (
          (
            .kind == "value"
            and exact_keys(["version", "kind", "value"])
            and (.value | type) == "string"
            and (.value | length) > 0
            and (.value | length) <= 2048
          )
          or (
            .kind == "dns_record"
            and exact_keys([
              "version", "kind", "zone", "name", "type", "ttl", "data"
            ])
            and (.zone | type) == "string"
            and (.zone | length) > 0
            and (.name | type) == "string"
            and (.name | length) > 0
            and (.type | type) == "string"
            and (.type | test("^[A-Z][A-Z0-9]*$"))
            and (.ttl | type) == "number"
            and .ttl >= 0
            and (.data | type) == "array"
            and all(.data[]; type == "string")
          )
          or (
            .kind == "api_token"
            and exact_keys([
              "version", "kind", "name", "created_at", "expires_at",
              "has_full_access", "features", "whitelist_ips"
            ])
            and (.name | type) == "string"
            and (.name | test("^tfcpaneltoken[a-z0-9]+$"))
            and (.created_at | type) == "number"
            and .created_at > 0
            and (.expires_at | type) == "number"
            and .expires_at >= 0
            and (.has_full_access == 0 or .has_full_access == 1)
            and (.features | type) == "array"
            and all(.features[]; type == "string")
            and (.whitelist_ips | type) == "array"
            and all(.whitelist_ips[]; type == "string")
          )
          or (
            .kind == "api_token_candidate"
            and exact_keys([
              "version", "kind", "name", "not_before", "not_after"
            ])
            and (.name | type) == "string"
            and (.name | test("^tfcpaneltoken[a-z0-9]+$"))
            and (.not_before | type) == "number"
            and .not_before > 0
            and (.not_after | type) == "number"
            and .not_after >= .not_before
          )
        );
      all(.[]; valid_record)
    ' \
    "${file}" >/dev/null; then
    printf '%s contains an invalid artifact record\n' "${label}" >&2
    return 1
  fi
}

cpanel_ftp_home_inventory_is_safe_to_delete() {
  local file="${1:-}"

  jq -e \
    '
      (.data | type) == "array"
      and (
        (.data | length) == 0
        or (
          (.data | length) == 1
          and .data[0].file == ".ftpquota"
          and .data[0].type == "file"
          and ((.data[0].size | tonumber?) == 4)
        )
      )
    ' \
    "${file}" >/dev/null
}

cpanel_ftp_quota_file_is_empty() {
  local file="${1:-}"

  jq -e \
    '.data.content == "0 0\n"' \
    "${file}" >/dev/null
}

cpanel_ssl_certificate_cleanup_artifact() {
  local file="${1:-}"
  local certificate_id="${2:-}"

  jq -e -r \
    --arg id "${certificate_id}" \
    '
      def automatic_test_certificate:
        .friendly_name
        | try capture(
            "^Cert for \u201c(?<domain>tfcpanel(?:sub|addon)[a-z0-9.-]+)\u201d$"
          )
          catch null;
      [
        .data[]
        | select(
            ((.id? | type) == "string" or (.id? | type) == "number")
            and ((.id | tostring) == $id)
          )
      ] as $matches
      | select(($matches | length) == 1)
      | $matches[0]
      | if (
          (.friendly_name? | type) == "string"
          and (.friendly_name | startswith("tfcpanelsslcert"))
        ) then
          .friendly_name
        elif ((automatic_test_certificate | type) == "object") then
          (automatic_test_certificate).domain
        elif (
          ((.id? | type) == "string" or (.id? | type) == "number")
          and (.id | tostring | startswith("tfcpanel"))
        ) then
          (.id | tostring)
        else
          empty
        end
      | select(type == "string" and length > 0)
    ' \
    "${file}"
}

cpanel_validate_host() {
  local host="${1:-}"
  local authority
  local hostname
  local port=""
  local label

  if [[
    "${host}" != https://*
    || "${host}" == *'?'*
    || "${host}" == *'#'*
    || "${host}" =~ [[:space:]]
  ]]; then
    printf '%s\n' \
      'CPANEL_HOST must be an HTTPS URL without a query, fragment, or whitespace.' >&2
    return 1
  fi

  authority="${host#https://}"
  authority="${authority%%/*}"
  if [[ -z "${authority}" || "${authority}" == *'@'* ]]; then
    printf '%s\n' \
      'CPANEL_HOST must include a hostname and must not contain credentials.' >&2
    return 1
  fi

  if [[ "${authority}" == \[* ]]; then
    if [[ ! "${authority}" =~ ^\[([0-9A-Fa-f:.]+)\](:([0-9]+))?$ ]]; then
      printf '%s\n' \
        'CPANEL_HOST must include a valid hostname and optional numeric port.' >&2
      return 1
    fi
    hostname="${BASH_REMATCH[1]}"
    port="${BASH_REMATCH[3]:-}"
    if [[ "${hostname}" != *:* ]]; then
      printf '%s\n' \
        'CPANEL_HOST contains an invalid bracketed hostname.' >&2
      return 1
    fi
    if ! command -v python3 >/dev/null 2>&1; then
      printf '%s\n' \
        'python3 is required to validate a bracketed IPv6 CPANEL_HOST.' >&2
      return 1
    fi
    if ! python3 -c \
        'import ipaddress, sys; ipaddress.IPv6Address(sys.argv[1])' \
        "${hostname}" >/dev/null 2>&1; then
      printf '%s\n' \
        'CPANEL_HOST contains an invalid IPv6 address.' >&2
      return 1
    fi
  else
    if [[ "${authority}" == *:* ]]; then
      if [[ "${authority}" != *:* || "${authority#*:}" == *:* ]]; then
        printf '%s\n' \
          'CPANEL_HOST must bracket IPv6 addresses.' >&2
        return 1
      fi
      hostname="${authority%%:*}"
      port="${authority#*:}"
    else
      hostname="${authority}"
    fi

    if [[ -z "${hostname}" || ! "${hostname}" =~ ^[A-Za-z0-9.-]+$ ]]; then
      printf '%s\n' \
        'CPANEL_HOST must include a valid hostname.' >&2
      return 1
    fi
    IFS='.' read -r -a labels <<<"${hostname}"
    for label in "${labels[@]}"; do
      if [[
        -z "${label}"
        || ! "${label}" =~ ^[A-Za-z0-9]([A-Za-z0-9-]*[A-Za-z0-9])?$
      ]]; then
        printf '%s\n' \
          'CPANEL_HOST must include a valid hostname.' >&2
        return 1
      fi
    done
  fi

  if [[ -n "${port}" ]]; then
    if [[ ! "${port}" =~ ^[0-9]+$ ]] ||
      ((10#${port} < 1 || 10#${port} > 65535)); then
      printf '%s\n' \
        'CPANEL_HOST port must be an integer between 1 and 65535.' >&2
      return 1
    fi
  elif [[ "${authority}" == *: ]]; then
    printf '%s\n' \
      'CPANEL_HOST port must not be empty.' >&2
    return 1
  fi
}

cpanel_curl() {
  local authorization

  for variable in CPANEL_USERNAME CPANEL_API_TOKEN; do
    if [[ -z "${!variable:-}" || "${!variable}" == *$'\n'* || "${!variable}" == *$'\r'* ]]; then
      printf 'Invalid cPanel credential variable: %s\n' "${variable}" >&2
      return 1
    fi
  done

  authorization="Authorization: cpanel ${CPANEL_USERNAME}:${CPANEL_API_TOKEN}"
  authorization="${authorization//\\/\\\\}"
  authorization="${authorization//\"/\\\"}"
  printf 'header = "%s"\n' "${authorization}" |
    command curl --config - "$@"
}
