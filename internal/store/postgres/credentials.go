package postgres

import (
	"context"

	"paymatch/internal/crypto"
)

type ProviderCredential struct {
	ID                int64
	TenantID          int64
	Provider          string
	Shortcode         string
	Environment       string
	WebhookToken      string
	IsActive          bool
	PasskeyEnc        string
	ConsumerKeyEnc    string
	ConsumerSecretEnc string

	// C2B behaviour per shortcode/tenant
	C2BMode         string // 'paybill' | 'buygoods'
	BillRefRequired bool
	BillRefRegex    string
}

func (r *Repo) ResolveCredential(ctx context.Context, tenantID int64, shortcode string) (ProviderCredential, error) {
	if shortcode == "" {
		row := r.db.QueryRow(ctx, `
			SELECT id, tenant_id, provider, shortcode, environment, webhook_token, is_active,
			       passkey_enc, consumer_key_enc, consumer_secret_enc,
			       COALESCE(c2b_mode,'paybill'),
			       COALESCE(bill_ref_required, TRUE),
			       COALESCE(bill_ref_regex,'')
			FROM provider_credentials
			WHERE tenant_id=$1 AND is_active=true
			ORDER BY id LIMIT 1`, tenantID)
		var c ProviderCredential
		if err := row.Scan(
			&c.ID, &c.TenantID, &c.Provider, &c.Shortcode, &c.Environment, &c.WebhookToken, &c.IsActive,
			&c.PasskeyEnc, &c.ConsumerKeyEnc, &c.ConsumerSecretEnc,
			&c.C2BMode, &c.BillRefRequired, &c.BillRefRegex,
		); err != nil {
			return ProviderCredential{}, err
		}
		return c, nil
	}

	row := r.db.QueryRow(ctx, `
		SELECT id, tenant_id, provider, shortcode, environment, webhook_token, is_active,
		       passkey_enc, consumer_key_enc, consumer_secret_enc,
		       COALESCE(c2b_mode,'paybill'),
		       COALESCE(bill_ref_required, TRUE),
		       COALESCE(bill_ref_regex,'')
		FROM provider_credentials
		WHERE tenant_id=$1 AND shortcode=$2 AND is_active=true`, tenantID, shortcode)
	var c ProviderCredential
	if err := row.Scan(
		&c.ID, &c.TenantID, &c.Provider, &c.Shortcode, &c.Environment, &c.WebhookToken, &c.IsActive,
		&c.PasskeyEnc, &c.ConsumerKeyEnc, &c.ConsumerSecretEnc,
		&c.C2BMode, &c.BillRefRequired, &c.BillRefRegex,
	); err != nil {
		return ProviderCredential{}, err
	}
	return c, nil
}

func (r *Repo) FindCredentialByShortcode(ctx context.Context, shortcode string) (ProviderCredential, Tenant, error) {
	row := r.db.QueryRow(ctx, `
		SELECT c.id, c.tenant_id, c.provider, c.shortcode, c.environment, c.webhook_token, c.is_active,
			c.passkey_enc, c.consumer_key_enc, c.consumer_secret_enc,
			COALESCE(c.c2b_mode,'paybill'),
			COALESCE(c.bill_ref_required, TRUE),
			COALESCE(c.bill_ref_regex,''),
			t.id, t.name, t.status
		FROM provider_credentials c
		JOIN tenants t ON t.id=c.tenant_id
		WHERE c.shortcode=$1 AND c.is_active=true`, shortcode)
	var c ProviderCredential
	var t Tenant
	if err := row.Scan(
		&c.ID, &c.TenantID, &c.Provider, &c.Shortcode, &c.Environment, &c.WebhookToken, &c.IsActive,
		&c.PasskeyEnc, &c.ConsumerKeyEnc, &c.ConsumerSecretEnc,
		&c.C2BMode, &c.BillRefRequired, &c.BillRefRegex,
		&t.ID, &t.Name, &t.Status,
	); err != nil {
		return ProviderCredential{}, Tenant{}, err
	}
	return c, t, nil
}

func (r *Repo) FindCredentialByWebhookToken(ctx context.Context, token string) (ProviderCredential, Tenant, error) {
	row := r.db.QueryRow(ctx, `
		SELECT
			c.id, c.tenant_id, c.provider, c.shortcode, c.environment, c.webhook_token, c.is_active,
			c.passkey_enc, c.consumer_key_enc, c.consumer_secret_enc,
			COALESCE(c.c2b_mode,'paybill'),
			COALESCE(c.bill_ref_required, TRUE),
			COALESCE(c.bill_ref_regex,''),
			t.id, t.name, t.status
		FROM provider_credentials c
		JOIN tenants t ON t.id=c.tenant_id
		WHERE c.webhook_token=$1 AND c.is_active=true`, token)
	var c ProviderCredential
	var t Tenant
	if err := row.Scan(
		&c.ID, &c.TenantID, &c.Provider, &c.Shortcode, &c.Environment, &c.WebhookToken, &c.IsActive,
		&c.PasskeyEnc, &c.ConsumerKeyEnc, &c.ConsumerSecretEnc,
		&c.C2BMode, &c.BillRefRequired, &c.BillRefRegex,
		&t.ID, &t.Name, &t.Status,
	); err != nil {
		return ProviderCredential{}, Tenant{}, err
	}
	return c, t, nil
}

// InsertProviderCredential saves Daraja creds + C2B rules.
func (r *Repo) InsertProviderCredential(ctx context.Context, c ProviderCredential) (ProviderCredential, error) {
	row := r.db.QueryRow(ctx, `
		INSERT INTO provider_credentials(
		  tenant_id, provider, shortcode, passkey_enc, consumer_key_enc, consumer_secret_enc,
		  environment, webhook_token, is_active, c2b_mode, bill_ref_required, bill_ref_regex
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,true,$9,$10,$11)
		RETURNING id, tenant_id, provider, shortcode, environment, webhook_token, is_active,
		          passkey_enc, consumer_key_enc, consumer_secret_enc,
		          c2b_mode, bill_ref_required, bill_ref_regex
	`,
		c.TenantID, c.Provider, c.Shortcode, c.PasskeyEnc, c.ConsumerKeyEnc, c.ConsumerSecretEnc,
		c.Environment, c.WebhookToken, c.C2BMode, c.BillRefRequired, c.BillRefRegex,
	)
	var out ProviderCredential
	if err := row.Scan(
		&out.ID, &out.TenantID, &out.Provider, &out.Shortcode, &out.Environment, &out.WebhookToken, &out.IsActive,
		&out.PasskeyEnc, &out.ConsumerKeyEnc, &out.ConsumerSecretEnc,
		&out.C2BMode, &out.BillRefRequired, &out.BillRefRegex,
	); err != nil {
		return ProviderCredential{}, err
	}
	return out, nil
}

// Decrypt secrets
func (r *Repo) DecryptPasskey(ctx context.Context, c ProviderCredential) (string, error) {
	return crypto.DecryptString(r.cfg.Sec.AESKey, c.PasskeyEnc)
}
func (r *Repo) DecryptConsumer(ctx context.Context, c ProviderCredential) (string, string, error) {
	ck, err := crypto.DecryptString(r.cfg.Sec.AESKey, c.ConsumerKeyEnc)
	if err != nil {
		return "", "", err
	}
	cs, err := crypto.DecryptString(r.cfg.Sec.AESKey, c.ConsumerSecretEnc)
	if err != nil {
		return "", "", err
	}
	return ck, cs, nil
}
