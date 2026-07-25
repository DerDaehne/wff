# Plain-Docker build path, no Nix required (Nix stays the primary/reference
# build via `nix build .#docker` — this exists so the image can be built on
# any machine with just Docker). Mirrors flake.nix's approach: build the
# frontend, embed its static output into the Go binary via go:embed, run
# from a minimal base.
#
#   docker build -t wff-backend:latest .

# --- Frontend build ---
FROM node:22-alpine AS frontend
WORKDIR /src
RUN corepack enable && corepack prepare pnpm@11.15.0 --activate
COPY frontend/package.json frontend/pnpm-lock.yaml frontend/pnpm-workspace.yaml ./
RUN pnpm install --frozen-lockfile
COPY frontend/ ./
RUN pnpm build

# --- Backend build (embeds the frontend build via go:embed, see internal/webui) ---
FROM golang:1.26-alpine AS backend
WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN rm -rf internal/webui/dist/*
COPY --from=frontend /src/build/. ./internal/webui/dist/
RUN CGO_ENABLED=0 go build -o /wff .

# --- Runtime ---
FROM alpine:latest
RUN apk add --no-cache ca-certificates
COPY --from=backend /wff /usr/local/bin/wff
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/wff"]
