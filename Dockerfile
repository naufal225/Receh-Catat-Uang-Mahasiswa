# Receh: one image serving the Go API and the built React app.

FROM node:22-alpine AS web
WORKDIR /src/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

FROM golang:1.24-alpine AS api
WORKDIR /src/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/receh ./cmd/api

FROM alpine:3.21
# tzdata: APP_TIMEZONE drives recurring transactions. ca-certificates: outgoing HTTPS.
RUN apk add --no-cache ca-certificates tzdata && adduser -D -H receh
WORKDIR /app
COPY --from=api /out/receh ./receh
COPY --from=web /src/frontend/dist ./web
ENV ADDR=:8080 STATIC_DIR=/app/web
USER receh
EXPOSE 8080
ENTRYPOINT ["/app/receh"]
