FROM golang:1.25-alpine AS builder
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/agent-dance ./cmd/server

FROM alpine:3.22 AS runner
RUN apk add --no-cache ca-certificates curl ffmpeg
WORKDIR /app
RUN addgroup -S -g 10001 agentdance \
    && adduser -S -D -H -u 10001 -G agentdance agentdance \
    && mkdir -p /app/runtime /app/uploads \
    && chown -R agentdance:agentdance /app
COPY --from=builder /out/agent-dance /usr/local/bin/agent-dance
USER agentdance
EXPOSE 8080
CMD ["agent-dance"]
