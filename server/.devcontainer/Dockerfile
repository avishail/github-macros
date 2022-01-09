FROM golang:1.16

ENV CACHE_BREAKER 1
ENV DEBIAN_FRONTEND noninteractive
RUN apt update \
    && apt install -y --no-install-recommends git wget unzip \
    && apt-get autoremove -y \
    && apt-get clean -y \
    && rm -rf /var/lib/apt/lists/*

RUN go get -v github.com/golangci/golangci-lint/cmd/golangci-lint@latest

RUN go get -v golang.org/x/tools/gopls \
    golang.org/x/lint/golint@latest \
    github.com/uudashr/gopkgs/v2/cmd/gopkgs \
    github.com/ramya-rao-a/go-outline@latest \
    github.com/cweill/gotests/...@latest \
    github.com/go-delve/delve/cmd/dlv@master \
    github.com/vektra/mockery/v2/.../ \
    github.com/josharian/impl@latest \
    github.com/fatih/gomodifytags@latest \
    github.com/golang/protobuf/protoc-gen-go@latest \
    golang.org/x/tools/gopls@latest

# https://github.com/golang/vscode-go/blob/master/docs/debugging.md#bookmark=id.xuaxofprncd5
RUN GOBIN=/tmp/ go install github.com/go-delve/delve/cmd/dlv@master && mv /tmp/dlv $GOPATH/bin/dlv-dap

ARG PROTOC=protoc-3.18.1-linux-x86_64.zip
RUN wget -P /tmp https://github.com/protocolbuffers/protobuf/releases/download/v3.18.1/${PROTOC}
RUN unzip -u -j /tmp/${PROTOC} "bin/protoc" -d /usr/local/bin/
RUN rm /tmp/${PROTOC}
