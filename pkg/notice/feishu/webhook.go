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

// Package feishu provides a function to send the message notice by feishu webhook.
package feishu

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/xgfone/emailmanager/pkg/notice"
	"github.com/xgfone/go-binder"
	"github.com/xgfone/go-structs"
)

const urlprefix = "https://open.feishu.cn/open-apis/bot/v2/hook/"

func init() {
	notice.RegisterNotifierBuilder("feishuwebhook", func(configs map[string]interface{}) (notice.Notifier, error) {
		var config WebhookConfig
		if err := binder.BindStructToMap(&config, "json", configs); err != nil {
			return nil, err
		}
		if err := structs.Reflect(nil, config); err != nil {
			return nil, err
		}
		return NewWebhookNotifier(config.GroupID, config.Secret), nil
	})
}

// WebhookConfig is the webhook config.
type WebhookConfig struct {
	GroupID string `validate:"required" json:"GroupId"`
	Secret  string
}

// NewWebhookNotifier returns a notifier based on feishu webhook.
func NewWebhookNotifier(groupID, secret string) notice.Notifier {
	if groupID == "" {
		panic("NewWebhookNotifier: groupID must not be empty")
	}

	desc := fmt.Sprintf("FeiShuWebhook(groupid=%s)", groupID)
	return notice.NewNotifier(desc, func(ctx context.Context, emails ...notice.Email) error {
		return SendWebhook(ctx, groupID, secret, emails...)
	})
}

// SendWebhook sends the webhook message notice.
func SendWebhook(ctx context.Context, groupID, secret string, emails ...notice.Email) (err error) {
	if len(emails) == 0 {
		return
	}

	timestamp := fmt.Sprint(time.Now().Unix())
	signature, err := genFeishuSign(secret, timestamp)
	if err != nil {
		return
	}

	contents := make([]string, 1, len(emails)+1)
	contents[0] = fmt.Sprintf("您有%d封未读邮件:", len(emails))
	for i, email := range emails {
		if i > 10 {
			contents = append(contents, "......")
			break
		}
		contents = append(contents, fmt.Sprintf("%d. %s(%s)", i+1, email.Subject, email.Sender()))
	}
	content := strings.Join(contents, "\n")

	body := bytes.NewBuffer(make([]byte, 0, 512))
	err = json.NewEncoder(body).Encode(map[string]interface{}{
		"sign":      signature,
		"timestamp": timestamp,
		"msg_type":  "text",
		"content":   map[string]interface{}{"text": content},
	})
	if err != nil {
		return
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, urlprefix+groupID, body)
	if err != nil {
		return
	}

	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	var result struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return
	} else if result.Code != 0 {
		return fmt.Errorf("code=%d, msg=%s", result.Code, result.Msg)
	}

	return
}

func genFeishuSign(secret, timestamp string) (string, error) {
	h := hmac.New(sha256.New, []byte(fmt.Sprintf("%s\n%s", timestamp, secret)))
	if _, err := h.Write(nil); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(h.Sum(nil)), nil
}
