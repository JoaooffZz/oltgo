package oltgo

type Service struct {
	ID          *string   `json:"id,omitempty"`
	Name        string    `json:"name"`
	Description *string   `json:"description,omitempty"`
	Version     string    `json:"version"`
	Tags        *[]string `json:"tags,omitempty"`
}

