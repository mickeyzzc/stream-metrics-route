FROM golang:1.20.5-alpine AS build_base

# 为我们的镜像设置必要的环境变量
ENV GO111MODULE=on \
    CGO_ENABLED=1 \
    GOPROXY="https://goproxy.cn,direct"

# 移动到工作目录：/home
WORKDIR /home/stream-metrics-route

# 将代码复制到容器中
COPY . .

RUN sed -i 's/dl-cdn.alpinelinux.org/mirror.tuna.tsinghua.edu.cn/g' /etc/apk/repositories && \
    apk add --no-cache gcc musl-dev 
# 将我们的代码编译成二进制可执行文件 
RUN cd /home/stream-metrics-route && \
    go build -ldflags='-w -s -extldflags "-static"' -tags musl,static,netgo  -v -o /bin/stream-metrics-route ./cmd/stream-metrics-route/

# Start fresh from a smaller image
FROM alpine:3.18
COPY --from=build_base /bin/stream-metrics-route /bin/stream-metrics-route

RUN sed -i 's/dl-cdn.alpinelinux.org/mirror.tuna.tsinghua.edu.cn/g' /etc/apk/repositories && \
    apk add tzdata curl && \
    cp /usr/share/zoneinfo/Asia/Shanghai /etc/localtime && \
    echo 'Asia/Shanghai' > /etc/timezone && \
    mkdir -p /stream-metrics-route/conf && \
    chmod +x /bin/stream-metrics-route && \
    chown -R nobody:nobody /stream-metrics-route 

USER       nobody

ENTRYPOINT [ "/bin/stream-metrics-route" ]
CMD        [ "-config.path=/stream-metrics-route/config/", \
             "-config.name=config.yaml", "-log.level debug" ]


