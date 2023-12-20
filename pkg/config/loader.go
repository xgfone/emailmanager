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
	"bytes"
	"encoding/json"
	"os"

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
	data, err := os.ReadFile(l.filepath)
	if err != nil {
		return
	}

	err = json.Unmarshal(removeLineComments(data), &controlers)
	if err == nil {
		for _, c := range controlers {
			err = structs.Reflect(&c)
			if err != nil {
				return
			}
		}
	}
	return
}

var (
	doublequote = []byte{'"'}
	jsonComment = []byte("//")
)

func removeLineComments(data []byte) []byte {
	result := make([]byte, 0, len(data))
	for len(data) > 0 {
		var line []byte
		if index := bytes.IndexByte(data, '\n'); index == -1 {
			line = data
			data = nil
		} else {
			line = data[:index]
			data = data[index+1:]
		}

		orig := line
		line = bytes.TrimLeft(line, " \t")
		if len(line) == 0 || bytes.HasPrefix(line, jsonComment) {
			continue
		}

		// Line Suffix Comment
		if index := bytes.Index(orig, jsonComment); index == -1 {
			result = append(result, orig...)
		} else if bytes.IndexByte(orig[index:], '"') == -1 {
			result = append(result, bytes.TrimRight(orig[:index], " \t")...)
		} else {
			if bytes.Count(orig[:index], doublequote)%2 == 0 {
				/* The case: ... "...." ... // the trailling comment containing ". */
				result = append(result, bytes.TrimRight(orig[:index], " \t")...)
			} else {
				/* "//" is contained in a string. */
				result = append(result, orig...)
			}
		}
		result = append(result, '\n')
	}
	return result
}
