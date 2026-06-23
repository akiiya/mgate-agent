package transport

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"time"

	"mgate-agent/internal/audit"
	"mgate-agent/internal/outbox"
	"mgate-agent/internal/protocol"
)

type ResultSender func(context.Context, outbox.Record) error

type ResultDispatcher struct {
	store  *outbox.Store
	logger *slog.Logger
	audit  *audit.Writer

	mu         sync.Mutex
	wsSender   ResultSender
	pullSender ResultSender
	sending    bool
}

func NewResultDispatcher(store *outbox.Store, auditor *audit.Writer, logger *slog.Logger) (*ResultDispatcher, error) {
	if store == nil {
		return nil, errors.New("result dispatcher requires outbox store")
	}
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &ResultDispatcher{store: store, audit: auditor, logger: logger}, nil
}

func (d *ResultDispatcher) SetWebSocketSender(sender ResultSender) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.wsSender = sender
}

func (d *ResultDispatcher) SetPullSender(sender ResultSender) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.pullSender = sender
}

func (d *ResultDispatcher) Enqueue(ctx context.Context, env protocol.Envelope) error {
	// outbox 只保存 result，不保存 command。发送失败只能补发结果，不能触发本地命令重放。
	if _, err := d.store.Save(env); err != nil {
		return err
	}
	return d.DispatchOnce(ctx)
}

func (d *ResultDispatcher) Run(ctx context.Context, interval time.Duration) error {
	if interval <= 0 {
		interval = time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			_ = d.DispatchOnce(ctx)
		}
	}
}

func (d *ResultDispatcher) DispatchOnce(ctx context.Context) error {
	d.mu.Lock()
	if d.sending {
		d.mu.Unlock()
		return nil
	}
	d.sending = true
	d.mu.Unlock()
	defer func() {
		d.mu.Lock()
		d.sending = false
		d.mu.Unlock()
	}()

	records, err := d.store.Due(time.Now().UTC(), 16)
	if err != nil {
		return err
	}
	for _, record := range records {
		if ctx.Err() != nil {
			return nil
		}
		sender, transportName := d.sender()
		if sender == nil {
			return nil
		}
		d.writeAudit(audit.Event{
			Event:     audit.EventOutboxRecordSendStarted,
			RecordID:  record.RecordID,
			CommandID: record.CommandID,
			Attempts:  record.Attempts,
			Transport: transportName,
		})
		if err := sender(ctx, record); err != nil {
			updated, markErr := d.store.MarkFailure(record.RecordID, err)
			if markErr != nil {
				return markErr
			}
			d.writeAudit(audit.Event{
				Event:     audit.EventOutboxRecordSendFailed,
				RecordID:  updated.RecordID,
				CommandID: updated.CommandID,
				Attempts:  updated.Attempts,
				Transport: transportName,
				ErrorCode: "send_failed",
			})
			d.logger.Warn("outbox result 发送失败，保留等待重试", "command_id", updated.CommandID, "transport", transportName, "attempts", updated.Attempts)
			continue
		}
		d.writeAudit(audit.Event{
			Event:     audit.EventOutboxRecordSendSucceeded,
			RecordID:  record.RecordID,
			CommandID: record.CommandID,
			Attempts:  record.Attempts,
			Transport: transportName,
		})
		if err := d.store.Delete(record.RecordID); err != nil {
			return err
		}
	}
	return nil
}

func (d *ResultDispatcher) PendingCount() int {
	return d.store.PendingCount()
}

func (d *ResultDispatcher) sender() (ResultSender, string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.wsSender != nil {
		return d.wsSender, "websocket"
	}
	if d.pullSender != nil {
		return d.pullSender, "pull"
	}
	return nil, ""
}

func (d *ResultDispatcher) writeAudit(event audit.Event) {
	if d.audit == nil {
		return
	}
	_ = d.audit.Write(event)
}
