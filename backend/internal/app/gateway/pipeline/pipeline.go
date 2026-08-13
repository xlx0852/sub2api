package pipeline

// Pipeline 按固定顺序执行生命周期阶段。
// 任一阶段返回错误或设置 Exchange.Err 即停止后续阶段。
type Pipeline struct {
	stages []Stage
}

func New(stages ...Stage) *Pipeline {
	out := make([]Stage, 0, len(stages))
	for _, stage := range stages {
		if stage != nil {
			out = append(out, stage)
		}
	}
	return &Pipeline{stages: out}
}

func (p *Pipeline) Stages() []Stage {
	if p == nil {
		return nil
	}
	return append([]Stage(nil), p.stages...)
}

func (p *Pipeline) Run(ex *Exchange) error {
	if p == nil {
		return nil
	}
	if ex == nil {
		return errNilExchange
	}
	for _, stage := range p.stages {
		if err := stage.Run(ex); err != nil {
			if ex.Err == nil {
				ex.Err = err
			}
			return err
		}
		if ex.Err != nil {
			return ex.Err
		}
	}
	return nil
}
