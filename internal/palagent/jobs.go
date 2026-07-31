package palagent

import (
	"bufio"
	"context"
	"errors"
	"log/slog"
	"os/exec"
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
	// nothing is running — what /v1/health reports so palcon can
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

// run executes the command, streaming output into the job's capped log.
// Deliberately detached from any request context: the whole point of job
// semantics is that the caller can go away.
func (r *jobRunner) run(id, command string, args []string) {
	ctx, cancel := context.WithTimeout(context.Background(), jobTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, command, args...)
	stdout, err := cmd.StdoutPipe()
	if err == nil {
		cmd.Stderr = cmd.Stdout
		err = cmd.Start()
	}
	if err != nil {
		r.finish(id, "failed to start: "+err.Error())
		return
	}

	// SteamCMD's exit code is not always trustworthy: disk/corruption
	// failures like "Error! App '2394010' state is 0x602 after update job"
	// have shipped with exit 0. Watch the output for the telltale line.
	steamError := ""
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		r.appendLog(id, line)
		if steamError == "" && strings.HasPrefix(strings.TrimSpace(line), "Error!") {
			steamError = strings.TrimSpace(line)
		}
	}

	switch waitErr := cmd.Wait(); {
	case ctx.Err() != nil:
		r.finish(id, "timed out after "+jobTimeout.String())
	case waitErr != nil:
		r.finish(id, waitErr.Error())
	case steamError != "":
		r.finish(id, steamError)
	default:
		r.finish(id, "")
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
