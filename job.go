package worker

import (
	"github.com/aghape/admin"
	"github.com/aghape/core"
	"github.com/aghape/roles"
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
func (job *Job) NewStruct(site core.SiteInterface) interface{} {
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

func (job Job) HasPermissionE(mode roles.PermissionMode, context *core.Context) (ok bool, err error) {
	if job.Permission == nil {
		return true, roles.ErrDefaultPermission
	}
	var roles_ = []interface{}{}
	for _, role := range context.Roles {
		roles_ = append(roles_, role)
	}
	return roles.HasPermissionDefaultE(true, job.Permission, mode, roles_...)
}
