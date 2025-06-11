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

func ToString(p *string) (v string) {
	if p == nil {
		return v
	}

	return *p
}

func ToFloat64(p *float64) (v float64) {
	if p == nil {
		return v
	}

	return *p
}

func ToFloat32(p *float32) (v float32) {
	if p == nil {
		return v
	}

	return *p
}

func String(v string) *string {
	return &v
}
