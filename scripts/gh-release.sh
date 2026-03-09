#!/bin/sh
set -e
. "${0%/*}"/logger.sh

APP_NAME=${APP_NAME:-solana-validator-failover}
GITHUB_TOKEN=${GITHUB_TOKEN:-}
GITHUB_REPO=${GITHUB_REPO:-}
REPO_TAG=${REPO_TAG:-}
APP_VERSION=${APP_VERSION:-}

gh_create_release() {
    log_info "fetching auto-generated changelog from GitHub API"
    PREV_TAG=$(gh release list --repo "${GITHUB_REPO}" --limit 1 --json tagName -q '.[0].tagName' 2>/dev/null || echo "")

    if [ -n "${PREV_TAG}" ]; then
        log_info "generating notes from ${PREV_TAG} to ${REPO_TAG}"
        AUTO_NOTES=$(gh api repos/${GITHUB_REPO}/releases/generate-notes \
            --method POST \
            --field tag_name=${REPO_TAG} \
            --field previous_tag_name=${PREV_TAG} \
            -q '.body')
    else
        log_info "no previous release found, generating notes from the beginning"
        AUTO_NOTES=$(gh api repos/${GITHUB_REPO}/releases/generate-notes \
            --method POST \
            --field tag_name=${REPO_TAG} \
            -q '.body')
    fi

    log_info "creating release notes"
    cat > release-notes.md <<EOF
${AUTO_NOTES}

---

### Installation
Download the appropriate binary for your platform and extract it:
\`\`\`bash
wget https://github.com/sol-strategies/solana-validator-failover/releases/download/${REPO_TAG}/solana-validator-failover-${APP_VERSION}-<platform>.gz
gunzip solana-validator-failover-${APP_VERSION}-<platform>.gz
chmod +x solana-validator-failover-${APP_VERSION}-<platform>
\`\`\`
EOF

    log_info "creating GitHub release"
    gh release create ${REPO_TAG} \
        --repo ${GITHUB_REPO} \
        --title "Release ${REPO_TAG}" \
        --notes-file release-notes.md \
        --draft=false \
        --prerelease=false
}

gh_release_upload_assets() {
    log_info "uploading release assets"
    cd bin
    
    # Upload all files
    for file in *; do
        if [ -f "$file" ]; then
            echo "Uploading $file..."
            gh release upload ${REPO_TAG} "$file" \
                --repo ${GITHUB_REPO} \
                --clobber
        fi
    done
}

main() {
    gh_create_release
    gh_release_upload_assets
}

main
