// SPDX-License-Identifier: Apache-2.0 OR GPL-2.0-or-later
package main

import (
	"context"
	"io"
	"log"

	"github.com/swinslow/peridot-jobrunner/pkg/agent"
)

func (n *nop) receiver(
	ctx context.Context,
	stream *agent.Agent_NewJobServer,
	recvReq chan<- reqMsg,
) {
	defer log.Printf("==> CLOSING receiver")
	// receiver owns recvReq
	defer close(recvReq)

	exiting := false

	// receive and process messages continually, until we are done.
	// FIXME determine whether it also needs to check ctx.Done() periodically
	for !exiting {
		in, err := (*stream).Recv()
		log.Printf("== agent RECV %s\n", in.String())
		if err == io.EOF {
			// the controller closed the channel
			exiting = true
			break
		}
		if err != nil {
			// error in receiving gRPC message
			exiting = true
			break
		}

		// what type of controller message was this?
		switch x := in.Cm.(type) {
		case *agent.ControllerMsg_Start:
			recvReq <- reqMsg{t: reqStart, cfg: x.Start.Config}
		case *agent.ControllerMsg_Status:
			recvReq <- reqMsg{t: reqStatus}
		}
	}
}
