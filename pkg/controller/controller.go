// Copyright 2023 xgfone
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package controller provides a controller to control the check and notice
// of the new emails.
package controller

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/xgfone/emailmanager/pkg/email"
	"github.com/xgfone/emailmanager/pkg/notice"
	"github.com/xgfone/go-defaults"
)

// EmailOption returns an option about email.
//
// Required: addr, username, password.
// Optional: tls, num(default: 100).
func EmailOption(addr, username, password string, enableTLS, skipTLSVerify bool, num uint32) Option {
	if num <= 0 {
		num = 100
	}

	var tlsconf *tls.Config
	if enableTLS {
		tlsconf = &tls.Config{InsecureSkipVerify: skipTLSVerify}
	}

	return func(c *config) {
		c.Email = emailConfig{
			Num:      num,
			Addr:     addr,
			Username: username,
			Password: password,
			TLSConf:  tlsconf,
		}
	}
}

// EmailHandlerOption returns an option about email handler, which will append the handler.
func EmailHandlerOption(handlers ...email.Handler) Option {
	return func(c *config) {
		c.Handlers = append([]email.Handler{}, handlers...)
	}
}

// NotifierOption returns an option about notifier, which will append the notifier.
//
// The notifier will be tries to send the notice in turn until someone does it successfully.
func NotifierOption(notifiers ...notice.Notifier) Option {
	return func(c *config) {
		c.Notifiers = append([]notice.Notifier{}, notifiers...)
	}
}

// DelayOption returns a delay option.
func DelayOption(delay time.Duration) Option {
	return func(c *config) { c.Delay = delay }
}

// TimeoutOption returns a timeout option.
func TimeoutOption(timeout time.Duration) Option {
	return func(c *config) { c.Timeout = timeout }
}

// IntervalOption returns a interval option.
func IntervalOption(interval time.Duration) Option {
	return func(c *config) { c.Interval = interval }
}

type emailConfig struct {
	Num      uint32
	Addr     string
	Username string
	Password string
	TLSConf  *tls.Config
}

func (c *emailConfig) check() error {
	if c.Addr == "" || c.Username == "" || c.Password == "" {
		return fmt.Errorf("email is not configured")
	}
	if c.Num <= 0 {
		c.Num = 100
	}
	return nil
}

type config struct {
	// Common
	Delay    time.Duration
	Timeout  time.Duration
	Interval time.Duration

	// Email
	Email    emailConfig
	Handlers []email.Handler

	// Notifiers
	Notifiers []notice.Notifier
}

func (c *config) reconfigure(options ...Option) error {
	var new config
	for _, option := range options {
		option(&new)
	}

	c.merge(new)
	return c.Email.check()
}

func (c *config) merge(new config) {
	if new.Delay > 0 {
		c.Delay = new.Delay
	}
	if new.Timeout > 0 {
		c.Timeout = new.Timeout
	}

	var empty emailConfig
	if new.Email != empty {
		c.Email = new.Email
	}

	if new.Handlers != nil {
		c.Handlers = new.Handlers
	}

	if new.Notifiers != nil {
		c.Notifiers = new.Notifiers
	}
}

// Option is used to configure the controller.
type Option func(*config)

// Controller is used to control the check and notice of the new emails.
type Controller struct {
	config atomic.Value
}

// NewController returns a new controller.
func NewController(options ...Option) (*Controller, error) {
	var config config
	if err := config.reconfigure(options...); err != nil {
		return nil, err
	}

	c := new(Controller)
	c.saveConfig(config)
	return c, nil
}

func (c *Controller) loadConfig() config       { return c.config.Load().(config) }
func (c *Controller) saveConfig(config config) { c.config.Store(config) }

// Reconfigure reconfigures the controller.
func (c *Controller) Reconfigure(options ...Option) (err error) {
	config := c.loadConfig()
	if err = config.reconfigure(options...); err == nil {
		c.saveConfig(config)
	}
	return
}

// Run runs until ctx is done.
func (c *Controller) Run(ctx context.Context, interval time.Duration) {
	if !c.firstRun(ctx) {
		return
	}

	cinterval := c.loadConfig().Interval
	if cinterval <= 0 {
		if cinterval = interval; cinterval <= 0 {
			cinterval = time.Minute * 15
		}
	}

	ticker := time.NewTicker(cinterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case <-ticker.C:
			c.CheckEmails(ctx)
		}
	}
}

func (c *Controller) firstRun(ctx context.Context) (next bool) {
	config := c.loadConfig()
	if config.Delay > 0 {
		timer := time.NewTimer(config.Delay)
		select {
		case <-timer.C:
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return
		}
	}
	c.CheckEmails(ctx)
	return true
}

// CheckEmails checks all the emails immediately.
func (c *Controller) CheckEmails(ctx context.Context) {
	for c.checkEmails(ctx) {
	}
}

func (c *Controller) checkEmails(ctx context.Context) (goon bool) {
	defer defaults.Recover(ctx)
	defer slog.Info("end to check the emails")
	slog.Info("start to check the emails")

	config := c.loadConfig()
	if config.Timeout > 0 {
		var cancel func()
		ctx, cancel = context.WithTimeout(ctx, config.Timeout)
		defer cancel()
	}

	emails, goon, err := email.FetchEmails(ctx, config.Email.Addr,
		config.Email.Username, config.Email.Password, email.Inbox,
		config.Email.TLSConf, config.Email.Num, config.Handlers...)
	if err != nil {
		slog.Error("fail to fetch emails", "addr", config.Email.Addr,
			"email", config.Email.Username, "mailbox", email.Inbox, "err", err)
		return
	} else if len(emails) == 0 {
		slog.Debug("no emails to be sent")
		return
	}

	for _, notifier := range config.Notifiers {
		if err := notifier.Notify(ctx, emails...); err != nil {
			slog.Error("fail to send notice", "email", config.Email.Username,
				"notifier", notifier.String(), "err", err)
		} else {
			slog.Info("send new email notice", "email", config.Email.Username,
				"notifier", notifier.String())
			break
		}
	}

	return
}
