package credential

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"regexp"
	"strings"
)

// ProviderCredential represents encrypted provider credentials
type ProviderCredential struct {
	ID                int64
	TenantID          int64
	Provider          string
	ProviderType      ProviderType
	Shortcode         string
	Environment       Environment
	WebhookToken      string
	IsActive          bool
	C2BConfiguration  C2BConfig
	EncryptedCredentials map[string]string // Encrypted credential fields
}

// ProviderType represents different payment providers
type ProviderType string

const (
	ProviderMpesa       ProviderType = "mpesa_daraja"
	ProviderAirtelMoney ProviderType = "airtel_money"
	ProviderTKash       ProviderType = "tkash"
	ProviderEquitel     ProviderType = "equitel"
)

// Environment represents provider environment
type Environment string

const (
	EnvironmentSandbox    Environment = "sandbox"
	EnvironmentProduction Environment = "production"
)

// C2BConfig represents Customer-to-Business configuration
type C2BConfig struct {
	Mode            C2BMode
	BillRefRequired bool
	BillRefRegex    string
}

// C2BMode represents C2B transaction mode
type C2BMode string

const (
	C2BModePaybill  C2BMode = "paybill"
	C2BModeBuygoods C2BMode = "buygoods"
)

// NewProviderCredential creates a new provider credential with validation
func NewProviderCredential(tenantID int64, provider string, providerType ProviderType, shortcode string, env Environment, webhookToken string, c2bConfig C2BConfig) (*ProviderCredential, error) {
	if err := validateCredential(tenantID, provider, shortcode, webhookToken); err != nil {
		return nil, err
	}
	
	if err := c2bConfig.Validate(); err != nil {
		return nil, fmt.Errorf("invalid C2B configuration: %w", err)
	}
	
	return &ProviderCredential{
		TenantID:             tenantID,
		Provider:             provider,
		ProviderType:         providerType,
		Shortcode:            shortcode,
		Environment:          env,
		WebhookToken:         webhookToken,
		IsActive:             true,
		C2BConfiguration:     c2bConfig,
		EncryptedCredentials: make(map[string]string),
	}, nil
}

// IsValidForEnvironment checks if credential is valid for the specified environment
func (c *ProviderCredential) IsValidForEnvironment() bool {
	return c.Environment == EnvironmentSandbox || c.Environment == EnvironmentProduction
}

// SupportsC2B checks if the credential supports C2B transactions
func (c *ProviderCredential) SupportsC2B() bool {
	return c.ProviderType == ProviderMpesa // Only M-Pesa supports C2B for now
}

// ValidateC2BTransaction validates a C2B transaction against credential rules
func (c *ProviderCredential) ValidateC2BTransaction(billRef string) error {
	if !c.SupportsC2B() {
		return fmt.Errorf("provider %s does not support C2B", c.ProviderType)
	}
	
	return c.C2BConfiguration.ValidateTransaction(billRef)
}

// Deactivate marks the credential as inactive
func (c *ProviderCredential) Deactivate() {
	c.IsActive = false
}

// Activate marks the credential as active
func (c *ProviderCredential) Activate() error {
	if c.TenantID <= 0 || c.Shortcode == "" {
		return fmt.Errorf("cannot activate invalid credential")
	}
	c.IsActive = true
	return nil
}

// Validate validates C2B configuration
func (c *C2BConfig) Validate() error {
	if c.Mode != C2BModePaybill && c.Mode != C2BModeBuygoods {
		return fmt.Errorf("invalid C2B mode: %s", c.Mode)
	}
	
	if c.BillRefRegex != "" {
		if _, err := regexp.Compile(c.BillRefRegex); err != nil {
			return fmt.Errorf("invalid bill reference regex: %w", err)
		}
	}
	
	return nil
}

// ValidateTransaction validates a transaction against C2B rules
func (c *C2BConfig) ValidateTransaction(billRef string) error {
	if c.Mode == C2BModePaybill {
		if c.BillRefRequired && strings.TrimSpace(billRef) == "" {
			return fmt.Errorf("bill reference is required for paybill transactions")
		}
		
		if c.BillRefRegex != "" && billRef != "" {
			matched, err := regexp.MatchString(c.BillRefRegex, billRef)
			if err != nil {
				return fmt.Errorf("regex validation error: %w", err)
			}
			if !matched {
				return fmt.Errorf("bill reference does not match required format")
			}
		}
	}
	
	return nil
}

// validateCredential validates credential creation parameters
func validateCredential(tenantID int64, provider, shortcode, webhookToken string) error {
	if tenantID <= 0 {
		return fmt.Errorf("invalid tenant ID: %d", tenantID)
	}
	
	if strings.TrimSpace(provider) == "" {
		return fmt.Errorf("provider is required")
	}
	
	if strings.TrimSpace(shortcode) == "" {
		return fmt.Errorf("shortcode is required")
	}
	
	if strings.TrimSpace(webhookToken) == "" {
		return fmt.Errorf("webhook token is required")
	}
	
	return nil
}

// SetEncryptedField stores an encrypted credential field
func (c *ProviderCredential) SetEncryptedField(fieldName, value string, encryptionKey []byte) error {
	if c.EncryptedCredentials == nil {
		c.EncryptedCredentials = make(map[string]string)
	}
	
	encrypted, err := encrypt(value, encryptionKey)
	if err != nil {
		return fmt.Errorf("failed to encrypt field %s: %w", fieldName, err)
	}
	
	c.EncryptedCredentials[fieldName] = encrypted
	return nil
}

// GetDecryptedField retrieves and decrypts a credential field
func (c *ProviderCredential) GetDecryptedField(fieldName string, encryptionKey []byte) string {
	if c.EncryptedCredentials == nil {
		return ""
	}
	
	encrypted, exists := c.EncryptedCredentials[fieldName]
	if !exists {
		return ""
	}
	
	decrypted, err := decrypt(encrypted, encryptionKey)
	if err != nil {
		return "" // Return empty on decryption failure
	}
	
	return decrypted
}

// encrypt encrypts a plaintext string using AES-GCM
func encrypt(plaintext string, key []byte) (string, error) {
	if len(key) != 32 {
		return "", fmt.Errorf("encryption key must be 32 bytes")
	}
	
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	
	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	
	nonce := make([]byte, aesGCM.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	
	ciphertext := aesGCM.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// decrypt decrypts a base64 encoded ciphertext using AES-GCM
func decrypt(ciphertext string, key []byte) (string, error) {
	if len(key) != 32 {
		return "", fmt.Errorf("decryption key must be 32 bytes")
	}
	
	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", err
	}
	
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	
	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	
	nonceSize := aesGCM.NonceSize()
	if len(data) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}
	
	nonce, ciphertext_bytes := data[:nonceSize], data[nonceSize:]
	plaintext, err := aesGCM.Open(nil, nonce, ciphertext_bytes, nil)
	if err != nil {
		return "", err
	}
	
	return string(plaintext), nil
}