// SPDX-License-Identifier: Apache-2.0 OR GPL-2.0-or-later
package main

import (
	"context"
	"log"
	"time"

	"github.com/swinslow/peridot-core/pkg/agent"
)

type idsearcher struct {
	name        string
	agentConfig string
}

type reqType uint8

const (
	reqDescribe = reqType(iota)
	reqStart
	reqStatus
)

type reqMsg struct {
	t   reqType
	cfg *agent.JobConfig
}

type statusCurrent struct {
	run            agent.JobRunStatus
	health         agent.JobHealthStatus
	started        time.Time
	finished       time.Time
	outputMessages string
	errorMessages  string
}

type statusUpdate struct {
	run       agent.JobRunStatus
	health    agent.JobHealthStatus
	now       time.Time
	outputMsg string
	errorMsg  string
}

type rptType struct {
	dRpt   bool
	sRpt   bool
	status statusCurrent
}

// NewJob is the bidirectional streaming RPC that communicates with
// the Controller.
func (i *idsearcher) NewJob(stream agent.Agent_NewJobServer) error {
	defer log.Printf("==> CLOSING NewJob")
	// now in a new, separate goroutine to handle this stream.

	// this main goroutine is responsible for tracking the job's status
	status := statusCurrent{
		run:            agent.JobRunStatus_STARTUP,
		health:         agent.JobHealthStatus_OK,
		started:        time.Now(),
		outputMessages: "",
		errorMessages:  "",
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
	go i.sender(ctx, &stream, rptWanted)

	// create receiver goroutine
	go i.receiver(ctx, &stream, recvReq)

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
			if su.run != agent.JobRunStatus_STATUS_SAME {
				status.run = su.run
			}
			if su.health != agent.JobHealthStatus_HEALTH_SAME {
				status.health = su.health
			}
			if su.outputMsg != "" {
				status.outputMessages += su.outputMsg
			}
			if su.errorMsg != "" {
				status.errorMessages += su.errorMsg
			}
			// additionally, if run status is now STOPPED, we are finished
			// and exiting
			if su.run == agent.JobRunStatus_STOPPED {
				status.finished = su.now
				exiting = true
			}
			// finally, tell sender to send a status update
			rptWanted <- rptType{sRpt: true, status: status}
		case r, ok := <-recvReq:
			if !ok {
				// gRPC reads are now closed; time to wrap up
				cancel()
				exiting = true
				break
			}
			switch r.t {
			case reqDescribe:
				rptWanted <- rptType{dRpt: true}
			case reqStart:
				// create agent goroutine
				go i.runAgent(ctx, *r.cfg, setStatus)
				createdAgent = true
			case reqStatus:
				rptWanted <- rptType{sRpt: true, status: status}
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
