#!/usr/bin/env bash
#
# build-pf.sh — build the privacy-filter.cpp `pf-cli` binary inside a Docker
# container and install it into the pfcheck cache so pfcheck can find it.
#
# Usage:
#   scripts/build-pf.sh [output-dir]
#
# Environment:
#   PF_REF        Git ref of privacy-filter.cpp to build (default: master)
#   PF_REPO       Git repository URL (default: upstream)
#   PFCHECK_CACHE_DIR  Override the pfcheck cache directory
#   DOCKER        Container CLI to use (default: docker)
#
# Without an output-dir argument the binary is installed to:
#   ${PFCHECK_CACHE_DIR:-$HOME/.cache/pfcheck}/bin/pf-cli
# which is on pfcheck's binary search path.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

DOCKER="${DOCKER:-docker}"
PF_REF="${PF_REF:-master}"
PF_REPO="${PF_REPO:-https://github.com/localai-org/privacy-filter.cpp}"
IMAGE_TAG="pfcheck/pf-cli:${PF_REF}"

# Resolve the install directory.
if [[ $# -ge 1 ]]; then
    OUT_DIR="$1"
else
    CACHE_DIR="${PFCHECK_CACHE_DIR:-${XDG_CACHE_HOME:-$HOME/.cache}/pfcheck}"
    OUT_DIR="${CACHE_DIR}/bin"
fi

echo ">> Building ${IMAGE_TAG} (ref=${PF_REF})"
"${DOCKER}" build \
    --target runtime \
    --build-arg "PF_REPO=${PF_REPO}" \
    --build-arg "PF_REF=${PF_REF}" \
    -t "${IMAGE_TAG}" \
    -f "${REPO_ROOT}/Dockerfile.pf" \
    "${REPO_ROOT}"

echo ">> Extracting pf-cli to ${OUT_DIR}"
mkdir -p "${OUT_DIR}"
# Use the export stage to write the binary directly to the host.
"${DOCKER}" build \
    --target export \
    --build-arg "PF_REPO=${PF_REPO}" \
    --build-arg "PF_REF=${PF_REF}" \
    --output "type=local,dest=${OUT_DIR}" \
    -f "${REPO_ROOT}/Dockerfile.pf" \
    "${REPO_ROOT}"

chmod +x "${OUT_DIR}/pf-cli"
echo ">> Installed: ${OUT_DIR}/pf-cli"
"${OUT_DIR}/pf-cli" --info 2>/dev/null || true
echo ">> Done. pfcheck will pick this up automatically (or set PFCHECK_PF_CLI=${OUT_DIR}/pf-cli)."
