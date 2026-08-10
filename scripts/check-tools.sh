#!/usr/bin/env bash

set -euo pipefail

script_directory="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repository_directory="$(cd "${script_directory}/.." && pwd)"
tools_directory="${TOOLS_DIR:-${repository_directory}/.git/tools}"

# shellcheck source=tool-versions.sh
source "${script_directory}/tool-versions.sh"

check_release_tools=0
case "${1:-}" in
  "")
    ;;
  --release)
    check_release_tools=1
    ;;
  *)
    printf 'Unsupported argument: %s\n' "$1" >&2
    exit 1
    ;;
esac

required_tools=(
  actionlint
  gitleaks
  golangci-lint
  goreleaser
  govulncheck
  shellcheck
)
if [[ "${check_release_tools}" == "1" ]]; then
  required_tools+=(syft)
fi

for tool in "${required_tools[@]}"; do
  if [[ ! -x "${tools_directory}/${tool}" ]]; then
    printf 'Missing pinned tool: %s. Run make tools.\n' "${tool}" >&2
    exit 1
  fi
done

actionlint_output="$("${tools_directory}/actionlint" -version 2>&1)"
actionlint_actual="${actionlint_output%%$'\n'*}"
if [[ "${actionlint_actual}" != "v${actionlint_version}" ]]; then
  printf 'Unexpected actionlint version: %s; expected v%s. Run make tools.\n' \
    "${actionlint_actual}" \
    "${actionlint_version}" >&2
  exit 1
fi

gitleaks_actual="$("${tools_directory}/gitleaks" version 2>&1)"
if [[ "${gitleaks_actual}" != "${gitleaks_version}" ]]; then
  printf 'Unexpected Gitleaks version: %s; expected %s. Run make tools.\n' \
    "${gitleaks_actual:-unknown}" \
    "${gitleaks_version}" >&2
  exit 1
fi

golangci_lint_output="$("${tools_directory}/golangci-lint" version 2>&1)"
if ! grep -Fq \
  "golangci-lint has version ${golangci_lint_version} " \
  <<<"${golangci_lint_output}"; then
  printf 'Unexpected golangci-lint version; expected %s. Run make tools.\n' \
    "${golangci_lint_version}" >&2
  exit 1
fi

goreleaser_output="$("${tools_directory}/goreleaser" --version 2>&1)"
goreleaser_actual="$(
  awk '$1 == "GitVersion:" {print $2}' <<<"${goreleaser_output}"
)"
if [[ "${goreleaser_actual}" != "v${goreleaser_version}" ]]; then
  printf 'Unexpected GoReleaser version: %s; expected v%s. Run make tools.\n' \
    "${goreleaser_actual:-unknown}" \
    "${goreleaser_version}" >&2
  exit 1
fi

govulncheck_output="$("${tools_directory}/govulncheck" -version 2>&1)"
if ! grep -Fxq \
  "Scanner: govulncheck@v${govulncheck_version}" \
  <<<"${govulncheck_output}"; then
  printf 'Unexpected govulncheck version; expected v%s. Run make tools.\n' \
    "${govulncheck_version}" >&2
  exit 1
fi

shellcheck_actual="$("${tools_directory}/shellcheck" --version 2>&1 | awk '$1 == "version:" {print $2}')"
if [[ "${shellcheck_actual}" != "${shellcheck_version}" ]]; then
  printf 'Unexpected ShellCheck version: %s; expected %s. Run make tools.\n' \
    "${shellcheck_actual:-unknown}" \
    "${shellcheck_version}" >&2
  exit 1
fi

if [[ "${check_release_tools}" == "1" ]]; then
  syft_output="$("${tools_directory}/syft" version 2>&1)"
  syft_actual="$(awk '$1 == "Version:" {print $2}' <<<"${syft_output}")"
  if [[ "${syft_actual}" != "${syft_version}" ]]; then
    printf 'Unexpected Syft version: %s; expected %s. Run make tools.\n' \
      "${syft_actual:-unknown}" \
      "${syft_version}" >&2
    exit 1
  fi
fi
