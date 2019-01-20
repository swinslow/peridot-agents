// SPDX-License-Identifier: Apache-2.0 OR GPL-2.0-or-later
package main

import (
	"log"
	"time"

	"github.com/swinslow/peridot-core/pkg/agent"
)

type statusUpdate struct {
	run       agent.JobRunStatus
	health    agent.JobHealthStatus
	cancel    agent.JobCancelStatus
	now       time.Time
	outputMsg string
	errorMsg  string
}

type msgType struct {
	describe bool
	status   bool
}

func (i *idsearcher) getDescribeReport() *agent.DescribeReport {
	return &agent.DescribeReport{
		Name:        i.name,
		Type:        "idsearcher",
		AgentConfig: i.agentConfig,
		Capabilities: []string{
			"codereader",
			"spdxwriter",
		},
	}
}

// sender is the only goroutine permitted to make Send calls on the
// gRPC stream. Even the main handler will not call Send.
// sender is also responsible for listening for status change requests
// from runAgent.
func (i *idsearcher) sender(
	stream *agent.Agent_NewJobServer,
	jobExited <-chan interface{},
	setStatus <-chan statusUpdate,
	msgWanted <-chan msgType,
	doneSending chan<- interface{},
) {
	// when we are exiting, we should close the doneSending channel,
	// which sender owns
	defer close(doneSending)

	// this is where this job's status variables are actually stored
	run := agent.JobRunStatus_STARTUP
	health := agent.JobHealthStatus_OK
	cancel := agent.JobCancelStatus_NOCANCEL
	started := time.Now()
	var finished time.Time
	outputMessages := ""
	errorMessages := ""

	// these are flags for when to send a report and for when we are exiting
	var describeWanted bool
	var statusWanted bool
	var exiting bool

	// start listening to channels in infinite loop
	for !exiting {
		// FIXME consider whether adding a timeout is appropriate
		select {
		case <-jobExited:
			// the main goroutine (NewJob handler) signalled that the
			// stream is closed. Go ahead and wrap up without sending
			// a status report back.
			exiting = true
		case mw := <-msgWanted:
			// the NewJob handler received a message that the client
			// wants a report sent. Set the appropriate variable(s),
			// and we'll actually send when we get out of the current
			// loop.
			if mw.describe {
				describeWanted = true
			}
			if mw.status {
				statusWanted = true
			}
		case st := <-setStatus:
			// update status values where filled in
			if st.run != agent.JobRunStatus_STATUS_SAME {
				run = st.run
			}
			if st.health != agent.JobHealthStatus_HEALTH_SAME {
				health = st.health
			}
			if st.cancel != agent.JobCancelStatus_CANCEL_SAME {
				cancel = st.cancel
			}
			if st.outputMsg != "" {
				outputMessages += st.outputMsg
			}
			if st.errorMsg != "" {
				errorMessages += st.errorMsg
			}

			statusWanted = true

			// additionally, if run status is now STOPPED, we are finished
			// and exiting
			if st.run == agent.JobRunStatus_STOPPED {
				finished = st.now
				exiting = true
			}
		}

		// we're past the channel signal check and/or timeout.
		// check whether there are messages to be sent
		if describeWanted {
			// send back a DescribeReport now
			rpt := i.getDescribeReport()
			am := &agent.AgentMsg{Am: &agent.AgentMsg_Describe{Describe: rpt}}
			log.Printf("== agent SEND %#v\n", am)
			if err := (*stream).Send(am); err != nil {
				// error in sending gRPC message; fail handler
				exiting = true
			}
			describeWanted = false
		}

		if statusWanted {
			// send back a StatusReport now
			am := &agent.AgentMsg{Am: &agent.AgentMsg_Status{Status: &agent.StatusReport{
				RunStatus:      run,
				HealthStatus:   health,
				CancelStatus:   cancel,
				TimeStarted:    started.Unix(),
				TimeFinished:   finished.Unix(),
				OutputMessages: outputMessages,
				ErrorMessages:  errorMessages,
			}}}
			log.Printf("== agent SEND %#v\n", am)
			if err := (*stream).Send(am); err != nil {
				// error in sending gRPC message; fail handler
				exiting = true
			}
			statusWanted = false
		}
	}

	// if we get here, we're exiting
	// doneSending will get closed because we deferred it
}
