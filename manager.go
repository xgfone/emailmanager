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

package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"sync"
	"time"

	_ "github.com/xgfone/emailmanager/pkg/notice/feishu"

	"github.com/xgfone/emailmanager/pkg/config"
	"github.com/xgfone/emailmanager/pkg/controller"
	"github.com/xgfone/go-atexit"
	"github.com/xgfone/go-defaults"
)

func run(loader config.Loader) {
	m, err := newManager(loader)
	if err != nil {
		slog.Error("fail to new manager", "err", err)
		defaults.Exit(1)
	}

	m.Start(atexit.Context())
	atexit.Wait()
}

type ctrl struct {
	cancel     context.CancelFunc
	config     config.Controller
	controller *controller.Controller
}

func (c *ctrl) Run(ctx context.Context, interval time.Duration) {
	if c.cancel == nil {
		ctx, c.cancel = context.WithCancel(ctx)
		c.controller.Run(ctx, interval)
		slog.Info("controller has stopped")
	} else {
		slog.Warn("controller has been started")
	}
}

func (c *ctrl) Stop() {
	if c.cancel != nil {
		c.cancel()
		c.cancel = nil
	}
}

type manager struct {
	loader config.Loader

	lock    sync.RWMutex
	ctrls   map[string]*ctrl
	context context.Context
	cancel  context.CancelFunc
}

func newManager(loader config.Loader) (m *manager, err error) {
	m = &manager{loader: loader, ctrls: make(map[string]*ctrl, 4)}
	err = m.sync()
	return
}

func joinErrors(err1, err2 error) error {
	if err1 == nil {
		return err2
	}
	return errors.Join(err1, err2)
}

func (m *manager) sync() (err error) {
	controllers, err := m.loader.LoadController()
	if err != nil {
		return fmt.Errorf("fail to load config from loader: %w", err)
	}

	m.lock.Lock()
	defer m.lock.Unlock()

	for _, c := range controllers {
		if ctrl, ok := m.ctrls[c.Email.Address]; ok {
			if !reflect.DeepEqual(ctrl.config, c) {
				options, _err := c.Options()
				if _err != nil {
					_err = fmt.Errorf("fail to build controller options for %s: %w", c.Email.Address, _err)
					err = joinErrors(err, _err)
				} else {
					_err = ctrl.controller.Reconfigure(options...)
					if _err != nil {
						err = joinErrors(err, _err)
					}
				}
			}
		} else {
			if controller, _err := c.Controller(); _err != nil {
				err = joinErrors(err, _err)
			} else {
				m.addController(controller, c)
			}
		}
	}

	return
}

func (m *manager) addController(c *controller.Controller, config config.Controller) {
	ctrl := &ctrl{controller: c, config: config}
	m.ctrls[config.Email.Address] = ctrl
	if m.context != nil {
		go ctrl.Run(m.context, 0)
	}
}

func (m *manager) Start(ctx context.Context) {
	m.lock.Lock()
	defer m.lock.Unlock()

	if m.context == nil {
		m.context, m.cancel = context.WithCancel(ctx)
		for _, ctrl := range m.ctrls {
			go ctrl.Run(m.context, 0)
		}
	}
}
