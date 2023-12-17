package hystrix

// executorPool 令牌法流量控制，拿到令牌可以执行后续的工作，执行完归还令牌
type executorPool struct {
	Name    string         // 名称
	Metrics *poolMetrics   //
	Max     int            // 最大并发数
	Tickets chan *struct{} // 令牌桶，这里定义指针是为了方便判断 nil？
}

func newExecutorPool(name string) *executorPool {
	p := &executorPool{}
	p.Name = name
	p.Metrics = newPoolMetrics(name)
	p.Max = getSettings(name).MaxConcurrentRequests // 获取配置项最大并发数

	// 初始往令牌桶中放满令牌
	p.Tickets = make(chan *struct{}, p.Max)
	for i := 0; i < p.Max; i++ {
		p.Tickets <- &struct{}{}
	}

	return p
}

func (p *executorPool) Return(ticket *struct{}) {
	if ticket == nil {
		return
	}

	p.Metrics.Updates <- poolMetricsUpdate{
		activeCount: p.ActiveCount(),
	}
	// 归还令牌
	p.Tickets <- ticket
}

func (p *executorPool) ActiveCount() int {
	return p.Max - len(p.Tickets)
}
