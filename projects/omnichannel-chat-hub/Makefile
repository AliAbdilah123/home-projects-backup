SHELL := /usr/bin/env bash

.PHONY: install test build e2e backend-run backend-test frontend-dev frontend-build worker-dev worker-test worker-build fmt clean

install:
	cd frontend && npm install
	cd worker && npm install
	cd backend && go mod download

test: backend-test frontend-build worker-test worker-build

e2e:
	cd backend && go test ./internal/app -run TestMVPE2EVerificationSuite -count=1 -v

build: frontend-build worker-build
	cd backend && go build ./...

backend-run:
	cd backend && go run ./cmd/api

backend-test:
	cd backend && go test ./...

frontend-dev:
	cd frontend && npm run dev -- --host 127.0.0.1

frontend-build:
	cd frontend && npm run build

worker-dev:
	cd worker && npm run dev

worker-test:
	cd worker && npm test

worker-build:
	cd worker && npm run build

fmt:
	cd backend && gofmt -w $$(find . -name '*.go')
	cd frontend && npm run format
	cd worker && npm run format

clean:
	rm -rf frontend/dist worker/dist backend/bin
