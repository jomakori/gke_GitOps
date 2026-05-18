CUE       := $(or $(shell which cue 2>/dev/null),$(HOME)/.local/bin/cue)
PYTHON    := python3
SCRIPTS   := scripts

.PHONY: gen gen-verify validate install-hooks

# Generate all YAML from CUE specs into repo root
gen:
	$(CUE) vet ./cue/ -c
	$(PYTHON) $(SCRIPTS)/gen-split.py

# CI check: verify generated YAML is up to date
gen-verify:
	$(CUE) vet ./cue/ -c
	$(PYTHON) $(SCRIPTS)/gen-split.py --check

# Validate CUE schemas only
validate:
	$(CUE) vet ./cue/ -c

# Install git hooks
install-hooks:
	git config core.hooksPath .githooks
	@echo "Git hooks installed from .githooks/"
