#!/bin/sh
set -e
. "${0%/*}"/logger.sh

BUILD_DIR=${BUILD_DIR:-bin}
APP_NAME=${APP_NAME:-solana-validator-failover}
APP_VERSION=${APP_VERSION:-dev}

# Get version from git tag if available
if [ -d .git ]; then
    GIT_TAG=$(git describe --tags --exact-match 2>/dev/null || echo "")
    if [ -n "$GIT_TAG" ]; then
        APP_VERSION=$GIT_TAG
    fi
fi

APP_VERSION=$(echo "${APP_VERSION}" | sed 's|/|-|g')
# strip v prefix
APP_VERSION=${APP_VERSION#v}

go_tidy() {
    log_info "ensuring go depedencies"
    go install golang.org/x/lint/golint@latest
    go mod tidy || exit 1
}

go_lint() {
    log_info "go linting"
    golint --set_exit_status $(go list ./... | grep -v vendor/)
}

go_fmt() {
    log_info "go formatting"
    go fmt ./... || exit 1
}

go_test() {
    log_info "go testing"
    go test ./... || exit 1
}

go_build() {
    osarchList="$(echo "${BUILD_OS_ARCH_LIST}" | sed 's/,/ /g')"
    log_info "building ${osarchList}"
    for osarch in ${osarchList}; do
        os=$(echo "${osarch}" | cut -d '-' -f1)
        arch=$(echo "${osarch}" | cut -d '-' -f2)
        binOutput="${BUILD_DIR}/${APP_NAME}-${APP_VERSION}-${os}-${arch}"
        log_info "building ${binOutput}"
        GOOS=${os} GOARCH=${arch} CGO_ENABLED=0 go build -ldflags="-s -w" -trimpath -o "${binOutput}" || exit 1
        log_info "building ${binOutput} - complete"
        if [ "${CI}" = "true" ]; then
            # create gzipped binary (GitHub shows sha256 for each asset automatically)
            gzip -9 "${binOutput}"
            log_info "created gzipped binary for ${binOutput}"
        fi
    done
}

main() {
    log_info "BUILD_DIR=${BUILD_DIR}"
    log_info "APP_VERSION=${APP_VERSION}"
    log_info "BUILD_OS_ARCH_LIST=${BUILD_OS_ARCH_LIST}"
    log_info "CI=${CI}"
    
    # Write version file early so it's available for linting and testing
    log_info "writing version ${APP_VERSION} -> pkg/constants/app.version"
    echo -n "${APP_VERSION}" >"pkg/constants/app.version"
    
    go_tidy
    go_lint
    go_fmt
    if [ "${CI}" = "true" ]; then
        go_test
    fi
    go_build
    log_info "build complete - ${BUILD_DIR}:"
    ls -lahrt "${BUILD_DIR}"
}

main
