package worker

import (
	"github.com/ecletus/admin"
	"github.com/ecletus/core"
	"github.com/ecletus/roles"
)

// Job is a struct that hold Qor Job definations
type Job struct {
	Key        string
	Name       string
	Group      string
	Handler    func(interface{}, QorJobInterface) error
	Permission *roles.Permission
	Queue      Queue
	Resource   *admin.Resource
	Worker     *Worker
}

// NewStruct initialize job struct
func (job *Job) NewStruct(site *core.Site) interface{} {
	qorJobInterface := job.Worker.JobResource.NewStruct(site).(QorJobInterface)
	qorJobInterface.SetJob(job)
	return qorJobInterface
}

// GetQueue get defined job's queue
func (job *Job) GetQueue() Queue {
	if job.Queue != nil {
		return job.Queue
	}
	return job.Worker.Queue
}

func (job Job) HasPermission(mode roles.PermissionMode, context *core.Context) (perm roles.Perm) {
	if job.Permission == nil {
		return
	}
	return job.Permission.HasPermission(context, mode, context.Roles.Interfaces()...)
}
