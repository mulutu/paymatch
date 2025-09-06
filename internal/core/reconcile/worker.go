package reconcile

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"paymatch/internal/store/postgres"

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

// NOTE: this expects your queue item to include:
//   - QueueID
//   - EventID
//   - TenantID
//   - ProviderCredentialID
//   - Type (e.g., "stk", "c2b")
//   - ExtID
//   - RawJSON
//
// Make sure your postgres.DueEvent matches this (see queue.go).
func (w *Worker) tick(ctx context.Context) {
	items, err := w.repo.FetchDueEvents(ctx, w.batch)
	if err != nil {
		log.Error().Err(err).Msg("worker: fetch due events failed")
		return
	}
	if len(items) == 0 {
		return
	}

	for _, it := range items {
		// Default to success; flip to failed if any step errors.
		handleErr := w.handleOne(ctx, it)
		if handleErr != nil {
			log.Warn().
				Err(handleErr).
				Int64("queue_id", it.QueueID).
				Int64("event_id", it.EventID).
				Str("external_id", it.ExtID).
				Msg("worker: event processing failed")
			// repo increments attempts and sets exponential backoff
			_ = w.repo.MarkEventFailed(ctx, it.QueueID, it.EventID, 0, handleErr.Error())
			continue
		}
		if err := w.repo.MarkEventDone(ctx, it.QueueID, it.EventID); err != nil {
			log.Error().Err(err).Int64("queue_id", it.QueueID).Msg("worker: mark done failed")
		}
	}
}

type stkPayload struct {
	Body struct {
		StkCallback struct {
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

func (w *Worker) handleOne(ctx context.Context, it postgres.DueEvent) error {
	switch it.Type {
	case "stk":
		var sp stkPayload
		if err := json.Unmarshal(it.RawJSON, &sp); err != nil {
			return fmt.Errorf("bad stk json: %w", err)
		}
		isSuccess := sp.Body.StkCallback.ResultCode == 0

		log.Info().
			Int64("event_id", it.EventID).
			Str("external_id", it.ExtID).
			Bool("success", isSuccess).
			Msg("worker: processing stk event")

		var amount int
		var msisdn string
		var ref string
		for _, field := range sp.Body.StkCallback.CallbackMetadata.Item {
			switch field.Name {
			case "Amount":
				switch v := field.Value.(type) {
				case float64:
					amount = int(v)
				case int:
					amount = v
				}
			case "PhoneNumber":
				switch v := field.Value.(type) {
				case float64:
					msisdn = fmt.Sprintf("%.0f", v)
				case string:
					msisdn = v
				}
			case "AccountReference":
				if s, ok := field.Value.(string); ok {
					ref = s
				}
			}
		}

		st := "failed"
		if isSuccess {
			st = "matched"
			log.Info().
				Int64("event_id", it.EventID).
				Str("external_id", it.ExtID).
				Int("amount", amount).
				Str("msisdn", msisdn).
				Str("ref", ref).
				Msg("worker: stk success")
		}

		if err := w.repo.UpsertPayment(
			ctx,
			it.TenantID,
			it.ProviderCredentialID, // ensure DueEvent has this
			ref,
			msisdn,
			amount,
			"KES",
			"mpesa",
			it.ExtID,
			st,
		); err != nil {
			return fmt.Errorf("upsert payment failed (stk): %w", err)
		}
		return nil

	case "c2b":
		var c2b map[string]any
		if err := json.Unmarshal(it.RawJSON, &c2b); err != nil {
			return fmt.Errorf("bad c2b json: %w", err)
		}

		ref := ""
		if s, ok := c2b["BillRefNumber"].(string); ok {
			ref = s
		}
		msisdn := ""
		if s, ok := c2b["MSISDN"].(string); ok {
			msisdn = s
		}
		amount := 0
		if f, ok := c2b["TransAmount"].(float64); ok {
			amount = int(f)
		}

		if err := w.repo.UpsertPayment(
			ctx,
			it.TenantID,
			it.ProviderCredentialID, // ensure DueEvent has this
			ref,
			msisdn,
			amount,
			"KES",
			"mpesa",
			it.ExtID,
			"matched",
		); err != nil {
			return fmt.Errorf("upsert payment failed (c2b): %w", err)
		}
		return nil

	default:
		// Unknown/unsupported types shouldn't poison the queue; mark as done by returning nil.
		log.Info().
			Int64("event_id", it.EventID).
			Str("type", it.Type).
			Msg("worker: ignoring event type")
		return nil
	}
}
