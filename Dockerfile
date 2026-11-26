FROM node:24-alpine AS web
RUN npm install -g pnpm@12.23.0
WORKDIR /web
COPY web/package.json web/pnpm-lock.yaml web/.npmrc ./
RUN pnpm install --frozen-lockfile
COPY web/ ./
RUN pnpm build

FROM golang:1.27.4-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /web/dist ./web/dist
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /out/pida ./cmd/pida \
    && mkdir -p /data && chmod 0777 /data

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /out/pida /app/pida
COPY --from=build --chown=65532:65532 /data /data
ENV DATA_DIR=/data PORT=3000 NODE_ENV=production NODE_NO_WARNINGS=1
USER 65532:65532
EXPOSE 3000
ENTRYPOINT ["/app/pida", "serve"]
