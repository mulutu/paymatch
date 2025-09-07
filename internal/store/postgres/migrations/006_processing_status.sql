-- Migration: Add processing_status column to payment_events table
-- This column tracks the processing state of events in the event queue

DO $$
BEGIN
    -- Add processing_status column if it doesn't exist
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns 
                   WHERE table_name = 'payment_events' 
                   AND column_name = 'processing_status') THEN
        ALTER TABLE payment_events 
        ADD COLUMN processing_status TEXT NOT NULL DEFAULT 'pending';
        
        -- Create index for efficient querying of processing status
        CREATE INDEX idx_payment_events_processing_status 
        ON payment_events(processing_status, received_at) 
        WHERE processing_status IN ('pending', 'queued');
    END IF;
END $$;