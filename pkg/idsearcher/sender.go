// SPDX-License-Identifier: Apache-2.0 OR GPL-2.0-or-later
package main

import (
	"context"
	"log"

	"github.com/swinslow/peridot-core/pkg/agent"
)

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
	ctx context.Context,
	stream *agent.Agent_NewJobServer,
	rptWanted <-chan rptType,
) {
	defer log.Printf("==> CLOSING sender")
	// these are flags for when to send a report and for when we are exiting
	var exiting bool

	// start listening to channels in infinite loop
	for !exiting {
		// FIXME consider whether adding a timeout is appropriate
		select {
		case <-ctx.Done():
			// the main goroutine (NewJob handler) signalled that the
			// stream is closed. Go ahead and wrap up.
			exiting = true
		case mw := <-rptWanted:
			// the NewJob handler received a message that the client
			// wants a report sent. Set the appropriate variable(s),
			// and we'll actually send when we get out of the current
			// loop.
			if mw.dRpt {
				// send back a DescribeReport now
				rpt := i.getDescribeReport()
				am := &agent.AgentMsg{Am: &agent.AgentMsg_Describe{Describe: rpt}}
				log.Printf("== agent SEND Describe %#v\n", rpt)
				if err := (*stream).Send(am); err != nil {
					// error in sending gRPC message; fail handler
					exiting = true
				}
			}
			if mw.sRpt {
				// send back a StatusReport now
				rpt := &agent.StatusReport{
					RunStatus:      mw.status.run,
					HealthStatus:   mw.status.health,
					TimeStarted:    mw.status.started.Unix(),
					TimeFinished:   mw.status.finished.Unix(),
					OutputMessages: mw.status.outputMessages,
					ErrorMessages:  mw.status.errorMessages,
				}
				am := &agent.AgentMsg{Am: &agent.AgentMsg_Status{Status: rpt}}
				log.Printf("== agent SEND Status %#v\n", rpt)
				if err := (*stream).Send(am); err != nil {
					// error in sending gRPC message; fail handler
					exiting = true
				}
			}
		}
	}

	// if we get here, we're exiting
}
