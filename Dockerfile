# Stage 1: Build React frontend
FROM node:22-alpine AS frontend
WORKDIR /app
COPY app/package.json app/pnpm-lock.yaml ./
RUN corepack enable && pnpm install --frozen-lockfile
COPY app/ .
RUN pnpm build

# Stage 2: Build Go backend with embedded frontend
FROM golang:1.26-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Embed frontend dist into Go binary
COPY --from=frontend /app/dist/ core/internal/frontend/dist/
RUN CGO_ENABLED=0 go build -o /src/bin/doc-engine ./core/cmd/api

# Stage 3: Runtime
FROM alpine:3.21
RUN apk add --no-cache ca-certificates

# Install Typst
COPY --from=ghcr.io/typst/typst:latest /usr/local/bin/typst /usr/local/bin/typst

WORKDIR /app
COPY --from=builder /src/bin/doc-engine .
COPY core/settings/ ./settings/

EXPOSE 8080
CMD ["./doc-engine"]
