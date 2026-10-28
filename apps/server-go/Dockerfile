FROM golang:1.26.5-alpine AS build
ARG APP_VERSION=development
ARG GIT_COMMIT=unknown
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X github.com/fixpro/server/internal/platform/logging.Version=${APP_VERSION} -X github.com/fixpro/server/internal/platform/logging.GitCommit=${GIT_COMMIT}" -o /out/server ./cmd/server && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X github.com/fixpro/server/internal/platform/logging.Version=${APP_VERSION} -X github.com/fixpro/server/internal/platform/logging.GitCommit=${GIT_COMMIT}" -o /out/migrate ./cmd/migrate && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X github.com/fixpro/server/internal/platform/logging.Version=${APP_VERSION} -X github.com/fixpro/server/internal/platform/logging.GitCommit=${GIT_COMMIT}" -o /out/init-customer-auth ./cmd/init-customer-auth && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X github.com/fixpro/server/internal/platform/logging.Version=${APP_VERSION} -X github.com/fixpro/server/internal/platform/logging.GitCommit=${GIT_COMMIT}" -o /out/bootstrap-admin ./cmd/bootstrap-admin
FROM alpine:3.22
RUN addgroup -S fixpro && adduser -S -G fixpro fixpro
WORKDIR /app
COPY --from=build /out/server /out/migrate /out/init-customer-auth /out/bootstrap-admin /app/
COPY --from=build /src/db/migrations /app/db/migrations
RUN mkdir -p /data/media && chown -R fixpro:fixpro /data
USER fixpro
EXPOSE 8080
ENTRYPOINT ["/app/server"]
