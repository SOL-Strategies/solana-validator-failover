FROM golang:1.26.0-trixie
ARG APP_VERSION
ARG APP_NAME
ARG BUILD_OS_ARCH_LIST
ARG CI
ENV APP_VERSION=$APP_VERSION
ENV APP_NAME=$APP_NAME
ENV BUILD_OS_ARCH_LIST=$BUILD_OS_ARCH_LIST
ENV CI=$CI
RUN apt-get update \
    && apt-get install -y \
        jq \
        zip \
        unzip \
    && go install github.com/air-verse/air@latest \
    && go install golang.org/x/lint/golint@latest \
    && go install github.com/git-chglog/git-chglog/cmd/git-chglog@latest \
    && mkdir -p /usr/local/go/src/github.com/sol-strategies/$APP_NAME /build

COPY ./ /usr/local/go/src/github.com/sol-strategies/$APP_NAME

WORKDIR /usr/local/go/src/github.com/sol-strategies/$APP_NAME

VOLUME ["/build"]

CMD [ "build" ]

ENTRYPOINT [ "/usr/local/go/src/github.com/sol-strategies/solana-validator-failover/scripts/entrypoint.sh" ]
