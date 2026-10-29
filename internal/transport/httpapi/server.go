package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/barlus-developer/arro-microservice-sms-laos/internal/application"
	"github.com/barlus-developer/arro-microservice-sms-laos/internal/domain"
	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
)

type SMSService interface {
	Send(context.Context, domain.SendSMSInput) (domain.SMS, error)
	Get(context.Context, string) (domain.SMS, error)
	List(context.Context, domain.HistoryFilter) ([]domain.SMS, int64, error)
	UpdateDeliveryStatus(context.Context, string, domain.Status, string) (domain.SMS, error)
}

type Readiness interface{ Ping(context.Context) error }

type Server struct {
	*echo.Echo
	service   SMSService
	readiness Readiness
}

var uuidPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func New(service SMSService, readiness Readiness, logger *zap.Logger, bodyLimit string) *Server {
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true
	e.Use(RequestLogger(logger))
	e.Use(echo.MiddlewareFunc(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if c.Request().Body != nil {
				c.Request().Body = http.MaxBytesReader(c.Response(), c.Request().Body, parseBodyLimit(bodyLimit))
			}
			return next(c)
		}
	}))
	e.HTTPErrorHandler = errorHandler(logger)
	s := &Server{Echo: e, service: service, readiness: readiness}
	e.GET("/healthz", s.health)
	e.GET("/readyz", s.ready)
	e.POST("/v1/sms", s.send)
	e.GET("/v1/sms", s.list)
	e.GET("/v1/sms/:id", s.get)
	e.POST("/internal/v1/sms/:id/delivery-status", s.deliveryStatus)
	return s
}

func parseBodyLimit(value string) int64 {
	var limit int64 = 1 << 20
	if strings.HasSuffix(strings.ToUpper(value), "M") {
		if parsed, err := strconv.ParseInt(value[:len(value)-1], 10, 64); err == nil && parsed > 0 {
			limit = parsed << 20
		}
	} else if parsed, err := strconv.ParseInt(value, 10, 64); err == nil && parsed > 0 {
		limit = parsed
	}
	return limit
}

type sendRequest struct {
	Recipient string `json:"recipient"`
	Content   string `json:"content"`
	Title     string `json:"title"`
}

func (s *Server) send(c echo.Context) error {
	var request sendRequest
	if err := decodeJSON(c, &request); err != nil {
		return badRequest("invalid JSON body")
	}
	if err := (domain.SendSMSInput{Recipient: request.Recipient, Content: request.Content, Title: request.Title}).Validate(); err != nil {
		return badRequest("recipient, content, and a supported title are required")
	}
	item, err := s.service.Send(c.Request().Context(), domain.SendSMSInput{
		Recipient: request.Recipient, Content: request.Content, Title: request.Title,
	})
	if err != nil {
		if errors.Is(err, application.ErrProvider) {
			return c.JSON(http.StatusBadGateway, map[string]any{"data": item, "error": map[string]string{"code": "provider_error", "message": "SMS provider submission failed or could not be confirmed"}})
		}
		return err
	}
	return c.JSON(http.StatusAccepted, map[string]any{"data": item})
}

func (s *Server) list(c echo.Context) error {
	limit := queryInt(c, "limit", 20)
	offset := queryInt(c, "offset", 0)
	filter := domain.HistoryFilter{Status: domain.Status(c.QueryParam("status")), Recipient: c.QueryParam("recipient"), Limit: limit, Offset: offset}
	var err error
	if value := c.QueryParam("from"); value != "" {
		parsed, parseErr := time.Parse(time.RFC3339, value)
		if parseErr != nil {
			return badRequest("from must be RFC3339")
		}
		filter.From = &parsed
	}
	if value := c.QueryParam("to"); value != "" {
		parsed, parseErr := time.Parse(time.RFC3339, value)
		if parseErr != nil {
			return badRequest("to must be RFC3339")
		}
		filter.To = &parsed
	}
	if err = filter.Validate(); err != nil {
		return badRequest(err.Error())
	}
	items, total, err := s.service.List(c.Request().Context(), filter)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]any{"data": items, "pagination": map[string]any{"limit": limit, "offset": offset, "total": total}})
}

func (s *Server) get(c echo.Context) error {
	id := c.Param("id")
	if !uuidPattern.MatchString(id) {
		return badRequest("id must be a UUID")
	}
	item, err := s.service.Get(c.Request().Context(), id)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]any{"data": item})
}

type deliveryStatusRequest struct {
	Status domain.Status `json:"status"`
	Detail string        `json:"detail"`
}

func (s *Server) deliveryStatus(c echo.Context) error {
	if !uuidPattern.MatchString(c.Param("id")) {
		return badRequest("id must be a UUID")
	}
	var request deliveryStatusRequest
	if err := decodeJSON(c, &request); err != nil {
		return badRequest("invalid JSON body")
	}
	item, err := s.service.UpdateDeliveryStatus(c.Request().Context(), c.Param("id"), request.Status, request.Detail)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]any{"data": item})
}

func (s *Server) health(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) ready(c echo.Context) error {
	ctx, cancel := context.WithTimeout(c.Request().Context(), 2*time.Second)
	defer cancel()
	if err := s.readiness.Ping(ctx); err != nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"status": "not_ready"})
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ready"})
}

func queryInt(c echo.Context, key string, fallback int) int {
	value := c.QueryParam(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return -1
	}
	return parsed
}

func decodeJSON(c echo.Context, destination any) error {
	if !strings.HasPrefix(c.Request().Header.Get(echo.HeaderContentType), echo.MIMEApplicationJSON) {
		return fmt.Errorf("content type must be application/json")
	}
	decoder := json.NewDecoder(c.Request().Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return fmt.Errorf("body must contain one JSON object")
	}
	return nil
}

func badRequest(message string) error { return echo.NewHTTPError(http.StatusBadRequest, message) }

func errorHandler(logger *zap.Logger) echo.HTTPErrorHandler {
	return func(err error, c echo.Context) {
		if c.Response().Committed {
			return
		}
		var httpErr *echo.HTTPError
		if errors.As(err, &httpErr) {
			_ = c.JSON(httpErr.Code, map[string]any{"error": map[string]string{"code": "request_error", "message": fmt.Sprint(httpErr.Message)}})
			return
		}
		if errors.Is(err, application.ErrNotFound) {
			_ = c.JSON(http.StatusNotFound, map[string]any{"error": map[string]string{"code": "not_found", "message": "SMS transaction not found"}})
			return
		}
		logger.Error("request failed", zap.Error(err))
		_ = c.JSON(http.StatusInternalServerError, map[string]any{"error": map[string]string{"code": "internal_error", "message": "internal server error"}})
	}
}

func RequestLogger(logger *zap.Logger) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			started := time.Now()
			err := next(c)
			status := c.Response().Status
			if err != nil {
				var httpErr *echo.HTTPError
				switch {
				case errors.As(err, &httpErr):
					status = httpErr.Code
				case errors.Is(err, application.ErrNotFound):
					status = http.StatusNotFound
				default:
					status = http.StatusInternalServerError
				}
			}
			logger.Info("http request", zap.String("method", c.Request().Method), zap.String("path", c.Path()),
				zap.Int("status", status), zap.Duration("duration", time.Since(started)))
			return err
		}
	}
}
