FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /doc-engine ./cmd/api

FROM alpine:3.21
RUN apk add --no-cache ca-certificates
# Install typst
RUN wget -qO- https://github.com/typst/typst/releases/download/v0.13.1/typst-x86_64-unknown-linux-musl.tar.xz | tar -xJ -C /usr/local/bin --strip-components=1 typst-x86_64-unknown-linux-musl/typst
COPY --from=builder /doc-engine /usr/local/bin/doc-engine
COPY settings/ /app/settings/
WORKDIR /app
EXPOSE 8080
CMD ["doc-engine"]
