package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/sethvargo/go-envconfig"
	"google.golang.org/grpc/connectivity"

	"github.com/warden-protocol/wardenprotocol/keychain-sdk"
	"github.com/warden-protocol/wardenprotocol/warden/x/warden/types"
)

type Config struct {
	ChainID        string `env:"CHAIN_ID, default=wardenprotocol"`
	GRPCURL        string `env:"GRPC_URL, default=localhost:9090"`
	GRPCInsecure   bool   `env:"GRPC_INSECURE, default=true"`
	DerivationPath string `env:"DERIVATION_PATH, default=m/44'/118'/0'/0/0"`
	Mnemonic       string `env:"MNEMONIC, default=exclude try nephew main caught favorite tone degree lottery device tissue tent ugly mouse pelican gasp lava flush pen river noise remind balcony emerge"`
	KeychainId     uint64 `env:"KEYCHAIN_ID, default=1"`

	KeyringMnemonic string `env:"KEYRING_MNEMONIC, required"`
	KeyringPassword string `env:"KEYRING_PASSWORD, required"`

	BatchTimeout time.Duration `env:"BATCH_TIMEOUT, default=8s"`
	BatchSize    int           `env:"BATCH_SIZE, default=20"`
	GasLimit     uint64        `env:"GAS_LIMIT, default=400000"`

	HttpAddr string `env:"HTTP_ADDR, default=:8080"`

	LogLevel slog.Level `env:"LOG_LEVEL, default=debug"`
}

func main() {
	var cfg Config
	if err := envconfig.Process(context.Background(), &cfg); err != nil {
		log.Fatal(err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: cfg.LogLevel,
	}))

	bip44, err := FromSeedPhrase(cfg.KeyringMnemonic, cfg.KeyringPassword)
	if err != nil {
		logger.Error("failed to initialize bip44 keychain", "error", err)
		return
	}

	app := keychain.NewApp(keychain.Config{
		Logger:         logger,
		ChainID:        cfg.ChainID,
		GRPCURL:        cfg.GRPCURL,
		GRPCInsecure:   cfg.GRPCInsecure,
		DerivationPath: cfg.DerivationPath,
		Mnemonic:       cfg.Mnemonic,
		KeychainId:     cfg.KeychainId,
		GasLimit:       cfg.GasLimit,
		BatchTimeout:   cfg.BatchTimeout,
		BatchSize:      cfg.BatchSize,
	})

	app.SetKeyRequestHandler(func(w keychain.KeyResponseWriter, req *keychain.KeyRequest) {
		if req.KeyType != types.KeyType_KEY_TYPE_ECDSA_SECP256K1 {
			_ = w.Reject("unsupported key type")
			return
		}

		id, err := bigEndianBytesFromUint32(req.Id)
		if err != nil {
			logger.Error("failed to convert key id to big endian bytes", "error", err)
			_ = w.Reject("request ID is too large")
			return
		}

		pubKey, err := bip44.PublicKey(id)
		if err != nil {
			logger.Error("failed to get public key", "error", err)
			_ = w.Reject("failed to get public key")
			return
		}

		err = w.Fulfil(pubKey)
		if err != nil {
			logger.Error("failed to fulfil key request", "error", err)
		}
	})

	app.SetSignRequestHandler(func(w keychain.SignResponseWriter, req *keychain.SignRequest) {
		keyID, err := bigEndianBytesFromUint32(req.KeyId)
		if err != nil {
			logger.Error("failed to convert Key ID to big endian bytes", "error", err)
			_ = w.Reject("Key ID is too large")
			return
		}

		signature, err := bip44.Sign(keyID, req.DataForSigning)
		if err != nil {
			logger.Error("failed to sign message", "error", err)
			_ = w.Reject("failed to sign message")
			return
		}

		err = w.Fulfil(signature)
		if err != nil {
			logger.Error("failed to fulfil sign request", "error", err)
		}
	})

	if cfg.HttpAddr != "" {
		logger.Info("starting HTTP server", "addr", cfg.HttpAddr)
		http.HandleFunc("/healthcheck", func(w http.ResponseWriter, r *http.Request) {
			if app.ConnectionState() == connectivity.Ready {
				w.WriteHeader(http.StatusOK)
			} else {
				w.WriteHeader(http.StatusServiceUnavailable)
			}
		})
		go func() { _ = http.ListenAndServe(cfg.HttpAddr, nil) }()
	}

	err = app.Start(context.TODO())
	if err != nil {
		logger.Error("failed to start keychain app", "error", err)
	}
}

func bigEndianBytesFromUint32(n uint64) ([4]byte, error) {
	if n > 0xffffffff {
		return [4]byte{}, fmt.Errorf("number is too large to fit in 4 bytes")
	}

	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, uint32(n))
	return [4]byte(b), nil
}
