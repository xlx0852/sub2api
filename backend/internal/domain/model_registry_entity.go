package domain

// ModelRegistryEntry 是 models 表的一行（后台模型管理页使用）。
type ModelRegistryEntry struct {
	ID               string
	Platform         string
	DisplayName      string
	Family           string
	OwnedBy          string
	ContextWindow    *int
	MaxContextWindow *int
	Media            map[string]bool
	IsReasoning      bool
	Enabled          bool
	Retired          bool
	IsDefaultChat    bool
	IsDefaultTest    bool
	UpstreamID       string
	Version          int64
	Notes            string
	// 读视图：已配价格行数（默认价 + 各组价）
	PriceRowCount int
}

// ModelAliasEntry 是 model_aliases 的一行。
type ModelAliasEntry struct {
	Alias    string
	Platform string
	ModelID  string
	Kind     string
}
