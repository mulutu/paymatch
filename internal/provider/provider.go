package provider

type STKPushReq struct {
	Amount      int
	Phone       string
	AccountRef  string
	Description string
}

type STKPushResp struct {
	CheckoutRequestID string
	CustomerMessage   string
}

type EventType string

const (
	EventSTK EventType = "stk"
	EventC2B EventType = "c2b"
)

type Event struct {
	Type       EventType
	ExternalID string
	Amount     int
	MSISDN     string
	InvoiceRef string
	RawJSON    []byte
}
