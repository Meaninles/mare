package platform

import (
	"time"

	"mam/backend/internal/buildinfo"
	cd2client "mam/backend/internal/cd2/client"
	cd2runtime "mam/backend/internal/cd2/runtime"
	"mam/backend/internal/config"
)

type SystemState struct {
	config       config.Config
	buildInfo    buildinfo.Info
	modules      []ModuleStatus
	dependencies DependencySnapshot
	database     DatabaseState
	cd2Runtime   cd2runtime.State
	cd2Client    cd2client.State
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
	CD2Runtime   cd2runtime.State   `json:"cd2Runtime"`
	CD2Client    cd2client.State    `json:"cd2Client"`
}

func NewSystemState(cfg config.Config, build buildinfo.Info, modules []ModuleStatus, dependencies DependencySnapshot, database DatabaseState, cd2RuntimeState cd2runtime.State, cd2ClientState cd2client.State) SystemState {
	return SystemState{
		config:       cfg,
		buildInfo:    build,
		modules:      modules,
		dependencies: dependencies,
		database:     database,
		cd2Runtime:   cd2RuntimeState,
		cd2Client:    cd2ClientState,
		startedAt:    time.Now().UTC(),
	}
}

func (state SystemState) Bootstrap() Bootstrap {
	return state.BootstrapWithCD2(state.database, state.cd2Runtime, state.cd2Client)
}

func (state SystemState) BootstrapWithDatabase(database DatabaseState) Bootstrap {
	return state.BootstrapWithCD2(database, state.cd2Runtime, state.cd2Client)
}

func (state SystemState) BootstrapWithRuntime(database DatabaseState, cd2State cd2runtime.State) Bootstrap {
	return state.BootstrapWithCD2(database, cd2State, state.cd2Client)
}

func (state SystemState) BootstrapWithCD2(database DatabaseState, cd2RuntimeState cd2runtime.State, cd2ClientState cd2client.State) Bootstrap {
	var payload Bootstrap
	payload.App.Name = state.config.AppName
	payload.App.Env = state.config.AppEnv
	payload.App.Address = state.config.HTTPAddress()
	payload.App.BuildInfo = state.buildInfo
	payload.App.StartedAt = state.startedAt
	payload.Modules = cloneModules(state.modules)
	for index := range payload.Modules {
		switch payload.Modules[index].Name {
		case "cd2-runtime":
			payload.Modules[index].Ready = cd2RuntimeState.Ready
		case "cd2-client":
			payload.Modules[index].Ready = cd2ClientState.Ready
		}
	}
	payload.Dependencies = state.dependencies
	payload.Database = database
	payload.CD2Runtime = cd2RuntimeState
	payload.CD2Client = cd2ClientState
	return payload
}

func cloneModules(modules []ModuleStatus) []ModuleStatus {
	if len(modules) == 0 {
		return nil
	}

	cloned := make([]ModuleStatus, len(modules))
	copy(cloned, modules)
	return cloned
}
