package plugin

import (
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

// WithLogger returns an Option which configures a manager
// to use logger.
func WithLogger(logger *log.Logger) Option {
	return func(cfg *managerConfig) {
		cfg.logger = logger
	}
}

// WithHandshakeTimeout returns an Option which
// configures a manager to timeout plugins if handshake is not
// completed within d.
func WithHandshakeTimeout(d time.Duration) Option {
	return func(cfg *managerConfig) {
		cfg.handshakeTimeout = d
	}
}

// WithHealthTimeout returns an Option which
// configures a manager to timeout if plugins do not respond
// to health checks within d.
func WithHealthTimeout(d time.Duration) Option {
	return func(cfg *managerConfig) {
		cfg.healthCheckTicker = d
	}
}

// WithHealthRetries returns an Option which
// configures a manager to try to assert a plugin's health
// up to 1 + retries before giving up.
func WithHealthRetries(retries int) Option {
	return func(cfg *managerConfig) {
		cfg.healthRetries = retries
	}
}

// WithLocker returns an Option that configures
// the manager resources sync.Locker to be the given
// locker.
func WithLocker(locker sync.Locker) Option {
	return func(cfg *managerConfig) {
		cfg.locker = locker
	}
}
