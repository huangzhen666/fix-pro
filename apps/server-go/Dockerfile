FROM golang:1.26.5-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/migrate ./cmd/migrate
FROM alpine:3.22
RUN addgroup -S fixpro && adduser -S -G fixpro fixpro
WORKDIR /app
COPY --from=build /out/server /out/migrate /app/
COPY --from=build /src/db/migrations /app/db/migrations
RUN mkdir -p /data/media && chown -R fixpro:fixpro /data
USER fixpro
EXPOSE 8080
ENTRYPOINT ["/app/server"]
