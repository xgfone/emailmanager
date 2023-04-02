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

package config

import (
	"encoding/json"
	"io/ioutil"

	"github.com/xgfone/go-structs"
)

// Loader is used to load the config.
type Loader interface {
	LoadController() ([]Controller, error)
}

// FileLoader returns a file loader to load the config from the given file.
func FileLoader(filepath string) Loader {
	return fileLoader{filepath: filepath}
}

type fileLoader struct {
	filepath string
}

func (l fileLoader) LoadController() (controlers []Controller, err error) {
	data, err := ioutil.ReadFile(l.filepath)
	if err != nil {
		return
	}

	err = json.Unmarshal(data, &controlers)
	if err == nil {
		for _, c := range controlers {
			err = structs.Reflect(nil, &c)
			if err != nil {
				return
			}
		}
	}
	return
}
