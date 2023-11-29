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
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"sort"
	"time"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
	"github.com/emersion/go-message/charset"
)

// Predefine some mailboxes.
const (
	Inbox = "INBOX"
)

var (
	emailFetchItems1 = []imap.FetchItem{imap.FetchInternalDate, imap.FetchEnvelope, imap.FetchUid, imap.FetchFlags}
	emailFetchItems2 = []imap.FetchItem{imap.FetchInternalDate, imap.FetchEnvelope, imap.FetchUid, imap.FetchFlags, imap.FetchBody}

	emailStoreItem = imap.FormatFlagsOp(imap.AddFlags, true)
	emailReadFlags = []interface{}{imap.SeenFlag}
)

func init() {
	// Use charset.Reader to support the encodings, such as GB2312, GB18030, etc.
	imap.CharsetReader = charset.Reader
}

// Address represents an email address.
type Address struct {
	Name string
	Addr string
}

// FullAddress returns the full address with the name if name is not empty.
func (a Address) FullAddress() string {
	if a.Name == "" {
		return a.Addr
	}
	return fmt.Sprintf("%s<%s>", a.Name, a.Addr)
}

var _ json.Marshaler = Address{}

// MarshalJSON implements the interface json.Marshaler.
func (a Address) MarshalJSON() ([]byte, error) {
	return json.Marshal(a.FullAddress())
}

// Email represents an email message.
type Email struct {
	Froms        []Address
	Senders      []Address
	Subject      string
	SentDate     time.Time // The date when the message is sent.
	RecievedDate time.Time // The date when the mail server recieves the message.

	uid     uint32
	read    bool
	mailbox string
	client  *client.Client
}

func newEmail(client *client.Client, mailbox string, msg *imap.Message) (m Email) {
	m.Senders = make([]Address, len(msg.Envelope.Sender))
	for i, sender := range msg.Envelope.Sender {
		m.Senders[i] = Address{
			Name: sender.PersonalName,
			Addr: sender.Address(),
		}
	}

	m.Froms = make([]Address, len(msg.Envelope.From))
	for i, sender := range msg.Envelope.From {
		m.Froms[i] = Address{
			Name: sender.PersonalName,
			Addr: sender.Address(),
		}
	}

	m.Subject = msg.Envelope.Subject
	m.SentDate = msg.Envelope.Date
	m.RecievedDate = msg.InternalDate
	m.read = slices.Contains(msg.Flags, imap.SeenFlag)
	m.mailbox = mailbox
	m.client = client
	m.uid = msg.Uid
	return
}

var _ json.Marshaler = Email{}

// MarshalJSON implements the interface json.Marshaler.
func (m Email) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]interface{}{
		"Froms":   m.Froms,
		"Senders": m.Senders,
		"Subject": m.Subject,
		"IsRead":  m.IsRead(),
		"Mailbox": m.Mailbox(),
		"Date":    m.Date(),
	})
}

// Sender returns the email address of the first sender.
func (m Email) Sender() (sender string) {
	if len(m.Senders) > 0 {
		sender = m.Senders[0].Addr
	} else if len(m.Froms) > 0 {
		sender = m.Froms[0].Addr
	}
	return
}

// Date returns the date when the message is sent.
func (m Email) Date() (date time.Time) {
	if !m.SentDate.IsZero() {
		return m.SentDate
	}
	return m.RecievedDate
}

// UID returns the uid of the email.
func (m Email) UID() uint32 { return m.uid }

// IsRead reports whether the message has been read.
func (m Email) IsRead() bool { return m.read }

// Mailbox returns the current mailbox which the message is in.
func (m Email) Mailbox() string { return m.mailbox }

// SetRead marks the message to be read.
func (m *Email) SetRead() (err error) {
	if m.IsRead() {
		return
	}

	seqSet := new(imap.SeqSet)
	seqSet.AddNum(m.uid)
	err = m.client.UidStore(seqSet, emailStoreItem, emailReadFlags, nil)
	if err == nil {
		m.read = true
	}

	return
}

// Move moves the message to the given box.
func (m *Email) Move(box string) (err error) {
	if m.Mailbox() == box {
		return
	}

	seqSet := new(imap.SeqSet)
	seqSet.AddNum(m.uid)
	err = m.client.UidMove(seqSet, box)
	if err == nil {
		m.mailbox = box
	}

	return
}

// FetchEmails fetches the emails from the mailbox.
//
// If mailbox is eqial to "", use Inbox instead.
// If maxnum is equal 0, use 100 instead.
func FetchEmails(ctx context.Context, addr, username, password, mailbox string,
	tls bool, maxnum uint32, chains ...Handler) (emails []Email, err error) {
	return fetchEmails(ctx, addr, username, password, mailbox, tls, false, maxnum, chains...)
}

func fetchEmails(ctx context.Context, addr, username, password, mailbox string,
	tls, body bool, maxnum uint32, chains ...Handler) (emails []Email, err error) {

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
		mailbox = Inbox
	}

	mailboxStatus, err := imapClient.Select(mailbox, false)
	if err != nil {
		return
	}

	if maxnum <= 0 {
		maxnum = 100
	}

	var startid uint32
	stopid := mailboxStatus.Messages
	if stopid > maxnum {
		startid = stopid - maxnum - 1
	}

	done := make(chan error)
	emails = make([]Email, 0, maxnum)
	messages := make(chan *imap.Message, maxnum)

	defer func() {
		_emails := make([]Email, 0, len(emails))
		for i := range emails {
			if email := emails[i]; handleEmailMessage(&email, chains) {
				_emails = append(_emails, email)
			}
		}

		emails = _emails
		sort.SliceStable(emails, func(i, j int) bool {
			return emails[j].uid < emails[i].uid
		})
	}()

	go func() {
		fetchItems := emailFetchItems1
		if body {
			fetchItems = emailFetchItems2
		}

		seqset := new(imap.SeqSet)
		seqset.AddRange(startid, stopid)
		done <- imapClient.Fetch(seqset, fetchItems, messages)
	}()

	for {
		select {
		case <-ctx.Done():
			err = ctx.Err()
			return

		case err = <-done:
			if err != nil {
				return
			}

		case msg, ok := <-messages:
			if !ok {
				return
			}
			emails = append(emails, newEmail(imapClient, mailbox, msg))
		}
	}
}

func handleEmailMessage(e *Email, chains []Handler) bool {
	for _, h := range chains {
		next, err := h.Handle(e)
		if err != nil {
			slog.Error("fail to handle the email",
				"handler", h.Type(), "mailbox", e.mailbox, "uid", e.uid,
				"sender", e.Sender(), "subject", e.Subject, "err", err.Error())
		} else if !next {
			if !e.IsRead() {
				slog.Debug("ignore the email",
					"handler", h.Type(), "mailbox", e.mailbox, "uid", e.uid,
					"sender", e.Sender(), "subject", e.Subject)
			}
			return false
		}
	}

	return true
}
