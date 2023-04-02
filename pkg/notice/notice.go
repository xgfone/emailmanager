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

// Package notice provides some common interfaces or functions.
package notice

import (
	"context"

	"github.com/xgfone/emailmanager/pkg/email"
)

// Email represents an email message.
type Email = email.Email

// Notifier is a notifier to notice someone.
type Notifier interface {
	Notify(ctx context.Context, emails ...Email) error
	String() string
}

// NewNotifier returns a new Notifier.
func NewNotifier(desc string, notify func(ctx context.Context, emails ...Email) error) Notifier {
	return notifier{desc: desc, send: notify}
}

type notifier struct {
	send func(ctx context.Context, emails ...Email) error
	desc string
}

func (n notifier) Notify(c context.Context, e ...Email) error { return n.send(c, e...) }
func (n notifier) String() string                             { return n.desc }
