// SPDX-License-Identifier: Apache-2.0 OR GPL-2.0-or-later
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	sid "github.com/spdx/tools-golang/v0/idsearcher"
	"github.com/spdx/tools-golang/v0/tvsaver"
	"github.com/swinslow/peridot-jobrunner/pkg/agent"
	"github.com/swinslow/peridot-jobrunner/pkg/status"
)

// setStatusError is a helper function to send a statusUpdate
// to the setStatus channel with ERROR status, and with the specified
// error message.
func setStatusError(setStatus chan<- statusUpdate, msg string) {
	setStatus <- statusUpdate{
		run:       status.Status_STOPPED,
		health:    status.Health_ERROR,
		now:       time.Now(),
		outputMsg: msg,
	}
}

// runAgent is the function that actually carries out the substantive
// action of the agent, for this job. It does not do any gRPC communication
// itself, but instead uses signals back to the separate sender goroutine
// to set job status information.
func (i *idsearcher) runAgent(
	ctx context.Context,
	cfg agent.JobConfig,
	setStatus chan<- statusUpdate,
) {
	defer log.Printf("==> CLOSING runAgent")

	// now that we exist, we own setStatus and are responsible for
	// closing it when we are done
	defer close(setStatus)

	// set up package name based on job ID
	// FIXME consider making package name configurable
	packageName := "primary"

	// get searching directory and output file path from configuration
	var packageRootDir string
	for _, codeInput := range cfg.CodeInputs {
		if codeInput.Source == "primary" {
			packageRootDir = codeInput.Path
		}
	}

	// check that we found a primary input with a path
	if packageRootDir == "" {
		// we didn't; error out
		setStatusError(setStatus, "no primary codeInputs specified")
		return
	}

	// check that we got a non-empty output directory
	if cfg.SpdxOutputDir == "" {
		// we didn't; error out
		setStatusError(setStatus, "no spdxOutputDir specified")
		return
	}

	fileOut := filepath.Join(cfg.SpdxOutputDir, "primary.spdx")

	// set up SPDX idsearcher configuration
	searchConfig := &sid.Config{
		// FIXME consider adding unique value (such as job ID or UUID)
		// FIXME to make this unique
		NamespacePrefix: "https://peridot/primary/idsearcher",
		BuilderPathsIgnored: []string{
			"/.git/",
		},
		// FIXME consider making Builder... and SearcherPathsIgnored configurable
	}

	// we're all configured; set status as running
	setStatus <- statusUpdate{run: status.Status_RUNNING}

	// build the SPDX document
	doc, err := sid.BuildIDsDocument(packageName, packageRootDir, searchConfig)
	if err != nil {
		// searcher failed for some reason; error out
		setStatusError(setStatus, fmt.Sprintf("tools-golang/idsearcher failed: %v", err))
		return
	}

	// save the SPDX document to disk
	w, err := os.Create(fileOut)
	if err != nil {
		// can't open file to write SPDX document to disk; error out
		setStatusError(setStatus, fmt.Sprintf("can't open file to write SPDX document to disk: %v", err))
		return
	}
	defer w.Close()

	err = tvsaver.Save2_1(doc, w)
	if err != nil {
		// can't write SPDX document to disk; error out
		setStatusError(setStatus, fmt.Sprintf("can't write SPDX document to disk: %v", err))
		return
	}

	// success!
	setStatus <- statusUpdate{
		run: status.Status_STOPPED,
		now: time.Now(),
	}
}
