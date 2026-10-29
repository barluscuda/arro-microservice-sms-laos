package telbiz

import (
	"context"
	"errors"
	"time"

	"github.com/barlus-developer/arro-microservice-sms-laos/internal/application"
	"github.com/barlus-developer/arro-microservice-sms-laos/internal/domain"
	sdk "github.com/barlus-developer/golang-sdk-telbiz"
)

type Client struct{ client *sdk.Client }

func New(clientID, clientSecret string, timeout time.Duration) *Client {
	return &Client{client: sdk.New(sdk.Config{ClientID: clientID, ClientSecret: clientSecret, Timeout: timeout})}
}

func (c *Client) Send(ctx context.Context, input domain.SendSMSInput) (application.ProviderReceipt, error) {
	result, err := c.client.SendSMS(ctx, sdk.SMSRequest{
		Title: sdk.SMSTitle(input.Title), Phone: input.Recipient, Message: input.Content,
	})
	if err != nil {
		var providerErr *sdk.Error
		failure := &application.ProviderFailure{}
		if errors.As(err, &providerErr) {
			failure.Code = providerErr.Code
			failure.Message = providerErr.Message
			failure.Detail = providerErr.Detail
			failure.OutcomeUnknown = providerErr.OutcomeUnknown
		} else {
			failure.Message = "provider request failed"
		}
		return application.ProviderReceipt{}, failure
	}
	return application.ProviderReceipt{
		Code: result.Response.Code, Message: result.Response.Message, Detail: result.Response.Detail,
		PartitionKey: result.Key.PartitionKey, RangeKey: result.Key.RangeKey, SubmittedAt: time.Now().UTC(),
	}, nil
}

func (c *Client) Close() error { return c.client.Close() }
