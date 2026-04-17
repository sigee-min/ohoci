package store

import (
	"context"
	"database/sql"
	"strings"
	"time"
)

type Event struct {
	DeliveryID  string     `json:"deliveryId"`
	EventType   string     `json:"eventType"`
	Action      string     `json:"action"`
	RepoOwner   string     `json:"repoOwner"`
	RepoName    string     `json:"repoName"`
	PayloadJSON string     `json:"payloadJson,omitempty"`
	ProcessedAt *time.Time `json:"processedAt,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
}

type EventLog struct {
	ID          int64     `json:"id"`
	DeliveryID  string    `json:"deliveryId,omitempty"`
	Level       string    `json:"level"`
	Message     string    `json:"message"`
	DetailsJSON string    `json:"detailsJson,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
}

func (s *Store) CreateEventDelivery(ctx context.Context, event Event) (bool, error) {
	_, err := s.db.ExecContext(ctx, `INSERT INTO events (delivery_id, event_type, action, repo_owner, repo_name, payload_json, processed_at, created_at) VALUES (?, ?, ?, ?, ?, ?, NULL, ?)`,
		event.DeliveryID, event.EventType, event.Action, event.RepoOwner, event.RepoName, event.PayloadJSON, s.now().UTC())
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") || strings.Contains(strings.ToLower(err.Error()), "duplicate") {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (s *Store) MarkEventProcessed(ctx context.Context, deliveryID string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE events SET processed_at = ? WHERE delivery_id = ?`, s.now().UTC(), deliveryID)
	return err
}

func (s *Store) AddEventLog(ctx context.Context, log EventLog) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO event_logs (delivery_id, level, message, details_json, created_at) VALUES (?, ?, ?, ?, ?)`, nullableString(log.DeliveryID), strings.TrimSpace(log.Level), strings.TrimSpace(log.Message), strings.TrimSpace(log.DetailsJSON), s.now().UTC())
	return err
}

func (s *Store) ListEvents(ctx context.Context, limit int) ([]Event, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT delivery_id, event_type, action, repo_owner, repo_name, payload_json, processed_at, created_at FROM events ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Event{}
	for rows.Next() {
		var event Event
		var processed sql.NullTime
		if err := rows.Scan(&event.DeliveryID, &event.EventType, &event.Action, &event.RepoOwner, &event.RepoName, &event.PayloadJSON, &processed, &event.CreatedAt); err != nil {
			return nil, err
		}
		if processed.Valid {
			value := processed.Time.UTC()
			event.ProcessedAt = &value
		}
		out = append(out, event)
	}
	return out, rows.Err()
}

func (s *Store) ListEventLogs(ctx context.Context, limit int) ([]EventLog, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, delivery_id, level, message, details_json, created_at FROM event_logs ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []EventLog{}
	for rows.Next() {
		var log EventLog
		var delivery sql.NullString
		if err := rows.Scan(&log.ID, &delivery, &log.Level, &log.Message, &log.DetailsJSON, &log.CreatedAt); err != nil {
			return nil, err
		}
		if delivery.Valid {
			log.DeliveryID = delivery.String
		}
		out = append(out, log)
	}
	return out, rows.Err()
}
