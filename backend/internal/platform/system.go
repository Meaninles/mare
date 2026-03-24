package platform

import (
	"time"

	"mam/backend/internal/buildinfo"
	"mam/backend/internal/config"
)

type SystemState struct {
	config       config.Config
	buildInfo    buildinfo.Info
	modules      []ModuleStatus
	dependencies DependencySnapshot
	database     DatabaseState
	startedAt    time.Time
}

type DatabaseState struct {
	Driver           string `json:"driver"`
	Path             string `json:"path"`
	Ready            bool   `json:"ready"`
	MigrationVersion string `json:"migrationVersion"`
}

type Bootstrap struct {
	App struct {
		Name      string         `json:"name"`
		Env       string         `json:"env"`
		Address   string         `json:"address"`
		BuildInfo buildinfo.Info `json:"buildInfo"`
		StartedAt time.Time      `json:"startedAt"`
	} `json:"app"`
	Modules      []ModuleStatus     `json:"modules"`
	Dependencies DependencySnapshot `json:"dependencies"`
	Database     DatabaseState      `json:"database"`
}

func NewSystemState(cfg config.Config, build buildinfo.Info, modules []ModuleStatus, dependencies DependencySnapshot, database DatabaseState) SystemState {
	return SystemState{
		config:       cfg,
		buildInfo:    build,
		modules:      modules,
		dependencies: dependencies,
		database:     database,
		startedAt:    time.Now().UTC(),
	}
}

func (state SystemState) Bootstrap() Bootstrap {
	return state.BootstrapWithDatabase(state.database)
}

func (state SystemState) BootstrapWithDatabase(database DatabaseState) Bootstrap {
	var payload Bootstrap
	payload.App.Name = state.config.AppName
	payload.App.Env = state.config.AppEnv
	payload.App.Address = state.config.HTTPAddress()
	payload.App.BuildInfo = state.buildInfo
	payload.App.StartedAt = state.startedAt
	payload.Modules = state.modules
	payload.Dependencies = state.dependencies
	payload.Database = database
	return payload
}
