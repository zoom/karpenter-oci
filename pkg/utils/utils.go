/*
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package utils

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

var qualifiedNameRegexp = regexp.MustCompile(`[^a-zA-Z0-9\-_.]`)
var startQualifiedNameRegexp = regexp.MustCompile(`^[a-zA-Z0-9]`)
var endQualifiedNameRegexp = regexp.MustCompile(`[a-zA-Z0-9]$`)

// PrettySlice truncates a slice after a certain number of max items to ensure
// that the Slice isn't too long
func PrettySlice[T any](s []T, maxItems int) string {
	var sb strings.Builder
	for i, elem := range s {
		if i > maxItems-1 {
			fmt.Fprintf(&sb, " and %d other(s)", len(s)-i)
			break
		} else if i > 0 {
			fmt.Fprint(&sb, ", ")
		}
		fmt.Fprint(&sb, elem)
	}
	return sb.String()
}

func SanitizeLabelValue(value string) string {
	// strict length
	if len(value) > 63 {
		value = value[:63]
	}

	// replace "_"
	value = qualifiedNameRegexp.ReplaceAllString(value, "_")

	// empty
	if value == "" {
		return value
	}

	// start with alphanumeric
	if !startQualifiedNameRegexp.MatchString(value) {
		value = "a_" + value
	}

	// end with alphanumeric
	if !endQualifiedNameRegexp.MatchString(value) {
		value = value + "_z"
	}

	return value
}

func FilterMap[K comparable, V any](m map[K]V, f func(K, V) bool) map[K]V {
	ret := map[K]V{}
	for k, v := range m {
		if f(k, v) {
			ret[k] = v
		}
	}
	return ret
}

// WithDefaultFloat64 returns the float64 value of the supplied environment variable or, if not present,
// the supplied default value. If the float64 conversion fails, returns the default
func WithDefaultFloat64(key string, def float64) float64 {
	val, ok := os.LookupEnv(key)
	if !ok {
		return def
	}
	f, err := strconv.ParseFloat(val, 64)
	if err != nil {
		return def
	}
	return f
}
