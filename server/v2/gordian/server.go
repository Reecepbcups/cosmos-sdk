package gordian

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"log/slog"
	"os"

	"cosmossdk.io/log"
	"cosmossdk.io/store/types"
	"github.com/libp2p/go-libp2p"
	libp2pcrypto "github.com/libp2p/go-libp2p/core/crypto"
	libp2ppeer "github.com/libp2p/go-libp2p/core/peer"
	"github.com/rollchains/gordian/gcrypto"
	"github.com/rollchains/gordian/tm/tmcodec/tmjson"
	"github.com/rollchains/gordian/tm/tmp2p/tmlibp2p"

	"github.com/spf13/viper"
	"golang.org/x/crypto/blake2b"

	"cosmossdk.io/core/transaction"
	serverv2 "cosmossdk.io/server/v2"
	"cosmossdk.io/server/v2/appmanager"
	"cosmossdk.io/server/v2/gordian/mempool"
)

var (
	p2pListenAddr = []string{"/ip4/0.0.0.0/tcp/9999"}
)

var _ serverv2.ServerModule = (*GordianServer[transaction.Tx])(nil)

type GordianServer[T transaction.Tx] struct {
	// Node *node.Node
	// App    *Consensus[T]
	sdklogger log.Logger // TODO: remove / wrap together
	slog      *slog.Logger

	config    Config
	cleanupFn func()
}

// App is an interface that represents an application in the Gordian server.
// It provides methods to access the app manager, logger, and store.
type App[T transaction.Tx] interface {
	// GetApp() *appmanager.AppManager[T]
	GetLogger() log.Logger
	GetStore() types.Store
}

func NewGordianServer[T transaction.Tx](
	app *appmanager.AppManager[T],
	store types.Store,
	logger log.Logger,
	cfg Config,
	txCodec transaction.Codec[T],
) *GordianServer[T] {
	logger = logger.With("module", "gordian-server")

	// create noop mempool
	mempool := mempool.NoOpMempool[T]{}

	// create consensus
	// TODO: need to do for gordian config setup
	consensus := NewConsensus[T](app, mempool, store, cfg, txCodec, logger)
	fmt.Println(consensus)

	return &GordianServer[T]{
		sdklogger: logger,
		slog:      slog.New(slog.NewTextHandler(os.Stdout, nil)).With("module", "gordian-server"),
		config:    cfg,
	}
}

// Name implements serverv2.ServerModule.
func (s *GordianServer[T]) Name() string {
	return "gordian"
}

// privKeyToFile saves the priv key to a file, then loads it into the config
func (s *GordianServer[T]) privKeyToFile(insecurePassword, file string) error {
	c, err := os.ReadFile(file)
	if err != nil {
		// save to a file so we have deterministic keys based on input generation
		privKey, err := libp2pKeyFromInsecurePassphrase(insecurePassword)
		if err != nil {
			return fmt.Errorf("failed to create libp2p key: %w", err)
		}

		bz, err := libp2pcrypto.MarshalPrivateKey(privKey)
		if err != nil {
			return fmt.Errorf("failed to marshal libp2p private key: %w", err)
		}

		// save to file
		if err := os.WriteFile(file, bz, 0644); err != nil {
			return fmt.Errorf("failed to write libp2p private key to file: %w", err)
		}
		c = bz // so we can use it to unmarshal below
	}

	s.config.PrivateKey, err = libp2pcrypto.UnmarshalPrivateKey(c)
	if err != nil {
		return fmt.Errorf("failed to unmarshal libp2p private key: %w", err)
	}

	return nil
}

// Start implements serverv2.ServerModule.
func (s *GordianServer[T]) Start(context.Context) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// l := gordianlog.GordianLoggerWrapper{Logger: }
	l := s.sdklogger
	l.Info("Starting Gordian server")

	listenAddrs := p2pListenAddr
	h, err := tmlibp2p.NewHost(
		ctx,
		tmlibp2p.HostOptions{
			Options: []libp2p.Option{
				// Not signing, so not setting libp2p.Identity -- allow system to generate a random ID.
				libp2p.ListenAddrStrings(listenAddrs...),

				// Ideally we would provide a way to prefer using a relayer circuit,
				// but for demo and prototyping this will be fine.
				libp2p.ForceReachabilityPublic(),
			},
		},
	)
	if err != nil {
		return fmt.Errorf("failed to create libp2p host: %w", err)
	}

	defer func() {
		if err := h.Close(); err != nil {
			l.Warn("Error closing libp2p host", "err", err)
		}
	}()

	if err := s.privKeyToFile("myPass", "p2pPriv.key"); err != nil {
		return fmt.Errorf("failed to save or load libp2p private key: %w", err)
	}

	id, err := libp2ppeer.IDFromPrivateKey(s.config.PrivateKey)
	if err != nil {
		return fmt.Errorf("failed to generate ID from libp2p private key: %w", err)
	}
	l.Info("Loaded libp2p private key", "id", id)

	reg := new(gcrypto.Registry)
	gcrypto.RegisterEd25519(reg)
	codec := tmjson.MarshalCodec{
		CryptoRegistry: reg,
	}
	conn, err := tmlibp2p.NewConnection(
		ctx,
		s.slog.With("sys", "libp2pconn"),
		h,
		codec,
	)
	if err != nil {
		return fmt.Errorf("failed to build libp2p connection: %w", err)
	}
	defer conn.Disconnect()

	return nil
}

// Stop implements serverv2.ServerModule.
func (s *GordianServer[T]) Stop(context.Context) error {
	defer s.cleanupFn()
	// if s.Node != nil {
	// 	return s.Node.Stop()
	// }
	return nil
}

func (s *GordianServer[T]) Config() (any, *viper.Viper) {
	v := viper.New()
	v.SetConfigFile("???") // TODO: where do we set this
	v.SetConfigName("config")
	v.SetConfigType("toml")
	v.ReadInConfig()
	return nil, nil
}

// from [cmd/gordian-echo/main.go]
// state v3 has lib p2p Idenity option, not in mirror
func libp2pKeyFromInsecurePassphrase(insecurePassphrase string) (libp2pcrypto.PrivKey, error) {
	bh, err := blake2b.New(ed25519.SeedSize, nil)
	if err != nil {
		return nil, err
	}
	bh.Write([]byte("gordian-echo:network|")) // .
	bh.Write([]byte(insecurePassphrase))
	seed := bh.Sum(nil)

	privKey := ed25519.NewKeyFromSeed(seed)

	priv, _, err := libp2pcrypto.KeyPairFromStdKey(&privKey)
	if err != nil {
		return nil, err
	}

	return priv, nil
}
