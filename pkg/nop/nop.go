// SPDX-License-Identifier: Apache-2.0 OR GPL-2.0-or-later
package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/swinslow/peridot-jobrunner/pkg/agent"
	"github.com/swinslow/peridot-jobrunner/pkg/status"
)

// setStatusError is a helper function to send a statusUpdate
// to the setStatus channel with ERROR status, and with the specified
// error message.
func setStatusError(setStatus chan<- statusUpdate, msg string) {
	setStatus <- statusUpdate{
		run:      status.Status_STOPPED,
		health:   status.Health_ERROR,
		now:      time.Now(),
		errorMsg: msg,
	}
}

// runAgent is the function that actually carries out the substantive
// action of the agent, for this job. It does not do any gRPC communication
// itself, but instead uses signals back to the separate sender goroutine
// to set job status information.
func (n *nop) runAgent(
	ctx context.Context,
	cfg agent.JobConfig,
	setStatus chan<- statusUpdate,
) {
	defer log.Printf("==> CLOSING runAgent")

	// now that we exist, we own setStatus and are responsible for
	// closing it when we are done
	defer close(setStatus)

	// all we're going to do is to get the key-value pairs from
	// the job configuration; sleep for a couple of seconds; then
	// return them as output

	// we're all configured; set status as running
	setStatus <- statusUpdate{run: status.Status_RUNNING}

	// build output string
	strs := []string{}
	for _, jkv := range cfg.Jkvs {
		s := fmt.Sprintf("%s => %s", jkv.Key, jkv.Value)
		strs = append(strs, s)
	}

	// sleep for a couple of seconds
	time.Sleep(2 * time.Second)

	// success!
	setStatus <- statusUpdate{
		run:       status.Status_STOPPED,
		now:       time.Now(),
		outputMsg: strings.Join(strs, "\n"),
	}
}
