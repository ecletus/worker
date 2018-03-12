package worker

import (
	"errors"
	"net/http"

	"github.com/qor/admin"
	"github.com/qor/responder"
	"github.com/qor/roles"
)

type workerController struct {
	*Worker
}

func (wc workerController) Index(context *admin.Context) {
	context = context.NewResourceContext(wc.JobResource)
	context.SetDB(wc.Worker.ToDB(context.DB))
	result, err := context.FindMany()
	context.AddError(err)

	if context.HasError() {
		http.NotFound(context.Writer, context.Request)
	} else {
		responder.With("html", func() {
			context.Execute("index", result)
		}).With("json", func() {
			context.JSON("index", result)
		}).Respond(context.Request)
	}
}

func (wc workerController) Show(context *admin.Context) {
	job, err := wc.GetJob(context.Site, context.ResourceID)
	context.AddError(err)
	context.Execute("show", job)
}

func (wc workerController) New(context *admin.Context) {
	context.Execute("new", wc.Worker)
}

func (wc workerController) Update(context *admin.Context) {
	context.SetDB(wc.Worker.ToDB(context.DB))
	if job, err := wc.GetJob(context.Site, context.ResourceID); err == nil {
		if job.GetStatus() == JobStatusScheduled || job.GetStatus() == JobStatusNew {
			if job.GetJob().HasPermission(roles.Update, context.Context) {
				if context.AddError(wc.Worker.JobResource.Decode(context.Context, job)); !context.HasError() {
					context.AddError(wc.Worker.JobResource.CallSave(job, context.Context))
					context.AddError(wc.Worker.AddJob(job))
				}

				if !context.HasError() {
					context.Flash(string(context.Admin.T(context.Context, "qor_worker.form.successfully_updated", "{{.Name}} was successfully updated", wc.Worker.JobResource)), "success")
				}

				context.Execute("edit", job)
				return
			}
		}

		context.AddError(errors.New("not allowed to update this job"))
	} else {
		context.AddError(err)
	}

	http.Redirect(context.Writer, context.Request, context.Request.URL.Path, http.StatusFound)
}

func (wc workerController) AddJob(context *admin.Context) {
	context.SetDB(wc.Worker.ToDB(context.DB))
	jobResource := wc.Worker.JobResource
	result := jobResource.NewStruct(context.Site).(QorJobInterface)
	job := wc.Worker.GetRegisteredJob(context.Request.Form.Get("QorResource.Kind"))
	result.SetJob(job)

	if !job.HasPermission(roles.Create, context.Context) {
		context.AddError(errors.New("don't have permission to run job"))
	}

	if context.AddError(jobResource.Decode(context.Context, result)); !context.HasError() {
		// ensure job name is correct
		result.SetJob(job)
		context.AddError(jobResource.CallSave(result, context.Context))
		context.AddError(wc.Worker.AddJob(result))
	}

	if context.HasError() {
		responder.With("html", func() {
			context.Writer.WriteHeader(422)
			context.Execute("edit", result)
		}).With("json", func() {
			context.Writer.WriteHeader(422)
			context.JSON("index", map[string]interface{}{"errors": context.GetErrors()})
		}).Respond(context.Request)
		return
	}

	context.Flash(string(context.Admin.T(context.Context, "qor_worker.form.successfully_created", "{{.Name}} was successfully created", jobResource)), "success")
	http.Redirect(context.Writer, context.Request, context.Request.URL.Path, http.StatusFound)
}

func (wc workerController) RunJob(context *admin.Context) {
	context.SetDB(wc.Worker.ToDB(context.DB))
	if newJob := wc.Worker.saveAnotherJob(context.Site, context.ResourceID); newJob != nil {
		wc.Worker.AddJob(newJob)
	} else {
		context.AddError(errors.New("failed to clone job " + context.ResourceID))
	}

	http.Redirect(context.Writer, context.Request, context.URLFor(wc.Worker.JobResource), http.StatusFound)
}

func (wc workerController) KillJob(context *admin.Context) {
	context.SetDB(wc.Worker.ToDB(context.DB))
	if qorJob, err := wc.Worker.GetJob(context.Site, context.ResourceID); err == nil {
		if context.AddError(wc.Worker.KillJob(context.Site, qorJob.GetJobID())); !context.HasError() {
			context.Flash(string(context.Admin.T(context.Context, "qor_worker.form.successfully_killed", "{{.Name}} was successfully killed", wc.JobResource)), "success")
		} else {
			context.Flash(string(context.Admin.T(context.Context, "qor_worker.form.failed_to_kill", "Failed to kill job {{.Name}}", wc.JobResource)), "error")
		}
	}

	http.Redirect(context.Writer, context.Request, context.Request.URL.Path, http.StatusFound)
}
