package keychain

import (
	"context"
	"encoding/hex"
	"log/slog"
	"time"

	"github.com/warden-protocol/wardenprotocol/go-client"
	wardentypes "github.com/warden-protocol/wardenprotocol/warden/x/warden/types"
)

type KeyResponseWriter interface {
	Fulfil(publicKey []byte) error
	Reject(reason string) error
}

type KeyRequest wardentypes.KeyRequest

type KeyRequestHandler func(w KeyResponseWriter, req *KeyRequest)

type keyResponseWriter struct {
	ctx          context.Context
	txWriter     *TxWriter
	keyRequestID uint64
	logger       *slog.Logger
	onComplete   func()
}

func (w *keyResponseWriter) Fulfil(publicKey []byte) error {
	w.logger.Debug("fulfilling key request", "id", w.keyRequestID, "public_key", hex.EncodeToString(publicKey))
	defer w.onComplete()
	return w.txWriter.Write(w.ctx, client.KeyRequestFulfilment{
		RequestID: w.keyRequestID,
		PublicKey: publicKey,
	})
}

func (w *keyResponseWriter) Reject(reason string) error {
	w.logger.Debug("rejecting key request", "id", w.keyRequestID, "reason", reason)
	defer w.onComplete()
	return w.txWriter.Write(w.ctx, client.KeyRequestRejection{
		RequestID: w.keyRequestID,
		Reason:    reason,
	})
}

func (a *App) ingestKeyRequests(keyRequestsCh chan *wardentypes.KeyRequest) {
	for {
		reqCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		keyRequests, err := a.keyRequests(reqCtx)
		cancel()
		if err != nil {
			a.logger().Error("failed to get key requests", "error", err)
		} else {
			for _, keyRequest := range keyRequests {
				if !a.keyRequestTracker.IsNew(keyRequest.Id) {
					a.logger().Debug("skipping key request", "id", keyRequest.Id)
					continue
				}

				a.logger().Info("got key request", "id", keyRequest.Id)
				a.keyRequestTracker.Ingested(keyRequest.Id)
				keyRequestsCh <- keyRequest
			}
		}

		time.Sleep(5 * time.Second)
	}
}

func (a *App) handleKeyRequest(keyRequest *wardentypes.KeyRequest) {
	if a.keyRequestHandler == nil {
		a.logger().Error("key request handler not set")
		return
	}

	go func() {
		ctx := context.Background()
		w := &keyResponseWriter{
			ctx:          ctx,
			txWriter:     a.txWriter,
			keyRequestID: keyRequest.Id,
			logger:       a.logger(),
			onComplete: func() {
				a.keyRequestTracker.Done(keyRequest.Id)
			},
		}
		defer func() {
			if r := recover(); r != nil {
				a.logger().Error("panic in key request handler", "error", r)
				_ = w.Reject("internal error")
				return
			}
		}()

		a.keyRequestHandler(w, (*KeyRequest)(keyRequest))
	}()
}

func (a *App) keyRequests(ctx context.Context) ([]*wardentypes.KeyRequest, error) {
	return a.query.PendingKeyRequests(ctx, &client.PageRequest{Limit: defaultPageLimit}, a.config.KeychainId)
}
