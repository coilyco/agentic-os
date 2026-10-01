#!/usr/bin/env bash
# Publish a hosted build in dist/ to coilyco.dev/aterm/. See docs/deploy.md.
set -euo pipefail

registry="forgejo.coilysiren.me"
ci_image="${registry}/coilyco/agentic-os:release"
target="s3://coilyco-dev-site/aterm/"
no_proxy_hosts="127.0.0.1,localhost,forgejo.coilysiren.me,forgejo.forgejo.svc.cluster.local,.svc,.cluster.local"

for name in PROJECT_SITES_PUBLISH_AWS_ACCESS_KEY_ID PROJECT_SITES_PUBLISH_AWS_SECRET_ACCESS_KEY \
  REGISTRY_TOKEN FORGEJO_EGRESS_PROXY; do
  if [ -z "${!name:-}" ]; then
    echo "publish-coilyco-dev: ${name} is not set. This runs on a deploy runner holding the project-sites key." >&2
    exit 1
  fi
done

# sync --delete from a wrong or empty build would empty the live /aterm/.
[ -s dist/index.html ] || { echo "publish-coilyco-dev: dist/index.html is missing, refusing to sync." >&2; exit 1; }
grep -q '/aterm/assets/' dist/index.html || { echo "publish-coilyco-dev: dist/ was not built with base /aterm/, refusing to sync." >&2; exit 1; }

docker_config="$(mktemp -d)"
trap 'rm -rf "${docker_config}"' EXIT
chmod 700 "${docker_config}"
export DOCKER_CONFIG="${docker_config}"
printf '%s' "${REGISTRY_TOKEN}" | docker login "${registry}" --username coilyco-ops --password-stdin
docker pull --quiet "${ci_image}"

# Bare --env NAME keeps the key out of the runner's argv.
export AWS_ACCESS_KEY_ID="${PROJECT_SITES_PUBLISH_AWS_ACCESS_KEY_ID}"
export AWS_SECRET_ACCESS_KEY="${PROJECT_SITES_PUBLISH_AWS_SECRET_ACCESS_KEY}"
export AWS_DEFAULT_REGION="us-east-1"

# max-age=0 stands in for an invalidation, which would need the distribution id.
tar -C dist -cf - . \
  | docker run --rm -i \
    --env AWS_ACCESS_KEY_ID --env AWS_SECRET_ACCESS_KEY --env AWS_DEFAULT_REGION \
    --env HTTP_PROXY="${FORGEJO_EGRESS_PROXY}" --env HTTPS_PROXY="${FORGEJO_EGRESS_PROXY}" --env NO_PROXY="${no_proxy_hosts}" \
    --env http_proxy="${FORGEJO_EGRESS_PROXY}" --env https_proxy="${FORGEJO_EGRESS_PROXY}" --env no_proxy="${no_proxy_hosts}" \
    --entrypoint bash \
    "${ci_image}" -c "
      set -euo pipefail
      mkdir -p /tmp/site && tar -xf - -C /tmp/site
      aws s3 sync /tmp/site/ ${target} --delete --no-progress --cache-control 'public,max-age=0,must-revalidate'
      served=\$(aws s3api head-object --bucket coilyco-dev-site --key aterm/index.html --query ContentType --output text)
      case \"\${served}\" in
        text/html*) echo \"publish-coilyco-dev: index.html serves as \${served}\" ;;
        *) echo \"publish-coilyco-dev: index.html serves as \${served}, not text/html\" >&2; exit 1 ;;
      esac
    "

echo "publish-coilyco-dev: published dist/ to https://coilyco.dev/aterm/"
