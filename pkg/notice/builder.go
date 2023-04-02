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

package notice

import "fmt"

var builders = make(map[string]NotifierBuilder, 8)

// NotifierBuilder is used to build the notifier.
type NotifierBuilder func(configs map[string]interface{}) (Notifier, error)

// RegisterNotifierBuilder registers the notifier builder with the type.
func RegisterNotifierBuilder(_type string, builder NotifierBuilder) {
	if _type == "" {
		panic("RegisterNotifierBuilder: notifier builder type must not be empty")
	}
	if builder == nil {
		panic("RegisterNotifierBuilder: notifier builder must not be nil")
	}
	builders[_type] = builder
}

// GetAllNotifierBuidlerTypes returns the types of all the notifier builder.
func GetAllNotifierBuidlerTypes() (types []string) {
	types = make([]string, 0, len(builders))
	for _type := range builders {
		types = append(types, _type)
	}
	return
}

// GetNotifierBuilder returns the notifier builder by the type.
func GetNotifierBuilder(_type string) NotifierBuilder { return builders[_type] }

// BuildNotifier builds the notifier by type and configs, and returns it.
func BuildNotifier(_type string, configs map[string]interface{}) (Notifier, error) {
	if builder := GetNotifierBuilder(_type); builder != nil {
		return builder(configs)
	}
	return nil, fmt.Errorf("no notifier builder typed '%s'", _type)
}
