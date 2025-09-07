package base

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"paymatch/internal/provider"
)

// PhoneValidator provides phone number validation for different providers
type PhoneValidator struct {
	countryCode string
	patterns    []*regexp.Regexp
}

// NewPhoneValidator creates a validator for a specific country
func NewPhoneValidator(countryCode string) *PhoneValidator {
	var patterns []*regexp.Regexp
	
	switch countryCode {
	case "KE": // Kenya
		patterns = []*regexp.Regexp{
			regexp.MustCompile(`^254[17]\d{8}$`),     // Safaricom, Airtel
			regexp.MustCompile(`^254[78]\d{8}$`),     // Safaricom, Orange
			regexp.MustCompile(`^2547[0-9]\d{7}$`),   // Various providers
		}
	case "UG": // Uganda
		patterns = []*regexp.Regexp{
			regexp.MustCompile(`^256[37]\d{8}$`),     // MTN, Airtel
			regexp.MustCompile(`^256[47]\d{8}$`),     // Uganda Telecom
		}
	case "TZ": // Tanzania
		patterns = []*regexp.Regexp{
			regexp.MustCompile(`^255[67]\d{8}$`),     // Vodacom, Airtel
			regexp.MustCompile(`^255[78]\d{8}$`),     // Tigo, Halotel
		}
	}
	
	return &PhoneValidator{
		countryCode: countryCode,
		patterns:    patterns,
	}
}

// ValidatePhone validates and normalizes a phone number
func (v *PhoneValidator) ValidatePhone(phone string) (string, error) {
	// Remove any spaces, dashes, or plus signs
	normalized := strings.ReplaceAll(phone, " ", "")
	normalized = strings.ReplaceAll(normalized, "-", "")
	normalized = strings.ReplaceAll(normalized, "+", "")
	
	// If phone starts with 0, replace with country code
	if strings.HasPrefix(normalized, "0") && v.countryCode == "KE" {
		normalized = "254" + normalized[1:]
	}
	
	// Validate against patterns
	for _, pattern := range v.patterns {
		if pattern.MatchString(normalized) {
			return normalized, nil
		}
	}
	
	return "", &provider.ProviderError{
		Code:    provider.ErrInvalidPhone,
		Message: fmt.Sprintf("invalid phone number format for %s", v.countryCode),
	}
}

// AmountValidator validates payment amounts
type AmountValidator struct {
	minAmount int
	maxAmount int
	currency  string
}

// NewAmountValidator creates an amount validator with limits
func NewAmountValidator(currency string, minAmount, maxAmount int) *AmountValidator {
	return &AmountValidator{
		minAmount: minAmount,
		maxAmount: maxAmount,
		currency:  currency,
	}
}

// ValidateAmount validates payment amount
func (v *AmountValidator) ValidateAmount(amount int) error {
	if amount <= 0 {
		return &provider.ProviderError{
			Code:    provider.ErrInvalidAmount,
			Message: "amount must be greater than zero",
		}
	}
	
	if amount < v.minAmount {
		return &provider.ProviderError{
			Code:    provider.ErrInvalidAmount,
			Message: fmt.Sprintf("amount must be at least %d %s", v.minAmount, v.currency),
		}
	}
	
	if v.maxAmount > 0 && amount > v.maxAmount {
		return &provider.ProviderError{
			Code:    provider.ErrInvalidAmount,
			Message: fmt.Sprintf("amount must not exceed %d %s", v.maxAmount, v.currency),
		}
	}
	
	return nil
}

// RequestValidator provides common request validation
type RequestValidator struct {
	phoneValidator  *PhoneValidator
	amountValidator *AmountValidator
}

// NewRequestValidator creates a new request validator
func NewRequestValidator(countryCode, currency string, minAmount, maxAmount int) *RequestValidator {
	return &RequestValidator{
		phoneValidator:  NewPhoneValidator(countryCode),
		amountValidator: NewAmountValidator(currency, minAmount, maxAmount),
	}
}

// ValidateSTKPushReq validates STK push request
func (v *RequestValidator) ValidateSTKPushReq(req *provider.STKPushReq) error {
	// Validate amount
	if err := v.amountValidator.ValidateAmount(int(req.Amount)); err != nil {
		return err
	}
	
	// Validate and normalize phone
	normalizedPhone, err := v.phoneValidator.ValidatePhone(req.PhoneNumber)
	if err != nil {
		return err
	}
	req.PhoneNumber = normalizedPhone
	
	// Validate account reference
	if strings.TrimSpace(req.AccountReference) == "" {
		return &provider.ProviderError{
			Code:    "invalid_account_ref",
			Message: "account reference is required",
		}
	}
	
	// Validate description
	if strings.TrimSpace(req.Description) == "" {
		return &provider.ProviderError{
			Code:    "invalid_description",
			Message: "description is required",
		}
	}
	
	return nil
}

// ValidateB2CReq validates B2C request
func (v *RequestValidator) ValidateB2CReq(req *provider.B2CReq) error {
	// Validate amount
	if err := v.amountValidator.ValidateAmount(int(req.Amount)); err != nil {
		return err
	}
	
	// Validate and normalize phone
	normalizedPhone, err := v.phoneValidator.ValidatePhone(req.PhoneNumber)
	if err != nil {
		return err
	}
	req.PhoneNumber = normalizedPhone
	
	// Validate command ID
	validCommands := []string{"SalaryPayment", "BusinessPayment", "PromotionPayment"}
	if !contains(validCommands, req.CommandID) {
		return &provider.ProviderError{
			Code:    "invalid_command_id",
			Message: fmt.Sprintf("command_id must be one of: %s", strings.Join(validCommands, ", ")),
		}
	}
	
	return nil
}

// Utility functions

// contains checks if a slice contains a string
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// FormatAmount formats amount for display
func FormatAmount(amount int, currency string) string {
	switch currency {
	case "KES", "UGX", "TZS":
		return fmt.Sprintf("%s %d", currency, amount)
	default:
		// For currencies with decimal places, divide by 100
		return fmt.Sprintf("%s %.2f", currency, float64(amount)/100)
	}
}

// ParseAmount parses amount from string
func ParseAmount(amountStr string) (int, error) {
	// Remove currency symbols and spaces
	cleaned := strings.ReplaceAll(amountStr, ",", "")
	cleaned = strings.TrimSpace(cleaned)
	
	// Try to parse as integer first
	if amount, err := strconv.Atoi(cleaned); err == nil {
		return amount, nil
	}
	
	// Try to parse as float and convert to cents
	if amount, err := strconv.ParseFloat(cleaned, 64); err == nil {
		return int(amount * 100), nil
	}
	
	return 0, fmt.Errorf("invalid amount format: %s", amountStr)
}