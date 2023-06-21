NAME=stream-metrics-route
VERSION=v0.1.0
REPOSITORY=registry.cn-hangzhou.aliyuncs.com/mickeyzzc

.PHONY: all build 

all: build docker

build:
	@go build -ldflags -v -o ./packet/${VERSION}/${NAME} ./cmd/stream-metrics-route/
	@tar -zcvf ./packet/${VERSION}/${NAME}-${VERSION}.linux-amd64.tar.gz ./packet/${VERSION}/${NAME}

run: build
	@chmod +x ./packet/${NAME}
	./packet/${NAME} -h

docker:
	@docker build . -t ${REPOSITORY}/${NAME}:${VERSION}
	@docker push ${REPOSITORY}/${NAME}:${VERSION}
	@docker rmi -f ${REPOSITORY}/${NAME}:${VERSION}

docker_in_macos_arm:
	@docker buildx build --platform linux/amd64 --load . -t ${REPOSITORY}/${NAME}:${VERSION}
	@docker push ${REPOSITORY}/${NAME}:${VERSION}
	@docker rmi -f ${REPOSITORY}/${NAME}:${VERSION}
	
help:
	@echo "make 格式化go代码 并编译生成二进制文件"
	@echo "make build "
	@echo "make run "
	@echo "make docker "