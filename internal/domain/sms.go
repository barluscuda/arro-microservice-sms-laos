package domain

import (
	"errors"
	"strings"
	"time"
)

type Status string

const (
	StatusQueued      Status = "queued"
	StatusSent        Status = "sent"
	StatusFailed      Status = "failed"
	StatusDelivered   Status = "delivered"
	StatusUndelivered Status = "undelivered"
)

var ErrInvalidSMS = errors.New("invalid SMS")

type SMS struct {
	ID                   string     `json:"id"`
	Recipient            string     `json:"recipient"`
	Content              string     `json:"content"`
	Title                string     `json:"title"`
	SubmittedAt          *time.Time `json:"submitted_at,omitempty"`
	Status               Status     `json:"status"`
	ProviderCode         string     `json:"provider_code,omitempty"`
	ProviderMessage      string     `json:"provider_message,omitempty"`
	ProviderDetail       string     `json:"provider_detail,omitempty"`
	ProviderPartitionKey string     `json:"-"`
	ProviderRangeKey     string     `json:"-"`
	ErrorDetail          string     `json:"error_detail,omitempty"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

type SendSMSInput struct {
	Recipient string
	Content   string
	Title     string
}

func (in SendSMSInput) Validate() error {
	if strings.TrimSpace(in.Recipient) == "" || strings.TrimSpace(in.Content) == "" {
		return ErrInvalidSMS
	}
	switch in.Title {
	case "OTP", "Info", "Promotion", "News":
		return nil
	default:
		return ErrInvalidSMS
	}
}

type HistoryFilter struct {
	Status    Status
	Recipient string
	From      *time.Time
	To        *time.Time
	Limit     int
	Offset    int
}

func (f HistoryFilter) Validate() error {
	if f.Limit < 1 || f.Limit > 100 || f.Offset < 0 {
		return errors.New("invalid pagination")
	}
	if f.Status != "" && !validStatus(f.Status) {
		return errors.New("invalid status")
	}
	if f.From != nil && f.To != nil && f.From.After(*f.To) {
		return errors.New("from must be before or equal to to")
	}
	return nil
}

func validStatus(status Status) bool {
	switch status {
	case StatusQueued, StatusSent, StatusFailed, StatusDelivered, StatusUndelivered:
		return true
	default:
		return false
	}
}
