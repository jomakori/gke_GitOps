.PHONY: tests full_coverage clean tidy

KAGENT_PRIVATE_URL   ?= $(shell echo $$KAGENT_PRIVATE_URL)
KAGENT_A2A_URL       ?= $(shell echo $$KAGENT_A2A_URL)
CF_ACCESS_CLIENT_ID  ?= $(shell echo $$CF_ACCESS_CLIENT_ID)
CF_ACCESS_CLIENT_SECRET ?= $(shell echo $$CF_ACCESS_CLIENT_SECRET)

ifeq ($(KAGENT_A2A_URL),)
  ifneq ($(KAGENT_PRIVATE_URL),)
    KAGENT_A2A_URL := $(KAGENT_PRIVATE_URL)/a2a
  endif
endif

tests:
	go test ./... -v

full_coverage:
ifeq ($(KAGENT_A2A_URL),)
	$(error No A2A endpoint configured. Set one of:
	  export KAGENT_PRIVATE_URL=https://kagent.maklab.net
	  export KAGENT_A2A_URL=https://kagent.maklab.net/a2a)
endif
	@echo "Running integration tests against $(KAGENT_A2A_URL)"
	KAGENT_A2A_URL=$(KAGENT_A2A_URL) \
	CF_ACCESS_CLIENT_ID=$(CF_ACCESS_CLIENT_ID) \
	CF_ACCESS_CLIENT_SECRET=$(CF_ACCESS_CLIENT_SECRET) \
	go test ./... -v

tidy:
	go mod tidy

clean:
	go clean -testcache
