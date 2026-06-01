package oltgo

type Environment string

const (
	Production  Environment = "production"
	Development Environment = "development"
	Testing     Environment = "testing"
)

type Agent struct {
	Service       Service
	Env           Environment
	SchemaVersion string
	Shipping      *Shipping
}

// NewAgent inicializa o Agent de observabilidade.
// O parâmetro processLog é uma função opcional de callback para processamento dos logs estruturados.
// Se processLog for nil, os logs serão silenciosamente descartados da memória.
func NewAgent(service Service, env Environment, schemaVersion string, processLog func(LogSchema) error) *Agent {
	shipping := NewShipping(1000, processLog)
	return &Agent{
		Service:       service,
		Env:           env,
		SchemaVersion: schemaVersion,
		Shipping:      shipping,
	}
}

// NewCollection cria uma nova instância de Collection associada a este Agent para rastrear uma nova trace/requisição.
func (a *Agent) NewCollection(tracingID string) *Collection {
	return NewCollection(a, tracingID)
}

// Close encerra as goroutines do Shipping de forma limpa, garantindo o processamento de logs remanescentes.
func (a *Agent) Close() {
	if a.Shipping != nil {
		a.Shipping.Close()
	}
}

