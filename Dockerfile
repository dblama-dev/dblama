# syntax=docker/dockerfile:1
# Build the SPA first; the Go binary embeds server/web at compile time.
FROM --platform=$BUILDPLATFORM node:24-bookworm-slim AS frontend
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM --platform=$BUILDPLATFORM golang:1.26.8-bookworm AS backend
WORKDIR /src/server
COPY server/go.mod server/go.sum ./
RUN go mod download
COPY server/ ./
COPY --from=frontend /src/server/web ./web
ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /out/dbmanager ./cmd/dbmanager

FROM scratch AS architecture-check
COPY --from=backend /out/dbmanager /dbmanager

FROM gcr.io/distroless/static-debian13:nonroot
COPY --from=backend --chown=65532:65532 /out/dbmanager /dbmanager
USER 65532:65532
EXPOSE 8080
ENV LISTEN=:8080
ENTRYPOINT ["/dbmanager"]
