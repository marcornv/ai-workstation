# Bancada app image: builds the React UI, then the Go binary that embeds it.

# 1) Build the web interface (output goes to backend/web/dist).
FROM node:22-alpine AS ui
WORKDIR /src/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci --no-fund --no-audit
COPY frontend/ ./
RUN mkdir -p ../backend/web/dist && npm run build

# 2) Build the Go server (pure Go, no C compiler needed).
FROM golang:1.25-alpine AS server
WORKDIR /src/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
COPY --from=ui /src/backend/web/dist ./web/dist
RUN CGO_ENABLED=0 go build -o /bancada .

# 3) Small final image.
FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata su-exec
COPY --from=server /bancada /usr/local/bin/bancada
# Example experiment, evaluators and data, copied into the workspace on first start.
COPY experiments /app/examples/experiments
COPY evaluators /app/examples/evaluators
COPY data/exemplo-*.csv data/example-*.csv /app/examples/data/
COPY docker/entrypoint.sh /app/entrypoint.sh

# Everything the user creates lives in /workspace (a folder on the host, see docker-compose.yml).
WORKDIR /workspace
ENV BANCADA_HOST=0.0.0.0
EXPOSE 8080
ENTRYPOINT ["/app/entrypoint.sh"]
