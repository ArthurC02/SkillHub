# Build context must be the repo root: apps/web depends on packages/api-client-ts via a file: link.

# syntax=docker/dockerfile:1.7

FROM node:24.21.0-bookworm-slim@sha256:2fe369e969550cde8e867afc3fe370b260140cab4a23d467074295b42163d553 AS build

WORKDIR /src
COPY packages/api-client-ts/package.json packages/api-client-ts/package-lock.json ./packages/api-client-ts/
RUN cd packages/api-client-ts && npm ci --ignore-scripts
COPY packages/api-client-ts ./packages/api-client-ts
RUN cd packages/api-client-ts && npm run build

COPY apps/web/package.json apps/web/package-lock.json ./apps/web/
RUN cd apps/web && npm ci --ignore-scripts
COPY apps/web ./apps/web
RUN cd apps/web && npm run build

FROM nginx:1.31-alpine-slim@sha256:032d437f2945ec236b0d093d0a4865a4b8e39eea52f60a25103645c95ccb4de0 AS runtime

COPY --from=build /src/apps/web/dist /usr/share/nginx/html
COPY infra/images/web/nginx.conf /etc/nginx/conf.d/default.conf

EXPOSE 80

HEALTHCHECK --interval=10s --timeout=3s --start-period=5s \
    CMD wget -q -O /dev/null http://127.0.0.1:80/healthz || exit 1
