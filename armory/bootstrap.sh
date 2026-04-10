#!/usr/bin/env bash
#
# bootstrap.sh - Incremental Docker image builder
# Copyright (C) 2020 malzahar
#
# Inspired by laincloud/dockerfiles, simplified for personal use.
#
# Features:
#   - Incremental build based on git diff
#   - Support # TAGS comment in Dockerfile for multiple tags
#   - Auto-detect repo/tag from file path or # TAGS directive

set -euo pipefail

export REPO="${REPO:-kumquat/armory}"
export DOCKERFILE_DIFF=()
export IGNORE_ARRAY=()

# --- Logging ---

public::common::log() {
    echo -e "\033[0;32m[ $1 ]\033[0m"
}

public::common::err() {
    echo -e "\033[0;31m[ ERROR ] $1\033[0m" >&2
}

# --- Pre-flight checks ---

public::common::prepare() {
    local cmd
    for cmd in git docker; do
        if ! command -v "${cmd}" &>/dev/null; then
            public::common::err "${cmd} command not found"
            exit 1
        fi
    done
}

# --- Git diff: find changed Dockerfiles ---

private::git::diff() {
    if [[ ${COMMIT1:-} == "" && ${COMMIT2:-} == "" ]]; then
        local commits
        commits=$(git log -2 --pretty=format:"%h")
        COMMIT2=$(echo "${commits}" | awk 'NR==1{print $1}')
        COMMIT1=$(echo "${commits}" | awk 'NR==2{print $1}')
    fi

    while IFS= read -r file; do
        [[ "${file}" == *Dockerfile ]] && DOCKERFILE_DIFF+=("${file}")
    done < <(git diff --name-only "${COMMIT1}" "${COMMIT2}")
}

# --- Parse # TAGS from Dockerfile ---
# Returns space-separated tags (e.g., "3.21.3 latest")

private::docker::parse_tags() {
    local dockerfile="$1"
    local dir_tag="$2"  # fallback tag from directory name

    local first_line
    first_line=$(head -n 1 "${dockerfile}")

    if [[ "${first_line}" =~ ^#\ +TAGS\ +(.+)$ ]]; then
        echo "${BASH_REMATCH[1]}"
    else
        echo "${dir_tag}"
    fi
}

# --- Build images ---

public::common::ignore() {
    IFS=',' read -ra IGNORE_ARRAY <<< "${IGNORE:-}"
}

private::docker::build_one() {
    local file="$1"
    local repo dir tag tags ctx

    repo=$(echo "${file}" | awk -F"/" '{print $1}')
    dir=$(echo "${file}" | awk -F"/" '{print $2}')

    # Check ignore list
    for i in "${IGNORE_ARRAY[@]+"${IGNORE_ARRAY[@]}"}"; do
        if [[ "${i}" == "${repo}" ]]; then
            public::common::log "skip ignore repo ${file}"
            return 0
        fi
    done

    # Parse tags from # TAGS or fallback to directory name
    tags=$(private::docker::parse_tags "${file}" "${dir}")

    # Build context is the directory containing the Dockerfile
    ctx=$(dirname "${file}")

    public::common::log "Building: ${file}"

    # Build with all parsed tags
    local first_tag=true
    for tag in ${tags}; do
        local full_tag="${REPO}/${repo}:${tag}"
        if [[ "${first_tag}" == "true" ]]; then
            public::common::log "Build Command: docker build -t ${full_tag} -f ${file} ${ctx}"
            docker build -t "${full_tag}" -f "${file}" "${ctx}"
            first_tag=false
        else
            public::common::log "Tag Command: docker tag ${REPO}/${repo}:${tags%% *} ${full_tag}"
            docker tag "${REPO}/${repo}:${tags%% *}" "${full_tag}"
        fi
    done

    # Push if requested
    if [[ "${PUSH:-}" == "true" ]]; then
        for tag in ${tags}; do
            local full_tag="${REPO}/${repo}:${tag}"
            public::common::log "Push Command: docker push ${full_tag}"
            docker push "${full_tag}"
        done
    fi
}

private::docker::build() {
    if [[ ${#DOCKERFILE_DIFF[@]} -eq 0 ]]; then
        public::common::log "no image is required to build"
        exit 0
    fi

    public::common::ignore
    for file in "${DOCKERFILE_DIFF[@]}"; do
        private::docker::build_one "${file}"
    done
}

# --- Help ---

public::common::help() {
  cat <<'EOF'
bootstrap.sh - Incremental Docker image builder

Usage:
  bootstrap.sh [options]

Flags:
      --commit1 string              Old git commit id
      --commit2 string              New git commit id
      --ignore  list                Ignore repos, comma-separated (e.g. --ignore alpine,golang)
      --push    string              Push after build (must be "true")
      --help                         Show this help

Dockerfile TAGS:
  Add a # TAGS line at the top of your Dockerfile to specify multiple tags:
    # TAGS 3.21.3 latest
    FROM alpine:3.21.3
    ...

If no # TAGS directive is found, the directory name is used as the tag.

Environment:
  REPO    Docker repository prefix (default: kumquat/armory)

If commit1 and commit2 are not set, uses the last two commit IDs.
EOF
}

# --- Main ---

public::common::main() {
    while [[ $# -gt 0 ]]; do
        case "$1" in
        --help)
            public::common::help
            exit 0
            ;;
        --commit1)
            export COMMIT1="$2"; shift 2
            ;;
        --commit2)
            export COMMIT2="$2"; shift 2
            ;;
        --ignore)
            export IGNORE="$2"; shift 2
            ;;
        --push)
            export PUSH="$2"
            if [[ "${PUSH}" != "true" ]]; then
                public::common::err "If you want to push directly, set --push=true"
                exit 1
            fi
            shift 2
            ;;
        *)
            public::common::err "unknown option: $1"
            public::common::help
            exit 1
            ;;
        esac
    done

    if [[ -n "${COMMIT1:-}" && -z "${COMMIT2:-}" ]] || \
       [[ -z "${COMMIT1:-}" && -n "${COMMIT2:-}" ]]; then
        public::common::err "--commit1 and --commit2 must both be given"
        exit 1
    fi

    public::common::prepare
    private::git::diff
    private::docker::build
}

public::common::main "$@"
