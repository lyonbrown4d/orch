# Dev environment for orch (align with go.mod go 1.27+).
FROM golang:1.27-bookworm

RUN apt-get update && apt-get install -y --no-install-recommends \
	git \
	ca-certificates \
	curl \
	procps \
	&& rm -rf /var/lib/apt/lists/*

ENV GOTOOLCHAIN=local

# go-task (Taskfile.yml); Delve — /go/bin is on PATH in official golang images
RUN go install github.com/go-task/task/v3/cmd/task@latest \
	&& go install github.com/go-delve/delve/cmd/dlv@latest

WORKDIR /workspace
