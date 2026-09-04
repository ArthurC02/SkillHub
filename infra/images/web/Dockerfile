# Skill Hub web service image (ADR-019 CI job 5 / 04 丙-158).
#
# Builds the production apps/web bundle (the same two steps Taskfile.yml's
# build:web/build:api-client do: compile the generated TS client, then
# `npm run build`, which itself runs check-bundle-origins.mjs) and serves it
# from nginx, same-origin with the platform API behind a reverse proxy — see
# nginx.conf's own header comment for why the proxy is Accept-header-routed
# rather than a single path prefix.
#
# Build context MUST be the repo root: apps/web depends on
# packages/api-client-ts via a `file:../../packages/api-client-ts` link, so
# both trees must be in the build context together.
#
# Build:
#   docker build -f infra/images/web/Dockerfile -t skillhub/web:local .

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
# Production mode: this must NOT pick up apps/web/.env.development's
# VITE_API_BASE_URL=http://localhost:8080 (the two-origin dev-server shape).
# `vite build` already runs in production mode by default and ignores that
# file; check-bundle-origins.mjs (run by `npm run build` itself) is the check
# that a same-origin, empty-string API_BASE_URL build was actually produced.
RUN cd apps/web && npm run build

# Runtime: nginx serves the built SPA and reverse-proxies API calls to the
# platform-api container so the bundle stays same-origin (api/client.ts's
# empty-string default — see that file's header comment). Alpine, not the
# Debian nginx image: nginx itself needs no compiled extensions here, and this
# is the one image in this repo that terminates the deployment's only public
# HTTP port, so a smaller base is a smaller face for that role.
FROM nginx:1.29-alpine-slim@sha256:c9366b8c560169b101ca0e5422ed063b20779e6454c2326b9c9704225c9b0c08 AS runtime

COPY --from=build /src/apps/web/dist /usr/share/nginx/html
COPY infra/images/web/nginx.conf /etc/nginx/conf.d/default.conf

# nginx's stock image already runs its master process as root and its worker
# processes as the unprivileged `nginx` user (binding :80 needs the former);
# that split is upstream's own hardening and this Dockerfile does not
# second-guess it by trying to move to a fully non-root master, which the
# official image does not support without losing exec-form signal handling.

EXPOSE 80

HEALTHCHECK --interval=10s --timeout=3s --start-period=5s \
    CMD wget -q -O /dev/null http://127.0.0.1:80/healthz || exit 1
