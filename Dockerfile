# syntax=docker/dockerfile:1.7

# ---------- 前端构建 ----------
FROM node:24-alpine AS web-build
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# ---------- 后端构建（modernc.org/sqlite 纯 Go，CGO_ENABLED=0 静态链接） ----------
FROM golang:1.25-alpine AS backend-build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags "-s -w" -o /out/orangerepo . \
 && mkdir -p /out/data \
 && chown 65532:65532 /out/data

# ---------- 运行时：distroless static（无 shell / 无包管理器，含 CA 与 tzdata） ----------
# 以 root 启动是刻意的：二进制启动时自动把数据目录属主修正为 65532 并立刻降权，
# 从而让 ./data 绑定挂载在宿主机上免 chown 开箱即用（见 bootstrap_linux.go）。
FROM gcr.io/distroless/static-debian12
WORKDIR /app
COPY --from=backend-build /out/orangerepo /app/orangerepo
# 预置属主，保证命名卷首次挂载与自定义非 root --user 场景可直接写入
COPY --from=backend-build --chown=65532:65532 /out/data /app/data
COPY --from=web-build /src/web/dist /app/web/dist
COPY samples /app/samples

VOLUME ["/app/data"]
EXPOSE 8080

ENTRYPOINT ["/app/orangerepo"]
# 追加 -seed 可在空库时导入示例包：docker run image -seed
CMD ["-addr", ":8080", "-data", "/app/data", "-web", "/app/web/dist"]
