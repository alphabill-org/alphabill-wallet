package types

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/alphabill-org/alphabill/logger"
	"github.com/alphabill-org/alphabill/observability"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
	"gopkg.in/yaml.v3"
)

type (
	LoggerFactory func(cfg *logger.LogConfiguration) (*slog.Logger, error)

	Observability interface {
		Tracer(name string, options ...trace.TracerOption) trace.Tracer
		TracerProvider() trace.TracerProvider
		Meter(name string, opts ...metric.MeterOption) metric.Meter
		PrometheusRegisterer() prometheus.Registerer
		Shutdown() error
		Logger() *slog.Logger
	}

	BaseConfiguration struct {
		// The Alphabill home directory
		HomeDir string
		// Configuration file URL. If it's relative, then it's relative from the HomeDir.
		CfgFile string
		// Logger configuration file URL.
		LogCfgFile string

		Metrics string
		Tracing string

		ConsoleWriter ConsoleWrapper

		Observe Observability
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
	keyTracing = "tracing"

	flagNameLoggerCfgFile = "logger-config"
	flagNameLogOutputFile = "log-file"
	flagNameLogLevel      = "log-level"
	flagNameLogFormat     = "log-format"
)

func (c *BaseConfiguration) AddConfigurationFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().StringVar(&c.HomeDir, keyHome, "", fmt.Sprintf("set the AB_HOME for this invocation (default is %s)", alphabillHomeDir()))
	cmd.PersistentFlags().StringVar(&c.CfgFile, keyConfig, "", fmt.Sprintf("config file URL (default is $AB_HOME/%s)", defaultConfigFile))
	cmd.PersistentFlags().StringVar(&c.Metrics, keyMetrics, "", "metrics exporter, disabled when not set. One of: stdout, prometheus")
	cmd.PersistentFlags().StringVar(&c.Tracing, keyTracing, "", "traces exporter, disabled when not set. One of: stdout, otlptracehttp, otlptracegrpc, zipkin")
	cmd.PersistentFlags().StringVar(&c.LogCfgFile, flagNameLoggerCfgFile, defaultLoggerConfigFile, "logger config file URL. Considered absolute if starts with '/'. Otherwise relative from $AB_HOME.")
	// do not set default values for these flags as then we can easily determine whether to load the value from cfg file or not
	cmd.PersistentFlags().String(flagNameLogOutputFile, "", "log file path or one of the special values: stdout, stderr, discard")
	cmd.PersistentFlags().String(flagNameLogLevel, "", "logging level, one of: DEBUG, INFO, WARN, ERROR")
	cmd.PersistentFlags().String(flagNameLogFormat, "", "log format, one of: text, json, console, ecs")
}

func (c *BaseConfiguration) InitConfigFileLocation() {
	// Home directory and config file are special configuration values as these are used for loading in rest of the configuration.
	// Handle these manually, before other configuration loaded with Viper.

	// Home dir is loaded from command line argument. If it's not set, then from env. If that's not set, then default is used.
	if c.HomeDir == "" {
		c.HomeDir = os.Getenv(envKey(keyHome))
		if c.HomeDir == "" {
			c.HomeDir = alphabillHomeDir()
		}
	}

	// Config file name is loaded from command line argument. If it's not set, then from env. If that's not set, then default is used.
	if c.CfgFile == "" {
		c.CfgFile = os.Getenv(envKey(keyConfig))
		if c.CfgFile == "" {
			c.CfgFile = defaultConfigFile
		}
	}
	if !filepath.IsAbs(c.CfgFile) {
		c.CfgFile = filepath.Join(c.HomeDir, c.CfgFile)
	}
}

/*
LoggerCfgFilename always returns non-empty filename - either the value
of the flag set by user or default cfg location.
The flag will be assigned the default filename (ie without path) if user
doesn't specify that flag.
*/
func (c *BaseConfiguration) LoggerCfgFilename() string {
	if !filepath.IsAbs(c.LogCfgFile) {
		return filepath.Join(c.HomeDir, c.LogCfgFile)
	}
	return c.LogCfgFile
}

func (c *BaseConfiguration) ConfigFileExists() bool {
	_, err := os.Stat(c.CfgFile)
	return err == nil
}

func (c *BaseConfiguration) DefaultRootGenesisDir() string {
	return filepath.Join(c.HomeDir, defaultRootChainDir)
}

/*
InitLogger creates Logger based on configuration flags in "cmd".
*/
func (c *BaseConfiguration) InitLogger(cmd *cobra.Command, loggerBuilder LoggerFactory) (*slog.Logger, error) {
	cfg := &logger.LogConfiguration{}

	loggerCfgFile := c.LoggerCfgFilename()
	if f, err := os.Open(loggerCfgFile); err != nil {
		defaultLoggerCfg := filepath.Join(c.HomeDir, defaultLoggerConfigFile)
		if !(errors.Is(err, os.ErrNotExist) && loggerCfgFile == defaultLoggerCfg) {
			return nil, fmt.Errorf("opening logger configuration file: %w", err)
		}
	} else {
		if err := yaml.NewDecoder(f).Decode(cfg); err != nil {
			return nil, fmt.Errorf("decoding logger configuration (%s): %w", loggerCfgFile, err)
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
		return nil, err
	}
	if err := getFlagValueIfSet(flagNameLogFormat, &cfg.Format); err != nil {
		return nil, err
	}
	if err := getFlagValueIfSet(flagNameLogOutputFile, &cfg.OutputPath); err != nil {
		return nil, err
	}

	// if it is a wallet cmd and logging on console then use "wallet formatting"
	// unless user has requested specific format by flag
	if forceWalletLogFormat(cmd, cfg.OutputPath) {
		cfg.Format = "wallet"
	}

	log, err := loggerBuilder(cfg)
	if err != nil {
		return nil, fmt.Errorf("building logger: %w", err)
	}
	return log, nil
}

// InitializeConfig reads in config file and ENV variables if set.
func (c *BaseConfiguration) InitializeConfig(cmd *cobra.Command) error {
	v := viper.New()

	c.InitConfigFileLocation()

	if c.ConfigFileExists() {
		v.SetConfigFile(c.CfgFile)
	}

	// Attempt to read the config file, gracefully ignoring errors
	// caused by a config file not being found. Return an error
	// if we cannot parse the config file.
	if err := v.ReadInConfig(); err != nil {
		// It's okay if there isn't a config file
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return err
		}
	}

	// When we bind flags to environment variables expect that the
	// environment variables are prefixed, e.g. a flag like --number
	// binds to an environment variable AB_NUMBER. This helps
	// avoid conflicts.
	v.SetEnvPrefix(envPrefix)

	// Bind to environment variables
	// Works great for simple config names, but needs help for names
	// like --favorite-color which we fix in the bindFlags function
	v.AutomaticEnv()

	// Bind the current command's flags to viper
	if err := bindFlags(cmd, v); err != nil {
		return fmt.Errorf("binding flags: %w", err)
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

// Bind each cobra flag to its associated viper configuration (config file and environment variable)
func bindFlags(cmd *cobra.Command, v *viper.Viper) error {
	var bindFlagErr []error
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		if f.Name == keyHome || f.Name == keyConfig {
			// "home" and "config" are special configuration values, handled separately.
			return
		}

		// Environment variables can't have dashes in them, so bind them to their equivalent
		// keys with underscores, e.g. --favorite-color to AB_FAVORITE_COLOR
		if strings.Contains(f.Name, "-") {
			envVarSuffix := strings.ToUpper(strings.ReplaceAll(f.Name, "-", "_"))
			if err := v.BindEnv(f.Name, fmt.Sprintf("%s_%s", envPrefix, envVarSuffix)); err != nil {
				bindFlagErr = append(bindFlagErr, fmt.Errorf("binding env to flag %q: %w", f.Name, err))
				return
			}
		}

		// Apply the viper config value to the flag when the flag is not set and viper has a value
		if !f.Changed && v.IsSet(f.Name) {
			val := v.Get(f.Name)
			if err := cmd.Flags().Set(f.Name, fmt.Sprintf("%v", val)); err != nil {
				bindFlagErr = append(bindFlagErr, fmt.Errorf("seting flag %q value: %w", f.Name, err))
				return
			}
		}
	})

	return errors.Join(bindFlagErr...)
}

type Factory interface {
	Logger(cfg *logger.LogConfiguration) (*slog.Logger, error)
	Observability(metrics, traces string) (observability.MeterAndTracer, error)
}

func InitializeConfig(cmd *cobra.Command, config *BaseConfiguration, obsF Factory) error {
	var errs []error

	if err := config.InitializeConfig(cmd); err != nil {
		errs = append(errs, fmt.Errorf("reading configuration: %w", err))
	}

	log, err := config.InitLogger(cmd, obsF.Logger)
	if err != nil {
		errs = append(errs, fmt.Errorf("initializing logger: %w", err))
	}

	observe, err := obsF.Observability(config.Metrics, config.Tracing)
	if err != nil {
		errs = append(errs, fmt.Errorf("initializing observability: %w", err))
	}
	if observe != nil && log != nil {
		config.Observe = observability.WithLogger(observe, log)
	}
	if config.ConsoleWriter == nil {
		config.ConsoleWriter = NewStdoutWriter()
	}
	return errors.Join(errs...)
}
