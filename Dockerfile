FROM golang:1.26-alpine AS build_base

ENV CGO_ENABLED=0 \
    GO111MODULE=on

WORKDIR /home/stream-metrics-route

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN go build -ldflags='-w -s' -v -o /bin/stream-metrics-route ./cmd/stream-metrics-route/

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


