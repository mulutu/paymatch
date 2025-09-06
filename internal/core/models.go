package core

type PaymentStatus string

const (
	PaymentPending PaymentStatus = "pending"
	PaymentMatched PaymentStatus = "matched"
	PaymentFailed  PaymentStatus = "failed"
)
