package domain

const (
	ErrCodeAccountNotFound    = "ACCOUNT_NOT_FOUND"
	ErrCodeRateLimitExceeded  = "RATE_LIMIT_EXCEEDED"
	ErrCodeNoAvailableAccount = "NO_AVAILABLE_ACCOUNT"
	ErrCodeProviderNotFound   = "PROVIDER_NOT_FOUND"
	ErrCodeInvalidConfig      = "INVALID_CONFIG"
)

type DomainError struct {
	Code    string
	Message string
	Details map[string]interface{}
}

func (e *DomainError) Error() string {
	return e.Message
}

func NewDomainError(code, message string) *DomainError {
	return &DomainError{
		Code:    code,
		Message: message,
		Details: make(map[string]interface{}),
	}
}

func NewRateLimitError(limitType string, current, max int) *DomainError {
	return &DomainError{
		Code:    ErrCodeRateLimitExceeded,
		Message: "rate limit exceeded",
		Details: map[string]interface{}{
			"limit_type": limitType,
			"current":    current,
			"max":        max,
		},
	}
}
