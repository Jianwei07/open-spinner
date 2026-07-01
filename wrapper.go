package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// runWrapperCmd implements `open-spinner run [--agent name] [--id id] -- <agent> [args...]`.
//
// It exists for agents with no lifecycle hooks at all (the "pi, jcode, and
// many more" case). Coverage is coarse and honest: the whole child
// process lifetime is "busy," not the moments the agent is actually
// thinking versus idle at its own prompt. That's a real limitation, not a
// bug — anything finer requires scraping terminal output, which this
// project deliberately does not do.
func runWrapperCmd(args []string) error {
	fs := newFlagSet("run")
	agent := fs.String("agent", "", "agent name (defaults to the wrapped command's base name)")
	id := fs.String("id", "", "status id")

	flagArgs, cmdArgs := splitOnDoubleDash(args)
	if err := fs.Parse(flagArgs); err != nil {
		return err
	}
	if len(cmdArgs) == 0 {
		cmdArgs = fs.Args()
	}
	if len(cmdArgs) == 0 {
		return errors.New("usage: open-spinner run [--agent name] [--id id] -- <command> [args...]")
	}

	agentName := *agent
	if agentName == "" {
		agentName = filepath.Base(cmdArgs[0])
	}
	resolvedID := resolveID(*id, agentName)

	cwd, _ := os.Getwd()
	dir := statusDir()
	busy := Status{
		V:     1,
		ID:    resolvedID,
		Agent: agentName,
		State: "busy",
		CWD:   cwd,
		PID:   os.Getpid(),
		TTY:   resolveTTY(),
		// No TTL: unlike a hook-driven status (refreshed every turn, so a
		// TTL catches a crashed agent), this status is only ever written
		// once and explicitly removed when the wrapped process exits
		// below. A run longer than a TTL would otherwise be derived
		// "stale" and the renderer would restore the title while the
		// agent is still working.
		UpdatedAt: time.Now().UTC(),
	}
	if err := writeStatus(dir, busy); err != nil {
		return err
	}
	maybeSpawnRenderer(busy)

	cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	runErr := cmd.Run()

	// Clear rather than set idle: this triggers the renderer's immediate
	// restore path instead of waiting out the idle-grace window, since
	// the wrapped process is fully gone, not just between turns.
	_ = removeStatus(dir, resolvedID)

	return runErr
}

func splitOnDoubleDash(args []string) (before, after []string) {
	for i, a := range args {
		if a == "--" {
			return args[:i], args[i+1:]
		}
	}
	return args, nil
}
