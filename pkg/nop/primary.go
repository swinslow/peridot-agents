// SPDX-License-Identifier: Apache-2.0 OR GPL-2.0-or-later
package main

import (
	"context"
	"log"
	"time"

	"github.com/swinslow/peridot-jobrunner/pkg/agent"
	"github.com/swinslow/peridot-jobrunner/pkg/status"
)

type nop struct{}

type reqType uint8

const (
	reqStart = reqType(iota)
	reqStatus
)

type reqMsg struct {
	t   reqType
	cfg *agent.JobConfig
}

type statusCurrent struct {
	run            status.Status
	health         status.Health
	started        time.Time
	finished       time.Time
	outputMessages string
	errorMessages  string
}

type statusUpdate struct {
	run       status.Status
	health    status.Health
	now       time.Time
	outputMsg string
	errorMsg  string
}

type rptType struct {
	sRpt   bool
	status statusCurrent
}

// NewJob is the bidirectional streaming RPC that communicates with
// the Controller.
func (n *nop) NewJob(stream agent.Agent_NewJobServer) error {
	defer log.Printf("==> CLOSING NewJob")
	// now in a new, separate goroutine to handle this stream.

	// this main goroutine is responsible for tracking the job's status
	st := statusCurrent{
		run:            status.Status_STARTUP,
		health:         status.Health_OK,
		started:        time.Now(),
		outputMessages: "",
	}

	// create context for child goroutines
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// create channels for communication between goroutines
	// defer the ones that this function retains ownership of
	rptWanted := make(chan rptType)
	defer close(rptWanted)

	setStatus := make(chan statusUpdate)
	// runAgent will own setStatus channel, unless we never create
	// the runAgent goroutine -- in which case we need to close it
	createdAgent := false

	recvReq := make(chan reqMsg)
	// receiver will own recvReq channel

	// create sender goroutine
	go n.sender(ctx, &stream, rptWanted)

	// create receiver goroutine
	go n.receiver(ctx, &stream, recvReq)

	// now we just sit and listen on channels until it's time to exit
	exiting := false
	for !exiting {
		select {
		case <-stream.Context().Done():
			// gRPC stream from controller has set us to be done
			// signal the children and wrap up
			cancel()
			exiting = true
			break
		case su := <-setStatus:
			// update status values where filled in
			if su.run != status.Status_STATUS_SAME {
				st.run = su.run
			}
			if su.health != status.Health_HEALTH_SAME {
				st.health = su.health
			}
			if su.outputMsg != "" {
				st.outputMessages += su.outputMsg
			}
			// additionally, if run status is now STOPPED, we are finished
			// and exiting
			if su.run == status.Status_STOPPED {
				st.finished = su.now
				exiting = true
			}
			// finally, tell sender to send a status update
			rptWanted <- rptType{sRpt: true, status: st}
		case r, ok := <-recvReq:
			if !ok {
				// gRPC reads are now closed; time to wrap up
				cancel()
				exiting = true
				break
			}
			switch r.t {
			case reqStart:
				// create agent goroutine
				go n.runAgent(ctx, *r.cfg, setStatus)
				createdAgent = true
			case reqStatus:
				rptWanted <- rptType{sRpt: true, status: st}
			}
		}
	}

	// if we never got around to creating the runAgent goroutine,
	// we still own setStatus and should close it
	if !createdAgent {
		close(setStatus)
	} else {
		// need to make sure the setStatus channel gets unblocked
		// before we exit
		for {
			_, ok := <-setStatus
			if !ok {
				break
			}
		}
	}

	// also need to make sure the recvReq channel gets unblocked
	// before we exit
	for {
		_, ok := <-recvReq
		if !ok {
			break
		}
	}

	return nil
}
