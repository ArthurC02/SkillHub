# syntax=docker/dockerfile:1.7

FROM ghcr.io/astral-sh/uv:0.13.7@sha256:96f40d3904aad8d0bd010539001d36c8e2188b981e1334471a90c4a1d93bb490 AS uv

FROM ghcr.io/berriai/litellm:main-stable@sha256:11175975a3093e05e92faee2fe2fe3412a3ae2fcaa849eb0fe25d5f0d8bcb2e9 AS patched
RUN --mount=from=uv,source=/uv,target=/tmp/uv \
    --mount=type=bind,source=patch-dependencies.py,target=/fixture/patch-dependencies.py \
    --mount=type=bind,source=requirements.txt,target=/fixture/requirements.txt \
    python -B /fixture/patch-dependencies.py --self-test \
    && python -B /fixture/patch-dependencies.py
RUN apk add --no-cache --upgrade 'glibc-2.44=2.44-r16' 'glibc-2.44-locale-posix=2.44-r16' 'ld-linux-2.44=2.44-r16' 'libcrypt1-2.44=2.44-r16'

FROM patched AS tests
COPY test-libraries.py check-catalog-paths.py /fixture/
ADD --checksum=sha256:96530b7c538d4ea08493ab067247aab7948c54d8cd622cce711c97cccc5d9896 https://files.pythonhosted.org/packages/a3/1d/d730c664fcb4f1d264a722722a3134cf0881854f73c1fd58abf87572f232/filelock-4.2.9.tar.gz /tmp/filelock.tar.gz
ADD --checksum=sha256:3d73febdb6faa00f277c81fb04be7b7586fb96fc3b7f0b9c772dada69a25ede7 https://files.pythonhosted.org/packages/33/17/2bb072de0b990303c985f26869fd47bb179024a051f47e7f9d208eb92f8e/oauthlib-4.0.1.tar.gz /tmp/oauthlib.tar.gz
RUN python -B /fixture/test-libraries.py --self-test \
    && python -B /fixture/test-libraries.py prepare
RUN --mount=from=uv,source=/uv,target=/tmp/uv \
    --mount=type=bind,source=test-requirements.txt,target=/fixture/test-requirements.txt \
    /tmp/uv pip install --python /app/.venv/bin/python --constraint /tmp/runtime-constraints.txt --no-deps --require-hashes --only-binary :all: --no-cache -r /fixture/test-requirements.txt
RUN --network=none python -B /fixture/test-libraries.py run
RUN apk add --no-cache 'glibc-2.44-locale-de=2.44-r16'
RUN --network=none python -B /fixture/check-catalog-paths.py

FROM patched
RUN --network=none \
    --mount=from=tests,source=/tmp/library-proof.json,target=/tmp/library-proof.json \
    --mount=from=tests,source=/tmp/catalog-proof.json,target=/tmp/catalog-proof.json \
    --mount=from=tests,source=/tmp/runtime-inventory.json,target=/tmp/runtime-inventory.json \
    --mount=type=bind,source=test-libraries.py,target=/fixture/test-libraries.py \
    python -B /fixture/test-libraries.py verify \
    && python -c 'import json; result=json.load(open("/tmp/catalog-proof.json")); assert result == {"passed": 6}; print(result)'
