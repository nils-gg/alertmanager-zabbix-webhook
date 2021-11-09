SRCS = $(shell find . -name '*.go')
OBJS = alertmanager-zabbix-webhook

all: $(OBJS)
go-deps:
	go get -t ./...
alertmanager-zabbix-webhook: $(SRCS)
	CGO_ENABLED=0 GOOS=linux go build -a -ldflags '-extldflags "-static"' .
docker-build:
	which docker && docker build . -t alertmanager-zabbix-webhook
install:
	install -s alertmanager-zabbix-webhook /usr/local/sbin/zabbix_webhook
clean:
	bash -c "rm -f alertmanager-zabbix-webhook {,webhook/}go.{mod,sum}"
