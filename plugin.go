package worker

import (
	"github.com/aghape/cli"
	"github.com/aghape/db"
	"github.com/aghape/plug"
)

type Plugin struct {
	db.DBNames
	plug.EventDispatcher
	WorkerKey string
}

func (p *Plugin) RequireOptions() []string {
	return []string{p.WorkerKey}
}

func (p *Plugin) OnRegister() {
	db.Events(p).DBOnMigrate(func(e *db.DBEvent) error {
		worker := e.Options().GetInterface(p.WorkerKey).(*Worker)
		return e.AutoMigrate(worker.Config.Job).Error
	})
	p.On(cli.E_REGISTER, func(e plug.PluginEventInterface) {
		worker := e.Options().GetInterface(p.WorkerKey).(*Worker)
		root := e.(*cli.RegisterEvent).RootCmd
		root.AddCommand(worker.CreateCommand())
	})
}
