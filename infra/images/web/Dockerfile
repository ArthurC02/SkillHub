# Build context must be the repo root: apps/web depends on packages/api-client-ts via a file: link.

# syntax=docker/dockerfile:1.7

FROM node:26.12.0-bookworm-slim@sha256:742a0fe7320e09e0321222b68165e6ac731d30470e7ea7f98f9bfc3a6e1034fe AS build

WORKDIR /src
COPY packages/api-client-ts/package.json packages/api-client-ts/package-lock.json ./packages/api-client-ts/
RUN cd packages/api-client-ts && npm ci --ignore-scripts
COPY packages/api-client-ts ./packages/api-client-ts
RUN cd packages/api-client-ts && npm run build

COPY apps/web/package.json apps/web/package-lock.json ./apps/web/
RUN cd apps/web && npm ci --ignore-scripts
COPY apps/web ./apps/web
RUN cd apps/web && npm run build

FROM nginx:1.31-alpine-slim@sha256:f45171d36c76a8b2f5ef97108a3a8a2294248a6578a0a180edc2d78f7d2edc26 AS runtime

RUN apk upgrade --no-cache zlib pcre2 musl musl-utils

COPY --from=build /src/apps/web/dist /usr/share/nginx/html
COPY infra/images/web/nginx.conf /etc/nginx/conf.d/default.conf

EXPOSE 80

HEALTHCHECK --interval=10s --timeout=3s --start-period=5s \
    CMD wget -q -O /dev/null http://127.0.0.1:80/healthz || exit 1
