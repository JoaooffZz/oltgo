package oltgo

import (
	"sync"
)

type Shipping struct {
	mu         sync.RWMutex
	queue      chan LogSchema
	processLog func(LogSchema) error
	wg         sync.WaitGroup
	closed     bool
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
		return
	}
	select {
	case s.queue <- log:
	default:
		// Buffer cheio. Descartar silenciosamente para evitar overhead / bloquear a aplicação principal
	}
}

func (s *Shipping) worker() {
	defer s.wg.Done()
	for log := range s.queue {
		s.handleLog(log)
	}
}

func (s *Shipping) handleLog(log LogSchema) {
	if s.processLog != nil {
		_ = s.processLog(log)
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
