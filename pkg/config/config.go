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

// Package config provides a configuration interface.
package config

import (
	"fmt"
	"time"

	"github.com/xgfone/emailmanager/pkg/controller"
	"github.com/xgfone/emailmanager/pkg/email"
	"github.com/xgfone/emailmanager/pkg/notice"
)

// Builder is the builder config to build a notifier or handler.
type Builder struct {
	Configs map[string]interface{}
	Type    string
}

// BuildNotifier builds a notifier.
func (b Builder) BuildNotifier() (notice.Notifier, error) {
	return notice.BuildNotifier(b.Type, b.Configs)
}

// BuildEmailHandler builds an email handler.
func (b Builder) BuildEmailHandler() (email.Handler, error) {
	return email.BuildHandler(b.Type, b.Configs)
}

// Email is the email config.
type Email struct {
	Address  string `validate:"required"`
	Username string `validate:"required"`
	Password string `validate:"required"`
	Number   uint32
	UseTLS   bool `json:"UseTls"`

	SkipTLSVerify bool `validate:"SkipTlsVerify"`
}

// ControllerOptoin converts itself to the controller option.
func (e Email) ControllerOptoin() controller.Option {
	return controller.EmailOption(e.Address, e.Username, e.Password, e.UseTLS, e.SkipTLSVerify, e.Number)
}

// Controller is the controller config.
type Controller struct {
	Delay    int64
	Timeout  int64
	Interval int64

	Email     Email
	Handlers  []Builder
	Notifiers []Builder
}

// Options converts itself to controller options.
func (c Controller) Options() ([]controller.Option, error) {
	options := make([]controller.Option, 0, 8)
	options = append(options, controller.DelayOption(time.Duration(c.Delay)*time.Second))
	options = append(options, controller.TimeoutOption(time.Duration(c.Timeout)*time.Second))
	options = append(options, controller.IntervalOption(time.Duration(c.Interval)*time.Second))
	options = append(options, c.Email.ControllerOptoin())

	handlers := make([]email.Handler, len(c.Handlers))
	for i, h := range c.Handlers {
		handler, err := h.BuildEmailHandler()
		if err != nil {
			return nil, fmt.Errorf("fail to build email handler '%s' for %s: %w", h.Type, c.Email.Address, err)
		}
		handlers[i] = handler
	}
	options = append(options, controller.EmailHandlerOption(handlers...))

	notifiers := make([]notice.Notifier, len(c.Notifiers))
	for i, n := range c.Notifiers {
		notifier, err := n.BuildNotifier()
		if err != nil {
			return nil, fmt.Errorf("fail to build notifier '%s' for %s: %w", n.Type, c.Email.Address, err)
		}
		notifiers[i] = notifier
	}
	options = append(options, controller.NotifierOption(notifiers...))

	return options, nil
}

// Controller creates a controller with itself.
func (c Controller) Controller() (*controller.Controller, error) {
	options, err := c.Options()
	if err != nil {
		return nil, fmt.Errorf("fail to build controller options for %s: %w", c.Email.Address, err)
	}
	return controller.NewController(options...)
}
