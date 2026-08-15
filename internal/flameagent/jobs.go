package flameagent

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"
)

// errJobRunning means a job is already in flight; the agent runs one at a
// time — two concurrent SteamCMD invocations against one install would
// corrupt exactly the state this agent exists to repair.
var errJobRunning = errors.New("a job is already running")

// jobTimeout bounds a runaway SteamCMD. A full validate of Palworld's
// ~20GB install on slow disks can legitimately take a long while; this is
// a backstop, not an expectation.
const jobTimeout = 45 * time.Minute

// logTail caps how many output lines a job retains. The dashboard renders
// this tail live while a job runs, so it's sized for reading a validate's
// progress (~40KB worst case per poll on a LAN), not just for grabbing the
// final error line.
const logTail = 400

// Job is the API view of one unit of background work. Fields are value
// copies — handlers never hand out a pointer into the runner's mutable
// state.
type Job struct {
	ID         string    `json:"id"`
	Kind       string    `json:"kind"`
	State      string    `json:"state"` // running | done | failed
	StartedAt  time.Time `json:"startedAt"`
	FinishedAt time.Time `json:"finishedAt,omitzero"`
	// Error is the failure summary when State is failed: the exec error,
	// or the SteamCMD "Error!" line that betrayed a zero-exit failure.
	Error string   `json:"error,omitempty"`
	Log   []string `json:"log,omitempty"`
}

type jobRunner struct {
	logger *slog.Logger

	mu   sync.Mutex
	jobs map[string]*Job
	// cur is the running job's id, or the most recently started one once
	// nothing is running — what /v1/health reports so flamekeeper can
	// rediscover in-flight work after its own restart.
	cur string
}

func newJobRunner(logger *slog.Logger) *jobRunner {
	return &jobRunner{logger: logger, jobs: map[string]*Job{}}
}

// start launches command args as a new job, refusing while another runs.
func (r *jobRunner) start(kind, command string, args []string) (*Job, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if cur := r.jobs[r.cur]; cur != nil && cur.State == "running" {
		return nil, errJobRunning
	}

	job := &Job{ID: newJobID(), Kind: kind, State: "running", StartedAt: time.Now().UTC()}
	r.jobs[job.ID] = job
	r.cur = job.ID
	go r.run(job.ID, command, args)
	return copyJob(job), nil
}

// ansiEscape strips the terminal color codes SteamCMD sprinkles through
// its output ("Loading Steam API...\x1b[0mOK"); the dashboard renders the
// log as plain text, same as flamekeeper does for container logs.
var ansiEscape = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// retryableSteamError matches failures a fresh SteamCMD bootstrap commonly
// throws exactly once: "ERROR! Failed to install app '...' (Missing
// configuration)" happens when the first app_update runs before the
// app-info cache is populated (the agent's HOME is ephemeral, so every new
// container starts cold), and an immediate retry succeeds.
func retryableSteamError(errMsg string) bool {
	return strings.Contains(strings.ToLower(errMsg), "missing configuration")
}

// run executes the command, streaming output into the job's capped log,
// retrying once for SteamCMD's known cold-start flake. Deliberately
// detached from any request context: the whole point of job semantics is
// that the caller can go away.
func (r *jobRunner) run(id, command string, args []string) {
	const maxAttempts = 2
	for attemptNo := 1; ; attemptNo++ {
		errMsg := r.attempt(id, command, args)
		if errMsg == "" || attemptNo >= maxAttempts || !retryableSteamError(errMsg) {
			r.finish(id, errMsg)
			return
		}
		r.appendLog(id, fmt.Sprintf(
			"flameagent: retrying (%d/%d) — %q is SteamCMD's usual cold-bootstrap flake and normally clears on the second run",
			attemptNo+1, maxAttempts, errMsg))
	}
}

// attempt is one execution; "" means success.
func (r *jobRunner) attempt(id, command string, args []string) string {
	ctx, cancel := context.WithTimeout(context.Background(), jobTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, command, args...)
	stdout, err := cmd.StdoutPipe()
	if err == nil {
		cmd.Stderr = cmd.Stdout
		err = cmd.Start()
	}
	if err != nil {
		return "failed to start: " + err.Error()
	}

	// SteamCMD's exit code is not always trustworthy: failures like
	// "Error! App '2394010' state is 0x602 after update job" and "ERROR!
	// Failed to install app" (yes, both spellings) have shipped with exit
	// 0. Watch the output for the telltale line.
	steamError := ""
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := strings.TrimRight(ansiEscape.ReplaceAllString(scanner.Text(), ""), "\r")
		r.appendLog(id, line)
		trimmed := strings.TrimSpace(line)
		if steamError == "" && strings.HasPrefix(strings.ToLower(trimmed), "error!") {
			steamError = trimmed
		}
	}

	switch waitErr := cmd.Wait(); {
	case ctx.Err() != nil:
		return "timed out after " + jobTimeout.String()
	case steamError != "":
		// Prefer SteamCMD's own line over a bare "exit status 8" — it
		// names the cause, and the retry heuristic keys off it.
		return steamError
	case waitErr != nil:
		return waitErr.Error()
	default:
		return ""
	}
}

func (r *jobRunner) appendLog(id, line string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	job := r.jobs[id]
	job.Log = append(job.Log, line)
	if len(job.Log) > logTail {
		job.Log = job.Log[len(job.Log)-logTail:]
	}
}

func (r *jobRunner) finish(id, errMsg string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	job := r.jobs[id]
	job.FinishedAt = time.Now().UTC()
	if errMsg == "" {
		job.State = "done"
		r.logger.Info("job finished", "job", id, "kind", job.Kind)
	} else {
		job.State = "failed"
		job.Error = errMsg
		r.logger.Error("job failed", "job", id, "kind", job.Kind, "error", errMsg)
	}
}

// current returns the running job, or the most recently started one.
func (r *jobRunner) current() *Job {
	r.mu.Lock()
	defer r.mu.Unlock()
	return copyJob(r.jobs[r.cur])
}

func (r *jobRunner) get(id string) *Job {
	r.mu.Lock()
	defer r.mu.Unlock()
	return copyJob(r.jobs[id])
}

func copyJob(j *Job) *Job {
	if j == nil {
		return nil
	}
	out := *j
	out.Log = append([]string(nil), j.Log...)
	return &out
}
