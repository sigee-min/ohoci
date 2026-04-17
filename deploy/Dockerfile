# syntax=docker/dockerfile:1.7

FROM node:22-bookworm-slim AS web-build
WORKDIR /app/web
COPY web/package.json web/package-lock.json* ./
RUN npm install
COPY web/ ./
COPY docs/ /app/docs/
RUN npm run build

FROM --platform=$BUILDPLATFORM golang:1.24-bookworm AS go-build
WORKDIR /src
ARG TARGETOS=linux
ARG TARGETARCH
COPY go.mod go.sum* ./
RUN go mod download
COPY . ./
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /out/ohoci ./cmd/ohoci

FROM debian:bookworm-slim AS runtime-data
RUN install -d -o 65532 -g 65532 /var/lib/ohoci /var/lib/ohoci/data /var/lib/ohoci/ui

FROM gcr.io/distroless/base-debian12:nonroot
ENV OHOCI_SQLITE_PATH=/var/lib/ohoci/data/ohoci.db
ENV OHOCI_UI_DIR=/var/lib/ohoci/ui
WORKDIR /var/lib/ohoci
COPY --from=runtime-data --chown=nonroot:nonroot /var/lib/ohoci /var/lib/ohoci
COPY --from=web-build --chown=nonroot:nonroot /app/web/dist /var/lib/ohoci/ui
COPY --from=go-build --chown=nonroot:nonroot /out/ohoci /usr/local/bin/ohoci
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/ohoci"]
