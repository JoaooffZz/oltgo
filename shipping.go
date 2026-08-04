package oltgo

import (
	"sync"
	"sync/atomic"
)

type Shipping struct {
	mu         sync.RWMutex
	queue      chan LogSchema
	processLog func(LogSchema) error
	wg         sync.WaitGroup
	closed     bool
	onError    func(LogSchema, error)
	dropped    atomic.Int64
	shipped    atomic.Int64
	failed     atomic.Int64
}

// ShippingStats expõe contadores para métricas e diagnóstico.
type ShippingStats struct {
	Shipped       int64 // processados com sucesso
	Failed        int64 // ProcessLog devolveu erro
	Dropped       int64 // descartados por buffer cheio ou Shipping fechado
	QueueDepth    int   // ocupação instantânea
	QueueCapacity int
}

func NewShipping(bufferSize int, processLog func(LogSchema) error) *Shipping {
	if bufferSize <= 0 {
		bufferSize = 1000 // default size
	}
	s := &Shipping{
		queue:      make(chan LogSchema, bufferSize),
		processLog: processLog,
	}
	s.wg.Add(1)
	go s.worker()
	return s
}

func (s *Shipping) Ship(log LogSchema) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		s.dropped.Add(1)
		return
	}
	select {
	case s.queue <- log:
	default:
		// Buffer cheio: descarta para não bloquear a aplicação principal, mas contabiliza.
		s.dropped.Add(1)
	}
}

func (s *Shipping) worker() {
	defer s.wg.Done()
	for log := range s.queue {
		s.handleLog(log)
	}
}

func (s *Shipping) handleLog(log LogSchema) {
	if s.processLog == nil {
		return
	}
	if err := s.processLog(log); err != nil {
		s.failed.Add(1)
		if s.onError != nil {
			s.onError(log, err)
		}
		return
	}
	s.shipped.Add(1)
}

// Stats devolve um retrato dos contadores. Seguro para chamada concorrente.
func (s *Shipping) Stats() ShippingStats {
	return ShippingStats{
		Shipped:       s.shipped.Load(),
		Failed:        s.failed.Load(),
		Dropped:       s.dropped.Load(),
		QueueDepth:    len(s.queue),
		QueueCapacity: cap(s.queue),
	}
}

func (s *Shipping) Close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	close(s.queue)
	s.mu.Unlock()

	s.wg.Wait()
}
