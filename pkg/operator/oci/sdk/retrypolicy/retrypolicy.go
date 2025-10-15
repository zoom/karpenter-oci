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

package retrypolicy

import (
	"time"

	"github.com/oracle/oci-go-sdk/v65/common"
)

// defaultPolicy is built once and treated as immutable/read-only.
// You can expose it via a getter to avoid accidental mutation.
var defaultPolicy = common.NewRetryPolicyWithOptions(
	common.ReplaceWithValuesFromRetryPolicy(common.DefaultRetryPolicyWithoutEventualConsistency()),
	common.WithMaximumNumberAttempts(8),            // 1 try + 7 retries
	common.WithExponentialBackoff(30*time.Second, 2.0), // max sleep, base
	common.WithShouldRetryOperation(func(resp common.OCIOperationResponse) bool {
		if resp.Error == nil {
			return false
		}
		if svcErr, ok := common.IsServiceError(resp.Error); ok {
			switch svcErr.GetHTTPStatusCode() {
			case 429:
				return true
			case 409:
				return svcErr.GetCode() == "IncorrectState"
			default:
				return false
			}
		}
		return common.IsNetworkError(resp.Error)
	}),
)

// Default returns a pointer to the shared default retry policy.
func Default() *common.RetryPolicy {
	return &defaultPolicy
}

// SetGlobal applies Default() SDK-wide, so every OCI request uses it unless
// a client/request overrides RequestMetadata.RetryPolicy.
func SetGlobal() {
	common.GlobalRetry = Default()
}
