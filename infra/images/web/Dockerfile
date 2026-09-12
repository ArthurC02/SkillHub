# Build context must be the repo root: apps/web depends on packages/api-client-ts via a file: link.

# syntax=docker/dockerfile:1.7

FROM node:26.8-bookworm-slim@sha256:cd9f682fa2885cd1056e830424764158570061c59736a1da836bc3d73df095ae AS build

WORKDIR /src
COPY packages/api-client-ts/package.json packages/api-client-ts/package-lock.json ./packages/api-client-ts/
RUN cd packages/api-client-ts && npm ci --ignore-scripts
COPY packages/api-client-ts ./packages/api-client-ts
RUN cd packages/api-client-ts && npm run build

COPY apps/web/package.json apps/web/package-lock.json ./apps/web/
RUN cd apps/web && npm ci --ignore-scripts
COPY apps/web ./apps/web
RUN cd apps/web && npm run build

FROM nginx:1.31-alpine-slim@sha256:3b171d7224b669faa3cc2137fea0a65301791df1ec1f271ebd2a2b7461f7fade AS runtime

COPY --from=build /src/apps/web/dist /usr/share/nginx/html
COPY infra/images/web/nginx.conf /etc/nginx/conf.d/default.conf

EXPOSE 80

HEALTHCHECK --interval=10s --timeout=3s --start-period=5s \
    CMD wget -q -O /dev/null http://127.0.0.1:80/healthz || exit 1
