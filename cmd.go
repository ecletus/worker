package worker

import (
	"fmt"
	"os"

	"github.com/moisespsena/go-error-wrap"
	"github.com/ecletus/core"
	"github.com/spf13/cobra"
)

type CLIArgs struct {
	Name     string
	RootArgs []string
}

func (a *CLIArgs) GetName() string {
	if a.Name == "" {
		return "worker"
	}
	return a.Name
}

func (a *CLIArgs) Worker(args ...string) []string {
	return append(append(a.RootArgs, a.GetName()), args...)
}

func (a *CLIArgs) Job(job QorJobInterface, args ...string) []string {
	args = append(args, job.UID())
	return a.Worker(append([]string{"job"}, args...)...)
}

func (a *CLIArgs) ExecJob(job QorJobInterface) []string {
	return a.Job(job, "exec")
}

func (worker *Worker) CreateCommand() *cobra.Command {
	see := "From " + PREFIX
	cmd := &cobra.Command{
		Use:   worker.CLIArgs.GetName(),
		Short: "Manager Job Workers",
		Long:  see,
		Args:  cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			siteName, _ := cmd.Flags().GetString("site")
			err := worker.Sites.EachOrAll(siteName, func(site core.SiteInterface) (error) {
				// TODO: List Running, By State ...
				return nil
			})
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			os.Exit(0)
		},
	}

	jobCmd := &cobra.Command{
		Use:   "job JOB_UID...",
		Short: "Show Job",
		Long:  see,
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			do := func(prefix, uid string) error {
				site, qorJobID, err := worker.ParseJobUID(uid)
				if err != nil {
					return errwrap.Wrap(err, "Parse job UID %q failed", prefix, uid)
				}
				job, err := worker.GetJob(site, qorJobID)

				if err != nil {
					return errwrap.Wrap(err, "Get job %q failed", prefix, uid)
				}

				_, err = fmt.Fprintln(os.Stdout, prefix, job)
				return err
			}

			if len(args) == 1 {
				return do("", args[0])
			}

			for i, uid := range args {
				err = do(fmt.Sprintf("[%d]: ", i+1), uid)
				if err != nil {
					return
				}
			}
			return nil
		},
	}

	jobCmd.Flags().String("job", "", "Qor Job ID")

	jobExecCmd := &cobra.Command{
		Use:   "exec JOB_UID",
		Short: "Execute job",
		Long:  see,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			uid := args[0]
			site, qorJobID, err := worker.ParseJobUID(uid)
			if err != nil {
				return errwrap.Wrap(err, "Parse job UID %q failed", uid)
			}

			runAnother, _ := cmd.Flags().GetBool("run-another")

			if runAnother {
				if newJob := worker.saveAnotherJob(site, qorJobID); newJob != nil {
					newJobID := newJob.GetJobID()
					qorJobID = newJobID
				} else {
					return errwrap.Wrap(err, "failed to clone job", uid)
				}
			}

			return errwrap.Wrap(worker.RunJob(site, qorJobID), "failed to run job %[2]q", uid)
		},
	}
	jobExecCmd.Flags().BoolP("run-another", "A", false, "Run another job")
	jobCmd.AddCommand(jobExecCmd)

	jobKillCmd := &cobra.Command{
		Use:   "kill JOB_UID",
		Short: "Kill job",
		Long:  see,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			uid := args[0]
			site, qorJobID, err := worker.ParseJobUID(uid)
			if err != nil {
				return errwrap.Wrap(err, "Parse job UID %q failed", uid)
			}

			return errwrap.Wrap(worker.KillJob(site, qorJobID), "failed to kill job %[2]q", uid)
		},
	}
	jobCmd.AddCommand(jobKillCmd)

	cmd.AddCommand(jobCmd)

	return cmd
}

func ExitError(err error) {
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func ExitErrorFmt(tempate string, err error, args ...interface{}) {
	if err != nil {
		ExitError(fmt.Errorf(tempate, append([]interface{}{err}, args...)...))
	}
}
