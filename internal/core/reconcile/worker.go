package reconcile

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"paymatch/internal/store/postgres"

	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog/log"
)

type Worker struct {
	repo      *postgres.Repo
	pollEvery time.Duration
	batch     int
}

func NewWorker(repo *postgres.Repo) *Worker {
	return &Worker{repo: repo, pollEvery: 2 * time.Second, batch: 50}
}

func (w *Worker) Run(ctx context.Context) {
	log.Info().Msg("reconcile worker: started")
	t := time.NewTicker(w.pollEvery)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("reconcile worker: stopping")
			return
		case <-t.C:
			w.tick(ctx)
		}
	}
}

// Exact JSON shape we get from Safaricom STK callback
type stkPayload struct {
	Body struct {
		StkCallback struct {
			MerchantRequestID string `json:"MerchantRequestID"`
			CheckoutRequestID string `json:"CheckoutRequestID"`
			ResultCode        int    `json:"ResultCode"`
			ResultDesc        string `json:"ResultDesc"`
			CallbackMetadata  struct {
				Item []struct {
					Name  string `json:"Name"`
					Value any    `json:"Value"`
				} `json:"Item"`
			} `json:"CallbackMetadata"`
		} `json:"stkCallback"`
	} `json:"Body"`
}

func (w *Worker) tick(ctx context.Context) {
	evts, err := w.repo.FetchUnprocessedEvents(ctx, w.batch)
	if err != nil {
		log.Error().Err(err).Msg("worker: fetch events failed")
		return
	}
	if len(evts) == 0 {
		return
	}
	for _, e := range evts {
		if err := w.handleOne(ctx, e); err != nil {
			log.Error().Err(err).Int64("event_id", e.ID).Msg("worker: processing failed")
			// Leave processed_at=NULL so it can be retried manually/systemically.
		}
	}
}

func (w *Worker) handleOne(ctx context.Context, e postgres.EventRow) error {
	switch e.EventType {
	case "stk":
		var sp stkPayload
		if err := json.Unmarshal(e.PayloadJSON, &sp); err != nil {
			return w.finalize(ctx, e, "", "", 0, "invalid", fmt.Errorf("bad stk json: %w", err))
		}
		isSuccess := sp.Body.StkCallback.ResultCode == 0

		var amount int
		var msisdn, ref string
		for _, it := range sp.Body.StkCallback.CallbackMetadata.Item {
			switch it.Name {
			case "Amount":
				switch v := it.Value.(type) {
				case float64:
					amount = int(v)
				case json.Number:
					if f, err := v.Float64(); err == nil {
						amount = int(f)
					}
				case string:
					// some sandboxes serialize numbers as strings
					var f float64
					if err := json.Unmarshal([]byte(`"`+v+`"`), &f); err == nil {
						amount = int(f)
					}
				case int:
					amount = v
				}
			case "PhoneNumber":
				switch v := it.Value.(type) {
				case float64:
					msisdn = fmt.Sprintf("%.0f", v)
				case json.Number:
					msisdn = v.String()
				case string:
					msisdn = v
				}
			case "AccountReference":
				if s, ok := it.Value.(string); ok {
					ref = s
				}
			}
		}
		status := "failed"
		if isSuccess {
			status = "matched"
		}
		return w.finalize(ctx, e, ref, msisdn, amount, status, nil)

	case "c2b":
		var c2b map[string]any
		if err := json.Unmarshal(e.PayloadJSON, &c2b); err != nil {
			return w.finalize(ctx, e, "", "", 0, "invalid", fmt.Errorf("bad c2b json: %w", err))
		}
		ref, _ := c2b["BillRefNumber"].(string)
		msisdn, _ := c2b["MSISDN"].(string)
		amount := 0
		// TransAmount in C2B is often a string; handle both
		switch v := c2b["TransAmount"].(type) {
		case float64:
			amount = int(v)
		case string:
			var f float64
			if err := json.Unmarshal([]byte(`"`+v+`"`), &f); err == nil {
				amount = int(f)
			}
		}
		return w.finalize(ctx, e, ref, msisdn, amount, "matched", nil)

	default:
		// Unknown/unsupported types: mark processed to avoid poisoning the queue
		return w.finalize(ctx, e, "", "", 0, "ignored", nil)
	}
}

// finalize = ATOMIC (UpsertPayment + MarkEventProcessed) in a single tx
func (w *Worker) finalize(ctx context.Context, e postgres.EventRow, ref, msisdn string, amount int, status string, cause error) error {
	tx, err := w.repo.DB().BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Only write a payment row for meaningful statuses
	if status != "ignored" && status != "invalid" {
		if err := w.repo.UpsertPaymentTx(ctx, tx,
			e.TenantID, e.ProviderCredentialID,
			ref, msisdn, amount, "KES", "mpesa", e.ExternalID, status,
		); err != nil {
			return err
		}
	}

	if err := w.repo.MarkEventProcessedTx(ctx, tx, e.ID, status); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
