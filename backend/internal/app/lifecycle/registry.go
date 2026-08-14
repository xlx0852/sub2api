package lifecycle

import (
	"context"
	"log"
	"sync"
)

// Hook 是一个可停止的运行时组件。
type Hook struct {
	Name string
	Stop func() error
}

// Registry 按注册反序关停：先并行停应用层，再顺序停基础设施。
type Registry struct {
	mu         sync.Mutex
	parallel   []Hook
	sequential []Hook
}

func New() *Registry {
	return &Registry{}
}

func (r *Registry) AddParallel(name string, stop func() error) {
	if r == nil || stop == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.parallel = append(r.parallel, Hook{Name: name, Stop: stop})
}

func (r *Registry) AddSequential(name string, stop func() error) {
	if r == nil || stop == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sequential = append(r.sequential, Hook{Name: name, Stop: stop})
}

// Stop 先并行执行应用层 hook，再顺序执行基础设施 hook。
func (r *Registry) Stop(ctx context.Context) {
	if r == nil {
		return
	}
	r.mu.Lock()
	parallel := append([]Hook(nil), r.parallel...)
	sequential := append([]Hook(nil), r.sequential...)
	r.mu.Unlock()

	var wg sync.WaitGroup
	for i := range parallel {
		step := parallel[i]
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := step.Stop(); err != nil {
				log.Printf("[Cleanup] %s failed: %v", step.Name, err)
				return
			}
			log.Printf("[Cleanup] %s succeeded", step.Name)
		}()
	}
	wg.Wait()

	for i := range sequential {
		step := sequential[i]
		if err := step.Stop(); err != nil {
			log.Printf("[Cleanup] %s failed: %v", step.Name, err)
			continue
		}
		log.Printf("[Cleanup] %s succeeded", step.Name)
	}

	if ctx != nil {
		select {
		case <-ctx.Done():
			log.Printf("[Cleanup] Warning: cleanup timed out after 10 seconds")
		default:
			log.Printf("[Cleanup] All cleanup steps completed")
		}
	}
}

// StopFunc 返回可供 Wire cleanup 使用的无参关停函数。
func (r *Registry) StopFunc(timeoutCtx func() (context.Context, context.CancelFunc)) func() {
	return func() {
		ctx, cancel := timeoutCtx()
		defer cancel()
		r.Stop(ctx)
	}
}
