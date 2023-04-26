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
	"fmt"
	"regexp"
	"time"

	"github.com/xgfone/go-apiserver/log"
	"github.com/xgfone/go-binder"
)

// EmailMatcher is used to returns an matcher to check whether an email is matched.
var EmailMatcher func(senderMatcher, subjectMatcher string) (func(sender, subject string) (match bool), error) = matchEmail

func matchEmail(senderRegexp, subjectRegexp string) (match func(sender, subject string) bool, err error) {
	var senderRE *regexp.Regexp
	if senderRegexp != "" {
		senderRE, err = regexp.Compile(senderRegexp)
		if err != nil {
			return
		}
	}

	var subjectRE *regexp.Regexp
	if subjectRegexp != "" {
		subjectRE, err = regexp.Compile(subjectRegexp)
		if err != nil {
			return
		}
	}

	match = func(sender, subject string) bool {
		return (senderRE == nil || senderRE.MatchString(sender)) &&
			(subjectRE == nil || subjectRE.MatchString(subject))
	}
	return
}

func buildOrMatcher(matchers []matcher) (match func(sender, subject string) bool, err error) {
	_len := len(matchers)
	if _len == 0 {
		return nil, fmt.Errorf("missing the matcher")
	}

	matches := make([]func(sender, subject string) (match bool), _len)
	for i, m := range matchers {
		match, err := EmailMatcher(m.Sender, m.Subject)
		if err != nil {
			return nil, err
		}
		matches[i] = match
	}

	return func(sender, subject string) bool {
		for _, m := range matches {
			if m(sender, subject) {
				return true
			}
		}
		return false
	}, nil
}

var builders = make(map[string]HandlerBuilder, 8)

type matcher struct {
	Sender  string
	Subject string
}

func init() {
	RegisterHandlerBuilder(FilterAlarmedHandler().Type(), func(configs map[string]interface{}) (Handler, error) {
		return FilterAlarmedHandler(), nil
	})

	RegisterHandlerBuilder(FilterReadHandler().Type(), func(map[string]interface{}) (Handler, error) {
		return FilterReadHandler(), nil
	})

	RegisterHandlerBuilder(SetReadHandler(nil).Type(), func(configs map[string]interface{}) (Handler, error) {
		var config struct {
			Matchers []matcher
		}
		if err := binder.BindStructToMap(&config, "json", configs); err != nil {
			return nil, err
		}

		match, err := buildOrMatcher(config.Matchers)
		if err != nil {
			return nil, err
		}

		return SetReadHandler(match), nil
	})

	RegisterHandlerBuilder(MoveBoxHandler("", nil).Type(), func(configs map[string]interface{}) (Handler, error) {
		var config struct {
			Mailbox  string `validate:"required"`
			Matchers []matcher
		}
		if err := binder.BindStructToMap(&config, "json", configs); err != nil {
			return nil, err
		}

		match, err := buildOrMatcher(config.Matchers)
		if err != nil {
			return nil, err
		}

		return MoveBoxHandler(config.Mailbox, match), nil
	})
}

// GetHandlerBuilder returns the handler builder by the type.
func GetHandlerBuilder(_type string) HandlerBuilder { return builders[_type] }

// RegisterHandlerBuilder registers the handler builder.
func RegisterHandlerBuilder(_type string, build HandlerBuilder) {
	if _type == "" {
		panic("handler builder type must not be empty")
	}
	if build == nil {
		panic("handler builder must not be nil")
	}
	builders[_type] = build
}

// GetAllBuilderTypes returns the types of all the handler builders.
func GetAllBuilderTypes() (types []string) {
	types = make([]string, 0, len(builders))
	for _type := range builders {
		types = append(types, _type)
	}
	return
}

// BuildHandler builds a handler by the type and configs, and returns it.
func BuildHandler(_type string, configs map[string]interface{}) (Handler, error) {
	if build := GetHandlerBuilder(_type); build != nil {
		return build(configs)
	}
	return nil, fmt.Errorf("no handler buidler typed '%s'", _type)
}

// HandlerBuilder is used to build an email handler.
type HandlerBuilder func(configs map[string]interface{}) (Handler, error)

// Handler is used to process the email message.
type Handler interface {
	Type() string
	Handle(*Email) (next bool, err error)
}

type emailHandler struct {
	_type  string
	handle func(*Email) (next bool, err error)
}

func (h emailHandler) Type() string                  { return h._type }
func (h emailHandler) Handle(e *Email) (bool, error) { return h.handle(e) }

// NewHandler returns a new email handler.
func NewHandler(_type string, handle func(*Email) (next bool, err error)) Handler {
	return emailHandler{_type: _type, handle: handle}
}

// SetReadHandler returns an email handler to set the email to read.
func SetReadHandler(match func(sender, subject string) bool) Handler {
	return NewHandler("setread", func(e *Email) (next bool, err error) {
		if !e.IsRead() && match(e.Sender(), e.Subject) {
			err = e.SetRead()
			log.Info("set email to read", "mailbox", e.Mailbox(),
				"uid", e.uid, "sender", e.Sender(), "subject", e.Subject,
				"date", e.Date(), "err", err)
		}

		next = true
		return
	})
}

// FilterReadHandler returns an email handler to filter the read email.
func FilterReadHandler() Handler {
	return NewHandler("filterread", func(e *Email) (next bool, err error) {
		return !e.IsRead(), nil
	})
}

// MoveBoxHandler returns an email handler to move the matched email to other mailbox.
func MoveBoxHandler(mailbox string, match func(sender, subject string) bool) Handler {
	return NewHandler("movebox", func(e *Email) (next bool, err error) {
		srcbox := e.Mailbox()
		if match(e.Sender(), e.Subject) {
			err = e.Move(mailbox)
			log.Info("move email", "srcmailbox", srcbox, "newmailbox", mailbox,
				"uid", e.uid, "sender", e.Sender(), "subject", e.Subject,
				"date", e.Date(), "err", err)
		}

		next = true
		return
	})
}

// FilterAlarmedHandler returns an email handler to filter the alarmed email
// based on the memory.
func FilterAlarmedHandler() Handler {
	caches := make(map[string]struct{}, 256)
	return NewHandler("filteralarmed", func(e *Email) (next bool, err error) {
		key := fmt.Sprintf("%d_%s", e.UID(), e.Date().Format(time.RFC3339))
		_, ok := caches[key]
		if next = !ok; next {
			caches[key] = struct{}{}
		}
		return
	})
}
