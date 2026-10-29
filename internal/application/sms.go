package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/barlus-developer/arro-microservice-sms-laos/internal/domain"
)

var (
	ErrNotFound = errors.New("SMS transaction not found")
	ErrProvider = errors.New("SMS provider request failed")
)

type SMSStore interface {
	CreateQueued(context.Context, domain.SMS) (domain.SMS, error)
	MarkAccepted(context.Context, string, ProviderReceipt) (domain.SMS, error)
	MarkFailed(context.Context, string, ProviderFailure) (domain.SMS, error)
	KeepQueuedWithError(context.Context, string, ProviderFailure) (domain.SMS, error)
	UpdateDeliveryStatus(context.Context, string, domain.Status, string) (domain.SMS, error)
	Get(context.Context, string) (domain.SMS, error)
	List(context.Context, domain.HistoryFilter) ([]domain.SMS, int64, error)
}

type SMSProvider interface {
	Send(context.Context, domain.SendSMSInput) (ProviderReceipt, error)
	Close() error
}

type ProviderReceipt struct {
	Code         string
	Message      string
	Detail       string
	PartitionKey string
	RangeKey     string
	SubmittedAt  time.Time
}

type Service struct {
	store    SMSStore
	provider SMSProvider
}

const persistenceTimeout = 5 * time.Second

func NewSMSService(store SMSStore, provider SMSProvider) *Service {
	return &Service{store: store, provider: provider}
}

func (s *Service) Send(ctx context.Context, input domain.SendSMSInput) (domain.SMS, error) {
	if err := input.Validate(); err != nil {
		return domain.SMS{}, err
	}
	queued, err := s.store.CreateQueued(ctx, domain.SMS{
		Recipient: input.Recipient,
		Content:   input.Content,
		Title:     input.Title,
		Status:    domain.StatusQueued,
	})
	if err != nil {
		return domain.SMS{}, fmt.Errorf("persist queued SMS: %w", err)
	}
	receipt, err := s.provider.Send(ctx, input)
	if err != nil {
		var failure *ProviderFailure
		persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), persistenceTimeout)
		defer cancel()
		if !errors.As(err, &failure) {
			failure = &ProviderFailure{Message: "provider request failed"}
		}
		if failure.OutcomeUnknown {
			updated, updateErr := s.store.KeepQueuedWithError(persistCtx, queued.ID, *failure)
			if updateErr != nil {
				return domain.SMS{}, fmt.Errorf("provider outcome unknown and persistence failed: %w", updateErr)
			}
			return updated, ErrProvider
		}
		var updateErr error
		queued, updateErr = s.store.MarkFailed(persistCtx, queued.ID, *failure)
		if updateErr != nil {
			return domain.SMS{}, fmt.Errorf("provider failed and failure persistence failed: %w", updateErr)
		}
		return queued, ErrProvider
	}
	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), persistenceTimeout)
	defer cancel()
	updated, err := s.store.MarkAccepted(persistCtx, queued.ID, receipt)
	if err != nil {
		return domain.SMS{}, fmt.Errorf("provider accepted SMS but transaction update failed: %w", err)
	}
	return updated, nil
}

func (s *Service) Get(ctx context.Context, id string) (domain.SMS, error) {
	return s.store.Get(ctx, id)
}

func (s *Service) List(ctx context.Context, filter domain.HistoryFilter) ([]domain.SMS, int64, error) {
	if err := filter.Validate(); err != nil {
		return nil, 0, err
	}
	return s.store.List(ctx, filter)
}

func (s *Service) UpdateDeliveryStatus(ctx context.Context, id string, status domain.Status, detail string) (domain.SMS, error) {
	if status != domain.StatusDelivered && status != domain.StatusUndelivered {
		return domain.SMS{}, domain.ErrInvalidSMS
	}
	return s.store.UpdateDeliveryStatus(ctx, id, status, detail)
}

type ProviderFailure struct {
	Code           string
	Message        string
	Detail         string
	OutcomeUnknown bool
}

func (e *ProviderFailure) Error() string {
	parts := make([]string, 0, 3)
	if e.Code != "" {
		parts = append(parts, e.Code)
	}
	if e.Message != "" {
		parts = append(parts, e.Message)
	}
	if e.Detail != "" {
		parts = append(parts, e.Detail)
	}
	if len(parts) > 0 {
		return strings.Join(parts, ": ")
	}
	return "provider submission failed"
}
