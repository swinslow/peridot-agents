// SPDX-License-Identifier: Apache-2.0 OR GPL-2.0-or-later
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/swinslow/peridot-jobrunner/pkg/agent"
	git "gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing"
)

// setStatusError is a helper function to send a statusUpdate
// to the setStatus channel with ERROR status, and with the specified
// error message.
func setStatusError(setStatus chan<- statusUpdate, msg string) {
	setStatus <- statusUpdate{
		run:      agent.JobRunStatus_STOPPED,
		health:   agent.JobHealthStatus_ERROR,
		now:      time.Now(),
		errorMsg: msg,
	}
}

func getURLToRepo(org string, repo string) string {
	// FIXME check for e.g. no slashes or problematic chars in org or repo!
	// FIXME perhaps use path.Join, except not clear yet how to use it with two slashes
	// FIXME for protocol name
	return "https://github.com/" + org + "/" + repo + ".git"
}

// runAgent is the function that actually carries out the substantive
// action of the agent, for this job. It does not do any gRPC communication
// itself, but instead uses signals back to the separate sender goroutine
// to set job status information.
func (ag *retrieveGithub) runAgent(
	ctx context.Context,
	cfg agent.JobConfig,
	setStatus chan<- statusUpdate,
) {
	defer log.Printf("==> CLOSING runAgent")

	// now that we exist, we own setStatus and are responsible for
	// closing it when we are done
	defer close(setStatus)

	// make sure we've got the config values we need
	org := ""
	repo := ""
	branch := ""
	commit := ""

	for _, jkv := range cfg.Jkvs {
		if jkv.Key == "org" {
			org = jkv.Value
		}
		if jkv.Key == "repo" {
			repo = jkv.Value
		}
		if jkv.Key == "commit" {
			commit = jkv.Value
		}
		if jkv.Key == "branch" {
			branch = jkv.Value
		}
	}

	errorMsg := ""
	if org == "" {
		errorMsg += "org key/value not specified"
	}
	if repo == "" {
		if errorMsg != "" {
			errorMsg += "; "
		}
		errorMsg += "repo key/value not specified"
	}
	if errorMsg != "" {
		setStatusError(setStatus, errorMsg)
		return
	}

	if commit != "" && branch != "" {
		setStatusError(setStatus, "both commit and branch were specified, but are mutually exclusive")
		return
	}

	// check that we got a non-empty code output directory
	if cfg.CodeOutputDir == "" {
		// we didn't; error out
		setStatusError(setStatus, "no codeOutputDir specified")
		return
	}

	// try to create directory at this location
	err := os.MkdirAll(cfg.CodeOutputDir, os.ModePerm)
	if err != nil {
		// we couldn't; error out
		setStatusError(setStatus, fmt.Sprintf("couldn't create codeOutputDir %s: %v", cfg.CodeOutputDir, err))
		return
	}

	// check whether path exists in filesystem
	fi, err := os.Stat(cfg.CodeOutputDir)
	if err != nil {
		// it doesn't; error out
		setStatusError(setStatus, fmt.Sprintf("tried to create codeOutputDir %s but path not found: %v", cfg.CodeOutputDir, err))
		return
	}

	// check whether path is a directory
	if !fi.IsDir() {
		// it isn't; error out
		setStatusError(setStatus, fmt.Sprintf("tried to create codeOutputDir %s but it is not a directory: %v", cfg.CodeOutputDir, err))
		return
	}

	// // check whether path is writable
	// if unix.Access(path, unix.W_OK) != nil {
	// 	// it isn't; error out
	// 	setStatusError(setStatus, fmt.Sprintf("tried to create codeOutputDir %s but it is not writable: %v", cfg.CodeOutputDir, err))
	// 	return
	// }

	// we're all configured; set status as running
	setStatus <- statusUpdate{run: agent.JobRunStatus_RUNNING}

	// clone the repo
	srcURL := getURLToRepo(org, repo)
	cloneOpts := &git.CloneOptions{
		URL:               srcURL,
		RecurseSubmodules: git.DefaultSubmoduleRecursionDepth,
		Progress:          os.Stdout,
	}
	if branch != "" {
		cloneOpts.ReferenceName = plumbing.NewBranchReferenceName(branch)
	}

	r, err := git.PlainClone(cfg.CodeOutputDir, false, cloneOpts)
	if err != nil {
		// couldn't clone the repo; error out
		setStatusError(setStatus, fmt.Sprintf("failed to clone %s: %v", srcURL, err))
		return
	}

	// check that we can get the repo worktree
	w, err := r.Worktree()
	if err != nil {
		// couldn't get the repo worktree; error out
		setStatusError(setStatus, fmt.Sprintf("can't get worktree after cloning %s: %v", srcURL, err))
		return
	}

	// if a particular commit was specified, check it out
	if commit != "" {
		err = w.Checkout(&git.CheckoutOptions{
			Hash: plumbing.NewHash(commit),
		})
	}

	// success!
	setStatus <- statusUpdate{
		run: agent.JobRunStatus_STOPPED,
		now: time.Now(),
	}
}
