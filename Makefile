.DEFAULT_GOAL := help
.PHONY: help setup scan new doctor

help: ## Show this help
	@grep -hE '^[a-z-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-10s\033[0m %s\n", $$1, $$2}'

setup: ## Install and check the toolchain (idempotent)
	@./scripts/setup.sh

scan: ## Run an Aikido vulnerability scan over the repo
	@./scripts/scan.sh

new: ## Scaffold a new experiment: make new NAME=my-idea
	@test -n "$(NAME)" || { echo "usage: make new NAME=my-idea"; exit 1; }
	@./scripts/new-experiment.sh $(NAME)

doctor: ## Report the status of every tool in the stack
	@printf 'entire    '; command -v entire   >/dev/null 2>&1 && entire version | head -1 || echo 'MISSING'
	@printf 'genmedia  '; command -v genmedia >/dev/null 2>&1 && genmedia version | sed -n 's/.*"version": "\([^"]*\)".*/v\1/p' || echo 'MISSING'
	@printf 'aikido    '; command -v aikido-local-scanner >/dev/null 2>&1 && echo 'binary' || (command -v docker >/dev/null 2>&1 && echo 'via docker' || echo 'MISSING')
	@printf 'env       '; test -f .env && echo '.env present' || echo '.env MISSING (cp .env.example .env)'
