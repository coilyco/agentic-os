ARG BASE_IMAGE
FROM ${BASE_IMAGE} AS dev-base-lang-node

# Core owns Node because every specialist needs the npm-backed agent harnesses.
# This sibling stage gives Node consumers a language-specific published tag.
RUN set -eux; \
    node --version; \
    npm --version

WORKDIR /workspace
CMD ["bash"]
