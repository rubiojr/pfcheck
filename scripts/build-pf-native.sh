#!/usr/bin/env bash
#
# build-pf-native.sh — build the privacy-filter.cpp `pf-cli` binary natively on
# the host (no Docker) using CMake, and install it where pfcheck can find it.
#
# This is the build path for macOS (and any host with a C++ toolchain). It
# produces a binary for the host OS/arch, unlike scripts/build-pf.sh which
# always builds a Linux binary inside Docker.
#
# Usage:
#   scripts/build-pf-native.sh [output-dir]
#
# Requirements: git, cmake (>=3.21), and a C++17 compiler (Xcode CLT on macOS).
#
# Environment:
#   PF_REF             Git ref of privacy-filter.cpp to build (default: master)
#   PF_REPO            Git repository URL (default: upstream)
#   PF_SRC_DIR         Where to clone/build the sources (default: a temp dir)
#   PFCHECK_CACHE_DIR  Override the pfcheck cache directory
#
# Without an output-dir argument the binary is installed to <cache>/bin/pf-cli,
# where <cache> matches Go's os.UserCacheDir() for the host OS.
set -euo pipefail

PF_REF="${PF_REF:-master}"
PF_REPO="${PF_REPO:-https://github.com/localai-org/privacy-filter.cpp}"
PF_SRC_DIR="${PF_SRC_DIR:-${TMPDIR:-/tmp}/pfcheck-pf-src}"

# cache_dir mirrors Go's os.UserCacheDir() so pfcheck's ResolveBinary() finds
# the installed binary.
cache_dir() {
    if [[ -n "${PFCHECK_CACHE_DIR:-}" ]]; then
        printf '%s' "${PFCHECK_CACHE_DIR}"
        return
    fi
    case "$(uname -s)" in
        Darwin) printf '%s' "${HOME}/Library/Caches/pfcheck" ;;
        *)      printf '%s' "${XDG_CACHE_HOME:-${HOME}/.cache}/pfcheck" ;;
    esac
}

job_count() {
    getconf _NPROCESSORS_ONLN 2>/dev/null \
        || sysctl -n hw.ncpu 2>/dev/null \
        || nproc 2>/dev/null \
        || echo 4
}

# Resolve the install directory.
if [[ $# -ge 1 ]]; then
    OUT_DIR="$1"
else
    OUT_DIR="$(cache_dir)/bin"
fi

for tool in git cmake c++; do
    if ! command -v "${tool}" >/dev/null 2>&1; then
        echo "error: '${tool}' not found. Install git, CMake and a C++17 compiler" >&2
        echo "       (on macOS: 'xcode-select --install' and 'brew install cmake')." >&2
        exit 1
    fi
done

echo ">> Fetching privacy-filter.cpp (ref=${PF_REF}) into ${PF_SRC_DIR}"
if [[ -d "${PF_SRC_DIR}/.git" ]]; then
    git -C "${PF_SRC_DIR}" fetch --depth 1 origin "${PF_REF}"
    git -C "${PF_SRC_DIR}" checkout -q FETCH_HEAD
    git -C "${PF_SRC_DIR}" submodule update --init --recursive
else
    rm -rf "${PF_SRC_DIR}"
    git clone --recursive --depth 1 --branch "${PF_REF}" "${PF_REPO}" "${PF_SRC_DIR}"
fi

BUILD_DIR="${PF_SRC_DIR}/build/release"
echo ">> Configuring (CPU backend, static ggml)"
# Mirror the Docker build: statically link ggml so the binary is self-contained
# and disable -march=native for portability. Metal is disabled so no .metallib
# is needed at runtime (pf-cli runs on CPU by default).
cmake -S "${PF_SRC_DIR}" -B "${BUILD_DIR}" \
    -DCMAKE_BUILD_TYPE=Release \
    -DBUILD_SHARED_LIBS=OFF \
    -DGGML_NATIVE=OFF \
    -DGGML_METAL=OFF \
    -DPF_BUILD_TESTS=OFF

echo ">> Building pf-cli"
cmake --build "${BUILD_DIR}" --target pf-cli -j "$(job_count)"

PF_BIN="$(find "${BUILD_DIR}" -name pf-cli -type f | head -n1)"
if [[ -z "${PF_BIN}" ]]; then
    echo "error: pf-cli not found after build" >&2
    exit 1
fi

echo ">> Installing pf-cli to ${OUT_DIR}"
mkdir -p "${OUT_DIR}"
cp "${PF_BIN}" "${OUT_DIR}/pf-cli"
chmod +x "${OUT_DIR}/pf-cli"
strip "${OUT_DIR}/pf-cli" 2>/dev/null || true

echo ">> Installed: ${OUT_DIR}/pf-cli"
"${OUT_DIR}/pf-cli" --info 2>/dev/null || true
echo ">> Done. pfcheck will pick this up automatically (or set PFCHECK_PF_CLI=${OUT_DIR}/pf-cli)."
