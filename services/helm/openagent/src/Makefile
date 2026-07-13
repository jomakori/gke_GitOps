VERSION_FILE := VERSION
VERSION := $(shell cat $(VERSION_FILE) 2>/dev/null || echo "0.1.0")
IMAGE ?= ghcr.io/jomakori/gke_gitops/openagent-discord-bot

.PHONY: build image push all bump release test

build:
	CGO_ENABLED=0 go build -ldflags="-s -w" -o openagent-discord-bot .

image:
	docker build -t $(IMAGE):$(VERSION) .

push:
	docker push $(IMAGE):$(VERSION)

bump:
	@case "$(TYPE)" in \
		feat)  NEW=$$(echo $(VERSION) | awk -F. '{printf "%d.%d.0", $$1, $$2+1}') ;; \
		fix)   NEW=$$(echo $(VERSION) | awk -F. '{printf "%d.%d.%d", $$1, $$2, $$3+1}') ;; \
		chore) NEW=$(VERSION) ;; \
		*)     echo "Usage: make bump TYPE=feat|fix|chore"; exit 1 ;; \
	esac; \
	echo $$NEW > $(VERSION_FILE); \
	sed -i '' 's/tag: "$(VERSION)"/tag: "'$$NEW'"/' ../values.yaml; \
	echo "Bumped $(VERSION) -> $$NEW (VERSION + values.yaml)"

release: bump build image push
	@echo "Released $(shell cat $(VERSION_FILE))"

test:
	go test -v ./...

all: build image
