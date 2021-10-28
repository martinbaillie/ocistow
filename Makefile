SHELL := $(shell which bash)
.SHELLFLAGS = -o pipefail -c
.EXPORT_ALL_VARIABLES: ;
ifndef DEBUG
.SILENT: ;
endif

env/.bootstrap: ; npx cdk bootstrap && touch $@
deploy: env/.bootstrap; npx cdk deploy
destroy: ; npx cdk destroy
.PHONY: deploy destroy
