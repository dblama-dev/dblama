# syntax=docker/dockerfile:1
# Build the SPA first; the Go binary embeds frontend/dist at compile time.
FROM --platform=$BUILDPLATFORM node:24-bookworm-slim AS frontend
WORKDIR /src/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

FROM --platform=$BUILDPLATFORM golang:1.26.8-bookworm AS backend
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY web.go ./
COPY --from=frontend /src/frontend/dist ./frontend/dist
ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /out/dblama ./cmd/dblama

FROM scratch AS architecture-check
COPY --from=backend /out/dblama /dblama

FROM gcr.io/distroless/static-debian13:nonroot
COPY --from=backend --chown=65532:65532 /out/dblama /dblama
USER 65532:65532
EXPOSE 8080
ENV LISTEN=:8080
ENTRYPOINT ["/dblama"]
