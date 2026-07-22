ARG BASE_IMAGE
FROM ${BASE_IMAGE} AS dev-base-lang-python

# The core tier ships Python 3, pip, uv, and the fleet's managed interpreters.
# This specialist surface adds the conventional unversioned commands plus
# pipenv while inheriting the shared agent harnesses from core.
RUN set -eux; \
    ln -sf /usr/bin/python3 /usr/local/bin/python; \
    ln -sf /usr/bin/pip3 /usr/local/bin/pip; \
    uv tool install pipenv; \
    chmod -R a+rX /opt/uv; \
    python --version; \
    pip --version; \
    pipenv --version; \
    uv --version

WORKDIR /workspace
CMD ["bash"]
