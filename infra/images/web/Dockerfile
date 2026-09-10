# Build context must be the repo root: apps/web depends on packages/api-client-ts via a file: link.

# syntax=docker/dockerfile:1.7

FROM node:22.14.0-bookworm-slim@sha256:1c18d9ab3af4585870b92e4dbc5cac5a0dc77dd13df1a5905cea89fc720eb05b AS build

WORKDIR /src
COPY packages/api-client-ts/package.json packages/api-client-ts/package-lock.json ./packages/api-client-ts/
RUN cd packages/api-client-ts && npm ci
COPY packages/api-client-ts ./packages/api-client-ts
RUN cd packages/api-client-ts && npm run build

COPY apps/web/package.json apps/web/package-lock.json ./apps/web/
RUN cd apps/web && npm ci
COPY apps/web ./apps/web
RUN cd apps/web && npm run build

FROM nginx:1.29-alpine-slim@sha256:c9366b8c560169b101ca0e5422ed063b20779e6454c2326b9c9704225c9b0c08 AS runtime

COPY --from=build /src/apps/web/dist /usr/share/nginx/html
COPY infra/images/web/nginx.conf /etc/nginx/conf.d/default.conf

EXPOSE 80

HEALTHCHECK --interval=10s --timeout=3s --start-period=5s \
    CMD wget -q -O /dev/null http://127.0.0.1:80/healthz || exit 1
