package pipeline

type funcStage struct {
	name StageName
	fn   func(*Exchange) error
}

func (s funcStage) Name() StageName { return s.name }

func (s funcStage) Run(ex *Exchange) error {
	if s.fn == nil {
		return nil
	}
	return s.fn(ex)
}

// ParseStage 把 Provider.Parse 接到管道 parse 阶段。
func ParseStage(p Provider) Stage {
	if p == nil {
		return nil
	}
	return funcStage{name: StageParse, fn: p.Parse}
}

// RewriteStage 把 Provider.Rewrite 接到管道 rewrite 阶段。
func RewriteStage(p Provider) Stage {
	if p == nil {
		return nil
	}
	return funcStage{name: StageRewrite, fn: p.Rewrite}
}

// Func 把任意函数接到指定生命周期阶段（供平台适配器挂 Transport/Relay/Settle）。
func Func(name StageName, fn func(*Exchange) error) Stage {
	if fn == nil {
		return nil
	}
	return funcStage{name: name, fn: fn}
}
