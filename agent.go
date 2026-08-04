package oltgo

type Environment string

// Environment é uma string livre: as constantes abaixo são os valores
// convencionais, mas qualquer outro valor é aceito.
const (
	Production  Environment = "production"
	Staging     Environment = "staging"
	Development Environment = "development"
	Testing     Environment = "testing"
)

type Agent struct {
	Service       Service
	Env           Environment
	SchemaVersion string
	Shipping      *Shipping
}

// Options configura comportamentos opcionais do Agent.
type Options struct {
	// BufferSize é o tamanho da fila do Shipping. 0 usa o default (1000).
	BufferSize int
	// OnError é chamado quando ProcessLog devolve erro. Deve ser rápido e não
	// entrar em pânico: roda no worker do Shipping.
	OnError func(LogSchema, error)
}

// NewAgentWithOptions inicializa o Agent de observabilidade com opções extras.
// Ver NewAgent para o comportamento padrão.
func NewAgentWithOptions(service Service, env Environment, schemaVersion string, processLog func(LogSchema) error, opts Options) *Agent {
	shipping := NewShipping(opts.BufferSize, processLog)
	shipping.onError = opts.OnError
	return &Agent{
		Service:       service,
		Env:           env,
		SchemaVersion: schemaVersion,
		Shipping:      shipping,
	}
}

// NewAgent inicializa o Agent de observabilidade.
// O parâmetro processLog é uma função opcional de callback para processamento dos logs estruturados.
// Se processLog for nil, os logs serão silenciosamente descartados da memória.
func NewAgent(service Service, env Environment, schemaVersion string, processLog func(LogSchema) error) *Agent {
	return NewAgentWithOptions(service, env, schemaVersion, processLog, Options{})
}

// NewCollection cria uma nova instância de Collection associada a este Agent para rastrear uma nova trace/requisição.
func (a *Agent) NewCollection(tracingID string) *Collection {
	return NewCollection(a, tracingID)
}

// Stats é atalho para Agent.Shipping.Stats().
func (a *Agent) Stats() ShippingStats {
	if a.Shipping == nil {
		return ShippingStats{}
	}
	return a.Shipping.Stats()
}

// Close encerra as goroutines do Shipping de forma limpa, garantindo o processamento de logs remanescentes.
func (a *Agent) Close() {
	if a.Shipping != nil {
		a.Shipping.Close()
	}
}
