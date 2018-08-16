package worker

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"runtime/debug"
	"strings"
	"time"

	"os"

	errors2 "github.com/go-errors/errors"
	"github.com/moisespsena-go/aorm"
	"github.com/moisespsena/go-route"
	"github.com/aghape/admin"
	"github.com/aghape/aghape"
	"github.com/aghape/aghape/resource"
	"github.com/aghape/aghape/utils"
	"github.com/aghape/roles"
)

const (
	// JobStatusScheduled job status scheduled
	JobStatusScheduled = "scheduled"
	// JobStatusCancelled job status cancelled
	JobStatusCancelled = "cancelled"
	// JobStatusNew job status new
	JobStatusNew = "new"
	// JobStatusRunning job status running
	JobStatusRunning = "running"
	// JobStatusDone job status done
	JobStatusDone = "done"
	// JobStatusException job status exception
	JobStatusException = "exception"
	// JobStatusKilled job status killed
	JobStatusKilled = "killed"
)

// New create Worker with Config
func New(config ...*Config) *Worker {
	var cfg = &Config{}
	if len(config) > 0 {
		cfg = config[0]
	}

	if cfg.Job == nil {
		cfg.Job = &QorJob{}
	}

	if cfg.CLIArgs == nil {
		cfg.CLIArgs = &CLIArgs{"", []string{os.Args[0]}}
	}

	if cfg.Queue == nil {
		cfg.Queue = NewCronQueue(*cfg.CLIArgs)
	}

	return &Worker{Config: cfg, Jobs: make(map[string]*Job)}
}

// Config worker config
type Config struct {
	CLIArgs   *CLIArgs
	Sites     qor.SitesReaderInterface
	AdminSite string
	Queue     Queue
	Job       QorJobInterface
	Admin     *admin.Admin
}

// Worker worker definition
type Worker struct {
	*Config
	JobResource *admin.Resource
	Jobs        map[string]*Job
	mounted     bool
}

// ConfigureQorResourceBeforeInitialize a method used to config Worker for qor admin
func (worker *Worker) ConfigureQorResourceBeforeInitialize(res resource.Resourcer) {
	if res, ok := res.(*admin.Resource); ok {
		res.UseTheme("worker")

		worker.Admin = res.GetAdmin()
		worker.JobResource = worker.Admin.AddResource(worker.Config.Job, &admin.Config{Invisible: true, NotMount: true,
			Param: "workers"})
		worker.JobResource.Router.Intersept(&route.Middleware{
			Name: PREFIX + ".set_worker_to_db",
			Handler: func(chain *route.ChainHandler) {
				context := admin.ContextFromChain(chain)
				context.SetDB(worker.ToDB(context.GetDB()))
				context.PushI18nGroup(I18NGROUP)
				defer func() {
					context.PopI18nGroup()
				}()
				chain.Next()
			},
		})
		worker.JobResource.GetMeta("Kind").Type = "hidden"
		worker.JobResource.UseTheme("worker")
		worker.JobResource.Meta(&admin.Meta{Name: "Name", Valuer: func(record interface{}, context *qor.Context) interface{} {
			job := record.(QorJobInterface)
			if job.GetName() != job.GetJobName() {
				return job.GetName() + " [" + job.GetJobName() + "]"
			}
			return job.GetJobName()
		}})
		worker.JobResource.Meta(&admin.Meta{Name: "StatusAndTime", Valuer: func(record interface{}, context *qor.Context) interface{} {
			job := record.(QorJobInterface)
			if job.GetStatusUpdatedAt() != nil {
				return fmt.Sprintf("%v (at %v)", job.GetStatus(), job.GetStatusUpdatedAt().Format(time.RFC822Z))
			}
			return job.GetStatus()
		}})

		worker.JobResource.BeforeSave(&resource.Callback{
			PREFIX + ".set_site_name",
			func(resourcer resource.Resourcer, value interface{}, context *qor.Context, parent *resource.Parent) error {
				value.(*QorJob).SiteName = context.Site.Name()
				return nil
			},
		})

		worker.JobResource.Meta(&admin.Meta{Name: "SiteName", Enabled: func(recorde interface{}, context *admin.Context, meta *admin.Meta) bool {
			return false
		}})

		worker.JobResource.NewAttrs("Name", worker.JobResource.NewAttrs(), "-SiteName")
		worker.JobResource.EditAttrs("Name", worker.JobResource.EditAttrs(), "-SiteName")
		worker.JobResource.IndexAttrs("ID", "Name", "StatusAndTime", "CreatedAt")
		worker.JobResource.Name = res.Name

		for _, status := range []string{JobStatusScheduled, JobStatusNew, JobStatusRunning, JobStatusDone, JobStatusKilled, JobStatusException} {
			var status = status
			worker.JobResource.Scope(&admin.Scope{Name: status, Handler: func(db *aorm.DB, s *admin.Searcher, ctx *qor.Context) *aorm.DB {
				return db.Where("status = ?", status)
			}})
		}

		// default scope
		worker.JobResource.Scope(&admin.Scope{
			Handler: func(db *aorm.DB, s *admin.Searcher, ctx *qor.Context) *aorm.DB {
				db = db.Where("site_name = ?", ctx.Site.Name())

				if jobName := ctx.Request.URL.Query().Get("job"); jobName != "" {
					return db.Where("kind = ?", jobName)
				}

				if groupName := ctx.Request.URL.Query().Get("group"); groupName != "" {
					var jobNames []string
					for _, job := range worker.Jobs {
						if groupName == job.Group {
							jobNames = append(jobNames, job.Key)
						}
					}
					if len(jobNames) > 0 {
						return db.Where("kind IN (?)", jobNames)
					}
					return db.Where("kind IS NULL")
				}

				return db
			},
			Default: true,
		})

		// Configure jobs
		for _, job := range worker.Jobs {
			if job.Resource == nil {
				job.Resource = worker.Admin.NewResource(worker.JobResource.Value)
			}
		}
	}
}

// ConfigureQorResource a method used to config Worker for qor admin
func (worker *Worker) ConfigureQorResource(res resource.Resourcer) {
	if res, ok := res.(*admin.Resource); ok {
		worker.mounted = true

		// register view funcmaps
		worker.Admin.RegisterFuncMap("get_grouped_jobs", func(context *admin.Context) map[string][]*Job {
			var groupedJobs = map[string][]*Job{}
			var groupName = context.Request.URL.Query().Get("group")
			var jobName = context.Request.URL.Query().Get("job")
			for _, job := range worker.Jobs {
				if !(job.HasPermission(roles.Read, context.Context) && job.HasPermission(roles.Create, context.Context)) {
					continue
				}

				if (groupName == "" || groupName == job.Group) && (jobName == "" || jobName == job.Name) {
					groupedJobs[job.Group] = append(groupedJobs[job.Group], job)
				}
			}
			return groupedJobs
		})

		// configure routes
		controller := workerController{Worker: worker}

		handler := func(h admin.Handler) *admin.RouteHandler {
			rh := admin.NewHandler(h, &admin.RouteConfig{Resource: worker.JobResource})
			rh.Intercept(func(chain *admin.Chain) {
				chain.Context.PushI18nGroup(I18NGROUP)
				defer func() {
					chain.Context.PopI18nGroup()
				}()
				chain.Context.SetNewResourceForPath(res)
				chain.Context.SetDB(worker.ToDB(chain.Context.GetDB()))
				chain.Next()
			})
			return rh
		}

		res.Router.Get("/", handler(controller.Index))
		res.Router.Get("/new", handler(controller.New))
		res.Router.Post("/", handler(controller.AddJob))

		res.ObjectRouter.Get("/", handler(controller.Show))
		res.ObjectRouter.Get("/edit", handler(controller.Show))
		res.ObjectRouter.Post("/run", handler(controller.RunJob))
		res.ObjectRouter.Put("/", handler(controller.Update))
		res.ObjectRouter.Delete("/", handler(controller.DeleteJob))
		res.ObjectRouter.Post("/kill", handler(controller.KillJob))
	}
}

// SetQueue set worker's queue
func (worker *Worker) SetQueue(queue Queue) {
	worker.Queue = queue
}

// RegisterJob register a job into Worker
func (worker *Worker) RegisterJob(job *Job) error {
	if worker.mounted {
		debug.PrintStack()
		fmt.Printf("Job should be registered before Worker mounted into admin, but %v is registered after that", job.Name)
	}

	job.Worker = worker
	if job.Key == "" {
		if job.Resource == nil {
			job.Key = fmt.Sprintf("%x", sha256.Sum256([]byte(job.Name)))
		} else {
			job.Key = fmt.Sprintf("%x", sha256.Sum256([]byte(utils.TypeId(job.Resource.Value))))
		}
	}
	worker.Jobs[job.Key] = job
	return nil
}

// GetRegisteredJob register a job into Worker
func (worker *Worker) GetRegisteredJob(name string) *Job {
	return worker.Jobs[name]
}

// GetJob get job with id
func (worker *Worker) GetJob(site qor.SiteInterface, jobID string) (QorJobInterface, error) {
	qorJob := worker.JobResource.NewStruct(site).(QorJobInterface)

	context := worker.Admin.NewContext(site)
	context.SetDB(worker.ToDB(context.GetDB()))
	context.ResourceID = jobID
	context.Resource = worker.JobResource

	if err := worker.JobResource.CallFindOneHandler(worker.JobResource, qorJob, nil, context.Context); err == nil {
		if qorJob.GetJob() != nil {
			return qorJob, nil
		}
		return nil, fmt.Errorf("failed to load job: %v, unregistered job type: %v", JobUID(site, jobID), qorJob.GetJobName())
	}
	return nil, fmt.Errorf("failed to find job: %v", JobUID(site, jobID))
}

// AddJob add job to worker
func (worker *Worker) AddJob(qorJob QorJobInterface) error {
	return worker.Queue.Add(qorJob)
}

// RunJob run job with job id
func (worker *Worker) RunJob(site qor.SiteInterface, jobID string) error {
	qorJob, err := worker.GetJob(site, jobID)

	if err == nil {
		defer func() {
			if r := recover(); r != nil {
				qorJob.AddLog(string(errors2.Wrap(err, 2).ErrorStack()))
				qorJob.SetProgressText(fmt.Sprint(r))
				qorJob.SetStatus(JobStatusException)
			}
		}()

		if qorJob.GetStatus() != JobStatusNew && qorJob.GetStatus() != JobStatusScheduled {
			return errors.New("invalid job status, current status: " + qorJob.GetStatus())
		}

		if err = qorJob.SetStatus(JobStatusRunning); err == nil {
			if err = qorJob.GetJob().GetQueue().Run(qorJob); err == nil {
				return qorJob.SetStatus(JobStatusDone)
			}

			qorJob.SetProgressText(err.Error())
			qorJob.SetStatus(JobStatusException)
		}
	}

	return err
}

func (worker *Worker) saveAnotherJob(site qor.SiteInterface, jobID string) QorJobInterface {
	job, err := worker.GetJob(site, jobID)
	if err == nil {
		jobResource := worker.JobResource
		newJob := jobResource.NewStruct(site).(QorJobInterface)
		newJob.SetJob(job.GetJob())
		newJob.SetSerializableArgumentValue(job.GetArgument())
		context := site.PrepareContext(&qor.Context{})
		context.SetDB(worker.ToDB(context.DB))
		if err := jobResource.Save(newJob, context); err == nil {
			return newJob
		}
	}
	return nil
}

// KillJob kill job with job id
func (worker *Worker) KillJob(site qor.SiteInterface, jobID string) error {
	if qorJob, err := worker.GetJob(site, jobID); err == nil {
		if qorJob.GetStatus() == JobStatusRunning {
			if err = qorJob.GetJob().GetQueue().Kill(qorJob); err == nil {
				qorJob.SetStatus(JobStatusKilled)
				return nil
			}
			return err
		} else if qorJob.GetStatus() == JobStatusScheduled || qorJob.GetStatus() == JobStatusNew {
			qorJob.SetStatus(JobStatusKilled)
			return worker.RemoveJob(site, jobID)
		} else {
			return errors.New("invalid job status")
		}
	} else {
		return err
	}
}

// RemoveJob remove job with job id
func (worker *Worker) RemoveJob(site qor.SiteInterface, jobID string) error {
	qorJob, err := worker.GetJob(site, jobID)
	if err == nil {
		return qorJob.GetJob().GetQueue().Remove(qorJob)
	}
	return err
}

func (worker *Worker) ParseJobUID(uid string) (site qor.SiteInterface, jobID string, err error) {
	parts := strings.Split(uid, "@")
	if len(parts) != 2 {
		return nil, "", fmt.Errorf("Invalid uid %q.", uid)
	}
	siteName, jobID := parts[0], parts[1]
	site, err = worker.Sites.GetOrError(siteName)
	return
}

func (worker *Worker) ToDB(db *aorm.DB) *aorm.DB {
	return db.Set("qor:worker.worker", worker)
}

func WorkerFromDB(db *aorm.DB) *Worker {
	worker, ok := db.Get("qor:worker.worker")
	if ok {
		return worker.(*Worker)
	}
	return nil
}

func JobUID(site interface{}, job interface{}) string {
	var siteName, jobId string
	switch st := site.(type) {
	case string:
		siteName = st
	case qor.SiteInterface:
		siteName = st.Name()
	}
	switch jb := job.(type) {
	case string:
		jobId = jb
	case QorJobInterface:
		jobId = jb.GetJobID()
	}
	return siteName + "@" + jobId
}
