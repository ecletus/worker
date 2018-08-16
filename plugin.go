package worker

import (
	"github.com/aghape/plug"
	"github.com/aghape/db"
	"github.com/aghape/cli"
)

type Plugin struct {
	db.DisDBNames
	WorkerKey string
}

func (p *Plugin) RequireOptions() []string {
	return []string{p.WorkerKey}
}

func (p *Plugin) OnRegister() {
	p.DBOnMigrateGorm(func(e *db.GormDBEvent) error {
		worker := e.Options().GetInterface(p.WorkerKey).(*Worker)
		return e.DB.AutoMigrate(worker.Config.Job).Error
	})
	p.On(cli.E_REGISTER, func(e plug.PluginEventInterface) {
		worker := e.Options().GetInterface(p.WorkerKey).(*Worker)
		root := e.(*cli.RegisterEvent).RootCmd
		root.AddCommand(worker.CreateCommand())
	})
}