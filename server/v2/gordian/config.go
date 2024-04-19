package gordian

import libp2pcrypto "github.com/libp2p/go-libp2p/core/crypto"

// Config is the configuration for the CometBFT application
type Config struct {
	// app.toml config options
	Name          string `mapstructure:"name" toml:"name"`
	Version       string `mapstructure:"version" toml:"version"`
	InitialHeight uint64 `mapstructure:"initial_height" toml:"initial_height"`
	// MinRetainBlocks uint64              `mapstructure:"min_retain_blocks" toml:"min_retain_blocks"`
	// IndexEvents     map[string]struct{} `mapstructure:"index_events" toml:"index_events"`
	// HaltHeight      uint64              `mapstructure:"halt_height" toml:"halt_height"`
	// HaltTime        uint64              `mapstructure:"halt_time" toml:"halt_time"`
	// end of app.toml config options

	// AddrPeerFilter types.PeerFilter // filter peers by address and port
	// IdPeerFilter   types.PeerFilter // filter peers by node ID

	// Transport  string `mapstructure:"transport" toml:"transport"`
	// Addr       string `mapstructure:"addr" toml:"addr"`
	// Standalone bool   `mapstructure:"standalone" toml:"standalone"`
	// Trace      bool   `mapstructure:"trace" toml:"trace"`

	// GrpcConfig grpc.Config

	// MempoolConfig
	// CmtConfig *cmtcfg.Config

	// Must be set by the application to grant authority to the consensus engine to send messages to the consensus module
	// ConsensusAuthority string

	// Probably just for testing
	PrivateKey libp2pcrypto.PrivKey `mapstructure:"private_key"`
	ID         string               `mapstructure:"id"`

	// Must be set by the application to grant authority to the consensus engine to send messages to the consensus module
	ConsensusAuthority string
}

func NewDefaultConfig() Config {
	return Config{
		Name:          "gordian",
		Version:       "0.0.1",
		InitialHeight: 1,
	}
}
