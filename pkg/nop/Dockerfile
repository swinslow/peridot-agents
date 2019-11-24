# SPDX-License-Identifier: Apache-2.0 OR GPL-2.0-or-later

FROM golang:1.13

RUN mkdir -p /peridot-agent-nop
WORKDIR /peridot-agent-nop

ADD . /peridot-agent-nop

RUN go get -v ./...
RUN go build
RUN go install github.com/swinslow/peridot-agents/pkg/nop
