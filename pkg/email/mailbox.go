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

package email

import (
	"context"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
	"github.com/xgfone/go-generics/slices"
)

// Mailbox represents a mailbox information.
type Mailbox struct {
	Name        string
	HasChildren bool
}

// GetMailBoxes returns all the sub-mailboxes belonging on mailbox.
//
// If mailbox is "", use "*" instead.
//
// Example:
//
//	GetMailBoxes(context.Background(), "*", true)
//	GetMailBoxes(context.Background(), "Archives.*", true)
func GetMailBoxes(ctx context.Context, addr, username, password, mailbox string, tls bool) (mailboxes []Mailbox, err error) {
	if addr == "" {
		panic("mail server address must not be empty")
	}
	if username == "" {
		panic("email username must not be empty")
	}
	if password == "" {
		panic("email password must not be empty")
	}

	var imapClient *client.Client
	if tls {
		imapClient, err = client.DialTLS(addr, nil)
	} else {
		imapClient, err = client.Dial(addr)
	}
	if err != nil {
		return
	}
	defer imapClient.Terminate()

	err = imapClient.Login(username, password)
	if err != nil {
		return
	}
	defer imapClient.Logout()

	if mailbox == "" {
		mailbox = "*"
	}

	mbinfos := make(chan *imap.MailboxInfo, 10)
	done := make(chan error)
	go func() { done <- imapClient.List("", mailbox, mbinfos) }()

	for {
		select {
		case <-ctx.Done():
			err = ctx.Err()
			return

		case err = <-done:
			if err != nil {
				return
			}

		case mi, ok := <-mbinfos:
			if !ok {
				return
			}

			mailboxes = append(mailboxes, Mailbox{
				HasChildren: slices.Contains(mi.Attributes, imap.HasChildrenAttr),
				Name:        mi.Name,
			})
		}
	}
}
