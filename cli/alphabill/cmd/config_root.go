package cmd

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/alphabill-org/alphabill/pkg/logger"
)

type (
	LoggerFactory     func(cfg *logger.LogConfiguration) (*slog.Logger, error)
	baseConfiguration struct {
		// The Alphabill home directory
		HomeDir string
		// Configuration file URL. If it's relative, then it's relative from the HomeDir.
		CfgFile string
		// Logger configuration file URL.
		LogCfgFile string
		Metrics    bool

		Logger        *slog.Logger
		loggerBuilder LoggerFactory
	}
)

const (
	// The prefix for configuration keys inside environment.
	envPrefix = "AB"
	// The default name for config file.
	defaultConfigFile = "config.props"
	// the default alphabill directory.
	defaultAlphabillDir = ".alphabill"
	// The default logger configuration file name.
	defaultLoggerConfigFile = "logger-config.yaml"
	// The default rootchain directory
	defaultRootChainDir = "rootchain"
	// The configuration key for home directory.
	keyHome = "home"
	// The configuration key for config file name.
	keyConfig = "config"
	// Enables or disables metrics collection
	keyMetrics = "metrics"

	flagNameLoggerCfgFile = "logger-config"
	flagNameLogOutputFile = "log-file"
	flagNameLogLevel      = "log-level"
	flagNameLogFormat     = "log-format"
)

func (r *baseConfiguration) addConfigurationFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().StringVar(&r.HomeDir, keyHome, "", fmt.Sprintf("set the AB_HOME for this invocation (default is %s)", alphabillHomeDir()))
	cmd.PersistentFlags().StringVar(&r.CfgFile, keyConfig, "", fmt.Sprintf("config file URL (default is $AB_HOME/%s)", defaultConfigFile))
	cmd.PersistentFlags().BoolVar(&r.Metrics, keyMetrics, false, "Enables metrics collection.")

	cmd.PersistentFlags().StringVar(&r.LogCfgFile, flagNameLoggerCfgFile, defaultLoggerConfigFile, "logger config file URL. Considered absolute if starts with '/'. Otherwise relative from $AB_HOME.")
	// do not set default values for these flags as then we can easily determine whether to load the value from cfg file or not
	cmd.PersistentFlags().String(flagNameLogOutputFile, "", "log file path or one of the special values: stdout, stderr, discard")
	cmd.PersistentFlags().String(flagNameLogLevel, "", "logging level, one of: DEBUG, INFO, WARN, ERROR")
	cmd.PersistentFlags().String(flagNameLogFormat, "", "log format, one of: text, json, console, ecs")
}

func (r *baseConfiguration) initConfigFileLocation() {
	// Home directory and config file are special configuration values as these are used for loading in rest of the configuration.
	// Handle these manually, before other configuration loaded with Viper.

	// Home dir is loaded from command line argument. If it's not set, then from env. If that's not set, then default is used.
	if r.HomeDir == "" {
		r.HomeDir = os.Getenv(envKey(keyHome))
		if r.HomeDir == "" {
			r.HomeDir = alphabillHomeDir()
		}
	}

	// Config file name is loaded from command line argument. If it's not set, then from env. If that's not set, then default is used.
	if r.CfgFile == "" {
		r.CfgFile = os.Getenv(envKey(keyConfig))
		if r.CfgFile == "" {
			r.CfgFile = defaultConfigFile
		}
	}
	if !filepath.IsAbs(r.CfgFile) {
		r.CfgFile = filepath.Join(r.HomeDir, r.CfgFile)
	}
}

/*
LoggerCfgFilename always returns non empty filename - either the value
of the flag set by user or default cfg location.
The flag will be assigned the default filename (ie without path) if user
doesn't specify that flag.
*/
func (r *baseConfiguration) LoggerCfgFilename() string {
	if !filepath.IsAbs(r.LogCfgFile) {
		return filepath.Join(r.HomeDir, r.LogCfgFile)
	}
	return r.LogCfgFile
}

func (r *baseConfiguration) configFileExists() bool {
	_, err := os.Stat(r.CfgFile)
	return err == nil
}

func (r *baseConfiguration) defaultRootGenesisDir() string {
	return filepath.Join(r.HomeDir, defaultRootChainDir)
}

/*
initLogger initializes the Logger field so that from that point on it
contains valid logger. This method should be called only once.
*/
func (r *baseConfiguration) initLogger(cmd *cobra.Command) error {
	if r.Logger != nil {
		return fmt.Errorf("logger is already initialized")
	}

	cfg := &logger.LogConfiguration{}

	loggerCfgFile := r.LoggerCfgFilename()
	if f, err := os.Open(loggerCfgFile); err != nil {
		defaultLoggerCfg := filepath.Join(r.HomeDir, defaultLoggerConfigFile)
		if !(errors.Is(err, os.ErrNotExist) && loggerCfgFile == defaultLoggerCfg) {
			return fmt.Errorf("opening logger configuration file: %w", err)
		}
	} else {
		if err := yaml.NewDecoder(f).Decode(cfg); err != nil {
			return fmt.Errorf("decoding logger configuration (%s): %w", loggerCfgFile, err)
		}
	}

	getFlagValueIfSet := func(flagName string, value *string) error {
		if cmd.Flags().Changed(flagName) {
			var err error
			if *value, err = cmd.Flags().GetString(flagName); err != nil {
				return fmt.Errorf("failed to read %s flag value: %w", flagName, err)
			}
		}
		return nil
	}

	// flags override values loaded from cfg file.
	// NB! these flags mustn't have default values in Cobra cmd definition!
	if err := getFlagValueIfSet(flagNameLogLevel, &cfg.Level); err != nil {
		return err
	}
	if err := getFlagValueIfSet(flagNameLogFormat, &cfg.Format); err != nil {
		return err
	}
	if err := getFlagValueIfSet(flagNameLogOutputFile, &cfg.OutputPath); err != nil {
		return err
	}

	// if it is a wallet cmd and logging on console then use "wallet formatting"
	// unless user has requested specific format by flag
	if forceWalletLogFormat(cmd, cfg.OutputPath) {
		cfg.Format = "wallet"
	}

	var err error
	if r.Logger, err = r.loggerBuilder(cfg); err != nil {
		return fmt.Errorf("building logger: %w", err)
	}
	return nil
}

func forceWalletLogFormat(cmd *cobra.Command, outFilename string) bool {
	// if user has specified format flag respect that
	if cmd.Flags().Changed(flagNameLogFormat) {
		return false
	}

	switch outFilename {
	case "", "stdout", "stderr":
		for ; cmd != nil; cmd = cmd.Parent() {
			if cmd.Name() == "wallet" {
				return true
			}
		}
	}

	return false
}

func LoggerBuilder(cfg *logger.LogConfiguration) (*slog.Logger, error) {
	logger, err := logger.New(cfg)
	if err != nil {
		return nil, fmt.Errorf("creating logger: %w", err)
	}
	return logger, nil
}

func envKey(key string) string {
	return strings.ToUpper(envPrefix + "_" + key)
}

func alphabillHomeDir() string {
	dir, err := os.UserHomeDir()
	if err != nil {
		panic("default user home dir not defined: " + err.Error())
	}
	return filepath.Join(dir, defaultAlphabillDir)
}
