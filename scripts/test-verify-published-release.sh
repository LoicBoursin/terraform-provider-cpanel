#!/usr/bin/env bash

set -euo pipefail

script_directory="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repository_directory="$(cd "${script_directory}/.." && pwd)"
temporary_directory="$(mktemp -d)"
fixture_directory="${temporary_directory}/assets"
fake_bin_directory="${temporary_directory}/bin"
release_changelog="${temporary_directory}/CHANGELOG.md"

cleanup() {
  rm -rf "${temporary_directory}"
}
trap cleanup EXIT

mkdir -p "${fixture_directory}" "${fake_bin_directory}"
printf '# Changelog\n\n## 1.0.0 - 2026-07-20\n' >"${release_changelog}"
targets=(
  darwin_amd64
  darwin_arm64
  freebsd_386
  freebsd_amd64
  freebsd_arm
  freebsd_arm64
  linux_386
  linux_amd64
  linux_arm
  linux_arm64
  windows_386
  windows_amd64
  windows_arm64
)

write_valid_sbom() {
  local target="$1"
  local archive_name="terraform-provider-cpanel_1.0.0_${target}.zip"

  jq -n \
    --arg name "${archive_name}" \
    '
      {
        spdxVersion: "SPDX-2.3",
        dataLicense: "CC0-1.0",
        SPDXID: "SPDXRef-DOCUMENT",
        name: $name,
        documentNamespace: "https://example.test/fixture",
        creationInfo: {
          creators: ["Tool: release-fixture"],
          created: "2026-07-20T00:00:00Z"
        },
        packages: [
          {
            name: "provider",
            SPDXID: "SPDXRef-Package-provider",
            downloadLocation: "NOASSERTION",
            filesAnalyzed: false,
            checksums: [
              {
                algorithm: "SHA256",
                checksumValue: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
              }
            ],
            licenseConcluded: "NOASSERTION",
            licenseDeclared: "NOASSERTION",
            copyrightText: "NOASSERTION"
          }
        ],
        files: [
          {
            fileName: "terraform-provider-cpanel_v1.0.0",
            SPDXID: "SPDXRef-File-provider",
            fileTypes: ["APPLICATION", "BINARY"],
            checksums: [
              {
                algorithm: "SHA256",
                checksumValue: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
              }
            ],
            licenseConcluded: "NOASSERTION",
            licenseInfoInFiles: ["NOASSERTION"],
            copyrightText: "NOASSERTION"
          }
        ],
        relationships: [
          {
            spdxElementId: "SPDXRef-DOCUMENT",
            relationshipType: "DESCRIBES",
            relatedSpdxElement: "SPDXRef-Package-provider"
          }
        ]
      }
    ' \
    >"${fixture_directory}/${archive_name}.sbom.json"
}

for target in "${targets[@]}"; do
  staging_directory="${temporary_directory}/staging-${target}"
  mkdir -p "${staging_directory}"
  cp \
    "${repository_directory}/CHANGELOG.md" \
    "${repository_directory}/LICENSE" \
    "${repository_directory}/README.md" \
    "${staging_directory}/"
  binary="terraform-provider-cpanel_v1.0.0"
  if [[ "${target}" == windows_* ]]; then
    binary+='.exe'
  fi
  printf 'provider binary fixture\n' >"${staging_directory}/${binary}"
  (
    cd "${staging_directory}"
    zip -q \
      "${fixture_directory}/terraform-provider-cpanel_1.0.0_${target}.zip" \
      "${binary}" \
      CHANGELOG.md \
      LICENSE \
      README.md
  )
  write_valid_sbom "${target}"
done

cp \
  "${repository_directory}/terraform-registry-manifest.json" \
  "${fixture_directory}/terraform-provider-cpanel_1.0.0_manifest.json"

checksum_file="${fixture_directory}/terraform-provider-cpanel_1.0.0_SHA256SUMS"
write_checksums() {
  (
    cd "${fixture_directory}"
    if command -v sha256sum >/dev/null 2>&1; then
      sha256sum -- \
        *.zip \
        *.sbom.json \
        *_manifest.json >"${checksum_file}"
    else
      shasum -a 256 -- \
        *.zip \
        *.sbom.json \
        *_manifest.json >"${checksum_file}"
    fi
  )
}
rename_release_prefix() {
  local source_prefix="${fixture_directory}/$1"
  local destination_prefix="${fixture_directory}/$2"
  local file
  local suffix

  for file in "${source_prefix}"*; do
    suffix="${file#"${source_prefix}"}"
    mv "${file}" "${destination_prefix}${suffix}"
  done
}
write_checksums
printf 'detached signature fixture\n' >"${checksum_file}.sig"

cat >"${fake_bin_directory}/gh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
destination=''
while (($# > 0)); do
  if [[ "$1" == '--dir' ]]; then
    destination="$2"
    shift 2
    continue
  fi
  shift
done
cp "${PUBLISHED_RELEASE_FIXTURE_DIR}"/* "${destination}/"
EOF
cat >"${fake_bin_directory}/gpg" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
if [[ "$1" != '--batch' || "$2" != '--verify' ]]; then
  exit 1
fi
if [[ "${PUBLISHED_RELEASE_GPG_FAIL:-0}" == '1' ]]; then
  exit 1
fi
printf 'verified\n' >"${PUBLISHED_RELEASE_GPG_MARKER}"
EOF
chmod 0700 "${fake_bin_directory}/gh" "${fake_bin_directory}/gpg"

export GITHUB_REPOSITORY='LoicBoursin/terraform-provider-cpanel'
export CHANGELOG_PATH="${release_changelog}"
export PUBLISHED_RELEASE_FIXTURE_DIR="${fixture_directory}"
export PUBLISHED_RELEASE_GPG_MARKER="${temporary_directory}/gpg-verified"
PATH="${fake_bin_directory}:${PATH}" \
  "${script_directory}/verify-published-release.sh" v1.0.0 >/dev/null

if [[ ! -f "${PUBLISHED_RELEASE_GPG_MARKER}" ]]; then
  printf 'Published release verification did not verify the signature\n' >&2
  exit 1
fi

if GITHUB_REF_NAME=v1.0.0 PATH="${fake_bin_directory}:${PATH}" \
  "${script_directory}/verify-published-release.sh" '' \
  >/dev/null 2>&1; then
  printf 'Published release verification accepted an explicitly empty tag\n' >&2
  exit 1
fi
GITHUB_REF_NAME=v1.0.0 PATH="${fake_bin_directory}:${PATH}" \
  "${script_directory}/verify-published-release.sh" >/dev/null

export PUBLISHED_RELEASE_GPG_FAIL=1
if PATH="${fake_bin_directory}:${PATH}" \
  "${script_directory}/verify-published-release.sh" v1.0.0 \
  >/dev/null 2>&1; then
  printf 'Published release verification accepted a rejected signature\n' >&2
  exit 1
fi
unset PUBLISHED_RELEASE_GPG_FAIL

rename_release_prefix \
  'terraform-provider-cpanel_1.0.0' \
  'wrong-provider_9.9.9'
checksum_file="${fixture_directory}/wrong-provider_9.9.9_SHA256SUMS"
write_checksums
if PATH="${fake_bin_directory}:${PATH}" \
  "${script_directory}/verify-published-release.sh" v1.0.0 \
  >/dev/null 2>&1; then
  printf 'Published release verification accepted assets for a different release\n' >&2
  exit 1
fi
rename_release_prefix \
  'wrong-provider_9.9.9' \
  'terraform-provider-cpanel_1.0.0'
checksum_file="${fixture_directory}/terraform-provider-cpanel_1.0.0_SHA256SUMS"
write_checksums

archive_prefix="${fixture_directory}/terraform-provider-cpanel_1.0.0"
archive_file="${archive_prefix}_darwin_amd64.zip"
archive_backup="${temporary_directory}/darwin-amd64.zip"
staging_directory="${temporary_directory}/staging-darwin_amd64"
cp "${archive_file}" "${archive_backup}"
mv \
  "${staging_directory}/terraform-provider-cpanel_v1.0.0" \
  "${staging_directory}/terraform-provider-cpanel_v9.9.9"
rm -f "${archive_file}"
(
  cd "${staging_directory}"
  zip -q \
    "${archive_file}" \
    terraform-provider-cpanel_v9.9.9 \
    CHANGELOG.md \
    LICENSE \
    README.md
)
write_checksums
if PATH="${fake_bin_directory}:${PATH}" \
  "${script_directory}/verify-published-release.sh" v1.0.0 \
  >/dev/null 2>&1; then
  printf 'Published release verification accepted a mismatched binary version\n' >&2
  exit 1
fi
mv \
  "${staging_directory}/terraform-provider-cpanel_v9.9.9" \
  "${staging_directory}/terraform-provider-cpanel_v1.0.0"
cp "${archive_backup}" "${archive_file}"
write_checksums

rm -f "${fixture_directory}/terraform-provider-cpanel_1.0.0_manifest.json"
if PATH="${fake_bin_directory}:${PATH}" \
  "${script_directory}/verify-published-release.sh" v1.0.0 \
  >/dev/null 2>&1; then
  printf 'Published release verification accepted a missing Registry manifest\n' >&2
  exit 1
fi

cp \
  "${repository_directory}/terraform-registry-manifest.json" \
  "${fixture_directory}/terraform-provider-cpanel_1.0.0_manifest.json"
mv \
  "${archive_prefix}_darwin_amd64.zip" \
  "${archive_prefix}_solaris_amd64.zip"
mv \
  "${archive_prefix}_darwin_amd64.zip.sbom.json" \
  "${archive_prefix}_solaris_amd64.zip.sbom.json"
write_checksums
if PATH="${fake_bin_directory}:${PATH}" \
  "${script_directory}/verify-published-release.sh" v1.0.0 \
  >/dev/null 2>&1; then
  printf 'Published release verification accepted an unsupported target\n' >&2
  exit 1
fi
mv \
  "${archive_prefix}_solaris_amd64.zip" \
  "${archive_prefix}_darwin_amd64.zip"
mv \
  "${archive_prefix}_solaris_amd64.zip.sbom.json" \
  "${archive_prefix}_darwin_amd64.zip.sbom.json"

sbom_file="${archive_prefix}_darwin_amd64.zip.sbom.json"
jq '
  .relationships = [{
    "spdxElementId": "SPDXRef-DOCUMENT",
    "relationshipType": "DESCRIBES",
    "relatedSpdxElement": "SPDXRef-Package-missing"
  }]
' \
  "${sbom_file}" >"${sbom_file}.invalid"
mv "${sbom_file}.invalid" "${sbom_file}"
write_checksums
if PATH="${fake_bin_directory}:${PATH}" \
  "${script_directory}/verify-published-release.sh" v1.0.0 \
  >/dev/null 2>&1; then
  printf 'Published release verification accepted an invalid SPDX relationship\n' >&2
  exit 1
fi
write_valid_sbom 'darwin_amd64'

jq '.name = "wrong-archive.zip"' \
  "${sbom_file}" >"${sbom_file}.invalid"
mv "${sbom_file}.invalid" "${sbom_file}"
write_checksums
if PATH="${fake_bin_directory}:${PATH}" \
  "${script_directory}/verify-published-release.sh" v1.0.0 \
  >/dev/null 2>&1; then
  printf 'Published release verification accepted a mismatched SBOM name\n' >&2
  exit 1
fi
write_valid_sbom 'darwin_amd64'

write_checksums
awk 'NR == 1 { first = $0 } NR == 2 { $0 = first } { print }' \
  "${checksum_file}" >"${checksum_file}.duplicate"
mv "${checksum_file}.duplicate" "${checksum_file}"
if PATH="${fake_bin_directory}:${PATH}" \
  "${script_directory}/verify-published-release.sh" v1.0.0 \
  >/dev/null 2>&1; then
  printf 'Published release verification accepted duplicate checksum entries\n' >&2
  exit 1
fi
