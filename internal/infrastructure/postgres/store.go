package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/barlus-developer/arro-microservice-sms-laos/internal/application"
	"github.com/barlus-developer/arro-microservice-sms-laos/internal/domain"
)

type Store struct{ db *sql.DB }

func New(db *sql.DB) *Store { return &Store{db: db} }

func (s *Store) Ping(ctx context.Context) error { return s.db.PingContext(ctx) }

func (s *Store) CreateQueued(ctx context.Context, sms domain.SMS) (domain.SMS, error) {
	const query = `INSERT INTO sms_transactions (recipient, content, title, status)
VALUES ($1, $2, $3, 'queued')
RETURNING id::text, recipient, content, title, status, created_at, updated_at`
	err := s.db.QueryRowContext(ctx, query, sms.Recipient, sms.Content, sms.Title).Scan(
		&sms.ID, &sms.Recipient, &sms.Content, &sms.Title, &sms.Status, &sms.CreatedAt, &sms.UpdatedAt,
	)
	return sms, err
}

func (s *Store) MarkAccepted(ctx context.Context, id string, receipt application.ProviderReceipt) (domain.SMS, error) {
	const query = `UPDATE sms_transactions SET status='sent', submitted_at=$2, provider_code=$3,
provider_message=$4, provider_detail=$5, provider_partition_key=NULLIF($6, ''),
provider_range_key=NULLIF($7, ''), error_detail=NULL, updated_at=now()
WHERE id=$1::uuid RETURNING ` + columns
	return scanSMS(s.db.QueryRowContext(ctx, query, id, receipt.SubmittedAt, receipt.Code, receipt.Message,
		receipt.Detail, receipt.PartitionKey, receipt.RangeKey))
}

func (s *Store) MarkFailed(ctx context.Context, id string, failure application.ProviderFailure) (domain.SMS, error) {
	const query = `UPDATE sms_transactions SET status='failed', provider_code=NULLIF($2, ''),
provider_message=NULLIF($3, ''), provider_detail=NULLIF($4, ''), error_detail=$5, updated_at=now()
WHERE id=$1::uuid RETURNING ` + columns
	return scanSMS(s.db.QueryRowContext(ctx, query, id, failure.Code, failure.Message, failure.Detail, failure.Error()))
}

func (s *Store) KeepQueuedWithError(ctx context.Context, id string, failure application.ProviderFailure) (domain.SMS, error) {
	const query = `UPDATE sms_transactions SET provider_code=NULLIF($2, ''), provider_message=NULLIF($3, ''),
provider_detail=NULLIF($4, ''), error_detail=$5, updated_at=now()
WHERE id=$1::uuid RETURNING ` + columns
	return scanSMS(s.db.QueryRowContext(ctx, query, id, failure.Code, failure.Message, failure.Detail, failure.Error()))
}

func (s *Store) UpdateDeliveryStatus(ctx context.Context, id string, status domain.Status, detail string) (domain.SMS, error) {
	const query = `UPDATE sms_transactions SET status=$2, provider_detail=NULLIF($3, ''), updated_at=now()
WHERE id=$1::uuid RETURNING ` + columns
	return scanSMS(s.db.QueryRowContext(ctx, query, id, status, detail))
}

func (s *Store) Get(ctx context.Context, id string) (domain.SMS, error) {
	return scanSMS(s.db.QueryRowContext(ctx, `SELECT `+columns+` FROM sms_transactions WHERE id=$1::uuid`, id))
}

const columns = `id::text, recipient, content, title, submitted_at, status, provider_code,
provider_message, provider_detail, provider_partition_key, provider_range_key, error_detail, created_at, updated_at`

func scanSMS(row *sql.Row) (domain.SMS, error) {
	var sms domain.SMS
	var submitted sql.NullTime
	var providerCode, providerMessage, providerDetail, partitionKey, rangeKey, errorDetail sql.NullString
	err := row.Scan(&sms.ID, &sms.Recipient, &sms.Content, &sms.Title, &submitted, &sms.Status,
		&providerCode, &providerMessage, &providerDetail, &partitionKey, &rangeKey, &errorDetail, &sms.CreatedAt, &sms.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.SMS{}, application.ErrNotFound
	}
	if err != nil {
		return domain.SMS{}, err
	}
	if submitted.Valid {
		sms.SubmittedAt = &submitted.Time
	}
	sms.ProviderCode, sms.ProviderMessage = providerCode.String, providerMessage.String
	sms.ProviderDetail, sms.ProviderPartitionKey, sms.ProviderRangeKey = providerDetail.String, partitionKey.String, rangeKey.String
	sms.ErrorDetail = errorDetail.String
	return sms, nil
}

func (s *Store) List(ctx context.Context, filter domain.HistoryFilter) ([]domain.SMS, int64, error) {
	const base = ` FROM sms_transactions WHERE ($1 = '' OR status = $1)
AND ($2 = '' OR recipient = $2) AND ($3::timestamptz IS NULL OR submitted_at >= $3)
AND ($4::timestamptz IS NULL OR submitted_at <= $4)`
	args := []any{string(filter.Status), filter.Recipient, filter.From, filter.To}
	var total int64
	if err := s.db.QueryRowContext(ctx, `SELECT count(*)`+base, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count SMS history: %w", err)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT `+columns+base+` ORDER BY created_at DESC, id DESC LIMIT $5 OFFSET $6`, append(args, filter.Limit, filter.Offset)...)
	if err != nil {
		return nil, 0, fmt.Errorf("query SMS history: %w", err)
	}
	defer rows.Close()
	items := make([]domain.SMS, 0)
	for rows.Next() {
		var sms domain.SMS
		var submitted sql.NullTime
		var providerCode, providerMessage, providerDetail, partitionKey, rangeKey, errorDetail sql.NullString
		if err := rows.Scan(&sms.ID, &sms.Recipient, &sms.Content, &sms.Title, &submitted, &sms.Status,
			&providerCode, &providerMessage, &providerDetail, &partitionKey, &rangeKey, &errorDetail, &sms.CreatedAt, &sms.UpdatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan SMS history: %w", err)
		}
		if submitted.Valid {
			sms.SubmittedAt = &submitted.Time
		}
		sms.ProviderCode, sms.ProviderMessage = providerCode.String, providerMessage.String
		sms.ProviderDetail, sms.ProviderPartitionKey, sms.ProviderRangeKey = providerDetail.String, partitionKey.String, rangeKey.String
		sms.ErrorDetail = errorDetail.String
		items = append(items, sms)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("read SMS history: %w", err)
	}
	return items, total, nil
}

var _ application.SMSStore = (*Store)(nil)
