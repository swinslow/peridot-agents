// SPDX-License-Identifier: Apache-2.0 OR GPL-2.0-or-later
package main

import (
	"io"
	"log"

	"github.com/swinslow/peridot-core/pkg/agent"
)

type idsearcher struct {
	name        string
	agentConfig string
}

// NewJob is the bidirectional streaming RPC that communicates with
// the Controller.
func (i *idsearcher) NewJob(stream agent.Agent_NewJobServer) error {
	// now in a new, separate goroutine to handle this stream.

	// create channels for communication between goroutines
	// defer the ones that this function retains ownership of
	jobExited := make(chan interface{})
	defer close(jobExited)
	msgWanted := make(chan msgType)
	defer close(msgWanted)
	cancelWanted := make(chan interface{})
	defer close(cancelWanted)
	setStatus := make(chan statusUpdate)
	// if this job exits or cancels before runAgent is called,
	// we will still own setStatus and will need to close it manually
	doneSending := make(chan interface{})

	// flag for whether the runAgent goroutine has been created yet
	var createdAgent bool

	// create sender goroutine
	go i.sender(&stream, jobExited, setStatus, msgWanted, doneSending)

	// receive and process messages continually, until we are done.
	var retval error
	var exiting bool
	for !exiting {
		in, err := stream.Recv()
		log.Printf("== agent RECV %#v\n", in)
		if err == io.EOF {
			// the controller closed the channel
			exiting = true
			break
		}
		if err != nil {
			// error in receiving gRPC message; fail handler
			retval = err
			exiting = true
			break
		}

		// what type of controller message was this?
		switch x := in.Cm.(type) {
		case *agent.ControllerMsg_Describe:
			msgWanted <- msgType{describe: true}
		case *agent.ControllerMsg_Start:
			// now create runAgent goroutine
			go i.runAgent(*x.Start.Config, jobExited, setStatus, cancelWanted)
			createdAgent = true
		case *agent.ControllerMsg_Status:
			msgWanted <- msgType{status: true}
		case *agent.ControllerMsg_Cancel:
			// FIXME this agent cannot presently handle cancel signals
			//cancelWanted <- true
		}
	}

	// tell children we're closing
	close(jobExited)

	// if we never got around to creating the runAgent goroutine,
	// we still own setStatus and should close it
	if !createdAgent {
		close(setStatus)
	}
	// wait for sender goroutine to signal it is done before closing out
	<-doneSending

	return retval
}
