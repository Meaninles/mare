package platform

type ModuleStatus struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Ready       bool   `json:"ready"`
}

func DefaultModules() []ModuleStatus {
	return []ModuleStatus{
		{Name: "asset", Description: "Asset catalog and metadata aggregation", Ready: true},
		{Name: "storage", Description: "Storage endpoint registry and connection management", Ready: true},
		{Name: "sync", Description: "Replica reconciliation and transfer orchestration", Ready: true},
		{Name: "import", Description: "Removable media ingestion pipeline", Ready: true},
		{Name: "search", Description: "Keyword and semantic query service", Ready: true},
		{Name: "task", Description: "Background task queue facade", Ready: true},
		{Name: "media-worker", Description: "Media processing and FFmpeg job dispatch facade", Ready: true},
	}
}
