.DEFAULT_GOAL := build

GO ?= go
PYTHON ?= python3
GOOS ?= $(shell "$(GO)" env GOOS)
GOARCH ?= $(shell "$(GO)" env GOARCH)
VERSION ?= dev
WITH_LLAMA ?= 1
GAME ?= game.json
HOST_GOOS := $(shell "$(GO)" env GOHOSTOS)
HOST_GOARCH := $(shell "$(GO)" env GOHOSTARCH)

export GO GOOS GOARCH VERSION WITH_LLAMA GAME

EXE :=
GUI_LDFLAGS :=
ifeq ($(GOOS),windows)
EXE := .exe
GUI_LDFLAGS := -H=windowsgui
endif

.PHONY: build app llama-server dist test race vet check clean help

build:
	CGO_ENABLED=0 "$(GO)" build -mod=readonly -trimpath -buildvcs=false -ldflags="-s -w $(GUI_LDFLAGS)" -o "bin/aventure$(EXE)" ./cmd/aventure
	CGO_ENABLED=0 "$(GO)" build -mod=readonly -trimpath -buildvcs=false -ldflags="-s -w" -o "bin/catalogue$(EXE)" ./cmd/catalogue

llama-server:
	./scripts/build-llama-server.sh

app:
	"$(PYTHON)" scripts/package-release.py --check
	$(MAKE) build
	@if [ "$$WITH_LLAMA" = "1" ]; then $(MAKE) llama-server; fi
	"$(PYTHON)" scripts/package-release.py --app

# Validate before building; recursive make preserves command-line overrides.
dist:
	"$(PYTHON)" scripts/package-release.py --check
	$(MAKE) build
	@if [ "$$WITH_LLAMA" = "1" ]; then $(MAKE) llama-server; fi
	"$(PYTHON)" scripts/package-release.py

test:
	GOOS="$(HOST_GOOS)" GOARCH="$(HOST_GOARCH)" "$(GO)" test ./...

race:
	CGO_ENABLED=1 GOOS="$(HOST_GOOS)" GOARCH="$(HOST_GOARCH)" "$(GO)" test -race ./...

vet:
	GOOS="$(HOST_GOOS)" GOARCH="$(HOST_GOARCH)" "$(GO)" vet ./...

check: test race vet
	PYTHONDONTWRITEBYTECODE=1 "$(PYTHON)" -m unittest discover -s scripts -p 'test_*.py'

# Keep models, saves, the native runtime and its compilation cache.
clean:
	rm -f bin/aventure bin/aventure.exe bin/catalogue bin/catalogue.exe
	rm -rf dist

help:
	@printf '%s\n' \
	  'make build         Compile aventure et catalogue dans bin/ (sans CGO).' \
	  'make llama-server  Compile le runtime natif pour cette machine.' \
	  'make app           Application macOS dans build/AI Venture.app.' \
	  'make dist          Archive du jeu avec llama-server dans dist/.' \
	  'make dist VERSION=0.1.0  Choisit le nom de version de l’archive.' \
	  'make app GAME=autre-jeu/game.json  Choisit le contenu du bundle sans modifier Go.' \
	  'make dist WITH_LLAMA=0 GOOS=linux GOARCH=amd64  Jeu seul pour une autre cible.' \
	  'make check         Tests, détection de courses concurrentes et go vet.' \
	  'make clean         Supprime les binaires Go et dist/.'
