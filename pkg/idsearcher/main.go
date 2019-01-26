// SPDX-License-Identifier: Apache-2.0 OR GPL-2.0-or-later
package main

import (
	"log"
	"net"

	"google.golang.org/grpc"

	"github.com/swinslow/peridot-core/pkg/agent"
)

const (
	port = ":9001"
)

func main() {
	// open a socket for listening
	lis, err := net.Listen("tcp", port)
	if err != nil {
		log.Fatalf("couldn't open port %v: %v", port, err)
	}

	// create and register new GRPC server for agent
	server := grpc.NewServer()
	agent.RegisterAgentServer(server, &idsearcher{
		name:        "idsearcher",
		agentConfig: "",
	})

	// start grpc server
	if err := server.Serve(lis); err != nil {
		log.Fatalf("couldn't start server: %v", err)
	}
}
