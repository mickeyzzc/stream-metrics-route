NAME=stream-metrics-route
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || cat VERSION)
REPOSITORY=registry.cn-hangzhou.aliyuncs.com/mickeyzzc

.PHONY: all build test lint fmt clean folder x86 docker docker_in_macos_arm help

all: build docker

build: folder x86 

clean:
	rm -rf ./packet/

folder:
	@mkdir -p ./packet/${VERSION}/

x86:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags -v -o ./packet/amd64/${VERSION}/${NAME} ./cmd/${NAME}/
	tar -zcvf ./packet/${VERSION}/${NAME}-${VERSION}.linux-amd64.tar.gz ./packet/amd64/${VERSION}/${NAME}

docker:
	@docker build . -t ${REPOSITORY}/${NAME}:${VERSION}
	@docker push ${REPOSITORY}/${NAME}:${VERSION}
	@docker rmi -f ${REPOSITORY}/${NAME}:${VERSION}

docker_in_macos_arm:
	@docker buildx build --platform linux/amd64 --load . -t ${REPOSITORY}/${NAME}:${VERSION}
	@docker push ${REPOSITORY}/${NAME}:${VERSION}
	@docker rmi -f ${REPOSITORY}/${NAME}:${VERSION}
	
test:
	go test ./... -v -count=1

lint:
	go vet ./...

fmt:
	gofmt -w .

help:
	@echo "make 格式化go代码 并编译生成二进制文件"
	@echo "make build "
	@echo "make run "
	@echo "make docker "
	@echo "make test    run tests"
	@echo "make lint    run go vet"
	@echo "make fmt     format code"