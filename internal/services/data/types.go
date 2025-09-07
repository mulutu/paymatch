package data

// ListRequest represents a paginated list request
type ListRequest struct {
	Limit  int `json:"limit,omitempty"`
	Offset int `json:"offset,omitempty"`
}

// ListResponse represents a paginated list response
type ListResponse struct {
	Data   interface{} `json:"data"`
	Limit  int         `json:"limit"`
	Offset int         `json:"offset"`
	Total  int         `json:"total,omitempty"` // Optional: total count
}

// Validate validates and normalizes list request parameters
func (req *ListRequest) Validate() {
	// Set defaults
	if req.Limit <= 0 {
		req.Limit = 50
	}
	if req.Offset < 0 {
		req.Offset = 0
	}
	
	// Apply limits
	if req.Limit > 200 {
		req.Limit = 200
	}
}