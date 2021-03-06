// Copyright (c) 2018 IoTeX
// This is an alpha (internal) release and is not suitable for production. This source code is provided 'as is' and no
// warranties are given as to title or non-infringement, merchantability or fitness for purpose and, to the extent
// permitted by law, all liability for your use of the code is disclaimed. This source code is governed by Apache
// License 2.0 that can be found in the LICENSE file.

package config

import (
	"flag"
	"os"
	"time"

	"github.com/pkg/errors"
	uconfig "go.uber.org/config"
	"google.golang.org/grpc/keepalive"

	"github.com/iotexproject/iotex-core/address"
	"github.com/iotexproject/iotex-core/crypto"
	"github.com/iotexproject/iotex-core/iotxaddress"
	"github.com/iotexproject/iotex-core/pkg/enc"
	"github.com/iotexproject/iotex-core/pkg/keypair"
)

// IMPORTANT: to define a config, add a field or a new config type to the existing config types. In addition, provide
// the default value in Default var.

func init() {
	flag.StringVar(&_overwritePath, "config-path", "", "Config path")
	flag.StringVar(&_secretPath, "secret-path", "", "Secret path")
	flag.StringVar(&_subChainPath, "sub-config-path", "", "Sub chain Config path")
}

var (
	// overwritePath is the path to the config file which overwrite default values
	_overwritePath string
	// secretPath is the path to the  config file store secret values
	_secretPath   string
	_subChainPath string
)

const (
	// DelegateType represents the delegate node type
	DelegateType = "delegate"
	// FullNodeType represents the full node type
	FullNodeType = "full_node"
	// LightweightType represents the lightweight type
	LightweightType = "lightweight"

	// RollDPoSScheme means randomized delegated proof of stake
	RollDPoSScheme = "ROLLDPOS"
	// StandaloneScheme means that the node creates a block periodically regardless of others (if there is any)
	StandaloneScheme = "STANDALONE"
	// NOOPScheme means that the node does not create only block
	NOOPScheme = "NOOP"
)

var (
	// Default is the default config
	Default = Config{
		NodeType: FullNodeType,
		Network: Network{
			Host: "127.0.0.1",
			Port: 4689,
			MsgLogsCleaningInterval:             2 * time.Second,
			MsgLogRetention:                     5 * time.Second,
			HealthCheckInterval:                 time.Second,
			SilentInterval:                      5 * time.Second,
			PeerMaintainerInterval:              time.Second,
			PeerForceDisconnectionRoundInterval: 0,
			AllowMultiConnsPerHost:              false,
			NumPeersLowerBound:                  5,
			NumPeersUpperBound:                  5,
			PingInterval:                        time.Second,
			RateLimitEnabled:                    false,
			RateLimitPerSec:                     10000,
			RateLimitWindowSize:                 60 * time.Second,
			BootstrapNodes:                      make([]string, 0),
			TLSEnabled:                          false,
			CACrtPath:                           "",
			PeerCrtPath:                         "",
			PeerKeyPath:                         "",
			KLClientParams:                      keepalive.ClientParameters{},
			KLServerParams:                      keepalive.ServerParameters{},
			KLPolicy:                            keepalive.EnforcementPolicy{},
			MaxMsgSize:                          10485760,
			PeerDiscovery:                       true,
			TopologyPath:                        "",
			TTL:                                 3,
		},
		Chain: Chain{
			ChainDBPath: "/tmp/chain.db",
			TrieDBPath:  "/tmp/trie.db",
			// TODO: set default chain ID to 1 after deprecating iotxaddress.ChainID
			ID:                      enc.MachineEndian.Uint32(iotxaddress.ChainID),
			ProducerPubKey:          keypair.EncodePublicKey(keypair.ZeroPublicKey),
			ProducerPrivKey:         keypair.EncodePrivateKey(keypair.ZeroPrivateKey),
			InMemTest:               false,
			GenesisActionsPath:      "",
			NumCandidates:           101,
			EnableFallBackToFreshDB: false,
		},
		ActPool: ActPool{
			MaxNumActsPerPool: 32000,
			MaxNumActsPerAcct: 2000,
			MaxNumActsToPick:  0,
		},
		Consensus: Consensus{
			Scheme: NOOPScheme,
			RollDPoS: RollDPoS{
				DelegateInterval:         10 * time.Second,
				ProposerInterval:         10 * time.Second,
				UnmatchedEventTTL:        3 * time.Second,
				UnmatchedEventInterval:   100 * time.Millisecond,
				RoundStartTTL:            10 * time.Second,
				AcceptProposeTTL:         time.Second,
				AcceptProposalEndorseTTL: time.Second,
				AcceptCommitEndorseTTL:   time.Second,
				Delay:             5 * time.Second,
				NumSubEpochs:      1,
				EventChanSize:     10000,
				NumDelegates:      21,
				EnableDummyBlock:  true,
				TimeBasedRotation: false,
			},
			BlockCreationInterval: 10 * time.Second,
		},
		BlockSync: BlockSync{
			Interval:   10 * time.Second,
			BufferSize: 16,
		},
		Dispatcher: Dispatcher{
			EventChanSize: 10000,
		},
		Explorer: Explorer{
			Enabled:                 false,
			IsTest:                  false,
			Port:                    14004,
			TpsWindow:               10,
			MaxTransferPayloadBytes: 1024,
		},
		System: System{
			HeartbeatInterval: 10 * time.Second,
			HTTPProfilingPort: 0,
			HTTPMetricsPort:   8080,
		},
		DB: DB{
			NumRetries: 3,
		},
	}

	// ErrInvalidCfg indicates the invalid config value
	ErrInvalidCfg = errors.New("invalid config value")

	// Validates is the collection config validation functions
	Validates = []Validate{
		ValidateKeyPair,
		ValidateConsensusScheme,
		ValidateRollDPoS,
		ValidateDispatcher,
		ValidateExplorer,
		ValidateNetwork,
		ValidateActPool,
		ValidateChain,
	}
)

// Network is the config struct for network package
type (
	Network struct {
		Host                    string        `yaml:"host"`
		Port                    int           `yaml:"port"`
		MsgLogsCleaningInterval time.Duration `yaml:"msgLogsCleaningInterval"`
		MsgLogRetention         time.Duration `yaml:"msgLogRetention"`
		HealthCheckInterval     time.Duration `yaml:"healthCheckInterval"`
		SilentInterval          time.Duration `yaml:"silentInterval"`
		PeerMaintainerInterval  time.Duration `yaml:"peerMaintainerInterval"`
		// Force disconnecting a random peer every given number of peer maintenance round
		PeerForceDisconnectionRoundInterval int                         `yaml:"peerForceDisconnectionRoundInterval"`
		AllowMultiConnsPerHost              bool                        `yaml:"allowMultiConnsPerHost"`
		NumPeersLowerBound                  uint                        `yaml:"numPeersLowerBound"`
		NumPeersUpperBound                  uint                        `yaml:"numPeersUpperBound"`
		PingInterval                        time.Duration               `yaml:"pingInterval"`
		RateLimitEnabled                    bool                        `yaml:"rateLimitEnabled"`
		RateLimitPerSec                     uint64                      `yaml:"rateLimitPerSec"`
		RateLimitWindowSize                 time.Duration               `yaml:"rateLimitWindowSize"`
		BootstrapNodes                      []string                    `yaml:"bootstrapNodes"`
		TLSEnabled                          bool                        `yaml:"tlsEnabled"`
		CACrtPath                           string                      `yaml:"caCrtPath"`
		PeerCrtPath                         string                      `yaml:"peerCrtPath"`
		PeerKeyPath                         string                      `yaml:"peerKeyPath"`
		KLClientParams                      keepalive.ClientParameters  `yaml:"klClientParams"`
		KLServerParams                      keepalive.ServerParameters  `yaml:"klServerParams"`
		KLPolicy                            keepalive.EnforcementPolicy `yaml:"klPolicy"`
		MaxMsgSize                          int                         `yaml:"maxMsgSize"`
		PeerDiscovery                       bool                        `yaml:"peerDiscovery"`
		TopologyPath                        string                      `yaml:"topologyPath"`
		TTL                                 int32                       `yaml:"ttl"`
	}

	// Chain is the config struct for blockchain package
	Chain struct {
		ChainDBPath string `yaml:"chainDBPath"`
		TrieDBPath  string `yaml:"trieDBPath"`

		ID              uint32 `yaml:"id"`
		ProducerPubKey  string `yaml:"producerPubKey"`
		ProducerPrivKey string `yaml:"producerPrivKey"`

		// InMemTest creates in-memory DB file for local testing
		InMemTest               bool   `yaml:"inMemTest"`
		GenesisActionsPath      string `yaml:"genesisActionsPath"`
		NumCandidates           uint   `yaml:"numCandidates"`
		EnableFallBackToFreshDB bool   `yaml:"enablefallbacktofreshdb"`
	}

	// Consensus is the config struct for consensus package
	Consensus struct {
		// There are three schemes that are supported
		Scheme                string        `yaml:"scheme"`
		RollDPoS              RollDPoS      `yaml:"rollDPoS"`
		BlockCreationInterval time.Duration `yaml:"blockCreationInterval"`
	}

	// BlockSync is the config struct for the BlockSync
	BlockSync struct {
		Interval   time.Duration `yaml:"interval"` // update duration
		BufferSize uint64        `yaml:"bufferSize"`
	}

	// RollDPoS is the config struct for RollDPoS consensus package
	RollDPoS struct {
		DelegateInterval         time.Duration `yaml:"delegateInterval"`
		ProposerInterval         time.Duration `yaml:"proposerInterval"`
		UnmatchedEventTTL        time.Duration `yaml:"unmatchedEventTTL"`
		UnmatchedEventInterval   time.Duration `yaml:"unmatchedEventInterval"`
		RoundStartTTL            time.Duration `yaml:"roundStartTTL"`
		AcceptProposeTTL         time.Duration `yaml:"acceptProposeTTL"`
		AcceptProposalEndorseTTL time.Duration `yaml:"acceptProposalEndorseTTL"`
		AcceptCommitEndorseTTL   time.Duration `yaml:"acceptCommitEndorseTTL"`
		Delay                    time.Duration `yaml:"delay"`
		NumSubEpochs             uint          `yaml:"numSubEpochs"`
		EventChanSize            uint          `yaml:"eventChanSize"`
		NumDelegates             uint          `yaml:"numDelegates"`
		EnableDummyBlock         bool          `yaml:"enableDummyBlock"`
		TimeBasedRotation        bool          `yaml:"timeBasedRotation"`
	}

	// Dispatcher is the dispatcher config
	Dispatcher struct {
		EventChanSize uint `yaml:"eventChanSize"`
	}

	// Explorer is the explorer service config
	Explorer struct {
		Enabled   bool `yaml:"enabled"`
		IsTest    bool `yaml:"isTest"`
		Port      int  `yaml:"addr"`
		TpsWindow int  `yaml:"tpsWindow"`
		// MaxTransferPayloadBytes limits how many bytes a playload can contain at most
		MaxTransferPayloadBytes uint64 `yaml:"maxTransferPayloadBytes"`
	}

	// System is the system config
	System struct {
		HeartbeatInterval time.Duration `yaml:"heartbeatInterval"`
		// HTTPProfilingPort is the port number to access golang performance profiling data of a blockchain node. It is
		// 0 by default, meaning performance profiling has been disabled
		HTTPProfilingPort int `yaml:"httpProfilingPort"`
		HTTPMetricsPort   int `yaml:"httpMetricsPort"`
	}

	// ActPool is the actpool config
	ActPool struct {
		// MaxNumActsPerPool indicates maximum number of actions the whole actpool can hold
		MaxNumActsPerPool uint64 `yaml:"maxNumActsPerPool"`
		// MaxNumActsPerAcct indicates maximum number of actions an account queue can hold
		MaxNumActsPerAcct uint64 `yaml:"maxNumActsPerAcct"`
		// MaxNumActsToPick indicates maximum number of actions to pick to mint a block. Default is 0, which means no
		// limit on the number of actions to pick.
		MaxNumActsToPick uint64 `yaml:"maxNumActsToPick"`
	}

	// DB is the blotDB config
	DB struct {
		// NumRetries is the number of retries
		NumRetries uint8 `yaml:"numRetries"`
		// RDS is the config fot rds
		RDS RDS `yaml:"RDS"`
	}

	// RDS is the cloud rds config
	RDS struct {
		// AwsRDSEndpoint is the endpoint of aws rds
		AwsRDSEndpoint string `yaml:"awsRDSEndpoint"`
		// AwsRDSPort is the port of aws rds
		AwsRDSPort uint64 `yaml:"awsRDSPort"`
		// AwsRDSUser is the user to access aws rds
		AwsRDSUser string `yaml:"awsRDSUser"`
		// AwsPass is the pass to access aws rds
		AwsPass string `yaml:"awsPass"`
		// AwsDBName is the db name of aws rds
		AwsDBName string `yaml:"awsDBName"`
	}

	// Config is the root config struct, each package's config should be put as its sub struct
	Config struct {
		NodeType   string     `yaml:"nodeType"`
		Network    Network    `yaml:"network"`
		Chain      Chain      `yaml:"chain"`
		ActPool    ActPool    `yaml:"actPool"`
		Consensus  Consensus  `yaml:"consensus"`
		BlockSync  BlockSync  `yaml:"blockSync"`
		Dispatcher Dispatcher `yaml:"dispatcher"`
		Explorer   Explorer   `yaml:"explorer"`
		System     System     `yaml:"system"`
		DB         DB         `yaml:"db"`
	}

	// Validate is the interface of validating the config
	Validate func(*Config) error
)

// New creates a config instance. It first loads the default configs. If the config path is not empty, it will read from
// the file and override the default configs. By default, it will apply all validation functions. To bypass validation,
// use DoNotValidate instead.
func New(validates ...Validate) (*Config, error) {
	opts := make([]uconfig.YAMLOption, 0)
	opts = append(opts, uconfig.Static(Default))
	opts = append(opts, uconfig.Expand(os.LookupEnv))
	if _overwritePath != "" {
		opts = append(opts, uconfig.File(_overwritePath))
	}
	if _secretPath != "" {
		opts = append(opts, uconfig.File(_secretPath))
	}
	yaml, err := uconfig.NewYAML(opts...)
	if err != nil {
		return nil, errors.Wrap(err, "failed to init config")
	}

	var cfg Config
	if err := yaml.Get(uconfig.Root).Populate(&cfg); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal YAML config to struct")
	}

	// By default, the config needs to pass all the validation
	if len(validates) == 0 {
		validates = Validates
	}
	for _, validate := range validates {
		if err := validate(&cfg); err != nil {
			return nil, errors.Wrap(err, "failed to validate config")
		}
	}
	return &cfg, nil
}

// NewSub create config for sub chain.
func NewSub(validates ...Validate) (*Config, error) {
	if _subChainPath == "" {
		return nil, nil
	}
	opts := make([]uconfig.YAMLOption, 0)
	opts = append(opts, uconfig.Static(Default))
	opts = append(opts, uconfig.Expand(os.LookupEnv))
	opts = append(opts, uconfig.File(_subChainPath))
	if _secretPath != "" {
		opts = append(opts, uconfig.File(_secretPath))
	}
	yaml, err := uconfig.NewYAML(opts...)
	if err != nil {
		return nil, errors.Wrap(err, "failed to init config")
	}

	var cfg Config
	if err := yaml.Get(uconfig.Root).Populate(&cfg); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal YAML config to struct")
	}

	// By default, the config needs to pass all the validation
	if len(validates) == 0 {
		validates = Validates
	}
	for _, validate := range validates {
		if err := validate(&cfg); err != nil {
			return nil, errors.Wrap(err, "failed to validate config")
		}
	}
	return &cfg, nil
}

// IsDelegate returns true if the node type is Delegate
func (cfg *Config) IsDelegate() bool {
	return cfg.NodeType == DelegateType
}

// IsFullnode returns true if the node type is Fullnode
func (cfg *Config) IsFullnode() bool {
	return cfg.NodeType == FullNodeType
}

// IsLightweight returns true if the node type is Lightweight
func (cfg *Config) IsLightweight() bool {
	return cfg.NodeType == LightweightType
}

// BlockchainAddress returns the address derived from the configured chain ID and public key
func (cfg *Config) BlockchainAddress() (address.Address, error) {
	pk, err := keypair.DecodePublicKey(cfg.Chain.ProducerPubKey)
	if err != nil {
		return nil, errors.Wrapf(err, "error when decoding public key %s", cfg.Chain.ProducerPubKey)
	}
	pkHash := keypair.HashPubKey(pk)
	return address.New(cfg.Chain.ID, pkHash[:]), nil
}

// KeyPair returns the decoded public and private key pair
func (cfg *Config) KeyPair() (keypair.PublicKey, keypair.PrivateKey, error) {
	pk, err := keypair.DecodePublicKey(cfg.Chain.ProducerPubKey)
	if err != nil {
		return keypair.ZeroPublicKey,
			keypair.ZeroPrivateKey,
			errors.Wrapf(err, "error when decoding public key %s", cfg.Chain.ProducerPubKey)
	}
	sk, err := keypair.DecodePrivateKey(cfg.Chain.ProducerPrivKey)
	if err != nil {
		return keypair.ZeroPublicKey,
			keypair.ZeroPrivateKey,
			errors.Wrapf(err, "error when decoding private key %s", cfg.Chain.ProducerPrivKey)
	}
	return pk, sk, nil
}

// ValidateKeyPair validates the block producer address
func ValidateKeyPair(cfg *Config) error {
	priKey, err := keypair.DecodePrivateKey(cfg.Chain.ProducerPrivKey)
	if err != nil {
		return err
	}
	pubKey, err := keypair.DecodePublicKey(cfg.Chain.ProducerPubKey)
	if err != nil {
		return err
	}
	// Validate producer pubkey and prikey by signing a dummy message and verify it
	validationMsg := "connecting the physical world block by block"
	sig := crypto.EC283.Sign(priKey, []byte(validationMsg))
	if !crypto.EC283.Verify(pubKey, []byte(validationMsg), sig) {
		return errors.Wrap(ErrInvalidCfg, "block producer has unmatched pubkey and prikey")
	}
	return nil
}

// ValidateChain validates the chain configure
func ValidateChain(cfg *Config) error {
	if cfg.Chain.NumCandidates <= 0 {
		return errors.Wrapf(ErrInvalidCfg, "candidate number should be greater than 0")
	}
	if cfg.Consensus.Scheme == RollDPoSScheme && cfg.Chain.NumCandidates < cfg.Consensus.RollDPoS.NumDelegates {
		return errors.Wrapf(ErrInvalidCfg, "candidate number should be greater than or equal to delegate number")
	}
	return nil
}

// ValidateConsensusScheme validates the if scheme and node type match
func ValidateConsensusScheme(cfg *Config) error {
	switch cfg.NodeType {
	case DelegateType:
	case FullNodeType:
		if cfg.Consensus.Scheme != NOOPScheme {
			return errors.Wrap(ErrInvalidCfg, "consensus scheme of fullnode should be NOOP")
		}
	case LightweightType:
		if cfg.Consensus.Scheme != NOOPScheme {
			return errors.Wrap(ErrInvalidCfg, "consensus scheme of lightweight node should be NOOP")
		}
	default:
		return errors.Wrapf(ErrInvalidCfg, "unknown node type %s", cfg.NodeType)
	}
	return nil
}

// ValidateDispatcher validates the dispatcher configs
func ValidateDispatcher(cfg *Config) error {
	if cfg.Dispatcher.EventChanSize <= 0 {
		return errors.Wrap(ErrInvalidCfg, "dispatcher event chan size should be greater than 0")
	}
	return nil
}

// ValidateRollDPoS validates the roll-DPoS configs
func ValidateRollDPoS(cfg *Config) error {
	if cfg.Consensus.Scheme == RollDPoSScheme && cfg.Consensus.RollDPoS.EventChanSize <= 0 {
		return errors.Wrap(ErrInvalidCfg, "roll-DPoS event chan size should be greater than 0")
	}
	if cfg.Consensus.Scheme == RollDPoSScheme && cfg.Consensus.RollDPoS.NumDelegates <= 0 {
		return errors.Wrap(ErrInvalidCfg, "roll-DPoS event delegate number should be greater than 0")
	}
	if cfg.Consensus.Scheme == RollDPoSScheme &&
		cfg.Consensus.RollDPoS.EnableDummyBlock &&
		cfg.Consensus.RollDPoS.TimeBasedRotation {
		return errors.Wrap(ErrInvalidCfg, "roll-DPoS should enable dummy block when doing time based rotation")
	}
	return nil
}

// ValidateExplorer validates the explorer configs
func ValidateExplorer(cfg *Config) error {
	if cfg.Explorer.Enabled && cfg.Explorer.TpsWindow <= 0 {
		return errors.Wrap(ErrInvalidCfg, "tps window is not a positive integer when the explorer is enabled")
	}
	return nil
}

// ValidateNetwork validates the network configs
func ValidateNetwork(cfg *Config) error {
	if !cfg.Network.PeerDiscovery && cfg.Network.TopologyPath == "" {
		return errors.Wrap(ErrInvalidCfg, "either peer discover should be enabled or a topology should be given")
	}
	return nil
}

// ValidateActPool validates the given config
func ValidateActPool(cfg *Config) error {
	maxNumActPerPool := cfg.ActPool.MaxNumActsPerPool
	maxNumActPerAcct := cfg.ActPool.MaxNumActsPerAcct
	if maxNumActPerPool <= 0 || maxNumActPerAcct <= 0 {
		return errors.Wrap(
			ErrInvalidCfg,
			"maximum number of actions per pool or per account cannot be zero or negative",
		)
	}
	if maxNumActPerPool < maxNumActPerAcct {
		return errors.Wrap(
			ErrInvalidCfg,
			"maximum number of actions per pool cannot be less than maximum number of actions per account",
		)
	}
	return nil
}

// DoNotValidate validates the given config
func DoNotValidate(cfg *Config) error { return nil }
