package worker

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type cronJob struct {
	JobID   string
	Pid     int
	Command string
	Delete  bool `json:"-"`
}

func (job cronJob) ToString() string {
	marshal, _ := json.Marshal(job)
	return fmt.Sprintf("## BEGIN QOR JOB %v # %v\n%v\n## END QOR JOB\n", job.JobID, string(marshal), job.Command)
}

// Cron implemented a worker Queue based on cronjob
type Cron struct {
	CLIArgs  CLIArgs
	Jobs     []*cronJob
	CronJobs []string
	mutex    sync.Mutex `sql:"-"`
}

// NewCronQueue initialize a Cron queue
func NewCronQueue(cliArgs CLIArgs) *Cron {
	return &Cron{CLIArgs: cliArgs}
}

func (cron *Cron) parseJobs() []*cronJob {
	cron.mutex.Lock()

	cron.Jobs = []*cronJob{}
	cron.CronJobs = []string{}
	if out, err := exec.Command("crontab", "-l").Output(); err == nil {
		var inQorJob bool
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			if strings.HasPrefix(line, "## BEGIN QOR JOB") {
				inQorJob = true
				if idx := strings.Index(line, "{"); idx > 1 {
					var job cronJob
					if json.Unmarshal([]byte(line[idx-1:]), &job) == nil {
						cron.Jobs = append(cron.Jobs, &job)
					}
				}
			}

			if !inQorJob {
				cron.CronJobs = append(cron.CronJobs, line)
			}

			if strings.HasPrefix(line, "## END QOR JOB") {
				inQorJob = false
			}
		}
	}
	return cron.Jobs
}

func (cron *Cron) writeCronJob() error {
	defer cron.mutex.Unlock()
	cmd := exec.Command("crontab", "-")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	stdin, _ := cmd.StdinPipe()
	for _, cronJob := range cron.CronJobs {
		stdin.Write([]byte(cronJob + "\n"))
	}

	for _, job := range cron.Jobs {
		if !job.Delete {
			stdin.Write([]byte(job.ToString() + "\n"))
		}
	}
	stdin.Close()
	return cmd.Run()
}

// Add a job to cron queue
func (cron *Cron) Add(job QorJobInterface) (err error) {
	cron.parseJobs()
	defer cron.writeCronJob()
	jobId := job.UID()

	var jobs []*cronJob
	for _, cronJob := range cron.Jobs {
		if cronJob.JobID != jobId {
			jobs = append(jobs, cronJob)
		}
	}

	if scheduler, ok := job.GetArgument().(Scheduler); ok && scheduler.GetScheduleTime() != nil {
		cmdArgs := cron.CLIArgs.ExecJob(job)
		scheduleTime := scheduler.GetScheduleTime().In(time.Local)
		job.SetStatus(JobStatusScheduled)

		currentPath, _ := os.Getwd()
		jobCmd := fmt.Sprintf("%d %d %d %d * cd %v; %v\n", scheduleTime.Minute(), scheduleTime.Hour(),
			scheduleTime.Day(), scheduleTime.Month(), currentPath, strings.Join(cmdArgs, " "))
		jobs = append(jobs, &cronJob{
			JobID:   jobId,
			Command: jobCmd,
		})
	} else {
		cmdArgs := cron.CLIArgs.ExecJob(job)
		cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
		if err = cmd.Start(); err == nil {
			jobs = append(jobs, &cronJob{JobID: job.UID(), Pid: cmd.Process.Pid})
			cmd.Process.Release()
		}
	}
	cron.Jobs = jobs

	return
}

// Run a job from cron queue
func (cron *Cron) Run(qorJob QorJobInterface) error {
	job := qorJob.GetJob()
	uid := qorJob.UID()

	if job.Handler != nil {
		err := job.Handler(qorJob.GetSerializableArgument(qorJob), qorJob)
		if err == nil {
			cron.parseJobs()
			defer cron.writeCronJob()
			for _, cronJob := range cron.Jobs {
				if cronJob.JobID == uid {
					cronJob.Delete = true
				}
			}
		}
		return err
	}

	return errors.New("no handler found for job " + job.Name)
}

// Kill a job from cron queue
func (cron *Cron) Kill(job QorJobInterface) (err error) {
	cron.parseJobs()
	defer cron.writeCronJob()
	uid := job.UID()

	for _, cronJob := range cron.Jobs {
		if cronJob.JobID == uid {
			if process, err := os.FindProcess(cronJob.Pid); err == nil {
				if err = process.Kill(); err == nil {
					cronJob.Delete = true
					return nil
				}
			}
			return err
		}
	}
	return fmt.Errorf("failed to find job %q", uid)
}

// Remove a job from cron queue
func (cron *Cron) Remove(job QorJobInterface) error {
	cron.parseJobs()
	defer cron.writeCronJob()
	uid := job.UID()

	for _, cronJob := range cron.Jobs {
		if cronJob.JobID == uid {
			if cronJob.Pid == 0 {
				cronJob.Delete = true
				return nil
			}
			return fmt.Errorf("failed to remove current job %q as it is running", uid)
		}
	}
	return fmt.Errorf("failed to find job %q", uid)
}
