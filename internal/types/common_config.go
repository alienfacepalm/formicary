package types

import (
	"context"
	"crypto/sha256"
	"crypto/sha512"
	"fmt"
	"github.com/sirupsen/logrus"
	"github.com/xdg-go/scram"
	"os"
	"os/signal"
	"plexobject.com/formicary/internal/buildversion"
	"runtime"
	"syscall"
	"time"
)

var (
	// SHA256 constant
	SHA256 scram.HashGeneratorFcn = sha256.New
	// SHA512 constant
	SHA512 scram.HashGeneratorFcn = sha512.New
)

const httpPrefix = "http"
const httpsPrefix = "https"

// JobSchedulerLeaderTopic topic for leader election of scheduler as there can be only a single scheduler
const JobSchedulerLeaderTopic = "job-scheduler-leader-topic"

// JobExecutionLaunchTopic topic
const JobExecutionLaunchTopic = "job-execution-launch-topic"

// JobDefinitionLifecycleTopic topic for lifecycle
const JobDefinitionLifecycleTopic = "job-definition-lifecycle-topic"

// JobRequestLifecycleTopic topic for lifecycle
const JobRequestLifecycleTopic = "job-request-lifecycle-topic"

// JobExecutionLifecycleTopic topic for lifecycle
const JobExecutionLifecycleTopic = "job-execution-lifecycle-topic"

// TaskExecutionLifecycleTopic topic for task execution lifecycle
const TaskExecutionLifecycleTopic = "task-execution-lifecycle-topic"

// ContainerLifecycleTopic topic for container lifecycle
const ContainerLifecycleTopic = "container-lifecycle-topic"

// ForkJobTaskletTopic topic for fork-job
const ForkJobTaskletTopic = "fork-job-tasklet-topic"

// WaitForkJobTaskletTopic topic for wait-fork-job
const WaitForkJobTaskletTopic = "wait-fork-job-tasklet-topic"

// LogTopic topic for logs
const LogTopic = "log-topic"

// HealthErrorTopic topic for health errors
const HealthErrorTopic = "health-error-topic"

// RequestTopicPrefix topic for incoming requests
const RequestTopicPrefix = "ant-request"

// MessagingProvider defines enum for messaging provider
type MessagingProvider string

const (
	// RedisMessagingProvider uses redis
	RedisMessagingProvider MessagingProvider = "REDIS_MESSAGING"

	// PulsarMessagingProvider uses apache pulsar
	PulsarMessagingProvider MessagingProvider = "PULSAR_MESSAGING"

	// KafkaMessagingProvider uses apache kafka
	KafkaMessagingProvider MessagingProvider = "KAFKA_MESSAGING"
)

var listeningForStackTraceDumps = false

// CommonConfig -- common config between ant and server
type CommonConfig struct {
	ID                         string             `yaml:"id" mapstructure:"id"`
	UserAgent                  string             `yaml:"user_agent" mapstructure:"user_agent"`
	ProxyURL                   string             `yaml:"proxy_url" mapstructure:"proxy_url"`
	ExternalBaseURL            string             `yaml:"external_base_url" mapstructure:"external_base_url"`
	HTTPPort                   int                `yaml:"http_port" mapstructure:"http_port"`
	Pulsar                     PulsarConfig       `yaml:"pulsar" mapstructure:"pulsar"`
	Kafka                      KafkaConfig        `yaml:"kafka" mapstructure:"kafka"`
	S3                         S3Config           `yaml:"s3" mapstructure:"s3"`
	Redis                      RedisConfig        `yaml:"redis" mapstructure:"redis"`
	Auth                       AuthConfig         `yaml:"auth" mapstructure:"auth" env:"AUTH"`
	MessagingProvider          MessagingProvider  `yaml:"messaging_provider" mapstructure:"messaging_provider"`
	ContainerReaperInterval    time.Duration      `yaml:"container_reaper_interval" mapstructure:"container_reaper_interval"`
	MonitorInterval            time.Duration      `yaml:"monitor_interval" mapstructure:"monitor_interval"`
	MonitoringURLs             map[string]string  `yaml:"monitoring_urls" mapstructure:"monitoring_urls"`
	RegistrationInterval       time.Duration      `yaml:"registration_interval" mapstructure:"registration_interval"`
	MaxStreamingLogMessageSize int                `yaml:"max_streaming_log_message_size" mapstructure:"max_streaming_log_message_size" json:"max_streaming_log_message_size"`
	MaxJobTimeout              time.Duration      `yaml:"max_job_timeout" mapstructure:"max_job_timeout"`
	MaxTaskTimeout             time.Duration      `yaml:"max_task_timeout" mapstructure:"max_task_timeout"`
	RateLimitPerSecond         float64            `yaml:"rate_limit_sec" mapstructure:"rate_limit_sec" json:"rate_limit_sec"`
	ShuttingDown               bool               `yaml:"-" mapstructure:"-" json:"-"`
	Development                bool               `yaml:"development" mapstructure:"development" json:"development"`
	Version                    *buildversion.Info `yaml:"-" mapstructure:"-" json:"-"`
	Debug                      bool               `yaml:"debug"`
}

// PersistentTopic builds persistent topic
func PersistentTopic(provider MessagingProvider, tenant string, namespace string, name string) string {
	if provider == PulsarMessagingProvider {
		return fmt.Sprintf("persistent://%s/%s/formicary-%s", tenant, namespace, name)
	}
	return fmt.Sprintf("formicary-queue-%s", name)
}

// NonPersistentTopic builds non-persistent topic
func NonPersistentTopic(provider MessagingProvider, tenant string, namespace string, name string) string {
	if provider == PulsarMessagingProvider {
		return fmt.Sprintf("non-persistent://%s/%s/formicary-%s", tenant, namespace, name)
	}
	return fmt.Sprintf("formicary-pubsub-%s", name)
}

// GetExternalBaseURL url
func (c *CommonConfig) GetExternalBaseURL() string {
	if c.ExternalBaseURL != "" {
		return c.ExternalBaseURL
	} else if c.Auth.Secure {
		return fmt.Sprintf("%s://%s:%d", httpsPrefix, c.Auth.GithubCallbackHost, c.HTTPPort)
	} else {
		return fmt.Sprintf("%s://%s:%d", httpPrefix, c.Auth.GithubCallbackHost, c.HTTPPort)
	}
}

// AddSignalHandlerForStackTrace listen for signal to print stack trace
func (c *CommonConfig) AddSignalHandlerForStackTrace() {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGHUP)
	listeningForStackTraceDumps = true

	go func() {
		for sig := range signals {
			stacktrace := make([]byte, 8192)
			length := runtime.Stack(stacktrace, true)
			logrus.WithFields(logrus.Fields{
				"Component": "CommonConfig",
				"ID":        c.ID,
				"Signal":    sig,
			}).Warnf("dumping stack trace")
			fmt.Println(string(stacktrace[:length]))
		}
	}()
	logrus.WithFields(logrus.Fields{
		"Component": "CommonConfig",
		"ID":        c.ID,
		"Signal":    syscall.SIGHUP,
	}).Infof("adding signal handler to dump stack trace")
}

// AddSignalHandlerForShutdown listen for signal to shutdown cleanly
func (c *CommonConfig) AddSignalHandlerForShutdown(shutdownFunc context.CancelFunc) {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGQUIT, syscall.SIGTERM)

	countSignalsReceived := 0
	go func() {
		for sig := range signals {
			c.ShuttingDown = true
			if countSignalsReceived == 0 {
				shutdownFunc()
			}
			forced := countSignalsReceived > 1
			// notify only once
			if forced {
				logrus.WithFields(logrus.Fields{
					"Component":            "CommonConfig",
					"ID":                   c.ID,
					"Signal":               sig,
					"CountSignalsReceived": countSignalsReceived,
				}).Error("forced shutdown")
				os.Exit(0)
			} else {
				logrus.WithFields(logrus.Fields{
					"Component":            "CommonConfig",
					"ID":                   c.ID,
					"Signal":               sig,
					"CountSignalsReceived": countSignalsReceived,
				}).Warn("shutting down")
			}
			countSignalsReceived++
		}
	}()
}

// GetSource source
func (c *CommonConfig) GetSource() string {
	host, _ := os.Hostname()
	return fmt.Sprintf("%s@%s", c.ID, host)
}

// GetRegistrationTopic - registration topic
func (c *CommonConfig) GetRegistrationTopic() string {
	return NonPersistentTopic(
		c.MessagingProvider,
		c.Pulsar.TopicTenant,
		c.Pulsar.TopicNamespace,
		RegistrationTopic)
}

// GetContainerLifecycleTopic - container lifecycle topic
func (c *CommonConfig) GetContainerLifecycleTopic() string {
	return NonPersistentTopic(
		c.MessagingProvider,
		c.Pulsar.TopicTenant,
		c.Pulsar.TopicNamespace,
		ContainerLifecycleTopic)
}

// GetJobDefinitionLifecycleTopic topic
func (c *CommonConfig) GetJobDefinitionLifecycleTopic() string {
	return NonPersistentTopic(
		c.MessagingProvider,
		c.Pulsar.TopicTenant,
		c.Pulsar.TopicNamespace,
		JobDefinitionLifecycleTopic)
}

// GetJobRequestLifecycleTopic topic
func (c *CommonConfig) GetJobRequestLifecycleTopic() string {
	return NonPersistentTopic(
		c.MessagingProvider,
		c.Pulsar.TopicTenant,
		c.Pulsar.TopicNamespace,
		JobRequestLifecycleTopic)
}

// GetJobExecutionLifecycleTopic topic
func (c *CommonConfig) GetJobExecutionLifecycleTopic() string {
	return PersistentTopic(
		c.MessagingProvider,
		c.Pulsar.TopicTenant,
		c.Pulsar.TopicNamespace,
		JobExecutionLifecycleTopic)
}

// GetTaskExecutionLifecycleTopic topic
func (c *CommonConfig) GetTaskExecutionLifecycleTopic() string {
	return PersistentTopic(
		c.MessagingProvider,
		c.Pulsar.TopicTenant,
		c.Pulsar.TopicNamespace,
		TaskExecutionLifecycleTopic)
}

// GetForkJobTaskletTopic topic
func (c *CommonConfig) GetForkJobTaskletTopic() string {
	return PersistentTopic(
		c.MessagingProvider,
		c.Pulsar.TopicTenant,
		c.Pulsar.TopicNamespace,
		ForkJobTaskletTopic)
}

// GetWaitForkJobTaskletTopic topic
func (c *CommonConfig) GetWaitForkJobTaskletTopic() string {
	return PersistentTopic(
		c.MessagingProvider,
		c.Pulsar.TopicTenant,
		c.Pulsar.TopicNamespace,
		WaitForkJobTaskletTopic)
}

// GetLogTopic topic
func (c *CommonConfig) GetLogTopic() string {
	return NonPersistentTopic(
		c.MessagingProvider,
		c.Pulsar.TopicTenant,
		c.Pulsar.TopicNamespace,
		LogTopic)
}

// GetHealthErrorTopic topic
func (c *CommonConfig) GetHealthErrorTopic() string {
	return NonPersistentTopic(
		c.MessagingProvider,
		c.Pulsar.TopicTenant,
		c.Pulsar.TopicNamespace,
		HealthErrorTopic)
}

// GetRequestTopic request topic
func (c *CommonConfig) GetRequestTopic() string {
	return PersistentTopic(
		c.MessagingProvider,
		c.Pulsar.TopicTenant,
		c.Pulsar.TopicNamespace,
		RequestTopicPrefix+c.Pulsar.TopicSuffix)
}


// Validate - validates
func (c *AuthConfig) Validate() error {
	if c.Enabled {
		if c.JWTSecret == "" {
			return fmt.Errorf("jwt secret is not specified")
		}
		if c.GoogleClientID == "" && c.GithubClientID == "" {
			return fmt.Errorf("auth client_id is not specified for google or github")
		}
		if c.GoogleClientSecret == "" && c.GithubClientSecret == "" {
			return fmt.Errorf("auth client_secret is not specified for google or github")
		}
	}
	return nil
}

// Validate - validates
func (c *CommonConfig) Validate(_ []string) error {
	if c.MonitorInterval == 0 {
		c.MonitorInterval = 2 * time.Second
	}
	if c.ContainerReaperInterval == 0 {
		c.ContainerReaperInterval = 1 * time.Minute
	}
	if c.Redis.TTLMinutes == 0 {
		c.Redis.TTLMinutes = 5
	}
	if c.Redis.Port == 0 {
		c.Redis.Port = 6379
	}
	if c.RegistrationInterval == 0 {
		c.RegistrationInterval = 5 * time.Second
	}

	if c.MaxStreamingLogMessageSize == 0 {
		c.MaxStreamingLogMessageSize = 1024
	}

	// Note: Following config will limit the max runtime for a task with default value of about 1 hours
	if c.MaxTaskTimeout <= 0 {
		c.MaxTaskTimeout = 1 * time.Hour
	}
	// Note: Following config will limit the max runtime for a job with default value of about 3 hours
	if c.MaxJobTimeout <= 0 {
		c.MaxJobTimeout = 3 * time.Hour
	}

	if c.RateLimitPerSecond <= 0 {
		c.RateLimitPerSecond = 1
	}
	c.Kafka.clientID = c.ID

	if c.MessagingProvider == PulsarMessagingProvider {
		if err := c.Pulsar.Validate(); err != nil {
			return err
		}
	} else if c.MessagingProvider == KafkaMessagingProvider {
		if err := c.Kafka.Validate(); err != nil {
			return err
		}
	} else {
		if err := c.Redis.Validate(); err != nil {
			return err
		}
	}

	if err := c.S3.Validate(); err != nil {
		return err
	}

	if err := c.Auth.Validate(); err != nil {
		return err
	}

	if !listeningForStackTraceDumps {
		c.AddSignalHandlerForStackTrace()
	}
	return nil
}

