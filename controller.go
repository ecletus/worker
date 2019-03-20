package worker

import (
	"errors"
	"net/http"

	"github.com/ecletus/admin"
	"github.com/ecletus/core"
	"github.com/ecletus/core/utils/httputils"
	"github.com/ecletus/responder"
	"github.com/ecletus/roles"
)

type workerController struct {
	*Worker
}

func (wc workerController) Index(context *admin.Context) {
	context = context.NewResourceContext(wc.JobResource)
	result, err := context.FindMany()
	context.AddError(err)

	if context.HasError() {
		http.NotFound(context.Writer, context.Request)
	} else {
		responder.With("html", func() {
			context.Execute("index", result)
		}).With("json", func() {
			context.JSON(result, "index")
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
	if job, err := wc.GetJob(context.Site, context.ResourceID); err == nil {
		if job.GetStatus() == JobStatusScheduled || job.GetStatus() == JobStatusNew {
			if core.HasPermission(job.GetJob(), roles.Update, context.Context) {
				if context.AddError(wc.Worker.JobResource.Decode(context.Context, job)); !context.HasError() {
					context.AddError(wc.Worker.JobResource.Crud(context.Context).Update(job))
					context.AddError(wc.Worker.AddJob(job))
				}

				if !context.HasError() {
					context.Flash(string(context.Admin.TT(context.Context, I18NGROUP+".form.successfully_updated", wc.Worker.JobResource)), "success")
				}

				context.Execute("edit", job)
				return
			}
		}

		context.AddError(errors.New(string(context.Admin.TT(context.Context, I18NGROUP+".form.update_access_danied", context.ResourceID))))
	} else {
		context.AddError(err)
	}

	httputils.Redirect(context.Writer, context.Request, context.OriginalURL.Path, http.StatusFound)
}

func (wc workerController) AddJob(context *admin.Context) {
	jobResource := wc.Worker.JobResource
	result := jobResource.NewStruct(context.Site).(QorJobInterface)
	job := wc.Worker.GetRegisteredJob(context.Request.Form.Get("QorResource.Kind"))
	result.SetJob(job)

	if !core.HasPermission(job, roles.Create, context.Context) {
		context.AddError(errors.New(string(context.Admin.TT(context.Context, I18NGROUP+".form.run_access_danied", context.ResourceID))))
	}

	if context.AddError(jobResource.Decode(context.Context, result)); !context.HasError() {
		// ensure job name is correct
		result.SetJob(job)
		context.AddError(jobResource.Crud(context.Context).SaveOrCreate(result))
		context.AddError(wc.Worker.AddJob(result))
	}

	if context.HasError() {
		responder.With("html", func() {
			context.Writer.WriteHeader(422)
			context.Execute("edit", result)
		}).With("json", func() {
			context.Writer.WriteHeader(422)
			context.JSON(map[string]interface{}{"errors": context.GetErrors()}, "index")
		}).Respond(context.Request)
		return
	}

	context.Flash(string(context.Admin.TT(context.Context, I18NGROUP+".form.successfully_created", jobResource)), "success")
	httputils.Redirect(context.Writer, context.Request, context.OriginalURL.Path, http.StatusFound)
}

func (wc workerController) RunJob(context *admin.Context) {
	if newJob := wc.Worker.saveAnotherJob(context.Site, context.ResourceID); newJob != nil {
		wc.Worker.AddJob(newJob)
	} else {
		context.AddError(errors.New(string(context.Admin.TT(context.Context, I18NGROUP+".form.failed_to_clone", context.ResourceID))))
	}

	httputils.Redirect(context.Writer, context.Request, wc.Worker.JobResource.GetContextIndexURI(context.Context), http.StatusFound)
}

func (wc workerController) KillJob(context *admin.Context) {
	if qorJob, err := wc.Worker.GetJob(context.Site, context.ResourceID); err == nil {
		err = wc.Worker.KillJob(context.Site, qorJob.GetJobID())
		if err == nil {
			context.Flash(string(context.Admin.TT(context.Context, I18NGROUP+".form.successfully_killed", wc.JobResource)), "success")
		} else {
			context.AddError(err)
			context.Flash(string(context.Admin.TT(context.Context, I18NGROUP+".form.failed_to_kill", map[string]interface{}{"Job": qorJob, "Error": err.Error()})), "error")
		}
	}

	httputils.Redirect(context.Writer, context.Request, wc.Worker.JobResource.GetContextIndexURI(context.Context), http.StatusFound)
}

func (wc workerController) DeleteJob(context *admin.Context) {
	if qorJob, err := wc.Worker.GetJob(context.Site, context.ResourceID); err == nil {
		if qorJob.GetStatus() == JobStatusRunning {
			context.Flash(string(context.Admin.TT(context.Context, I18NGROUP+".form.job_is_running", wc.JobResource)), "error")
		} else {
			if err = wc.JobResource.Crud(context.Context).Delete(qorJob); err == nil {
				context.Flash(string(context.Admin.TT(context.Context, I18NGROUP+".form.successfully_deleted", wc.JobResource)), "success")
			} else {
				context.AddError(err)
				context.Flash(string(context.Admin.TT(context.Context, I18NGROUP+".form.failed_to_kill", map[string]interface{}{"Job": qorJob, "Error": err.Error()})), "error")
			}
		}

		httputils.Redirect(context.Writer, context.Request, wc.Worker.JobResource.GetContextIndexURI(context.Context), http.StatusFound)
		return
	}

	httputils.Redirect(context.Writer, context.Request, wc.Worker.JobResource.GetContextIndexURI(context.Context), http.StatusNotFound)
}
