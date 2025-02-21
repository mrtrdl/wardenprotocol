// Package keychain is an SDK for building Keychain on the Warden Protocol
// blockchain.
//
// To learn more about the Warden Protocol, visit https://docs.wardenprotocol.com/.
//
// For an example of an application built using this SDK, see the `wardenkms/` folder.
package keychain

import (
	"context"
	"fmt"
	"io"
	"log/slog"

	"github.com/warden-protocol/wardenprotocol/go-client"
	wardentypes "github.com/warden-protocol/wardenprotocol/warden/x/warden/types"
	"google.golang.org/grpc/connectivity"
)

type App struct {
	config             Config
	keyRequestHandler  KeyRequestHandler
	signRequestHandler SignRequestHandler

	query              *client.QueryClient
	txWriter           *TxWriter
	keyRequestTracker  *RequestTracker
	signRequestTracker *RequestTracker
}

func NewApp(config Config) *App {
	return &App{
		config:             config,
		keyRequestTracker:  NewRequestTracker(),
		signRequestTracker: NewRequestTracker(),
	}
}

func (a *App) logger() *slog.Logger {
	if a.config.Logger == nil {
		a.config.Logger = slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{}))
	}

	return a.config.Logger
}

func (a *App) SetKeyRequestHandler(handler KeyRequestHandler) {
	a.keyRequestHandler = handler
}

func (a *App) SetSignRequestHandler(handler SignRequestHandler) {
	a.signRequestHandler = handler
}

func (a *App) Start(ctx context.Context) error {
	a.logger().Info("starting keychain", "keychain_id", a.config.KeychainId)

	err := a.initConnections()
	if err != nil {
		return fmt.Errorf("failed to init connections: %w", err)
	}

	keyRequestsCh := make(chan *wardentypes.KeyRequest)
	defer close(keyRequestsCh)
	go a.ingestKeyRequests(keyRequestsCh)

	signRequestsCh := make(chan *wardentypes.SignRequest)
	defer close(signRequestsCh)
	go a.ingestSignRequests(signRequestsCh)

	flushErrors := make(chan error)
	defer close(flushErrors)
	go func() {
		if err := a.txWriter.Start(ctx, flushErrors); err != nil {
			a.logger().Error("tx writer exited with error", "error", err)
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-flushErrors:
			a.logger().Error("tx writer flush error", "error", err)
		case keyRequest := <-keyRequestsCh:
			go a.handleKeyRequest(keyRequest)
		case signRequest := <-signRequestsCh:
			go a.handleSignRequest(signRequest)
		}
	}
}

func (a *App) ConnectionState() connectivity.State {
	return a.query.Conn().GetState()
}

func (a *App) initConnections() error {
	a.logger().Info("connecting to Warden Protocol using gRPC", "url", a.config.GRPCURL, "insecure", a.config.GRPCInsecure)
	query, err := client.NewQueryClient(a.config.GRPCURL, a.config.GRPCInsecure)
	if err != nil {
		return fmt.Errorf("failed to create query client: %w", err)
	}
	a.query = query

	conn := query.Conn()

	identity, err := client.NewIdentityFromSeed(a.config.DerivationPath, a.config.Mnemonic)
	if err != nil {
		return fmt.Errorf("failed to create identity: %w", err)
	}

	a.logger().Info("keychain party identity", "address", identity.Address.String())

	txClient := client.NewTxClient(identity, a.config.ChainID, conn, query)
	a.txWriter = NewTxWriter(txClient, a.config.BatchSize, a.config.BatchTimeout, a.logger())
	a.txWriter.GasLimit = a.config.GasLimit

	return nil
}

var defaultPageLimit = uint64(20)
