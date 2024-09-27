/*
 * Copyright 2024 Juicedata Inc
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package util

import (
	"math"
	"regexp"
	"strconv"
	"strings"
)

type ClientVersion struct {
	IsCe  bool
	Dev   bool
	Major int
	Minor int
	Patch int
}

const ceImageRegex = `ce-v(\d+)\.(\d+)\.(\d+)`
const eeImageRegex = `ee-(\d+)\.(\d+)\.(\d+)`

func (v ClientVersion) LessThan(o ClientVersion) bool {
	if o.Dev {
		return true
	}
	if o.Major > v.Major {
		return true
	}
	if o.Minor > v.Minor {
		return true
	}
	if o.Patch > v.Patch {
		return true
	}
	return false
}

func ParseClientVersion(image string) ClientVersion {
	if image == "" {
		return ClientVersion{}
	}
	imageSplits := strings.SplitN(image, ":", 2)
	if len(imageSplits) < 2 {
		// latest
		return ClientVersion{IsCe: true, Major: math.MaxInt32}
	}
	_, tag := imageSplits[0], imageSplits[1]
	version := ClientVersion{Dev: true}
	var re *regexp.Regexp

	if strings.HasPrefix(tag, "ce-") {
		version.IsCe = true
		re = regexp.MustCompile(ceImageRegex)
	} else if strings.HasPrefix(tag, "ee-") {
		version.IsCe = false
		re = regexp.MustCompile(eeImageRegex)
	}

	if re != nil {
		matches := re.FindStringSubmatch(tag)
		if len(matches) == 4 {
			version.Major, _ = strconv.Atoi(matches[1])
			version.Minor, _ = strconv.Atoi(matches[2])
			version.Patch, _ = strconv.Atoi(matches[3])
			version.Dev = false
		}
	}

	return version
}
