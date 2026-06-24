VERSION ?= 0.1.0
IMAGE ?= ghcr.io/jmakori/kagent-discord-bot

.PHONY: build image push all

build:
	CGO_ENABLED=0 go build -ldflags="-s -w" -o kagent-discord-bot .

image:
	docker build -t $(IMAGE):$(VERSION) .

push:
	docker push $(IMAGE):$(VERSION)

all: build image
