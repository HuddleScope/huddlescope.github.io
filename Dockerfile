# huddlescope runtime image (issue #161 AC-5, published by issue #204).
#
# Multi-stage: stage 1 cross-compiles the static binary (CGO_ENABLED=0,
# -trimpath, stamped with the same version/commit/built-at the Makefile and
# release workflow use); stage 2 is the minimal gcr.io/distroless/static
# runtime (no shell, no package manager, ~2 MB) running as a non-root user.
# distroless/static ships the ca-certificates bundle, so the Turso HTTPS
# connection works without any package-manager install step.
# No `.env` or credentials are ever copied into the image — runtime config is
# supplied as environment variables at `docker run` time, identical to the
# native binary's precedence chain (flags > env > file > defaults), so
# container and native share the same execution model.
#
# The image is published to ghcr.io on every merge to main by
# .github/workflows/docker-build.yaml (issue #204). Local builds remain
# available via `make docker-build`.

# ---- Stage 1: build the static binary -------------------------------------
# The golang base tag is kept in sync with go.mod (see dependabot docker
# group `docker-base` tracking `golang*`); CGO_ENABLED=0 below keeps the
# binary static regardless of the base.
FROM golang:1.27 AS build

# Build args mirror the Makefile stamp defaults so the image's binary reports
# the same version/revision/build-info as a `make build` of the same commit.
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILT_AT=dev

WORKDIR /src

# Cache module downloads separately from source for faster rebuilds.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .

# CGO_ENABLED=0 + -trimpath + deterministic BUILT_AT => static, reproducible.
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath \
      -ldflags="-s -w \
        -X github.com/HuddleScope/huddlescope.github.io/internal/cli.cliVersion=${VERSION} \
        -X github.com/HuddleScope/huddlescope.github.io/internal/cli.cliCommit=${COMMIT} \
        -X github.com/HuddleScope/huddlescope.github.io/internal/cli.cliBuiltAt=${BUILT_AT}" \
      -o /out/huddlescope ./cmd/huddlescope

# ---- Stage 2: minimal runtime ---------------------------------------------
FROM gcr.io/distroless/static

# The distroless/static base ships the system ca-certificates bundle and has
# no shell or package manager; nothing to install.

# Copy only the compiled binary; no source, no .env, no secrets in layers.
COPY --from=build /out/huddlescope /usr/local/bin/huddlescope

# Non-root user (nobody, uid/gid 65534) — the binary does not need privileges.
USER 65534:65534

# Runtime configuration is supplied via environment variables at `docker run`
# time (HSC_*, TURSO_*) — identical to the native binary. Example:
#   docker run --rm -e TURSO_DATABASE_URL=... -e TURSO_AUTH_TOKEN=... \
#     -e HSC_STORAGE_BACKEND=turso ghcr.io/huddlescope/huddlescope.github.io:latest ingest run
ENTRYPOINT ["/usr/local/bin/huddlescope"]
